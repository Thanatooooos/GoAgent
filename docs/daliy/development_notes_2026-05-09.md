# Development Notes - 2026-05-09

## 今日主题

今天的工作前半段集中在 `ingestion` 的最小收口和文档整理，后半段切回 `RAG retrieve`，目标逐步转为：

1. reconcile 修复行为做成可见信息
2. `internal/app/ingestion/service` 目录结构收拾清楚
3. 在项目层面明确“短期不再以 ingestion 为主工作模块”
4. 补强 retrieve 对文件名 / 标题 / 章节类 query 的召回能力
5. 建立 retrieve 的离线评估与真实回放评估基础设施

在此基础上，后续又把当天的主目标进一步收敛为：

6. 将单次 `tool workflow` 升级成 `AgentLoop V1`
7. 在不改 `RagChatService` 外壳的前提下，做出可循环、可观测、可终止的最小 Agent 闭环

## 本次改动

### 1. 做了一版 ingestion 最小收口

本轮没有继续扩展 ingestion 主执行链，而是在现有主链之上补了一层“修复行为留痕”。

改动点：

- `internal/app/knowledge/service/knowledge_document_ingestion_reconcile.go`
  - 将 reconcile 的执行结果收敛为结构化结果
  - 区分：
    - `skipped`
    - `documentUpdated`
    - `chunkLogUpdated`
    - `chunkLogCreated`
    - `errorMessage`

- `internal/app/knowledge/service/knowledge_document_service.go`
  - 新增 `IngestionReconcileRecorder`
  - `KnowledgeDocumentService` 可注入 reconcile recorder

- `internal/bootstrap/knowledge/ingestion_bridge.go`
  - 新增 knowledge -> ingestion metrics 的 recorder adapter

- `internal/app/ingestion/service/observer_metrics.go`
  - ingestion metrics 新增 `reconcile` 维度
  - 增加：
    - `attempts`
    - `skipped`
    - `documentUpdated`
    - `chunkLogUpdated`
    - `chunkLogCreated`
    - `failures`
    - `lastFailure`

- `cmd/server/main.go`
  - runtime 装配时将 reconcile recorder 接入 ingestion metrics

结果：

- `GET /ingestion/metrics` 不再只展示 task/node 运行态，还能看到 reconcile 修复是否发生、是否失败、最近一次失败是什么

### 2. 整理了 `internal/app/ingestion/service` 的文件结构

当前目录已经统一为按职责分组的命名方式：

- `service_*`
- `executor_* / workflow_*`
- `runner_*`
- `observer_*`

同时新增：

- `internal/app/ingestion/service/doc.go`

用于说明这一层文件的分组语义。

这次整理只做“低风险可读性收敛”：

- 不拆 package
- 不改变对外 API
- 不调整 runtime 装配方式
- 不改变主执行逻辑

### 3. 更新了项目进度文档

已更新：

- `docs/project_progress_context.md`

更新内容包括：

- 新增 2026-05-09 进展
- 写明 ingestion 已阶段性收口
- 明确短期不再以 ingestion 为主工作模块
- 将近期待办重心切回：
  - `RAG retrieve`
  - `diagnose`
  - `trace / tool / fallback`

### 4. 新增 ingestion 目标方向文档目录

新建目录：

- `docs/ingestion/`

并补充 ingestion 的目标方向文档，方便后续中期再回到该模块时快速对齐。

### 5. 补强了 retrieve 的精确匹配与 metadata 检索能力

这一轮围绕 `RAG retrieve` 做了第一批针对性补强，目标不是继续加新模式，而是先把文件名 / 标题 / 章节这类 query 的召回基础打牢。

改动点：

- `internal/app/rag/core/retrieve/search_mode_decision.go`
  - 补强 `auto` 模式规则：
    - 架构 / 流程类问题
    - 标识符查找
    - 文件名查找
    - 章节 / 标题定位

- `testdata/retrieve_search_mode_samples.json`
  - 扩充样本：
    - 文件名查找
    - 标识符查找
    - 章节定位
    - 标题定位

- `internal/app/rag/core/retrieve/channels.go`
  - 新增 `metadata_title` 通道
  - 定向检索：
    - `document_name`
    - `source_file_name`
    - `section`

- `internal/adapter/vectorstore/pgvector/vector_store.go`
  - 新增 `SearchByMetadata(...)`
  - 使用 `word_similarity` 对固定 metadata 字段做定向检索

结果：

- `retrieve auto` 样本回放扩到 `23/23 PASS`
- `metadata_title` 通道已经能参与文件名 / 标题 / 章节类 query 的召回

### 6. 补齐了 knowledge 直传链路的向量 metadata

为了让 `metadata_title` 通道不仅对 ingestion 文档有效，也对 knowledge 直传链路有效，这一轮把 knowledge 侧向量 metadata 往 ingestion 侧对齐了一步。

改动点：

- `internal/app/knowledge/service/document_process_service.go`
  - 抽出 `buildKnowledgeVectorMetadata(...)`
  - 新增：
    - `source_type`
    - `source_file_name`
  - 合并 chunk 自带 metadata

- `internal/app/knowledge/service/knowledge_chunk_service.go`
  - chunk update / rebuild 向量时统一复用上述 metadata 组装逻辑

结果：

- knowledge 直传链路写入 vector store 时，不再只有基础 ID 和 `document_name`
- 后续可以继续利用 `section / heading_path / code_language` 等 chunk 级 metadata 做检索增强

### 7. 建立了 retrieve 离线评估与真实回放基础设施

这一轮先实现最小可用的离线评估机制，不急着做线上监控，先把“评估口径”和“样本回放”跑通。

改动点：

- 新增 `internal/app/rag/evaluation/`
  - 定义评估样本结构
  - 计算：
    - `Hit@K`
    - `Recall@K`
    - `MRR`
  - 支持按 `tag` 分组聚合

- 新增 `cmd/retrieve-eval`
  - 支持纯离线评估：
    - 直接消费样本中的 `retrieved`
  - 支持真实回放评估：
    - 复用 `internal/bootstrap/rag`
    - 直接执行当前 retrieve 实现
    - 将结果回填后再做评估

- 新增 `testdata/retrieve_eval_samples.json`
  - 提供 `chunk` / `document` 两种 target 样例
  - 覆盖：
    - 文件名查找
    - 章节标题定位
    - 语义概念问答

结果：

- retrieve 评估现在已经具备最小闭环：
  - 有样本格式
  - 有指标计算
  - 有 CLI
  - 有真实回放入口

### 8. 落地了 AgentLoop V1

这一轮没有直接上完整 LLM observer，而是先做“低风险、可跑通、可继续演进”的 V1。

改动点：

- `internal/app/rag/tool/workflow.go`
  - 扩展 `PlanInput`
    - `AgentState`
    - `PreviousResults`
  - 扩展 `WorkflowInput`
    - `EventSink`
  - 扩展 `WorkflowResult`
    - `Rounds`
  - 新增 workflow 事件结构
    - `ToolCallEvent`
    - `WorkflowEventSink`

- `internal/app/rag/tool/agent_loop.go`
  - 新增多轮 `Plan -> Act -> Observe` 循环执行器
  - 支持：
    - 每轮重新规划
    - 基于已执行 call 去重
    - 最大轮次截断
    - 汇总多轮 `Context / AnswerGuidance / Calls / Rounds`

- `internal/app/rag/tool/observer_rule.go`
  - 新增规则版 observer
  - 首版主要针对：
    - `document_ingestion_diagnose`
    - `task_ingestion_diagnose`
    - `document_query`
    - `document_chunk_log_query`
    - `ingestion_task_query`
  - 能根据已有结果决定是否继续下钻到 `task / node`

- `internal/bootstrap/rag/runtime.go`
  - runtime 默认从 `LocalWorkflow` 切换到 `AgentLoop`

结果：

- `tool workflow` 不再是一次性执行完就结束
- 对 `doc / task / trace` 这类结构化诊断问题，已经具备“查一轮 -> 看结果 -> 再查一轮”的最小 agent 能力

### 9. 打通了 Agent 过程可观测性

为了让 AgentLoop V1 不只是后端内部循环，这一轮同时把 SSE 和 trace 补齐到了能消费的状态。

改动点：

- `internal/app/rag/service/rag_chat_service.go`
  - `runToolWorkflowStage(...)` 接入 `EventSink`
  - `RagChatEventSink` 扩展：
    - `SendAgentThink(...)`
    - `SendToolStart(...)`
    - `SendToolResult(...)`

- `internal/adapter/http/rag/handlers.go`
  - `sseChatSink` 实现新事件
  - SSE 新增：
    - `agent_think`
    - `tool_start`
    - `tool_result`

- `internal/app/rag/service/chat_tracer.go`
  - trace 从单层 tool call 扩展到：
    - `agent_round`
    - `tool_call`
    - `agent_observation`

- `frontend/src/hooks/useStreamResponse.ts`
  - 前端新增新事件分发

- `frontend/src/stores/chatStore.ts`
  - `toolCalls` 支持按 `callId` 增量更新，而不是只追加最终摘要

- `frontend/src/components/chat/MessageItem.tsx`
  - 最小展示增强：
    - 轮次
    - 参数
    - 耗时
    - 运行中到完成的状态切换

结果：

- 前端不再只能看到最终 `tool` 摘要
- 现在已经能实时看到工具开始、返回，以及多轮调用过程

### 10. 补齐了 AgentLoop V1 的基础测试

改动点：

- 新增 `internal/app/rag/tool/agent_loop_test.go`
  - 覆盖多轮 planner 调用
  - 覆盖 `AgentState / PreviousResults` 透传
  - 覆盖 `tool_start / tool_result / agent_think` 事件发出

- 更新 `internal/app/rag/service/rag_chat_service_test.go`
  - 同步新的 sink 接口
  - 同步 `runToolWorkflowStage(...)` 新签名

## 验证

已通过：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service ./internal/adapter/http/ingestion -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/adapter/http/ingestion ./internal/bootstrap/ingestion ./internal/app/knowledge/service -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go run ./cmd/retrieve-debug -input testdata\retrieve_search_mode_samples.json
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/core/retrieve -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/knowledge/service ./internal/app/knowledge/service/test -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/evaluation ./cmd/retrieve-eval ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go run ./cmd/retrieve-eval -input testdata\retrieve_eval_samples.json
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
```

结果：

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS
- `internal/adapter/http/ingestion` PASS
- `internal/bootstrap/ingestion` 可正常编译
- `internal/app/rag/core/retrieve` PASS
- `internal/app/knowledge/service/test` PASS
- `internal/app/rag/evaluation` PASS
- `cmd/retrieve-eval` 可正常编译
- `internal/bootstrap/rag` 可正常编译
- `retrieve-debug` 样本回放 `23/23 PASS`
- `retrieve-eval` 样例评估可正常输出 `Hit@K / Recall@K / MRR`
- `internal/app/rag/tool` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）
- `internal/bootstrap/rag` PASS（无测试文件，包可正常编译）

补充说明：

- `frontend` 生产构建尝试执行过，但当前本地 Node 运行环境会因为权限问题访问 `C:\Users\1` 失败，暂时没完成 `vite build` 验证；这一步暴露的是环境权限问题，不是当前改动先报出的 TS 编译错误

## 当前判断

今天这轮之后，`ingestion` 的状态更适合被定义为：

- 主链已具备
- 一致性已有基础收口
- 修复行为开始可见
- 目录结构可读性已提升
- 但仍未达到“继续深挖最划算”的阶段

因此短期策略调整为：

- `ingestion` 暂停作为主推进模块
- 后续仅做必要修复和被动配套
- 主研发重心切回 `RAG / diagnose / tool / trace`

同时，`retrieve` 的状态也前进了一步：

- 不再只有 `vector_global / keyword` 两路主通道
- 已经具备 `metadata_title` 这个面向精确匹配 query 的补充通道
- `auto` 规则已有样本回放闭环
- 评估层已有离线指标和真实回放入口
- 下一步可以真正开始对比不同 query 类型下的召回收益

同时，`Agent / Tool` 这条线也进入了一个新的阶段：

- 已经不再只是单次 `tool workflow`
- 已经有了可演示的 AgentLoop V1
- 但当前更接近“结构化诊断 agent”，还不是完整自主 Agent
- 当前最适合承接的是：
  - `document` 诊断
  - `task` 诊断
  - `trace` 诊断
- 下一步重点不会是继续加更多 event，而是提升：
  - 增量规划质量
  - observer 判断质量
  - 多轮链路稳定性

## 后续保留事项

虽然短期不主攻 ingestion，但以下事项保留为中期待办：

- pending/running 超时治理
- reconcile 结果沉淀与修复审计
- 更完整的 `document / chunk_log / task` 不一致规则矩阵
- 更系统的恢复策略与告警暴露

对 `retrieve` 而言，接下来的近期待办会是：

- 扩充真实 query 评估集
- 对比 `keyword / metadata_title / vector_global` 的收益
- 逐步把评估样本从手工示例升级成更贴近真实项目语料的 `golden set`

对 `AgentLoop V1` 而言，接下来的近期待办会是：

- 打磨 planner 对 `AgentState / PreviousResults` 的利用率
- 补强高频 `doc -> task -> node` 链路的稳定性
- 继续减少重复调用和无效下钻
- 评估何时引入 LLM observer，而不是长期停留在规则 observer
