# Development Notes - 2026-05-07

## 背景

今天的工作围绕 `Agent MVP` 的第一阶段展开，目标不是直接做完整 function calling，而是先把系统里的 `tool` 基础层、自研 workflow、LLM planner、以及 `RagChatService` 扩展点打通，形成一个可继续演进的本地闭环。

这次的原则很明确：

1. 先不把 chat 主链迁到 EINO
2. 先自研 tool 基础层，保证和现有 `RagChatService` 完全兼容
3. 先做只读 tool，降低风险
4. 先做"LLM planner + 本地规则 fallback"的混合架构，后面再向完整 Agent 形态演进

## 本次新增

### 1. 建立了 tool 基础层

新增目录：

```text
internal/app/rag/tool/
```

新增核心能力：

- `tool.go`
  - 定义 `Tool` 接口
  - 定义 `Definition / Call / Result`
- `registry.go`
  - tool 注册、查找、去重
- `executor.go`
  - 统一执行 tool
  - 标准化未知 tool 和执行失败结果
- `renderer.go`
  - 把 tool 结果渲染成 `ToolContext`
  - 把 tool 结果转成 `CallSummary`
- `workflow.go`
  - 定义 `Workflow / WorkflowInput / WorkflowResult`
  - 定义 `Planner / PlanInput / PlanResult`

这一步的意义是：后续无论是继续自研，还是把 workflow 换成 EINO，这套业务级 tool 抽象都可以复用。

### 2. 落地了第一批只读 tool（4 个）

新增文件：

- `document_query_tool.go`
- `ingestion_task_query_tool.go`
- `ingestion_task_node_query_tool.go`
- `trace_node_query_tool.go`

当前能力如下：

#### `document_query`

输入：
- `documentId`

输出摘要：
- 文档状态
- 是否启用
- process mode
- pipeline id
- chunk count

#### `ingestion_task_query`

输入：
- `taskId`
- 可选 `includeNodes`

输出摘要：
- task 状态
- sourceType
- pipelineId
- 节点数

#### `ingestion_task_node_query`

输入：
- `taskId`
- 可选 `nodeId`（指定则查单节点，否则查全部）

输出摘要（全量模式）：
- task 总节点数
- failed 节点列表（含 errorMessage）
- running 节点列表

输出摘要（单节点模式）：
- node 类型、顺序、状态、耗时
- 错误信息、output 数据

#### `trace_node_query`

输入：
- `traceId`

输出摘要：
- trace 状态
- conversationId
- 节点数量
- 节点类型摘要

这些 tool 都直接复用现有 service / repository，不走 HTTP。

### 3. 实现了本地 workflow runner

新增文件：

- `local_workflow.go`

实现内容：

- 新增 `LocalWorkflow`
- 采用 LLM planner 优先 + 规则 fallback 的混合架构
- 规则驱动能从用户问题里识别：
  - `document`
  - `task`
  - `trace`
  - `node/节点/步骤`（触发 node query）
  - 显式 ID
- 默认最多串行执行 `3` 个 tool
- 调用结束后输出：
  - `Context`
  - `Calls`
  - `Degraded`
  - `DegradeReason`

当前规划规则很克制：

- 识别 `document/doc + doc-*` 时调用 `document_query`
- 识别 `ingestion/task/任务/导入 + task-*` 时调用 `ingestion_task_query`
- 识别 `node/节点/步骤 + task-*` 时额外调用 `ingestion_task_node_query`
- 识别 `trace/链路/检索/召回 + trace-*` 时调用 `trace_node_query`
- 对"本次 trace / 当前 trace"这类问法，允许回退使用当前 `TraceID`

### 4. 落地了 LLM tool planner

新增文件：

- `internal/app/rag/tool/planner/planner.go`

实现内容：

- 新增 `LLMPlanner`
  - 构造 system prompt，注入 tool 定义列表
  - 调用 `ChatWithRequest` + `JSONMode`，让 LLM 输出 JSON plan
  - 解析 JSON 响应，提取 tool call 列表
  - JSON 解析失败 / LLM 返回空 / 畸形响应 → 返回空 plan
- 新增 `ChatRequest.JSONMode` 字段
  - `openai_style_chat_client.go` 的 `buildRequestBody` 支持 `response_format: json_object`

架构：

```text
planCalls(ctx, input)
  ├── planner != nil → planWithLLM() → 成功且非空 → 返回
  └── 失败/空/无 planner → planWithRules() → 规则匹配
```

### 5. 把 workflow 串进了 `RagChatService`

之前已经给 `RagChatService` 预留了 `toolWorkflow` 扩展点，这次把它真正接起来了。

改动文件：

- `internal/bootstrap/rag/runtime.go`

启动时新增逻辑：

1. 初始化 knowledge document 相关依赖
2. 初始化 ingestion task 相关依赖
3. 复用现有 trace run / trace node repository
4. 注册首批 4 个 tool
5. 构造 `Executor`
6. 构造 `LLMPlanner`（注入 `aiRuntime.Chat`）
7. 构造 `LocalWorkflow` + 注入 planner
8. 调用 `chatService.SetToolWorkflow(...)`

这样现在的主链已经变成：

```text
prepareChat
-> rewrite
-> retrieve
-> local tool workflow（LLMPlanner 优先 → 规则 fallback）
-> prompt
-> stream chat
```

### 6. 实现了 tool 可观测性展示层

后端改动：

- `RagChatEventSink` 接口新增 `SendTool(name, status, summary) error`
- `sseChatSink` 实现 SSE `event: tool` 下发
- `rag_chat_service.go` 的 `Chat()` 中 tool workflow 完成后逐条发送 tool call 摘要
- test stub 同步更新 `SendTool` + 计数

前端改动：

- `types/index.ts`：新增 `ToolCallPayload` 接口；`Message` 新增 `toolCalls` 字段
- `useStreamResponse.ts`：`StreamHandlers` 新增 `onTool`；SSE 解析器新增 `"tool"` case
- `chatStore.ts`：新增 `appendToolCall()`；handlers 新增 `onTool`
- `MessageItem.tsx`：新增琥珀色可折叠工具调用卡片

SSE 事件格式：

```
event: tool
data: {"name":"ingestion_task_node_query","status":"success","summary":"task=task-test totalNodes=3 failed=[indexer(connection refused)]"}
```

前端效果：

```
🔧 工具调用 (2)         [部分失败]  ▼
├ ✅ ingestion_task_node_query    success
│   task=task-test totalNodes=3...
├ ❌ ingestion_task_query         failed
│   ingestion task not found
```

### 7. 补齐测试

新增和更新的测试包括：

- `tool_test.go`
  - tool 定义、registry、executor、renderer
- `query_tools_test.go`
  - 4 个只读 tool 的输入校验、成功执行、错误透传
  - ingestion_task_node_query：全量节点、单节点、节点不存在
- `local_workflow_test.go`
  - tool 规划
  - trace fallback
  - degrade 场景
  - 无匹配跳过
- `planner/planner_test.go`
  - LLMPlanner：单 tool、多 tool、空 tool、markdown JSON、LLM 错误、畸形 JSON、nil 安全、prompt 构建

### 8. 数据库迁移基础设施修复

- 修复 2 个迁移 SQL 文件：`CREATE TABLE` → `CREATE TABLE IF NOT EXISTS`，`CREATE INDEX` → `CREATE INDEX IF NOT EXISTS`
- 修复 knowledge 迁移中未注释的 `-- +goose Down` 段 `DROP TABLE` 语句（被 `splitSQLStatements` 误执行）
- 新增 `CREATE EXTENSION IF NOT EXISTS pg_trgm;` 到 vector 迁移
- `main.go` 调整启动顺序：先建临时库 → 跑迁移 → 关临时库 → 再启动 runtime
- 修复 rocketmq `go.sum` 缺失条目

## 当前验证状态

已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/tool/planner -count=1   # 12 tests PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/tool -count=1          # 18 tests PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/... -count=1           # ALL PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/bootstrap/rag -count=1         # PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/infra-ai/... -count=1          # ALL PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/framework/... -count=1         # ALL PASS
```

**端到端联调验证（2026-05-07）：**

- 后端启动 + 自动迁移 → 成功
- SSE `event: tool` 事件下发 → 已验证
- 前端 tool call 卡片渲染（琥珀色卡片 + 成功/失败状态图标 + summary） → 已验证
- 多条规则路线问题触发工具调用 → 已验证
- 数据库插入真实数据后 tool 查询成功路径（3 节点：2 success + 1 failed） → 已验证

结果说明：

- `tool` 包回归通过（18 tests）
- `planner` 子包回归通过（12 tests）
- `rag` 相关包回归通过（28 tests）
- `infra-ai` 所有子包回归通过
- `framework` 所有子包回归通过
- `bootstrap/rag` 接线可编译通过
- 前后端联调 tool 展示链路全通

## 当前结论

今天的结果可以概括为一句话：

`Agent MVP` 已经从"讨论架构"进入"后端最小闭环落地 + 前后端联调通过"阶段。

虽然现在还不是完整的模型驱动 function calling，但系统已经具备了以下基础：

1. 有统一的 tool 抽象
2. 有可注册、可执行的 tool 基础设施
3. 有 4 个真实只读 tool（含 node 粒度排障）
4. 有 LLMPlanner（LLM 决策 + 规则 fallback 混合架构）
5. 有接入 RAG chat 主链的运行时装配
6. 有 SSE tool 事件 + 前端可折叠卡片展示
7. 有 30 个单元测试覆盖关键路径
8. 有前后端联调通过的端到端验证

## 后续建议

### P0

- 补 `document_chunk_log_query`：knowledge / ingestion 联合排障

### P1

- 增强 tool trace 记录：每次 tool call 写入 `rag_trace_node`
- 完善 LLMPlanner：真实 ID 识别、与 rule fallback 的可观察性对比

### P1

- 继续补强 ingestion 生产化
  - indexer 幂等和补偿
  - task/document/chunk_log 状态联动保护

### P2

- 等 tool schema 和 workflow 稳定后，再评估是否把 workflow 执行层迁到 EINO

## 追加进展（同日补充）

### 9. 补齐了 `document_chunk_log_query`

- 新增 `internal/app/rag/tool/builtin/document_chunk_log_query_tool.go`
- 能基于 `documentId` 查询最近 chunk log，并聚合关联的 ingestion task、task nodes、失败节点、最新错误
- 重点用途从“单点状态查询”扩展为“knowledge / ingestion 联合排障证据查询”
- 更新规则规划后，类似以下问题会优先命中：
  - `帮我排查 document doc-1 的 chunk log`
  - `document doc-1 为什么 ingestion 失败`

### 10. 落地了第一个聚合型诊断 tool：`document_ingestion_diagnose`

- 新增 `internal/app/rag/tool/builtin/document_ingestion_diagnose_tool.go`
- 该 tool 不再只返回原始数据，而是直接输出诊断结构：
  - `conclusion`
  - `confidence`
  - `evidence`
  - `suggestions`
- 内部会综合 `document`、`recent chunk logs`、`ingestion task`、`task nodes`
- 目标是把系统从“查得到数据”提升到“能直接给出文档入库失败的高概率结论”

### 11. 为诊断回答增加了结构化回答引导

- 新增 `internal/app/rag/tool/answer_guidance.go`
- `WorkflowResult` 增加 `AnswerGuidance`
- `LocalWorkflow` 在识别到诊断型 tool 结果时，会生成“结论 / 证据 / 建议”式回答引导
- `internal/app/rag/core/prompt/service.go` 增加诊断回答要求注入
- `internal/app/rag/service/rag_chat_service.go` 已将该引导接入 prompt 构建主链
- 现在模型在消费 `*_diagnose` tool 结果时，回答会更稳定地按诊断结构输出，而不是只复述 tool summary

### 12. 造了一组本地诊断联调样本数据

- 新增临时脚本 `tmp/seed_diagnosis_sample.go`
- 插入了可用于联调的本地样本：
  - `documentId = doc-1`
  - `taskId = task-diag-1`
  - 诊断场景：`indexer` 节点失败，错误为 `connection refused`
- 前后端联调用该样本成功验证：
  - 问题：`帮我诊断 document doc-1 的 ingestion 失败原因`
  - 结果：已能返回文档失败结论、关键证据和下一步建议

### 13. 继续扩展了 task 入口诊断：`task_ingestion_diagnose`

- 新增 `internal/app/rag/tool/builtin/task_ingestion_diagnose_tool.go`
- 该 tool 面向 `task-*` 入口，基于：
  - `ingestion_task`
  - `ingestion_task_nodes`
- 输出与 document 诊断一致的结构：
  - `conclusion`
  - `confidence`
  - `evidence`
  - `suggestions`
- 解决的问题是：用户不一定从 `document` 入口排障，也可能直接问某个 `task` 为什么失败、卡在哪个节点

### 14. 继续扩展了 trace 入口诊断：`trace_retrieval_diagnose`

- 新增 `internal/app/rag/tool/builtin/trace_retrieval_diagnose_tool.go`
- 面向 `trace-*` 入口，综合：
  - `trace run`
  - `trace nodes`
  - `trace node extraData`
- 当前已支持几类典型判断：
  - trace 某关键节点执行失败
  - `retrieve` 阶段 `chunkCount = 0`
  - `retrieve` 命中 chunk 过少，疑似召回偏弱
  - trace 全链路成功，但问题更可能出在相关性或回答合成阶段
- 这样诊断能力已从 ingestion 扩展到 RAG 主链

### 15. 规范化了 `tool` 包目录结构

- 具体 tool 实现统一收敛到：`internal/app/rag/tool/builtin/`
- `internal/app/rag/tool/` 根目录保留：
  - 抽象定义
  - registry
  - executor
  - workflow
  - renderer
  - answer guidance
- `planner` 保持在独立子目录：`internal/app/rag/tool/planner/`
- `internal/bootstrap/rag/runtime.go` 已调整为从 builtin 子包集中注册内置 tool
- 这样后续新增 `query / diagnose / action` 类 tool 时，目录职责会更清晰

### 16. 本次新增/更新测试

- `internal/app/rag/tool/builtin/query_tools_test.go`
  - 覆盖 `document_chunk_log_query`
  - 覆盖 `document_ingestion_diagnose`
  - 覆盖 `task_ingestion_diagnose`
  - 覆盖 `trace_retrieval_diagnose`
- `internal/app/rag/tool/local_workflow_test.go`
  - 补充 document / task / trace 诊断类规则触发测试
- `internal/app/rag/tool/tool_test.go`
  - 随目录调整同步更新
- `internal/app/rag/core/prompt/service_test.go`
  - 覆盖 `AnswerGuidance` 注入行为

### 17. 截至当前的诊断智能阶段结论

- 第一阶段“诊断智能”已经形成最小闭环
- 当前系统已具备 3 类核心诊断入口：
  - `document_ingestion_diagnose`
  - `task_ingestion_diagnose`
  - `trace_retrieval_diagnose`
- 当前能力边界可以概括为：
  - 能识别 document / task / trace 诊断意图
  - 能自动调用只读 tool 拼接证据链
  - 能输出结构化的结论、证据、建议
  - 仍然不自动执行修复动作，只做高概率诊断与辅助排障

### 18. 截至当前补充验证状态

已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/core/prompt -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/... -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/bootstrap/rag -count=1
```

补充联调结果：

- `document` 诊断链路已在本地通过真实样本验证
- `document_ingestion_diagnose` 已实际命中并返回有效诊断
- 诊断回答已能稳定朝“结论 / 证据 / 建议”结构靠拢
- 前端暂未新增独立诊断卡片，但已有 tool 卡片足以支撑当前联调观察

### 19. 继续补强了 `document / task / trace` 诊断质量

- 新增 `internal/app/rag/tool/builtin/diagnose_helpers.go`
  - 抽出 ingestion 节点统计、状态冲突判断、trace extraData 读取等公共诊断辅助逻辑
- `document_ingestion_diagnose_tool.go`
  - 新增更细粒度 evidence：
    - `latestChunkLog.chunkCount`
    - `latestChunkLog.totalDurationMs`
    - `ingestionTask.chunkCount`
    - `ingestionNodes.total/success/failed/running/pending/lastNode/lastStatus`
  - 新增两类关键判断：
    - chunk log success 但 `chunkCount=0`
    - `document / chunk log / ingestion task` 三方状态不一致
- `task_ingestion_diagnose_tool.go`
  - 新增节点分布统计 evidence
  - 新增几类状态冲突诊断：
    - task success 但 `chunkCount=0`
    - task success 但仍有 running node
    - task failed 但节点记录未捕获 failed node、只剩 running 状态
- `trace_retrieval_diagnose_tool.go`
  - 新增 `retrieve.topScore` 证据
  - 新增对 `tool_workflow` extraData 的消费：
    - `toolWorkflow.status`
    - `toolWorkflow.callCount`
    - `toolWorkflow.degraded`
    - `toolWorkflow.degradeReason`
    - `toolWorkflow.toolNames`
  - 已能判断“trace 主链成功，但 tool workflow 降级导致诊断证据可能不完整”

### 20. 把每次 tool call 独立写入了 `rag_trace_node`

- `internal/app/rag/tool/workflow.go`
  - `CallSummary` 新增 `DurationMs`
- `internal/app/rag/tool/local_workflow.go`
  - 执行每次 tool 时统计耗时，并写入 `WorkflowResult.Calls`
- `internal/app/rag/service/rag_chat_service.go`
  - 新增 `recordTraceNodeAt(...)`
  - stage trace node 改为记录真实 `start_time / end_time / duration_ms`
  - 新增 `recordToolCallTraceNodes(...)`，把每次 tool call 作为独立 trace node 落库：
    - `node_id = tool_01 / tool_02 / ...`
    - `parent_node_id = tool_workflow`
    - `depth = 2`
    - `node_type = tool_call`
    - `node_name = tool name`
    - `error_message` 落失败摘要
    - `extraData` 落 `summary / durationMs / toolStatus / sequence`
- 这样现在 trace 里已经不只是有一个 `tool_workflow` 汇总节点，而是有完整的 tool 调用子链

### 21. 补齐了 trace 查询接口对 tool call 细节的透出

- `internal/adapter/http/rag/trace_handlers.go`
  - `ragTraceNodeVO` 新增 `extraData`
- `frontend/src/services/ragTraceService.ts`
  - `RagTraceNode` 类型新增 `extraData?: string | null`
- 这一步先把数据通路打通，后续前端 trace 详情页可以直接消费 tool call 细节

### 22. 本轮新增/更新测试与验证

- `internal/app/rag/tool/builtin/query_tools_test.go`
  - 新增 document 状态不一致诊断测试
  - 新增 trace tool workflow degraded 诊断测试
  - 继续覆盖 diagnosis evidence 输出
- `internal/app/rag/service/rag_chat_service_test.go`
  - 新增 `recordToolCallTraceNodes()` 单测
  - 验证 parent/depth、失败错误写入、duration 与 `extraData.summary`

本轮已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/service -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/... -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/bootstrap/rag -count=1
```

补充结果：

- `document / task / trace` 三类诊断已具备更细粒度证据输出
- `rag_trace_node` 已能记录每次 tool call 的独立节点
- trace 查询接口已能返回节点 `extraData`
