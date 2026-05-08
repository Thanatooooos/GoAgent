# Project Progress Context

更新时间：2026-05-08

这份文档用于维护 `goagent` 当前项目进度，帮助后续开发快速对齐当前阶段、已完成能力、最新进展、验证状态、已知风险和下一步计划。

## 当前阶段

项目已经从"基础能力搭建期"进入"主链路闭环、联调和质量收口期"。

当前可以分成四条主线来看：

1. `Knowledge`
   已具备完整的知识库、文档、chunk、调度和管理端能力，重点从"继续铺功能"转向一致性、状态联动和排障体验。

2. `Ingestion`
   已经跑通 `pipeline -> task -> task_node -> knowledge 回写` 的最小可用闭环，当前重点是生产化补强，包括幂等、补偿、状态收口和可观测性。

3. `RAG`
   已形成最小 chat 闭环，支持多轮对话、rewrite、retrieve、prompt、trace、fallback，重点开始转向检索策略优化、可解释性和 Agent 能力扩展。

4. `Agent / Tool`
   已完成第一阶段基础设施：自研 tool 抽象、tool registry、tool executor、本地 workflow runner、LLMPlanner，以及接入 `RagChatService` 的扩展点。当前已进入"LLM 决策 + 规则 fallback"的混合规划阶段，具备完整的 tool 调用链及 SSE 展示能力。

## 已完成能力

### 基础设施

- `infra-ai`
  - chat（含 JSONMode response_format 支持）
  - embedding
  - rerank
  - provider 路由与候选选择
- `core/parser`
  - Markdown parser
  - Tika parser
- `core/chunk`
  - fixed size chunker
  - markdown chunker
  - chunk selector
- Web 基础设施
  - Gin
  - request id
  - global error handler
  - user context middleware
  - Viper 配置加载
- 数据库迁移
  - 5 个嵌入式 SQL 迁移文件（knowledge / vector / user / rag / ingestion）
  - 自定义迁移执行器，幂等 `IF NOT EXISTS` 语义
  - 启动时自动执行迁移，不依赖外部工具

### Knowledge

- `KnowledgeBaseService`
  - create / get / update / delete / page
  - chunk strategies 查询
  - embedding model 更新校验
- `KnowledgeDocumentService`
  - upload / get / page / search / update / enable / delete
  - start chunk
  - chunk log page
  - schedule exec page
  - 支持 `sourceType=file / url / feishu`
  - 支持 `processMode=pipeline` 时创建 ingestion task
  - 支持 ingestion 完成后回写 document 状态与 chunk log
- `KnowledgeChunkService`
  - page / create / update / delete / enable
  - batch toggle enabled
  - 支持 chunk / vector 同步
- `DocumentProcessService`
  - 文件读取
  - 文本解析
  - chunk 切分
  - embedding
  - chunk/vector 持久化
  - chunk log 写入
  - 文档状态流转

### Ingestion

- 领域模型
  - `Pipeline`
  - `PipelineNode`
  - `Task`
  - `TaskNode`
- PostgreSQL 持久化
  - `pipeline / task / task_node`
- HTTP 接口
  - pipeline CRUD
  - task 创建 / 分页 / 详情 / 节点日志
  - `GET /ingestion/metrics`
- Runtime / 执行层
  - `WorkflowBuilder`
  - `NodeRunnerRegistry`
  - `TaskObserver`
  - `ExecutorService`
- 节点链路
  - `fetcher`
  - `parser`
  - `chunker`
  - `indexer`
- 已完成的生产化补强
  - `ExecutorService` 节点重试与指数退避
  - `Indexer` 失败补偿清理
  - `task_node` 重试信息持久化
  - `document` 级活动 task 保护
  - `indexer` 输出写入摘要
  - 旧 task 不覆盖新 task 的 knowledge 回写保护
- `TaskService.GetNode` 单节点查询方法

### RAG

- Domain / Repository
  - `conversation`
  - `conversation_message`
  - `conversation_summary`
  - `message_feedback`
  - `rag_trace_run`
  - `rag_trace_node`
- Core
  - `core/rewrite`
  - `core/retrieve`
  - `core/prompt`
  - `core/vector`
  - `core/memory`
- Service / HTTP
  - `ConversationService`
  - `ConversationMessageService`
  - `MessageFeedbackService`
  - `TraceService`
  - `RagChatService`
  - trace 查询接口
- 已完成的 RAG 增强
  - LLM rewrite
  - memory compression
  - `semantic / keyword / hybrid / auto` 检索模式
  - 多通道检索基础架构（`channel + processor + context`）
  - `vector_global / keyword` 两路检索通道
  - 低置信度 fallback
  - SSE `meta` 下发 `searchMode`
  - prompt 支持单独注入 `ToolContext`

### Agent / Tool

- `internal/app/rag/tool`
  - `Tool` 接口
  - `Definition / Call / Result`
  - `Registry`
  - `Executor`
  - `RenderContext`
  - `Workflow` 抽象
  - `Planner` 接口 + `PlanInput` / `PlanResult`
- 目录结构已规范化
  - 根目录保留抽象、registry、executor、workflow、renderer、answer guidance
  - 具体内置 tool 统一收敛到 `internal/app/rag/tool/builtin/`
  - `planner` 保持独立子目录 `internal/app/rag/tool/planner/`
- 已实现的查询 / 诊断 tool
  - `document_query`
  - `document_chunk_log_query`
  - `ingestion_task_query`
  - `ingestion_task_node_query`（支持全量节点 + 单节点查询）
  - `trace_node_query`
  - `document_ingestion_diagnose`
  - `task_ingestion_diagnose`
  - `trace_retrieval_diagnose`
- 已实现 LLM planner
  - `LLMPlanner`：构造 system prompt → `ChatWithRequest` + JSONMode → 解析 JSON plan
  - LLM planner 失败/空时自动回退到 `LocalWorkflow.planWithRules()`
- 已实现本地 workflow runner
  - 从问题中识别 `document/task/trace` 关键词与显式 ID
  - 支持 node/节点/步骤 关键词触发 `ingestion_task_node_query`
  - 支持诊断类问题优先命中 `*_diagnose` tool
  - 串行执行 tool
  - 汇总 `ToolContext`
  - 生成 `CallSummary`
  - 失败时记录 degrade 信息
- 已接入 `RagChatService`
  - tool workflow 位于 `retrieve` 之后、`prompt` 之前
  - RAG runtime 启动时默认注册并挂载本地 workflow + LLMPlanner
- 已实现诊断回答引导
  - `WorkflowResult` 支持 `AnswerGuidance`
  - 诊断类 tool 结果会生成“结论 / 证据 / 建议”式回答约束
  - prompt 构建阶段会把该引导注入 system message
- 已实现 tool 可观测性
  - SSE `tool` 事件：逐条下发 tool call 的 name / status / summary
  - 前端琥珀色可折叠卡片：展示每个 tool 调用的名称、状态图标、摘要
  - 失败时自动显示"部分失败"红色徽章
  - `rag_trace_node` 已记录 `tool_workflow` 子节点级别的每次 tool call（含 parent / depth / status / summary / duration）
  - trace 查询接口已透出节点 `extraData`，可用于后续展示 tool 调用链细节

## 最新进展

### 2026-05-08

#### 1. 把 retrieve 从模式分支重构为多通道检索基础架构

- `internal/app/rag/core/retrieve` 新增：
  - `search_types.go`
  - `channels.go`
  - `post_processors.go`
- 把原先 `service.go` 中写死的 `semantic / keyword / hybrid` 分支逻辑，重构为：
  - `SearchContext`
  - `SearchChannel`
  - `SearchChannelResult`
  - `SearchResultPostProcessor`
- 当前通道化后的实现仍然保持无 `intent` 依赖，但已为后续接入 `intent_directed channel` 预留扩展位

#### 2. 落地了首批无 intent 依赖的检索通道

- 新增 `vector_global` 通道
  - 负责全局向量语义检索
- 新增 `keyword` 通道
  - 负责关键词检索
- 当前对外仍兼容原有检索模式：
  - `semantic` → 仅启用 `vector_global`
  - `keyword` → 仅启用 `keyword`
  - `hybrid` → 同时启用 `vector_global + keyword`
  - `auto` → 仍由 query 特征推断模式，但内部执行已切到通道编排

#### 3. 落地了检索后处理链

- 新增 `fusion` 后处理器
  - 对多通道结果做统一聚合
- 新增 `dedup` 后处理器
  - 按 chunk ID 去重并保留高分结果
- 新增 `rerank` 后处理器
  - 在已有 rerank 能力之上统一进行重排
- 这样后续若接入更多通道（如 `intent_directed` / `title_match` / `metadata_filter`），不需要重写主检索流程

#### 4. 给 retrieve 结果补齐了可观测元数据

- `retrieve.Result` 新增：
  - `SearchChannels`
  - `ChannelStats`
- `RagChatService.runRetrieveStage()` 现在会把这些信息写入 retrieve trace 节点 `extraData`
- 当前 trace 已能记录：
  - 本次命中的检索通道列表
  - 每个通道的 chunk 数量
  - 每个通道的耗时
  - 通道级错误信息（若某一路失败但整体可降级继续）

#### 5. 调整了多子问题检索结果的聚合方式

- 之前 `RagChatService` 在 `rewrite` 产生多个子问题后，只按 chunk ID 做本地去重
- 现在改为先保留每个子问题的完整 `retrieve.Result`，再通过 `ragretrieve.MergeResults(...)` 聚合：
  - 合并 chunks
  - 合并 `SearchChannels`
  - 合并 `ChannelStats`
- 这样后续即使接入更多 channel / route hint，也不会丢失子问题级别的通道元信息

#### 6. 补齐了多通道检索相关测试

- 更新 `internal/app/rag/core/retrieve/service_test.go`
  - 覆盖 `semantic / keyword / hybrid` 下的通道输出
  - 覆盖一路失败、另一路成功的降级场景
  - 覆盖 `MergeResults` 的通道元数据聚合
- `internal/app/rag/service` 相关测试保持通过
- 验证命令：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/core/retrieve ./internal/app/rag/service -count=1
```

#### 7. 补了一轮 `retrieve auto` 决策校准与样本回放基础设施

- 新增 `internal/app/rag/core/retrieve/search_mode_decision.go`
  - 把 `auto` 模式决策统一收敛为 `AnalyzeSearchMode(...)`
  - 输出 `RequestedMode / ResolvedMode / Source / Reason / Signals`
- `retrieve` 内部与 trace 侧现在共享同一套检索模式决策结果
  - 通道执行使用同一份 `ResolvedMode`
  - retrieve trace 会补充 `searchDecisions`
  - channel metadata 会带上模式来源与降级信息
- 新增命令行回放工具 `cmd/retrieve-debug`
  - 支持批量回放 query 样本
  - 支持文本输出与 `-json` 输出
  - 可用 `expectedMode` 做 `PASS / FAIL` 对比
- 新增样本集 `testdata/retrieve_search_mode_samples.json`
  - 覆盖概念问答、精确匹配、资源 ID 查询、报错排障、代码符号定位、SSE 事件定位等真实问题
  - 当前样本回放结果已达到 `18/18 PASS`

#### 8. `retrieve` 后续明确计划

- 短期继续做“稳定性和可解释性”增强，而不是马上扩大量智能路由
  - 继续扩大真实 query 样本集，沉淀固定回放集
  - 继续校准 `auto` 模式边界，重点关注 query 误分流
  - 收口 `searchDecisions / channelStats` 的 trace 契约，方便前端消费
- 中期再补检索能力
  - 评估增加 `title_match`、`metadata_filter` 等无 intent 通道
  - 细化文档级去重、section/source 优先级、版本过滤
  - 继续明确部分失败、rerank 不可用等降级语义
- 当前对 `retrieve` 的定位是“持续优化项”
  - 优先级低于诊断质量、可观测性闭环和 ingestion 生产化一致性
  - 但高于更重的 Agent 智能化扩展

#### 9. 补了一轮 diagnose 结构化增强

- `document_ingestion_diagnose / task_ingestion_diagnose / trace_retrieval_diagnose` 三类结果统一补齐：
  - `diagnosisScope`
  - `facts`
  - `inferences`
  - `riskHints`
  - `nextActions`
- `confidence` 统一收口为固定口径：
  - `high`
  - `medium`
  - `low`
- `answer_guidance` 从“只消费 evidence”升级为“区分结论 / 事实 / 推断 / 风险提示 / 下一步建议”
- `trace_retrieval_diagnose` 新增弱相关命中判断：
  - 当 `retrieve.chunkCount > 0` 但 `retrieve.topScore` 偏低时，会输出“命中存在但 grounding 质量弱”的中置信度诊断
- 对应测试已补齐并通过：
  - diagnose payload 结构字段
  - degraded tool workflow 风险提示
  - weak topScore 检测

#### 10. 前端补齐了 `fallback` SSE 事件消费与可视化提示

- `frontend/src/hooks/useStreamResponse.ts`
  - 新增 `fallback` SSE 事件分支
  - `StreamHandlers` 新增 `onFallback`
- `frontend/src/types/index.ts`
  - `Message` 新增 `fallbackReason`
  - 新增 `FallbackPayload`
- `frontend/src/stores/chatStore.ts`
  - 在流式会话中把 `fallback` 事件绑定到当前 assistant 消息
- `frontend/src/components/chat/MessageItem.tsx`
  - 新增琥珀色提示条，明确提示“已回退到通用模型，需要注意核验”
- 至此后端已下发的 `fallback` 事件不再静默丢失，用户可以明确感知“低置信度检索 -> 通用模型回退”

#### 11. 固化了 integration test 的统一入口与 CI 骨架

- 新增仓库根目录 `Makefile`
  - `make test-go`
  - `make lint`
  - `make build`
  - `make integration-up`
  - `make test-integration`
  - `make integration-down`
- 新增 `.github/workflows/ci.yml`
  - frontend lint
  - backend unit test
  - frontend build
  - `docker compose` 拉起 `postgres + object-storage` 后执行 integration test
- 把 knowledge / ingestion pipeline integration test 从 `AutoMigrate` 改为 `postgresrepo.RunMigrations(db)`
  - 测试建表方式与 runtime 更一致
  - 降低“测试能过但运行时 schema 路径不一致”的风险

#### 12. 收口了一轮 ingestion 生产化一致性问题

- `KnowledgeDocumentService.OnIngestionTaskCompleted(...)`
  - 不再静默吞掉 `document / chunk_log` 回写错误
  - 改为返回聚合错误，便于上层日志和后续对账发现问题
- `finishPipelineChunkLogWithRecord(...)`
  - 优先按 `taskId` 精确命中 chunk log
  - 仅在按 task 未命中时才回退到 document 最新记录
  - 新增 task/document 不匹配保护，避免错误覆盖其他任务的 chunk log
- `MultiTaskObserver`
  - 从“某个 observer 失败就短路”调整为“所有 observer 尽量执行完，再聚合错误返回”
  - 避免 repository observer、metrics observer、knowledge bridge observer 互相拖累
- `ExecutorService`
  - task completion observer 失败时显式记录 error log
  - 修正成功日志中的 chunk 统计为 `current.Chunks`
- 新增测试
  - `internal/app/ingestion/service/multi_task_observer_test.go`
  - `internal/app/knowledge/service/knowledge_document_service_test.go` 中补齐 task-scoped chunk log 回写与 mismatch 场景

#### 13. 落地了 ingestion 对账规则与自动 reconcile 修复

- `internal/app/knowledge/service/knowledge_document_ingestion_reconcile.go`
  - 新增 `ReconcileIngestionTaskCompletion(...)`
  - 以 ingestion task 终态为准，对 `document / chunk_log` 做自动对账修复
  - 新增 task metadata 中 `documentId` 与入参 `documentId` 的 mismatch 保护，避免错误修复到其他文档
  - 当 `chunk_log` 缺失时，可按 `taskId` 自动补建最小可用记录
- `KnowledgeDocumentService.OnIngestionTaskCompleted(...)`
  - 在原有“即时回写”之后追加 reconcile，形成“完成回调修复 + 后续兜底修复”双层机制
- `KnowledgeDocumentService.ScanAndReconcileIngestionTasks(...)`
  - 新增按文档分页扫描的后台 reconcile 入口
  - 当前仅处理 `processMode=pipeline` 文档，并基于最新 `chunk_log.taskId` 反查 ingestion task 后做状态修复
- `internal/bootstrap/knowledge/runtime.go`
  - 将 ingestion reconcile scan 挂入现有 knowledge schedule loop，复用同一套后台 ticker 生命周期
- 新增测试
  - 覆盖 task 终态与 `document/chunk_log` 漂移时的自动修复
  - 覆盖 `chunk_log` 缺失时的自动补建
  - 覆盖 scan 入口按最新 `chunk_log.taskId` 触发 reconcile 的场景

#### 14. 补齐了 trace / tool / fallback 可观测性展示链路

- 后端 trace 数据补齐
  - `ChatTracer` 新增 trace run `extraData` 追加能力，用于把 fallback 等运行时元信息写入 `rag_trace_run`
  - `RagChatService` 在低置信度 fallback 触发时：
    - 写入 `trace run.extraData.fallback`
    - 额外落一条 `fallback` trace node
  - `trace_handlers.go` 现已透出 `rag_trace_run.extraData`
- 前端 trace 详情页增强
  - `frontend/src/pages/admin/traces/RagTraceDetailPage.tsx`
    - 新增 `Retrieve Observability` 面板：展示 `searchMode / chunkCount / topScore / searchChannels / channelStats / searchDecisions`
    - 新增 `Tool Workflow` 面板：展示 `used / toolCallCount / toolNames / degraded / degradeReason` 以及每次 `tool_call` 的状态、耗时、summary、error
    - 新增 `Fallback` 顶部提示：明确展示是否触发 fallback 及其原因
  - `frontend/src/services/ragTraceService.ts`
    - 补齐 `run.extraData` 与 `node.extraData` 类型字段
- 至此 trace 详情页已从“仅时间线”升级为“检索 + 工具 + fallback”三段式可解释观察面板

### 2026-05-07

#### 1. 建立了自研 tool 基础层

- 新增 `Tool` 接口与标准化的定义、调用和结果结构
- 新增 `Registry`，支持注册、查找、去重、列出定义
- 新增 `Executor`，统一执行 tool 并收口未知 tool / 调用失败
- 新增 `RenderContext` 和 `CallSummary`，把 tool 结果转为 prompt 可消费的上下文

#### 2. 落地了第一批只读 tool

- `document_query`：查询知识文档状态、处理模式、pipeline、chunkCount 等
- `ingestion_task_query`：查询 task 状态、来源、错误、可选节点摘要
- `trace_node_query`：查询 trace run 与节点摘要

#### 3. 实现了本地 workflow runner

- 新增 `LocalWorkflow`
- 采用规则驱动而非模型驱动
- 能从问题里识别显式 ID 并规划 tool call
- 默认最多串行调用 3 个 tool
- 失败时返回 degrade 信息，不阻断 chat 主链

#### 4. 已经把 workflow 接入 RAG runtime

- `RagChatService` 已支持注入 `toolWorkflow`
- `bootstrap/rag/runtime.go` 启动时会：
  - 创建 document / ingestion / trace 相关依赖
  - 注册首批 tool
  - 创建本地 `Executor + LocalWorkflow`
  - 调用 `chatService.SetToolWorkflow(...)`

#### 5. 落地了 LLM tool planner

- 新增 `Planner` 接口（`tool/workflow.go`）和 `PlanInput` / `PlanResult`
- 新增 `LLMPlanner`（`tool/planner/planner.go`）：通过 `ChatWithRequest` + JSONMode 让 LLM 输出 tool 调用计划
- `LocalWorkflow` 新增 `SetPlanner()`，planner 优先 → 规则 fallback
- `ChatRequest` 新增 `JSONMode` 字段，`buildRequestBody` 支持 `response_format: json_object`

#### 6. 补齐了第 4 个只读 tool: ingestion_task_node_query

- 全量模式（仅 taskId）：列出所有 node，summary 高亮 `failed=[...]` 和 `running=[...]`
- 单节点模式（taskId + nodeId）：返回节点完整详情（errorMessage、durationMs、output）
- `TaskService` 新增 `GetNode` 方法
- `LocalWorkflow.planWithRules()` 新增 node/节点/步骤 关键词识别

#### 7. 实现了 tool 可观测性展示层

- `RagChatEventSink` 接口新增 `SendTool(name, status, summary)`
- `sseChatSink` 实现 SSE `event: tool` 下发
- `Chat()` 中 tool workflow 完成后逐条向 sink 发送 tool call 摘要
- 前端 `Message` 类型新增 `toolCalls` 字段
- SSE 解析器新增 `"tool"` event case + `onTool` handler
- `chatStore` 新增 `appendToolCall()`，流式追加 tool call 到消息
- `MessageItem` 新增琥珀色可折叠工具调用卡片：Wrench 图标 + 状态图标 + summary

#### 8. 数据库迁移基础设施修复

- 修复所有迁移 SQL：`CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS`
- 修复 knowledge 迁移中未注释的 `DROP TABLE` Down 段
- 新增 `pg_trgm` 扩展到 vector 迁移
- `main.go` 调整启动顺序：先建临时库跑迁移，再启动 knowledge runtime

#### 9. 继续扩展了文档诊断能力

- 新增 `document_chunk_log_query`
  - 按 `documentId` 查询最近 chunk log，并聚合 ingestion task、task nodes、失败节点、最新错误
  - 用于 knowledge / ingestion 联合排障证据查询
- 新增 `document_ingestion_diagnose`
  - 综合 `document`、`recent chunk logs`、`ingestion task`、`task nodes`
  - 直接输出 `conclusion / confidence / evidence / suggestions`
  - 把系统从“查得到数据”推进到“能给出文档入库失败的高概率结论”

#### 10. 让诊断回答更稳定地结构化输出

- 新增 `tool/answer_guidance.go`
- `WorkflowResult` 增加 `AnswerGuidance`
- `LocalWorkflow` 在识别到诊断型 tool 结果时，生成“结论 / 证据 / 建议”式回答引导
- `core/prompt/service.go` 已支持把该引导注入 prompt
- `RagChatService` 已将诊断回答引导接入主链

#### 11. 把诊断入口扩展到 task 和 trace

- 新增 `task_ingestion_diagnose`
  - 面向 `task-*` 入口
  - 基于 `ingestion_task + ingestion_task_nodes` 输出结构化诊断结果
- 新增 `trace_retrieval_diagnose`
  - 面向 `trace-*` 入口
  - 综合 `trace run / trace nodes / trace node extraData`
  - 已能判断节点失败、`retrieve chunkCount=0`、命中过少、相关性偏弱等典型场景

#### 12. 完成了一轮真实样本联调验证

- 新增临时脚本 `tmp/seed_diagnosis_sample.go`
- 插入本地诊断样本：
  - `documentId = doc-1`
  - `taskId = task-diag-1`
  - 典型失败场景：`indexer` 节点 `connection refused`
- 前后端联调已验证：
  - 问题 `帮我诊断 document doc-1 的 ingestion 失败原因`
  - 能命中诊断 tool，并返回文档失败结论、关键证据和下一步建议

#### 13. 补强了诊断质量与 tool trace 落库

- 诊断质量增强
  - `document_ingestion_diagnose` 新增更细粒度证据：
    - `latestChunkLog.chunkCount`
    - `latestChunkLog.totalDurationMs`
    - `ingestionTask.chunkCount`
    - `ingestionNodes.total/success/failed/running/pending/lastNode/lastStatus`
  - `task_ingestion_diagnose` 新增节点分布统计与状态冲突判断：
    - task success 但 `chunkCount=0`
    - task success 但仍有 running node
    - task failed 但节点侧未捕获 failed node
  - `trace_retrieval_diagnose` 新增更细粒度 trace 证据：
    - `retrieve.topScore`
    - `toolWorkflow.status/callCount/degraded/degradeReason/toolNames`
    - 能识别“tool workflow 降级导致诊断证据可能不完整”
- tool trace 落库增强
  - `CallSummary` 新增 `DurationMs`
  - `LocalWorkflow` 执行时记录每次 tool call 的耗时
  - `RagChatService` 把每次 tool call 作为独立 `rag_trace_node` 落库：
    - `node_id = tool_01 / tool_02 / ...`
    - `parent_node_id = tool_workflow`
    - `depth = 2`
    - `node_type = tool_call`
    - `node_name = tool name`
    - `error_message` 与 `extraData.summary/duration/toolStatus` 同步写入
  - RAG 主链已有 trace node 的 `start_time / end_time / duration_ms` 改为真实耗时，不再统一为 `0ms`
- trace 查询接口增强
  - `trace_handlers.go` 已透出 `rag_trace_node.extraData`
  - 前端 `ragTraceService.ts` 已补齐 `extraData` 类型字段

### 2026-05-05

- Ingestion metrics API 落地
- Ingestion 管理页接入 metrics 面板
- RAG rewrite 产出 `PreferredSearchMode`
- retrieve 支持 `auto` 检索决策
- chat 前端展示本次检索策略

## 当前验证状态

截至 2026-05-07，以下验证已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/tool/planner -count=1   # 12 tests PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/tool/... -count=1      # ALL PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/core/prompt -count=1   # PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/... -count=1           # ALL PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/service -count=1       # 28 tests PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/bootstrap/rag -count=1         # PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/infra-ai/... -count=1          # ALL PASS
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/framework/... -count=1         # ALL PASS
```

**端到端联调验证（2026-05-07）：**

- 后端启动 + 自动迁移 → 成功
- SSE `event: tool` 事件下发 → 已验证
- 前端 tool call 卡片渲染（成功/失败状态图标 + summary） → 已验证
- 多条 rule 路线问题触发工具调用 → 已验证
- 数据库插入真实数据后 tool 查询成功路径 → 已验证
- `document_ingestion_diagnose` 已通过真实样本命中验证
- 诊断回答已能稳定朝“结论 / 证据 / 建议”结构靠拢
- `rag_trace_node` 已能记录每次 tool call 的独立节点（含状态、摘要、耗时、父子关系）
- trace 查询接口已能返回节点 `extraData`

**增量验证（2026-05-08）：**

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service -count=1
```

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS

**后续增量验证（2026-05-08）：**

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/knowledge/service ./internal/app/rag/service ./internal/adapter/http/rag -count=1
```

- `internal/app/knowledge/service` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）

## 当前已知问题与风险

1. `tool` 决策仍是本地规则 + LLM planner 双轨
   LLMPlanner 已落地，但"大模型判断是否需要调用 tool"的完整 Agent 形态尚未完全打磨（planner 与 rule fallback 的边界需继续调整）。

2. 多通道检索目前仍是“无 intent 依赖”版本
   已完成 `vector_global + keyword` 的通道化重构，但还没有接入 `intent_directed`、标题检索或 metadata 定向检索；当前提升集中在召回质量与可解释性，而非更高层的路由决策质量。

3. 诊断能力已形成最小闭环，但准确率和可信度仍需继续提升
   当前已具备 `document / task / trace` 三类诊断入口，但更多仍是基于状态和规则的高概率判断；后续需要补更细粒度证据提取、冲突证据处理和更稳定的置信度评估。

4. `RagChatEventSink` 接口扩展后需要更新所有实现
   已更新 `sseChatSink` 和 `fallbackSinkStub`，需关注是否有其他实现。

5. `ingestion` 生产化仍未完全收口
   虽然已补上 task 完成后的即时 reconcile 与后台定时 reconcile scan，但仍需要继续完善修复结果沉淀、异常统计、对账结果暴露和更系统的恢复策略。

6. integration test 尚未纳入固定执行流程
   已有集成测试能力，但仍未接入 CI 或固定 compose 场景。

7. trace 可观测性已具备首轮闭环，但仍需继续产品化
   trace 详情页已能展示 `retrieve / tool / fallback` 关键元数据，但列表页概览、聊天与 trace 联动、异常筛选和更多运行态摘要仍需继续补齐。

## 下一步计划

### P0

- 稳定多通道检索基础架构
  - 继续细化 `auto` 模式下的 channel 启停策略
  - 继续验证精确匹配类 query 的召回质量提升
  - 评估是否补充 `title / metadata` 等无 intent 依赖通道

- 提升诊断结果质量
  - 继续细化 `document / task / trace` 诊断证据提取
  - 继续让结论、事实、推断、建议边界更清晰
  - 继续稳定置信度判断逻辑

- 收口 ingestion 状态一致性与恢复能力
  - 增加 `document / chunk_log / task` 不一致状态的对账规则
  - 评估后台 reconcile loop 的最小落地方案
  - 继续完善 observer 错误观测与回写失败恢复

### P1

- 完善 tool trace 可视化与消费链路
  - 继续把 `rag_trace_run.extraData`、`rag_trace_node.extraData` 用于列表摘要、聊天/trace 联动和异常筛选
  - 进一步区分 `tool_workflow` 汇总节点与 `tool_call` 子节点的展示语义

- 增强诊断闭环价值
  - 对常见失败模式给出更具体的排障建议
  - 逐步沉淀结构化错误归因

- 继续补强 ingestion 生产化
  - indexer 幂等和补偿
  - task/document/chunk_log 状态联动保护
  - 失败排障信息结构化

### P1

- 完善 LLMPlanner
  - planner 识别真实 ID 的能力（目前依赖 LLM 理解问题中的显式 ID）
  - planner 与 rule fallback 的可观察性对比

### P2

- 评估 EINO 在 tool workflow 编排层的接入
  - 不改写 `RagChatService` 业务外壳
  - 只替换 tool workflow 的执行层
