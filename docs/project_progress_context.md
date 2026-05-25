# Project Progress Context

更新时间：2026-05-22

这份文档用于维护 `goagent` 当前项目进度，帮助后续开发快速对齐当前阶段、已完成能力、最新进展、验证状态、已知风险和下一步计划。

## 当前阶段

项目已经从“基础能力搭建期”进入“主链路闭环、联调和质量收口期”。

当前可以分成四条主线来看：

1. `Knowledge`
   已具备完整的知识库、文档、chunk、调度和管理端能力，重点从“继续铺功能”转向一致性、状态联动和排障体验。

2. `Ingestion`
   已经跑通 `pipeline -> task -> task_node -> knowledge 回写` 的最小可用闭环，也补上了重试、补偿、即时 reconcile、后台 reconcile scan 和基础 metrics。当前仍有后续优化空间，但**短期内不再作为主工作模块**，仅维持必要修复和被动配套。

3. `RAG`
   已形成最小 chat 闭环，支持多轮对话、rewrite、retrieve、prompt、trace、fallback，重点开始转向检索策略优化、可解释性和 Agent 能力扩展。

4. `Agent / Tool`
   已完成第一阶段基础设施：自研 tool 抽象、tool registry、tool executor、AgentLoop V1（支持并行执行）、LLMPlanner、LLMObserver（支持多 hint + think 隔离 + 解析失败日志），以及接入 `RagChatService` 的扩展点。工具集已从 8 个扩展至 15 个（+ document_list / task_list / web_search / web_fetch / think / 3 Eino Graph）。当前处于“LLM 主决策 + diagnose 一步到位 + 规则 fallback/guardrail + RAG 优先联网搜索 + 外部证据工作流”的稳定 agent 化阶段。

## 已完成能力

### 基础设施

- `infra-ai`
  - chat（含 JSONMode `response_format` 支持）
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
  - knowledge / vector / user / rag / ingestion 五组嵌入式 SQL 迁移
  - 自定义迁移执行器，支持幂等 `IF NOT EXISTS`
  - 启动时自动执行迁移

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
  - 节点重试与指数退避
  - `Indexer` 失败补偿清理
  - `task_node` 重试信息持久化
  - `document` 级活动 task 保护
  - task-scoped chunk log 回写保护
  - 即时 reconcile 与后台 reconcile scan
  - reconcile 结果接入 ingestion metrics
  - `internal/app/ingestion/service` 文件结构已按 `service / workflow / runner / observer` 分组整理

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
- 已完成的 RAG 增强
  - LLM rewrite
  - memory compression
  - `semantic / keyword / hybrid / auto` 检索模式
  - 多通道检索基础架构（`channel + processor + context`）
  - `vector_global / keyword / metadata_title` 三路检索通道
  - 低置信度 fallback
  - SSE `meta` 下发 `searchMode`
  - prompt 支持单独注入 `ToolContext`
- 检索评估基础设施
  - `internal/app/rag/evaluation`
  - `cmd/retrieve-eval`
  - `Hit@K / Recall@K / MRR`
  - 支持离线样本评估与真实 retrieve 回放执行

### Agent / Tool

- `internal/app/rag/tool`
  - `Tool`
  - `Definition / Call / Result`
  - `Registry`
  - `Executor`
  - `RenderContext`
  - `Workflow`
  - `Planner + PlanInput / PlanResult`
- 已实现 tool（15 个）
  - 诊断类：`document_ingestion_diagnose`（含 live task node 补齐）、`task_ingestion_diagnose`、`trace_retrieval_diagnose`
  - 查询类：`document_query`、`document_chunk_log_query`、`ingestion_task_query`、`ingestion_task_node_query`、`trace_node_query`
  - 发现类：`document_list`（按 status/query 分页）、`task_list`（按 status/pipelineId 分页）
  - 外部类：`web_search`（可配置 SearchProvider：DuckDuckGo / Tavily / Tavily MCP，支持 MCP 主路 + API fallback）、`web_fetch`（网页正文提取，并发支持）
  - 元工具：`think`（推理记录，无副作用）
  - Graph：`document_root_cause_diagnosis`、`document_diagnose_with_search`、`external_evidence_workflow`
- 已实现能力
  - LLM planner（含 retrieve/rewrite 上下文注入）
  - LLM observer（主 observer，支持多 hint、think 隔离、taskNodeSummary 强制下钻）+ RuleObserver fallback（跳过 think）
  - AgentLoop V1（Plan -> Act -> Observe，支持并行执行 + 墙钟/累计耗时观测）
  - 接入 `RagChatService`
  - 诊断回答引导（深度证据升级 + 状态冲突归一）
  - baseRules 开放问题处理（"最近哪些文档失败了？" → `document_list`）
  - SSE `tool / tool_start / tool_result / agent_think` 事件
  - trace `agent_round / tool_call / agent_observation` 落库
  - 通用 MCP 基础设施首版：stdio `Manager`、懒启动 session、`ListTools / CallTool`、runtime 生命周期回收
  - Tavily MCP 接入：`web_search` 可走 `tavily-mcp -> tavily/duckduckgo fallback`

## 最新进展

### 2026-05-18

#### 1. Tool 模块 P0 收口推进了一轮

- 新增 `docs/tool_module_constraints.md`，明确后续改造遵循：
  - `module-first`
  - 显式依赖注入优先
  - 顶层 `tool` compat 层不再继续承载新语义
  - assembly 失败优先降级而不是 `panic`
- 主生产路径进一步去全局化：
  - `AgentLoop` / workflow control 推断链路改为优先走显式 registry
  - assembly 默认 workflow 组装不再依赖包级 registry setter
- graph tool 装配失败从 `panic` 改为 warning + skip，对复杂 family 的降级更友好
- 顶层 compat 继续瘦身，已删除：
  - `agent_loop_forward.go`
  - `runtime_forward.go`
- 图工具对 `Executor` 的依赖也已切到 `runtime.Executor` 真源，减少 facade 依赖面

#### 2. 落地了 Tavily MCP 与通用 MCP 底座（首版）

- 新增 `internal/infra-mcp`
  - `Manager`
  - `ServerConfig`
  - stdio command transport 懒连接
  - `ListTools / CallTool / Close`
- `bootstrap/rag.Runtime` 现在会创建并持有 `mcpManager`，关闭 runtime 时统一回收 MCP 资源
- `config` 扩展：
  - `rag.mcp.servers.<name>`
  - `rag.search.web-search.fallback-provider`
  - `rag.search.web-search.mcp.server`
  - `rag.search.web-search.mcp.search-tool`
- `web_search` provider 层完成三件事：
  - 新增 `TavilyMCPProvider`
  - 新增 `FallbackSearchProvider`
  - `buildSearchProvider(...)` 支持 `tavily-mcp`
- 当前默认策略已调整为：
  - `provider=tavily-mcp`
  - `fallback-provider=tavily`
- `web_search` 对上层契约保持不变，但结果 metadata 现在可显式表达：
  - `provider=tavily-mcp`
  - `providerActual=tavily`
  - `providerFallbackUsed=true/false`

#### 3. 回归验证

- `go test ./internal/infra-mcp ./internal/app/rag/tool/... ./internal/bootstrap/rag ./internal/framework/config -count=1` PASS
- `go test ./... -run Test^$ -count=1` PASS
- Tool 行为未回退：
  - `web_search -> web_fetch`
  - `external_evidence_workflow`
  - `document_diagnose_with_search`

### 2026-05-10

#### 0. 补了一轮 `doc_run_01` / 并发联调的工程收口

- `AgentLoop` 已支持配置化：
  - `rag.agent.max-iterations`
  - `rag.agent.parallel-tool-calls.enabled`
  - `rag.agent.parallel-tool-calls.max-concurrency`
- 当前联调默认配置已打开 `parallel-tool-calls.enabled=true`
- 并发执行保留了稳定的展示/汇总语义：
  - `tool_start` 仍按规划顺序发出
  - `tool_result` 仍按规划顺序汇总发出
  - `WorkflowResult.Calls / RoundSummary.Calls / trace` 保持稳定顺序
- `RoundSummary` 与 `agt_round.extraData` 已补充：
  - `executionMode`
  - `toolCallCount`
  - `wallClockDurationMs`
  - `totalToolDurationMs`
- 受控测试中，2 个独立 tool 并发执行时：
  - `serial` 约 `80ms`
  - `parallel` 约 `40ms`
  说明并发调度已正确运行，且墙钟时间存在可观下降

#### 0.1 修复了联调中 `document / task / trace` 关键字被误识别为 ID 的问题

- `workflow_helpers.go` 里的 ID 正则此前过宽：
  - 会把自然语言中的 `document`
  - `task`
  - `trace`
  直接当成真实 ID
- 联调时已在日志中确认出现错误查询：
  - `SELECT ... WHERE id = 'document'`
- 现已将 ID 提取收紧为带分隔符的结构化 ID：
  - `doc_run_01`
  - `task_run_01`
  - `trace_bad_01`
- 修复后：
  - 普通关键词不会再被当成实体 ID
  - `doc_run_01 / task_run_01 / trace_bad_01` 这类联调样例可稳定命中真实实体

#### 0.2 收紧了 base rules，避免同一轮对同一实体做过深规划

- 此前对同一实体会在同一轮同时规划多层调用，例如：
  - `document_ingestion_diagnose + document_query`
  - `task_ingestion_diagnose + ingestion_task_query`
- 这会导致：
  - 后执行的浅层结果覆盖前面的深层诊断
  - `doc_run_01` 这类运行中场景出现“这轮说 failed、下一轮又说 running”的跳变
- 现在改为：
  - 同一轮只做当前最浅的一层必要 lookup
  - 后续通过下一轮继续下钻
- 对 `doc_run_01` 这类问法，已改为优先走 `document_ingestion_diagnose`，不再先被 `document_query` 的 `document.status` 带偏

#### 1. 补齐了 `AgentLoop V1` 在标准失败样例上的多轮下钻稳定性

- 通过 `doc_fail_01` 联调确认了一个关键问题：
  - `document_ingestion_diagnose` 只要拿到 `latestLogError` 就会提前结束
  - 导致链路停在 task/chunk log 级摘要，不能稳定继续下钻到 `task / node`
- 已调整 `RuleObserver`：
  - 只有拿到真正的 `latestNodeError` 才允许直接结束
  - 若只有 `latestTaskId + latestLogError`，继续走 `ingestion_task_query(includeNodes=true)`
  - `ingestion_task_query` 若已暴露 failed/running node，则继续走 `ingestion_task_node_query`
- 结果是 `doc_fail_01` 现在可以稳定走通：
  - `document_ingestion_diagnose`
  - `ingestion_task_query`
  - `ingestion_task_node_query`

#### 2. 补强了 `planCallsFromResults` 与 structured hint 的保底能力

- `planCallsFromResults` 新增回退链路：
  - `document_query -> document_ingestion_diagnose`
  - `document_chunk_log_query -> ingestion_task_query / document_ingestion_diagnose`
  - `ingestion_task_query -> ingestion_task_node_query / task_ingestion_diagnose`
- `buildNextHint(...)` 现在支持稳定序列化布尔参数，`includeNodes=true` 不再丢失
- planner prompt 增加 few-shot 风格约束，强化：
  - 优先使用 structured hint
  - 避免重复调用
  - 证据足够时返回空 `tools`

#### 3. 修复了 Agent trace 节点落库失败问题

- 联调期间 PostgreSQL 暴露：
  - `agent_observation`
  - `agent_observation_01`
  等命名超过了 `t_rag_trace_node.node_type varchar(16)` 限制
- 已压缩 trace 节点命名：
  - `agent_round` -> `agt_round`
  - `agent_observation` -> `agt_obs`
  - `agent_round_01` -> `agt_round_01`
  - `agent_observation_01` -> `agt_obs_01`
- 现在 `agent_round / agent_observation` 节点可以正常落库，便于复盘多轮 observe 行为

#### 4. 修正了最终回答优先采用较弱 diagnose 结论的问题

- `BuildAnswerGuidance(...)` 之前会优先取前面较弱的 diagnose 结果，导致即使后面已经拿到 node detail，最终回答仍可能说“未捕获到具体失败节点”
- 现在改为：
  - 优先选最新 diagnose
  - 再扫描后续更深一层的 `ingestion_task_node_query`
  - 若拿到同一 `taskId` 的节点级结果，则升级：
    - `conclusion`
    - `confidence`
    - `facts`
    - `inferences`
- `doc_fail_01` 最终已经能稳定收敛到：
  - 失败节点是 `indexer`
  - 节点错误是 `connection refused: vector store unavailable`
  - `high` 置信度

#### 5. 当前联调判断

- `doc_fail_01` 已达到当前阶段验收预期
- 说明 `document -> task -> node` 这条高频排障链路已经从“可运行”推进到“可稳定联调”
- 下一步更适合切到：
  - `doc_run_01`
  - `task_run_01`
  - `trace_bad_01`
  继续验证运行中与 retrieval 诊断场景

#### 5.1 对 `doc_run_01` 的最新联调判断

- 当前 `doc_run_01` 暴露出的主要问题不再是“基础链路不通”，而是：
  - 模糊运行态问法过早落到 `document_query`
  - 自然语言关键词被误识别为 `document / task / trace` ID
  - 同一轮对同一实体规划过深，导致浅层结果覆盖深层诊断
- 上述三类问题已完成首轮修复，并补了对应回归测试
- 当前更合理的判断是：
  - `doc_run_01` 已进入“运行中场景稳定性打磨”阶段
  - 后续重点从“能否下钻”转向“回答是否一致、是否优先采用真实运行态证据”

#### 6. 落地了 `LLMObserver`，并修复了其首轮参数幻觉回归

- `AgentLoop` 默认 observer 已从 `RuleObserver` 切到 `LLMObserver`
- `RuleObserver` 当前保留为 fallback/guardrail，处理：
  - LLM 不可用
  - LLM 输出非法 JSON
  - LLM 输出与已有证据不一致的 hint/call
- 联调 `doc_fail_01` 时暴露出新的回归：
  - `LLMObserver / LLMPlanner` 在 `task_query` 场景下只看到 `nodes=4` 这类弱摘要
  - 没有稳定拿到真实节点名 `indexer`
  - 进而幻觉出不存在的 `node_0`
  - 导致 `ingestion_task_node_query(task_fail_01, node_0)` 查询失败
  - 最终回答退回 `medium`，无法收敛到节点级根因
- 已完成修复：
  - 新增统一的 LLM 结果摘要，显式暴露：
    - `latestTaskId / latestNodeId / latestNodeError`
    - `taskNodeSummary`
    - `nodes`
  - `ingestion_task_query` summary 补充 `interestingNodes=[...]`
  - 对 `LLMObserver` 输出的 `nextHint` 做证据一致性校验
  - 对 `LLMPlanner` 真正规划出的 tool call 也做相同校验
  - 若参数不来自已有证据，则自动拒绝并退回规则路径
- 修复后：
  - `doc_fail_01` 再次稳定收敛到：
    - 失败节点是 `indexer`
    - 节点错误是 `connection refused: vector store unavailable`
    - `high` 置信度

#### 7. 对 `Planner / Observer / hint` 交互边界做了一轮工程收口

- `LLMPlanner` 已接入与 `LLMObserver` 同步的 `rewrite / retrieve` 摘要：
  - `SummarizeRewriteResultForLLM(...)`
  - `SummarizeRetrieveResultForLLM(...)`
  - Planner 不再只依赖 `Question / AgentState / PreviousResults`，避免对检索阶段“失明”
- `LLMObserver` 已移除 `Reasoning -> Hypothesis` 的错误回退：
  - `Hypothesis` 为空时仅继承 `PreviousState.Hypothesis`
  - 不再把“下一步动作说明”污染进“当前状态假设”
- `SummarizeResultDataForLLM(...)` 已从白名单改为黑名单模式：
  - 保留高频关键字段优先输出
  - 自动补充其余未知字段
  - 仅排除 `rawBody / fullText / rawText / rawContent / originalText` 等噪音字段
- 已清理 `LocalWorkflow` dead code：
  - 删除 `internal/app/rag/tool/local_workflow.go`
  - 删除 `internal/app/rag/tool/local_workflow_test.go`
  - 将仍被主链路复用的 helper 抽离到 `internal/app/rag/tool/workflow_helpers.go`
- `nextHint` 已完成“结构化优先、字符串兼容”的迁移：
  - 新增 `HintCall`
  - 新增 `AgentState.NextHintCalls`
  - 新增 `ObserveResult.NextHintCalls`
  - `RuleObserver / AgentLoop / LLMObserver / Planner` 已以 `nextHintCalls` 作为主语义
  - 旧 `NextHint string` 现主要保留为兼容输出与 trace/debug 可读字段
### 2026-05-11

#### 1. 工具集从 8 个扩展到 10 个

- 新增 `document_list` — 按 status/query/knowledgeBaseId 分页查询文档列表
- 新增 `task_list` — 按 status/pipelineId 分页查询 ingestion 任务列表
- 新增 `web_search` — 基于 DuckDuckGo Instant Answer API 的外部搜索，免费无需 API Key
- 新增 `think` — 元工具，Planner 记录推理过程，不产生副作用，不干扰 Observer 决策
- `document_list`/`task_list` 已接入 baseRules：当无具体 ID 匹配且检测到开放关键词时自动触发

#### 2. Observer 多 hint 放开

- Observer prompt 规则 #3 从 `exactly one` 改为 `one or more`
- 新增 few-shot 示例：同时 hint `ingestion_task_node_query` + `trace_node_query`
- `parseResponse` 校验改为遍历所有 hint name

#### 3. Observer 规则强化与 think 隔离

- 新增规则 #4：taskNodeSummary 中含 failed/running node 时 MUST 继续
- 新增规则 #9：think 结果不用于决策，Observer 取 `lastNonThinkResult`
- 新增 few-shot 示例 #3：task query 显示 indexer(failed) 但无 errorMessage → 继续
- `RuleObserver` 新增 `lastNonThinkResult()`

#### 4. `document_ingestion_diagnose` 一步到位

- 新增可选的 `ingestionTaskNodeReader` 依赖，通过 `taskService.ListNodes()` 补齐 chunk log 节点数据
- chunk log 无失败节点时从 live task nodes 获取节点名和错误
- conclusion 从 "no failed node was captured, medium" → "failed at node X, high"

#### 5. Answer guidance 状态冲突归一

- `enrichDiagnosisWithDeeperEvidence` 新增 `resolveStatusConflict`
- diagnose 说 failed 但 task/node 实际 running 时，覆盖结论为“仍在处理中”
- 同时查 task 和 node 两级证据（新增 `findLatestTaskResult`）
- guidance 文本改为强约束：状态不一致时以 task/node 为准

#### 6. LLMObserver 解析失败日志

- LLM 调用失败和 JSON 解析失败时 `log.Warnf` 记录降级原因和原始响应

#### 7. 测试覆盖 49 → 71

- 新增：document_list、task_list、web_search、think 集成、open-ended baseRules、多 hint 校验、状态冲突归一、空名拒绝


### 2026-05-11

#### 1. 检索模式简化（P0）

- 删除 `search_mode_decision.go`（~289 行），消除三层启发式打分
- `channels.go` 三个 `Enabled()` 方法改为纯基础设施检查，3 通道始终全部启用
- `search_types.go` 移除 `ResolvedMode / ModeDecision / QueryHints`
- `rag_chat_service.go` 的 `resolveRetrieveSearchMode()` 直接返回 `"hybrid"`
- 删除 `cmd/retrieve-debug/`（依赖已废弃的 `AnalyzeSearchMode`）
- 测试更新：通道数期望值更新为 3，删除 8 个模式决策测试
- 净删除 ~350 行

#### 2. 状态机去重（P0）

- 新增 `internal/app/rag/tool/next_action.go`（120 行）——单一决策源
- `nextAction(result)` 覆盖 5 种工具类型的 "结果 → 下一步" 映射
- `planCallsFromResults` 从 60 行退化为 9 行薄适配层
- `RuleObserver` 5 个 `observe*` 函数改为 `switch reason` 模式
- 净减少 ~39 行，关键是消除双重维护风险

#### 3. Result 类型安全读取（P1）

- `Result` 新增 4 个方法：`GetString(key)`, `GetInt(key)`, `GetStringSlice(key)`, `PreferStringSlice(primary, fallback)`
- 替换 59 处 `readDataString(result.Data, ...)` → `result.GetString(...)`
- 替换 18 处 `result.Data["key"].(string)` → `result.GetString("key")`
- 独立 helper 函数保留给原生 `map[string]any` 场景

#### 4. Eino Graph as Tool 接入（P1）

- 引入 `github.com/cloudwego/eino v0.8.13` + 传递依赖
- 新增 `DiagnosisGraphTool`：Eino 线性 Graph，3 个 Lambda 节点（diagnose → task_query → node_query）
- Graph 编译为 `Runnable`，每个 Lambda 闭包捕获 `*Executor` 调用已有 tool
- 注册到 `buildLocalToolWorkflow`，`planWithBaseRules` 诊断关键词路由到 graph tool
- 新增 `agent_loop_graph_test.go`，9 个集成测试覆盖 7 种路由场景 + 真实 Eino 链执行

**效果：** 确定性诊断场景 LLM 调用从 6 次降到 0 次（baseRules 路由 + Eino 链 + RuleObserver 终止）

#### 5. AgentState 合并简化

- `llmObserverResponse` 去掉顶层 `confidence/nextHintCalls/nextHint`，LLM 只输出 `state` block
- `parseResponse` state block 成为单一数据源
- `agent_loop.go:148-169` 合并逻辑从 20 行减到 11 行

#### 6. Graph Tool 对 Observer 可读性提升

- `DiagnosisGraphTool` 产出新增 `diagnosisDepth` 字段：`node_level` / `task_level` / `diagnose_only`
- `RuleObserver` default 分支按 depth 分叉 confidence：0.95 / 0.75 / 0.6
- `result_summary.go` 新增 `diagnosisDepth` priority key（LLM 可见）

#### 7. Diagnose + Web Search Graph（新增能力）

- 新增 `DiagnoseSearchGraphTool`：Eino Graph `document_root_cause_diagnosis → web_search`
- 零 LLM 关键词提取：`extractSearchKeyword()` 匹配 20 个技术错误模式 + `looksLikeTechnicalError` 启发式
- 无技术错误时自动跳过搜索
- base rules 新增路由：含"解决/修复/solution/fix"关键词时路由到 diagnose+search

#### 8. 工具集现状

- 现有 13 个 tool：10 个 builtin + 1 个 think + 2 个 Eino Graph Tool
- Graph Tool：`document_root_cause_diagnosis`（3 跳诊断）、`document_diagnose_with_search`（诊断+搜索）

### 2026-05-12

#### 1. Agent Search 能力落地（RAG 优先）

核心原则：**只在知识库检索结果不足时（chunks=0、低分、通道全错误）才触发联网搜索**，诊断链路不受影响。

- 新增 `WebFetchTool`（`internal/app/rag/tool/builtin/web_fetch_tool.go`）
  - 支持 1-3 个 URL 并发抓取，提取网页正文（正则剥离标签、过滤导航行、截断 8KB）
- 抽象 `SearchProvider` 接口
  - `DuckDuckGoProvider`：原有逻辑（免费，国内被墙）
  - `TavilyProvider`：新增（`web_search_tavily.go`），Tavily Search API，国内可访问，免费 1000 次/月
- 新增 `RagWebSearchConfig`（`rag.search.web-search.provider` / `api-key`）
  - `runtime.go`：`buildSearchProvider(cfg)` 根据配置选择 provider

#### 2. 搜索链路集成到 AgentLoop

- `planWithBaseRules`：无特定 ID + `kbInsufficient(RetrieveResult)` → 自动规划 `web_search`
- `nextAction::nextActionWebSearch`：`web_search` 有结果 → `web_fetch(urls=[前3个URL])`
- `RuleObserver`：
  - 新增 `observeWebSearch()` / `observeWebFetch()` 观察函数
  - `kbInsufficient()`：`len(Chunks)==0` 或 `topScore<0.4` 或全通道错误
  - `document_list`/`task_list` 返回空 + KB 不足 → 触发 `web_search`
- `LLMObserver`：新增 3 个 few-shot 示例（#6-#8）+ 规则 #10（检索质量评估）
- `answer_guidance::buildWebSearchGuidance()`：信源标注、矛盾显式化、知识库优先、局限性说明

#### 3. 全链路日志补强

此前 tool 包仅 3 处日志调用，补强后覆盖：

- `executor.go`：`[tool] <name> started/success/failed (<N>ms)` + 参数 + 摘要
- `agent_loop.go`：`[agent] start` / `round N: M call(s) [names] (mode)` / `observer: DONE/CONTINUE` / `done: N rounds, M calls`
- `observer_rule.go`：`[observer] kb insufficient (chunks=N), triggering web_search`

#### 4. 工具集现状

- 现有 **14 个 tool**：11 个 builtin + 1 个 think + 2 个 Eino Graph Tool
- 新增：`web_fetch`

#### 5. P0 收口：结果视图层、执行语义与 capability trace 元数据

- 新增 `internal/app/rag/tool/result_views.go`
  - `WebSearchResultView`
  - `WebFetchResultView`
  - `DiagnosisResultView`
  - 统一承接 `Result.Data` 中常见的 `[]map[string]any` / `[]any` 混合结果，减少下游重复猜字段结构
- `answer_guidance.go` / `renderer.go` / `next_action.go` / `observer_rule.go` 已切到优先消费 typed view
  - `diagnose` guidance 通过 `DiagnosisResultView` 读取结果
  - `web_search` / `web_fetch` 的 renderer 与 observer 不再散落解析原始 map
  - `nextAction` 的 web 分支基于 view 提取 URL，降低链路耦合
- `buildWebSearchGuidance()` 补充“本地/知识库侧已知证据”与“外部网页来源”分层表达
  - 回答阶段会显式区分 KB 证据与外部来源，便于后续做信源评级与引用排序
- 新增 `internal/app/rag/tool/workflow_control.go`
  - `ExecutionMode`: `read_only` / `proposal_only` / `guarded_write`
  - `RiskLevel`: `low` / `medium` / `high`
  - `ApprovalRequirement`: `none` / `recommended` / `required`
  - `Capability`: `knowledge` / `diagnosis` / `search` / `general`
  - `WorkflowTraceMeta`: 记录能力域、证据来源与退化状态
- `WorkflowInput` / `WorkflowResult` 已显式携带 `Control` 与 `TraceMeta`
  - 当前 `runToolWorkflowStage(...)` 默认注入 `read_only + low + none`
  - `AgentLoop` 结束时会推导 capability 与 evidence sources
- capability 级 trace 元数据已补入运行链路
  - 证据来源当前识别：`knowledge_base` / `system_records` / `rag_trace` / `external_web`
  - `rag_chat_service.go` 会把 `toolWorkflow.control`、`toolWorkflow.traceMeta`、`toolCallCount`、`roundCount` 写入 trace run extraData
  - `chat_tracer.go` 会把 `capability` / `workflowMode` / `riskLevel` / `approvalRequirement` / `evidenceSources` 写入 `agt_round` / `agt_obs`
- prompt 上下文新增 `WorkflowPolicy`
  - `prompt.Context` 会额外渲染 `## 执行约束` 系统消息，让回答阶段显式知道当前工作模式与审批边界
- 这一轮 P0 的实际收益
  - 把高频结果消费从“散读 map”收成“typed result contract”
  - 把 workflow 运行语义从隐式约定收成显式上下文
  - 把 trace 从“看见调用过程”推进到“看见能力域、风险等级与证据类型”

#### 6. 外部证据工作流收口（web_search → 信源评估 → web_fetch → 质量审核 → readiness）

- 新增 `internal/app/rag/tool/graph/external_evidence_workflow_graph.go`
  - 以 Eino Graph 方式固化 `web_search -> select -> web_fetch -> assess`
  - `select` 阶段不再只是“拿前三个 URL”，而是显式产出 `sourceReview`
  - `assess` 阶段拆成两层：
    - `qualityAssessment`：外部证据质量、来源多样性、交叉印证、抓取成功/失败/截断情况
    - `readiness`：当前证据是否足以支持最终回答
- `external_evidence_workflow` 结果契约补齐：
  - `selectedUrls / selectedDomains / selectedSourceTypes`
  - `sourceCoverage / sourceReview`
  - `quality / qualityConfidence / qualityReasoning / qualityAssessment`
  - `readiness / readinessConfidence / readinessReasoning / citedUrls`
- `result_views.go` 新增 `ExternalEvidenceWorkflowView`
  - 统一读取 `sourceReview` 与 `qualityAssessment`
  - 为 renderer / answer guidance / 前端联调提供稳定消费入口
- `answer_guidance.go`
  - 新增 `buildExternalEvidenceGuidance(...)`
  - 回答阶段现在会显式区分：
    - 本地/知识库证据
    - 外部网页来源
    - 来源质量与局限
    - 引用 URL
- `renderer.go` / `result_summary.go`
  - `external_evidence_workflow` 现在会把 selected sources、quality、readiness 渲染进 ToolContext / LLM 摘要
- 新增测试：
  - `eino_external_evidence_workflow_graph_test.go`
  - `tool_test.go` 中 external evidence guidance / view 断言
  - `result_summary_test.go` 中 external evidence 字段摘要断言

#### 7. 联调收口：Tool 卡片事件字段对齐 + 工作流阶段日志

- 前后端联调中发现 Tool 卡片只显示空白 running 占位：
  - 后端 `ToolCallEvent` 无 JSON tag，SSE 序列化后字段为 `CallID / Name / Summary`
  - 前端按 `callId / name / summary` 读取，导致名称、摘要、参数、耗时全部丢失
- 已修复后端事件结构：
  - `internal/app/rag/tool/workflow.go` 为 `ToolCallEvent` 补齐 JSON tag
  - `tool_start / tool_result` 事件可稳定输出 `callId / round / sequence / name / status / summary / arguments / data`
- 已修复前端兼容消费：
  - `frontend/src/stores/chatStore.ts` 新增 `normalizeToolCallPayload(...)`
  - 同时兼容 camelCase / PascalCase，避免旧进程或历史事件导致卡片再次空白
- `external_evidence_workflow` 新增阶段级日志：
  - `workflow start / done`
  - `search start / done`
  - `select done`
  - `fetch start / done`
  - `assess done`
  - 现在联调时可以直接从日志确认：
    - 搜索命中了多少结果
    - 选中了哪些 URL
    - 来源 coverage / diversity
    - fetch 成功/失败/截断
    - 最终 quality / readiness 是否足够

### 2026-05-09

#### 1. 做了一版 ingestion 最小收口

- `KnowledgeDocumentService` 的 reconcile 流程开始产出结构化结果：
  - 是否 `skipped`
  - 是否修复了 `document`
  - 是否更新了 `chunk_log`
  - 是否补建了 `chunk_log`
  - 是否失败及失败摘要
- 通过 knowledge -> ingestion 的 bridge，把上述 reconcile 结果接入了现有 `ingestion metrics`
- `GET /ingestion/metrics` 现在额外暴露：
  - `attempts`
  - `skipped`
  - `documentUpdated`
  - `chunkLogUpdated`
  - `chunkLogCreated`
  - `failures`
  - `lastFailure`

#### 2. 整理了 `internal/app/ingestion/service` 目录结构

- 当前按职责统一命名：
  - `service_*`
  - `executor_* / workflow_*`
  - `runner_*`
  - `observer_*`
- 新增 `doc.go` 说明该层文件分组语义
- 目的不是重构包边界，而是先把目录阅读成本降下来，便于后续暂时不继续深入 ingestion 时仍可维护

#### 3. 明确短期工作重心调整

- `ingestion` 从“当前主推进模块”调整为“已阶段性收口、短期不再主攻的模块”
- 接下来主线切回：
  - `RAG retrieve` 稳定性和解释性
  - `diagnose` 质量
  - `tool / trace / fallback` 消费闭环

#### 4. 推进了 retrieve 的精确匹配与 metadata 检索能力

- 补强了 `auto` 模式规则：
  - 架构 / 流程类语义问题
  - 标识符查找
  - 文件名查找
  - 章节 / 标题定位
- 扩充 `retrieve-debug` 样本集并校准到 `23/23 PASS`
- 新增 `metadata_title` 检索通道：
  - 定向检索 `document_name`
  - 定向检索 `source_file_name`
  - 定向检索 `section`
- knowledge 直传链路补齐向量 metadata：
  - `source_type`
  - `source_file_name`
  - 合并 chunk 级 metadata，便于后续利用 `section / heading_path / code_language`

#### 5. 建立了 retrieve 离线评估基础设施

- 新增 `internal/app/rag/evaluation/`
- 新增 `cmd/retrieve-eval`
- 当前支持指标：
  - `Hit@K`
  - `Recall@K`
  - `MRR`
- 当前支持两种评估模式：
  - 基于样本中 `retrieved` 结果的纯离线评估
  - 基于当前配置直接执行真实 retrieve 的回放评估
- 新增评估样本：
  - `testdata/retrieve_eval_samples.json`

#### 6. 落地了 AgentLoop V1

- 将 `RagChatService` 内的单次 `tool workflow` 升级为多轮 `Plan -> Act -> Observe` 最小闭环
- 新增规则版 `Observer`，首版不依赖 LLM observer，而是基于已有 tool 结果决定：
  - 是否结束循环
  - 是否继续下钻 `task / node`
  - 是否输出下一轮 planner hint
- `PlanInput` 补充：
  - `AgentState`
  - `PreviousResults`
- runtime 默认从 `LocalWorkflow` 切到 `AgentLoop`

#### 7. 补齐了 Agent 过程可观测性

- SSE 新增事件：
  - `tool_start`
  - `tool_result`
  - `agent_think`
- trace 从“单层 tool_workflow + tool_call”扩展为：
  - `agent_round`
  - `tool_call`
  - `agent_observation`
- 前端最小消费已经接入：
  - `toolCalls` 按 `callId` 增量更新
  - 支持显示轮次、参数、耗时和运行态

### 2026-05-08

- `retrieve` 从模式分支重构成多通道检索基础架构
- 落地 `vector_global / keyword`
- 落地 `fusion / dedup / rerank`
- 增加 `searchChannels / channelStats / searchDecisions` trace 元数据
- 增加 `cmd/retrieve-debug` 与样本回放，`18/18 PASS`
- `diagnose` 补齐 `facts / inferences / riskHints / nextActions`
- 前端补齐 `fallback` SSE 事件消费
- 固化 integration test 入口与 CI 骨架
- 落地 ingestion reconcile 与 trace/tool/fallback 观测链路

## 当前验证状态

截至 2026-05-12，以下增量验证已通过（35 个包全量 PASS，零回归）：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service ./internal/adapter/http/ingestion -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/adapter/http/ingestion ./internal/bootstrap/ingestion ./internal/app/knowledge/service -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/tool/builtin -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/service ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/framework/config ./internal/app/rag/tool ./internal/app/rag/service ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool -run TestAgentLoopParallelToolCallsImproveWallClockDuration -v -count=1
```

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS
- `internal/adapter/http/ingestion` PASS
- `internal/bootstrap/ingestion` 可正常编译
- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/builtin` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）
- `internal/bootstrap/rag` PASS（无测试文件，包可正常编译）
- `internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/service ./internal/bootstrap/rag` PASS
- `internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/tool/builtin` PASS
- `internal/framework/config` PASS
- `TestAgentLoopParallelToolCallsImproveWallClockDuration` PASS

历史验证保持有效：

- `internal/app/rag/core/retrieve` PASS
- `internal/app/rag/service` PASS
- `internal/app/rag/tool/...` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）
- `internal/app/rag/evaluation` PASS
- `cmd/retrieve-eval` 可正常编译
- `internal/bootstrap/rag` 可正常编译

2026-05-12 增量验证：

```
# tool 全量
go test ./internal/app/rag/tool/... -count=1 → PASS

# builtin + tavily
go test ./internal/app/rag/tool/builtin/... -count=1 → PASS

# planner
go test ./internal/app/rag/tool/planner/... -count=1 → PASS

# service
go test ./internal/app/rag/service/... -count=1 → PASS

# config (含 RagWebSearchConfig)
go test ./internal/framework/config/... -count=1 → PASS

# 全量 internal（35 packages）
go test ./internal/... -count=1 → 35 PASS, 0 FAIL

# full build
go build ./... → PASS
```

2026-05-12 P0 收口增量验证：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool/... ./internal/app/rag/core/prompt ./internal/app/rag/service/... -count=1
```

- `internal/app/rag/tool/...` PASS
- `internal/app/rag/core/prompt` PASS
- `internal/app/rag/service/...` PASS

2026-05-12 外部证据工作流 / Tool 卡片联调增量验证：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool -count=1
```

- `internal/app/rag/tool/...` PASS（含 `external_evidence_workflow` 新增测试）
- `internal/app/rag/tool` PASS（含 `ToolCallEvent` JSON tag / Tool 卡片联调修复后回归）

## 当前已知问题与风险

1. `tool / agent` 决策已进入 “LLMObserver + LLMPlanner + 规则兜底” 的混合版本  
   `LLMObserver` 已成为默认主 observer，`RuleObserver` 已退为 fallback/guardrail。`doc_fail_01` 的标准失败样例可稳定走通，且 `diagnose` 一步到位大幅减少了对 LLM Observer 下钻决策的依赖。当前剩余主要风险已从”基础交互是否可用”转为”LLM 决策质量是否足够稳定”，在 diagnose 覆盖不到的场景下仍需防范参数幻觉和重复调用。

3. 运行中场景已回答一致性收口  
   `ID` 误识别、同轮过深规划、状态冲突归一已完成首轮修复。`answer_guidance` 已支持当 diagnose 结论与 task/node 实际状态冲突时以 task/node 为准，并显式标注不一致。

4. 多通道检索目前仍是”无 intent 依赖”版本  
   已完成 `vector_global + keyword + metadata_title` 通道化重构，但还没有接入 `intent_directed`、metadata 字段扩展策略或更细的 route hint。

5. 工具集已扩展至 15 个但仍缺少更多结构化外部工具与写操作工具  
   已含 web_search（DuckDuckGo/Tavily 双后端）+ web_fetch（网页抓取）+ external_evidence_workflow（外部证据工作流），但仍无更多结构化外部工具（如 JSON API / GitHub / SQL readonly）和写操作工具（如创建文档）。

6. `ingestion` 生产化仍未完全收口  
   已补上 reconcile 结果留痕与基础统计，但修复结果沉淀、异常统计、超时治理和更系统的恢复策略仍未完成。该项已转入”中期待办”。

7. trace 可观测性具备首轮闭环，但仍需继续产品化 
   round / observation extraData 中已补充 `capability`、`workflowMode`、`riskLevel`、`approvalRequirement`、`evidenceSources`、`nextHintCalls`、`toolCallCount` 等字段，但列表摘要、聊天到 trace 联动、异常筛选仍需继续补齐。

8. AgentLoop V1 已从”纯诊断”扩展为”诊断 + 发现 + 搜索”，但仍是单 Agent  
   已落地 RAG 优先的联网搜索（web_search + web_fetch + external_evidence_workflow + Tavily 国内可用后端），模糊问题处理和外部证据整合有所提升，但还不是多 Agent 协作系统。

10. Tool 卡片前端展示虽已修复基础事件字段对齐，但仍需继续产品化  
   当前已修复 `tool_start/tool_result` 的 `callId/name/summary` 对齐问题，工具调用名称、参数、摘要和耗时可重新展示；但 `external_evidence_workflow` 这类复杂结果仍更适合做专门卡片视图，而不是只显示通用摘要。

9. DuckDuckGo 在国内被墙，需配置 Tavily 替代  
   `rag.search.web-search.provider=tavily` + `api-key` 可解决，但需要用户注册免费 API key。未配置时回退 DuckDuckGo（国内超时）。

## 下一步计划

### 2026-05-17：Memory 架构设计讨论

详见 [memory_architecture_design.md](docs/memory_architecture_design.md)。

核心结论：
- **短期/长期记忆分层**：短期用 conversation_message + 上下文直传，长期走 knowledge document + vector retrieve
- **长消息处理**：写时摘要优于读时，规则优先 + LLM 补充，接入点在 `AddMessage` 入库前
- **长期记忆入库识别**：三层漏斗（显式标记 → 异步 LLM 分类 → 会话结束聚合），第一层可立即落地
- **长期记忆形态**：复用已有 ingestion pipeline，创建 knowledge document（sourceType=memory），打标签 memory_type=preference|knowledge|feedback

待决策：全局 user_memory 知识库 vs 按项目隔离。

### P0

- 联调验证 Agent Search 全链路
  - E2E 验证 "Go 泛型怎么用" → KB 无内容 → web_search → web_fetch → 综合回答
  - E2E 验证 "Go 泛型怎么用" → KB 无内容 → external_evidence_workflow → source review / quality / readiness → 综合回答
  - E2E 验证 "doc_fail_01 为什么导入失败" → 诊断链路，不触发搜索
  - E2E 验证 "最近有哪些文档" → document_list，不触发搜索
  - 验证 Tavily API key 配置生效，确认国内可用性
  - 联调验证 Tool 卡片是否稳定显示 `callId / name / summary / arguments / duration`

- 多通道检索优化
  - 基于 `retrieve-eval` 持续扩充真实 query 样本集
  - 评估是否接入 `intent_directed` 检索策略

### P1

- 继续扩展工具集
  - 评估是否需要更多结构化外部工具（如 JSON API fetch、GitHub 搜索、数据库 readonly 查询等）
  - 评估是否需要写操作工具（需配合确认机制）
- 继续把高频结果契约视图化
  - 为 `task_query` / `task_node_query` / `trace_node_query` 补齐 result view
  - 继续减少 renderer / observer / guidance 中对 `map[string]any` 的直接消费

- trace 产品化
  - 列表摘要、聊天到 trace 联动、异常筛选

### P2

---

## 2026-05-12 Additional Update: RAG Tool Modularization

### Status Summary

- The long-term RAG tool modularization effort has moved from architecture design into active rollout.
- The runtime is now effectively `module-first`, while central `switch toolName` logic is being reduced to legacy fallback.
- This makes future external tool integration much more realistic: new tools can increasingly be added as modules instead of framework patches.

### Completed So Far

#### Phase 1 Foundation: done

- `ToolModule / ToolSpec / ToolBehavior / ToolInvoker / ResultMeta / ModuleRegistry` are in place.
- `Executor` now runs modules and injects `Result.Meta`.
- `Registry` is module-centric while still preserving legacy `Tool` registration.
- `buildLocalToolWorkflow(...)` now registers modules instead of raw tools.

#### Phase 2 Web family migration: done

These tools already have their own module behavior:

- `web_search`
- `web_fetch`
- `external_evidence_workflow`

Each now owns its own:

- `Decode`
- `Next`
- `Observe`
- `RenderContext`
- `BuildGuidance`

#### Phase 3 Central orchestration slimming: largely done

- `AgentLoop` now prefers registry-driven behavior for next-step planning.
- `RuleObserver` now behaves like an orchestrator instead of the primary holder of tool-specific semantics.
- `RenderContext` and `AnswerGuidance` are now module-first.
- `workflow_control` already prefers `Result.Meta` over name-based inference.

#### Phase 4 System tool family migration: completed

Families already migrated to module behavior:

- `document_query / document_chunk_log_query / document_list`
- `ingestion_task_query / ingestion_task_node_query / task_list`
- `document_ingestion_diagnose / task_ingestion_diagnose / trace_retrieval_diagnose / trace_node_query`
- `think`
- `document_root_cause_diagnosis / document_diagnose_with_search`

### Important Engineering Result

- Legacy `MustRegister(tool)` now auto-infers known module behavior for migrated tool families.
- This means old tests and old registration style do not need a full rewrite immediately.
- `RuleObserver` no longer needs to carry the big document/task/web/trace branch table as the main path.

### Validation

Incremental validation for the modularization rollout passed on 2026-05-13:

```powershell
$env:GOCACHE=(Join-Path (Resolve-Path .).Path '.gocache'); go test ./internal/app/rag/tool/... ./internal/bootstrap/rag -count=1
```

Current result:

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/builtin` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/bootstrap/rag` PASS

### Remaining Work

#### Phase 4 wrap-up result

- `next_action.go` compatibility fallback now reuses inferred behavior instead of keeping central document/task/web name branching.
- `planCallsFromResults(...)` no longer keeps a separate `web_search` special case and now follows the same compatibility decision path.
- `workflow_control.go` fallback now resolves capability / evidence / execution metadata through legacy module spec inference instead of a tool-name-specific table.
- Physical typed-view relocation is not required for Phase 4 completion; the next meaningful architecture step is adding a truly new external module family on the current module-first runtime.

#### Phase 5 preparation

The recommended first external module families remain:

- `api/*` for readonly JSON API tools
- `github/*` for readonly repo / issue / release / file tools
- `db/*` for domain-specific readonly queries

### Current Conclusion

As of 2026-05-12, the project is no longer only “planning” modularization. The main RAG toolchain has already entered a practical modularized state, with system families mostly migrated and central fallback logic actively being retired.

- ingestion 收口
  - pending/running 超时治理
  - reconcile 结果沉淀与更系统恢复策略

- Planner/Observer 合并探索
  - 减少 LLM 调用次数
## 2026-05-15 Additional Update: Chunk Optimization + Retrieval Gating Cleanup

### Status Summary

- `chunk` 链路完成了一轮中文场景导向的小步优化，重点放在 fixed-size 切分边界和默认 overlap 兜底。
- `RagChatService` 现在已经接入 `rewrite -> need_retrieval` 决策，普通闲聊可以跳过 retrieval 和 tool workflow。
- `searchMode` 在 chat 主链路上已经不再作为真实决策字段，当前策略统一为 `hybrid`，并开始清理对应的前后端残留展示与测试。

### Completed So Far

#### 1. Chunk 中文友好性增强

- `internal/app/core/chunk/fixed_size_chunker.go`
  - 调整 fixed-size chunker 的句边界搜索逻辑
  - 扩大局部搜索窗口，优先命中更合理的句末和段落边界
  - 增加中文条款/标题类软边界识别，减少半句切断和条款标题被截断
- `internal/app/knowledge/service/document_process_service.go`
  - 当 chunk 配置缺省时，在文档处理主链路补上默认 overlap 兜底
  - 保留显式传入 `overlap=0` 的兼容语义，不在底层 normalize 阶段强改
- 新增/补强测试：
  - `internal/app/core/chunk/test/fixed_size_chunker_test.go`
  - `internal/app/knowledge/service/test/document_process_service_test.go`

#### 2. Rewrite 决定是否需要检索

- `internal/app/rag/core/rewrite/rewrite.go`
  - `rewrite.Result` 收口为 `RewrittenQuestion / SubQuestions / NeedRetrieval`
  - 默认兜底逻辑新增 `InferNeedRetrieval(...)`
- `internal/app/rag/core/rewrite/llm_rewrite_service.go`
  - rewrite prompt 新增 `need_retrieval`
  - LLM 返回 JSON 现在只要求：
    - `rewritten`
    - `sub_questions`
    - `need_retrieval`
- `internal/app/rag/service/rag_chat_service.go`
  - `prepareChat()` 已根据 rewrite 结果判断是否执行 retrieve
  - `NeedRetrieval=false` 或无 `KnowledgeBaseIDs` 时跳过 retrieval
  - `tool workflow` 只在真实 retrieval 场景里继续执行

#### 3. SearchMode 收口为统一 Hybrid

- chat 主链路不再把 `searchMode` 当作动态分支使用
- `internal/app/rag/service/rag_chat_service.go`
  - retrieval request 统一固定 `ragretrieve.SearchModeHybrid`
  - 删除 `resolveRetrieveSearchMode(...)`
  - tool workflow input 不再传递 `SearchMode`
- `internal/app/rag/tool/core/workflow.go`
  - `WorkflowInput.SearchMode` 已移除
- `internal/app/rag/tool/core/summary.go`
  - rewrite 摘要改为输出 `needRetrieval=true/false`
- 前端残留展示开始同步清理：
  - `frontend/src/components/chat/MessageItem.tsx` 删除“检索策略” badge
  - `frontend/src/pages/admin/traces/RagTraceDetailPage.tsx` 删除 mode/requested/resolved 展示
  - `frontend/src/stores/chatStore.ts` 不再从 SSE meta 读取 `searchMode`

### Validation

2026-05-15 增量验证通过：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/core/chunk/... -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/knowledge/service/test -run TestDocumentProcessServiceExecuteChunkUsesDefaultOverlapWhenChunkConfigMissing -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/core/rewrite -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/service -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/tool -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/tool/planner -count=1
```

Current result:

- `internal/app/core/chunk/...` PASS
- `internal/app/knowledge/service/test` 指定增量测试 PASS
- `internal/app/rag/core/rewrite` PASS
- `internal/app/rag/service` PASS
- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/planner` PASS

### Current Conclusion

As of 2026-05-15, the project has completed a meaningful cleanup on the chat retrieval path:

- retrieval is no longer mandatory for every request
- rewrite now owns the main retrieval-needed decision
- `searchMode` has effectively been downgraded from a strategy surface to an internal fixed constant on the chat path

This reduces unnecessary resource consumption for greeting/small-talk requests, while also making the RAG chain easier to reason about and easier to maintain.

## 2026-05-20 Additional Update: Tool Module Closure Progress

### Status Summary

- `tool / AgentLoop` has moved further from "usable but still debt-heavy" toward a more stable module-first runtime.
- The main focus of this round was not adding new tools, but reducing duplicated decision logic, slimming core runtime files, and tightening typed result-view boundaries.
- The current highest-value engineering result is that high-frequency `system` tool results are now much less dependent on ad hoc `map[string]any` reads in runtime/behavior code.

### Completed So Far

#### 1. AgentLoop and observer/runtime closure advanced by another round

- Added shared runtime helpers:
  - `internal/app/rag/tool/runtime/evidence_validation.go`
  - `internal/app/rag/tool/runtime/observation_state.go`
- Moved common task-node helper logic into:
  - `internal/app/rag/tool/core/data_helpers.go`
- Extracted base routing rules out of the orchestration file:
  - `internal/app/rag/tool/runtime/base_rules.go`
- Split large runtime/system files so core orchestration is thinner:
  - `internal/app/rag/tool/modules/system/document_behavior.go`
  - `internal/app/rag/tool/modules/system/task_behavior.go`
  - `internal/app/rag/tool/runtime/guidance_diagnosis.go`
  - `internal/app/rag/tool/runtime/guidance_web.go`

#### 2. Observer modularization continued

- `ToolBehavior` now supports module-provided observer examples.
- `LLMObserver` was split by responsibility:
  - `observer_llm.go`
  - `observer_llm_prompt.go`
  - `observer_llm_parse.go`
- High-frequency tool families can now contribute their own few-shot observer examples instead of growing a single central runtime constant.

#### 3. Typed result-view closure for the system tool family

- Added/exported typed views for these results:
  - `document_query`
  - `document_chunk_log_query`
  - `document_list`
  - `ingestion_task_query`
  - `ingestion_task_node_query`
  - `task_list`
- The new view layer lives mainly in:
  - `internal/app/rag/tool/modules/system/result_views.go`
  - `internal/app/rag/tool/views.go`
- `document_behavior.go` and `task_behavior.go` now render and branch on structured views instead of repeatedly reading raw maps.
- `IngestionTaskQueryResultView` now owns `LatestInterestingNode()` so "failed/running node" drilling no longer depends on scattered raw-map parsing.
- `document_root_cause_diagnosis` graph chaining was also updated to reuse the typed `ingestion_task_query` view.

#### 4. Small leftover runtime debt was trimmed

- Removed an unused legacy helper from:
  - `internal/app/rag/tool/runtime/renderer.go`
- Continued shrinking mixed old/new style consumption paths so future tool-family expansion is less likely to reintroduce raw data coupling.

#### 5. Test coverage was strengthened

- Added/expanded focused tests for:
  - base rules
  - evidence validation
  - observation state normalization
  - observer example aggregation
  - system typed view parsing for document/task/task-node result shapes

### Validation

Validated on 2026-05-20:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool/... -count=1
```

Current result:

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/assembly` PASS
- `internal/app/rag/tool/invokers/web` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/app/rag/tool/runtime` PASS

### Current Conclusion

As of 2026-05-20, the tool module is noticeably closer to a stage-stop "closure point":

- core AgentLoop orchestration is thinner
- observer behavior is more modular
- system-family high-frequency results are increasingly typed-view driven
- adding or extending a tool family now requires less direct editing of core runtime paths

### Remaining Work

- Continue reducing the remaining high-level raw `map[string]any` consumption boundaries, especially where evidence guardrails still operate on generic maps.
- Consider unifying the `trace` family result-view layout with the same style used by `system` and `web`.
- Evaluate whether the next best step is:
  - more tool-module closure
  - or switching the main engineering focus to `memory V1` retrieval-side integration

## 2026-05-20 Additional Update: Memory V1 Session Recall Closure

### Status Summary

- `memory V1` 的 `Phase 1.5` 已从“存储侧完成、召回侧待做”推进到“会话检索层闭环完成”。
- 当前系统已经具备：长消息写时摘要、会话内原文 chunk 存储、后续轮自动轻量召回、独立 prompt section 注入，以及基础 trace 可观测性。
- 这轮工作的重点不是长期记忆，而是把“当前 conversation 内记住长原文细节”的链路真正跑通。

### Completed So Far

#### 1. 修正了长消息存储覆盖范围

- `LongMessageContentProcessor` 现在不仅为超长消息生成 `SessionChunks`，也会为 `3000~12000 token` 的中等长消息生成 `SessionChunks`。
- 这使实现重新与设计文档对齐：
  - 中等长消息：摘要进上下文，原文 chunk 进入会话检索层
  - 超长消息：chunk 摘要后合并总摘要，原文 chunk 进入会话检索层

#### 2. 会话召回链路已落地

- `SessionChunkRepository` 新增：
  - `ExistsRecallable(...)`
  - `SearchRecallableByVector(...)`
- 新增 `SessionRecallService`
  - 作用域固定为当前 `conversation_id`
  - 仅召回 `user` 且 `is_summarized=true` 的历史长消息
  - 排除当前轮刚写入的消息
  - 先判断是否存在 recallable chunk，再决定是否执行 query embedding
- `RagChatService.prepareChat(...)` 现在按顺序执行：
  - `runMemoryStage`
  - `runUserMessageStage`
  - `runRuntimeStage`
  - `runRewriteStage`
  - `runSessionRecallStage`
  - `runRetrieveStage`

#### 3. Prompt 与 trace 已形成闭环

- prompt 新增独立的 `SessionContext`
  - 以 `## 会话上下文片段` system section 注入
  - 不混入 history，也不并入 `KnowledgeContext`
- excerpt 选择已按轻量 lexical overlap 落地
  - 小 chunk 直接回放完整原文
  - 大 chunk 二次切窗后再择优
  - 最终固定输出“摘要 + 原文 excerpt”
- `session_recall` 已有独立 trace node：
  - `node_id=session_recall`
  - `node_type=memory`
  - `node_name=session_chunk_recall`
- trace extraData 现可表达：
  - `used`
  - `candidateCount`
  - `excerptCount`
  - `topScore`
  - `excludedMessageId`
  - `selectedHits`
  - `skippedPerMessageLimit`
  - `truncatedBy`

#### 4. 已补近似真实场景样例

- `RagChatService` 已新增近似端到端样例：
  - 长日志追问：验证 `retriever timeout` 之类细节可从前一轮日志中补回
  - 长配置追问：验证 `provider: tavily-mcp` 之类配置细节可从前一轮配置中补回
- 这些样例同时覆盖：
  - 长消息写入
  - `SessionChunk` 落存
  - follow-up query 召回
  - prompt 中的 `Session Context Excerpts` 注入

### Validation

Validated on 2026-05-20:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/framework/config -count=1
```

Current result:

- `internal/app/rag/service` PASS
- `internal/app/rag/...` PASS
- `internal/bootstrap/rag` PASS
- `internal/framework/config` PASS

### Current Conclusion

As of 2026-05-20, `memory V1` has a genuinely usable session-recall layer:

- long user messages no longer only summarize away detail
- detail can be reintroduced in later turns within the same conversation
- the recall path is now observable and protected by realistic tests

The next major memory milestone is no longer “finish Phase 1.5”, but “start Phase 2 explicit long-term memory”.

## 2026-05-20 Additional Update: Memory V1 Phase 2 Foundation

### Status Summary

- `memory V1` 已经从“Phase 1.5 完成、Phase 2 待启动”推进到“Phase 2 基础闭环完成”。
- 当前系统已经具备：
  - `memory_item` 主模型持久化
  - 显式长期记忆保存入口
  - `global / kb` 双作用域
  - 聊天前的最小长期记忆 recall
  - 独立 prompt section 注入
- 这轮工作的重点仍然不是长期记忆向量化或自动抽取，而是先把“显式保存 -> 可被后续对话使用”这条最小链路跑通。

### Completed So Far

#### 1. `memory_item` 主模型与持久化已落地

- 新增 `domain.MemoryItem`
- 新增 migration：
  - `20260520120000_create_memory_item_table.sql`
- 新增 Postgres 持久化：
  - `MemoryItemModel`
  - `MemoryItemRepository`
- 当前主字段已覆盖设计中的核心语义：
  - `scope_type / scope_id`
  - `memory_type`
  - `source_message_id`
  - `content / summary`
  - `confidence / status`
  - `last_confirmed_at / expires_at`
  - `created_by / updated_by`

#### 2. 显式长期记忆服务已可用

- 新增 `MemoryService`
  - `SaveExplicitMemory(...)`
  - `ListMemories(...)`
  - `ExpireMemory(...)`
  - `RecallMemories(...)`
- 当前 `SaveExplicitMemory(...)` 支持：
  - 默认 `scope_type=global`
  - 默认 `memory_type=knowledge`
  - 自动生成 summary
  - 写入后直接落为 `status=active`
- 当前 recall 仍保持 Phase 2 的轻量实现边界：
  - 不是向量检索
  - 先按 `kb` / `global` 作用域取数
  - 再做轻量 lexical match 与优先级排序

#### 3. 长期记忆 recall 已接入 chat prepare 链路

- `RagChatService` 新增独立 `long_term_memory` stage
- 当前 `prepareChat(...)` 关键顺序变为：
  - `runMemoryStage`
  - `runUserMessageStage`
  - `runRuntimeStage`
  - `runRewriteStage`
  - `runLongTermMemoryStage`
  - `runSessionRecallStage`
  - `runRetrieveStage`
- recall 结果当前通过独立 `MemoryContext` 注入 prompt
  - 不混入 history
  - 不并入 `KnowledgeContext`
  - 与 `SessionContext` 分层保持一致

#### 4. HTTP 接口与 runtime wiring 已补齐

- 新增接口：
  - `POST /rag/v3/remember`
  - `POST /rag/v3/memories`
  - `GET /rag/v3/memories`
  - `POST /rag/v3/memories/:memoryId/expire`
- `bootstrap/rag.Runtime` 已注入：
  - `MemoryItemRepository`
  - `MemoryService`
  - `RagChatService.SetExplicitMemoryService(...)`

### Validation

Validated on 2026-05-20:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/... ./internal/bootstrap/rag ./cmd/server -count=1
```

Current result:

- `internal/app/rag/...` PASS
- `internal/bootstrap/rag` PASS
- `cmd/server` PASS

### Current Conclusion

As of 2026-05-20, `memory V1` is no longer only “session recall complete”:

- explicit long-term memory can now be saved
- saved memory can participate in later chat preparation
- Phase 2 has a usable foundation without yet committing to full retrieval projection

The next major memory milestone is now:

- finish the remaining productization / UX surface of Phase 2 if needed
- then move to `Phase 3` retrieval projection so long-term memory can join retrieval more naturally

## 2026-05-22 Additional Update: Memory Module Cleanup

### Status Summary

- `memory V1` 在保持 Phase 2 能力不变的前提下，完成了一轮“结构与语义收口”。
- 这轮工作的目标不是新增治理能力，而是先把当前代码里的 `STM / MTM / LTM` 边界收清楚，降低后续推进 `Memory Gate` 和事实型记忆检索投影时的认知成本。
- 当前代码语义已经明确为：
  - `STM`：会话历史与摘要
  - `MTM / evidence memory`：长消息处理、`SessionChunk`、`SessionRecall`
  - `LTM`：显式长期记忆保存、长期记忆 recall、`MemoryContext`

### Completed So Far

#### 1. `STM` 命名已从泛化 `memory` 收口为 `history`

- `internal/app/rag/core/memory` 已重命名为 `internal/app/rag/core/history`
- 当前 `core/history` 只承载：
  - history load
  - summary load / compress
  - chat history store adapter
- 这避免了把会话历史服务误解为长期记忆模块

#### 2. 长期记忆应用层已独立到 `service/longtermmemory`

- 新增目录：
  - `internal/app/rag/service/longtermmemory`
- 当前已按职责拆开：
  - `service.go`：`MemoryService`，负责 CRUD、embedding wiring、recall 委托
  - `recall_service.go`：候选获取、关键词/向量融合、排序
  - `context_renderer.go`：`MemoryContext` 渲染
  - `types.go`：长期记忆输入输出与 recall result types
- `domain.MemoryItem`、`port.MemoryItemRepository`、Postgres repo/model 本轮保持原位置，未做纵向大搬迁

#### 3. `RagChatService` 已收口为只依赖长期记忆 recall 接口

- chat 主链路不再直接感知长期记忆 CRUD service
- `RagChatService` 当前通过单一入口接收长期记忆 recall 能力：
  - `SetLongTermMemoryRecallService(...)`
- `bootstrap/rag.Runtime` 当前分工变为：
  - HTTP 管理面注入 `longtermmemory.MemoryService`
  - chat 主链路注入 `longtermmemory.RecallService`
  - `SessionRecallService` 继续独立注入，不与长期记忆合并

#### 4. 清理了一个失效/占位实现

- 已删除：
  - `internal/app/rag/service/conversation_message_content_processor.go`
- 当前仅保留 `LongMessageContentProcessor` 作为长消息处理器实现，避免消息内容处理双轨并存

### Validation

Validated on 2026-05-22:

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/... ./internal/app/rag/core/history ./internal/adapter/http/rag -count=1
```

Current result:

- `internal/app/rag/service` PASS
- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/core/history` PASS
- `internal/adapter/http/rag` PASS

Additional note:

- `./internal/bootstrap/rag` 与 `./cmd/server` 的本轮回归受到当前环境的 Go module cache 权限限制，未在本轮文档更新时补齐完整验证结果。

### Current Conclusion

As of 2026-05-22, `memory V1` has completed a meaningful structural cleanup:

- STM / MTM / LTM 的代码语义已经清晰分层
- 长期记忆已有独立应用层入口，不再与会话历史服务混名
- chat 主链路与长期记忆管理面的依赖方向更清楚

This does not yet add governance, but it creates a cleaner base for:

- `Phase 2.1` governance closure
- `Phase 3` fact-memory retrieval projection

## 2026-05-22 Additional Update: Memory V1 Phase 2.1 Governance Closure

### Status Summary

- `memory V1` 已从“结构与语义收口完成”推进到“Phase 2.1 治理闭环落地”。
- 这轮工作的重点不是把长期记忆接入主检索，而是先把显式保存路径从“直接存文本”升级为“结构化治理写入”。
- 当前长期记忆已经具备：
  - 结构化治理字段持久化
  - `Memory Gate`
  - `Conflict Detector`
  - 单值键 / 多值键规则
  - `superseded` 覆盖语义
  - 兼容旧 API 的渐进升级

### Completed So Far

#### 1. `memory_item` schema 已完成治理字段收口

- `t_memory_item` 已补齐：
  - `namespace`
  - `category`
  - `canonical_key`
  - `value_type`
  - `value_json`
  - `display_value`
  - `importance`
  - `last_used_at`
  - `supersedes_id`
  - `extraction_method`
- `domain.MemoryItem`、Gorm model、mapper、embedding search hit 扫描路径均已同步扩展
- 已新增按 `user/scope/key/status`、`user/namespace/category/status`、`supersedes_id` 的索引支持

#### 2. 长期记忆显式保存链路已切换到治理模式

- `internal/app/rag/service/longtermmemory` 已新增：
  - `governance_types.go`
  - `schema_registry.go`
  - `normalization.go`
  - `gate.go`
  - `conflict_detector.go`
  - `lifecycle.go`
  - `save_service.go`
  - `transaction.go`
- `SaveExplicitMemory(...)` 当前执行顺序已收口为：
  - normalize
  - gate validate
  - load active candidates
  - conflict / merge / supersede decision
  - transactional persist
- 显式保存入口仍保持“默认落 `active`”，本轮未引入自动抽取的 `pending` 流程

#### 3. canonical key 与 cardinality 规则已落第一版白名单

- 当前已内置首批 key：
  - `response.language`
  - `workflow.first_step`
  - `behavior.avoid`
  - `project.constraint.network`
  - `project.messaging.main_bus`
  - `project.fact.dependencies`
  - `project.integrations`
- 每个 key 当前都已明确：
  - `category`
  - `memory_type`
  - `value_type`
  - `single-valued / multi-valued`
  - 默认 `importance`
  - 允许的 `scope_type`

#### 4. 单值键 / 多值键治理语义已落地

- 单值键：
  - 同 scope + 同 key + 同值时不新建，只刷新 `last_confirmed_at / update_time`
  - 新值写入时，旧值改为 `superseded`，新值 `active`
  - 新记录通过 `supersedes_id` 指向被覆盖记录
- 多值键：
  - 同值重复保存走 merge / refresh
  - 不同值允许并存多个 `active`
- `feedback` 默认不参与覆盖旧事实，只允许 `create / ignore`

#### 5. HTTP 接口已完成兼容式升级

- 仍保留：
  - `POST /rag/v3/remember`
  - `POST /rag/v3/memories`
  - `GET /rag/v3/memories`
  - `POST /rag/v3/memories/:memoryId/expire`
- `remember` 现支持可选治理字段：
  - `category`
  - `canonicalKey`
  - `valueType`
  - `valueJson`
  - `displayValue`
  - `importance`
- 对旧请求体保持兼容：
  - 未传治理字段时自动补默认 `namespace / category / value_type / value_json / display_value`
- `list memories` 当前已支持新增过滤：
  - `category`
  - `canonicalKey`
  - `namespace`
- `memoryItemVO` 已补充治理字段透出，便于调试与管理面观察

#### 6. 覆盖语义已通过事务包装落地

- 已新增 memory mutation transaction
- `bootstrap/rag.Runtime` 当前已为 `MemoryService` 注入 transaction
- 单值键覆盖路径现在在同一事务内完成：
  - 旧记录 `superseded`
  - 新记录 `active`

### Validation

Validated on 2026-05-22:

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/longtermmemory -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/... -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/adapter/http/rag/... -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/adapter/repository/postgres/rag -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/bootstrap/rag -count=1
```

Current result:

- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag/test` PASS
- `internal/adapter/repository/postgres/rag` PASS
- `internal/bootstrap/rag` PASS

### Current Conclusion

As of 2026-05-22, `memory V1` has completed `Phase 2.1` governance closure on the explicit-save path:

- long-term memory is no longer only “storable”; it is now governed
- single-valued replacement and multi-valued merge semantics are explicit
- old API callers can continue working while structured governance fields are already available

The next major memory milestones are now:

- `Phase 3` fact-memory retrieval projection
- stronger lifecycle governance such as `last_used_at` updates, cleanup policy, and background maintenance

## 2026-05-23 Additional Update: Memory V1 Phase 3 Minimal Closure and Phase 4 Cache Skeleton

### Status Summary

- `memory V1` 已从“Phase 2.x 读侧收口完成、Phase 3 待启动”推进到“Phase 3 最小闭环已跑通，Phase 4 cache skeleton 已开始落地”。
- 这轮工作的重点先后分成两段：
  - 先把 `knowledge` 型长期记忆接入主 retrieve 管道，形成独立 `memory_fact` channel。
  - 再为下一阶段的生命周期治理补上 Redis 导向的 recall cache 骨架。
- 当前 memory 主线状态已经变成：
  - `Phase 2.1`：显式长期记忆治理闭环完成
  - `Phase 2.x`：规则型 / 事实型读路径分治完成
  - `Phase 3`：事实型长期记忆最小 retrieval projection 已完成
  - `Phase 4`：cache infrastructure 已起步，但 request-scope cache 仍未接入

### Completed So Far

#### 1. `knowledge` 型长期记忆已进入主检索链路

- `internal/app/rag/core/retrieve`
  - 新增 `memory_fact` search channel
  - 接入现有 `fusion / dedup / rerank` 流水线
- `internal/app/rag/service/longtermmemory/retrieve_projection.go`
  - `RecallService` 现在可把 `knowledge` 型长期记忆投影成 `RetrievedChunk`
  - 仅 `memory_type=knowledge` 进入该通道
- `RagChatService.prepareChat(...)`
  - retrieve 请求已把 `userID` 传入 retrieve engine

结果：
- 文档 miss 时，事实型长期记忆可以通过 `memory_fact` 救回 retrieve
- 文档与长期记忆同命中时，文档仍保持优先
- `preference` 继续只停留在 `MemoryContext`
- `feedback` 继续不进入 chat retrieve

#### 2. Phase 3 样例回归已形成最小集

- 已补回归样例，覆盖：
  - `doc miss, memory hit`
  - `doc + memory hit, doc priority preserved`
  - `preference` 不进入 retrieve
  - `main_bus removed`
  - `dependency constraint`
  - `kb fact > global fact`
- 已新增独立样本集：
  - `testdata/memory_fact_phase3_samples.json`
- `cmd/retrieve-eval` 已补 `userId` 字段透传，后续真实回放不会因缺失 user 维度失真

#### 3. `memory_fact` 融合权重已收敛到第一版策略

- 当前 channel 侧默认权重：
  - `vector_global = 1.0`
  - `memory_fact = 0.9`
  - `keyword = 0.85`
  - `metadata_title = 0.8`

这意味着：
- Phase 3 第一版里，长期事实记忆已是主检索体系的一部分
- 但它不会默认压过主文档语义检索结果

#### 4. Phase 4 cache skeleton 已开始落地

- 已新增 `RecallCache` 抽象与 Redis 适配器
- 已在 `longtermmemory` 层接入三类必做缓存的骨架：
  - `rule memories`
  - `fact ranking result`
  - `query embedding`
- 已接入 scope version 机制：
  - `global` version
  - `kb` version
- `SaveExplicitMemory(...)` 与 `ExpireMemory(...)` 成功后会 bump 对应 version
- `bootstrap/rag.Runtime` 已支持按配置装配 Redis recall cache

当前仍未完成：
- request-scope / conversation-scope 的 `L1` recall cache
- session recall cache
- cache hit 后的更细粒度观测指标

### Validation

Validated on 2026-05-23:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; $env:GOPATH='D:\goagent\.gopath'; $env:GOMODCACHE='D:\goagent\.gomodcache'; go test ./internal/bootstrap/rag ./internal/app/rag/service/longtermmemory ./internal/app/rag/service ./internal/app/rag/core/retrieve ./cmd/retrieve-eval -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go run ./cmd/retrieve-eval -input testdata\memory_fact_phase3_samples.json
```

Current result:

- `internal/bootstrap/rag` PASS
- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/service` PASS
- `internal/app/rag/core/retrieve` PASS
- `cmd/retrieve-eval` PASS
- `memory_fact_phase3_samples.json` 离线评估可正常输出 `Hit@K / Recall@K / MRR`

### Current Conclusion

As of 2026-05-23, `memory V1` is no longer only “governed but externally attached to chat”:

- fact-memory retrieval projection has entered the main retrieve pipeline in a controlled, minimal way
- `knowledge` 型长期记忆已经具备独立 retrieval channel 形态
- Phase 4 的缓存建设已从讨论进入基础设施落地

The next major memory milestones are now:

- finish `Phase 4` cache closure with request-scope cache and stronger observability
- add lifecycle governance such as cleanup / maintenance / metrics
- continue tuning `memory_fact` only after the cache and operations base is stable

## Additional Update: 2026-05-24 Memory P0 Hardening and P1 Retrieval Quality

### What Changed

Today the memory track moved from "Phase 4 completed, waiting for ops/lifecycle follow-up" into a short hardening and retrieval-quality pass focused on the existing implementation rather than new phases.

This round intentionally did **not** start a new Phase 5 capability. It focused on fixing correctness and quality issues inside the current `Phase 4+` memory stack:

- `P0` governance correctness and degradation closure
- `P1` rule-memory ordering and fact-memory retrieval quality

### P0 Completed

#### 1. Single-valued memory now has database-level active-version protection

- Added a new migration to enforce a unique active record for single-valued canonical keys:
  - `response.language`
  - `workflow.first_step`
  - `project.constraint.network`
  - `project.messaging.main_bus`
- The migration normalizes `scope_id` with `COALESCE(scope_id, '')`, so both `global` and `kb` scopes are covered consistently.
- The migration also performs a duplicate-active precheck and fails explicitly if dirty data already exists.

#### 2. Save path now converges on concurrent writes instead of writing bad data

- `SaveExplicitMemory(...)` now treats single-valued unique-index conflicts as a governance branch instead of a raw failure.
- On unique conflict:
  - reload current active record
  - if equal, return the existing record
  - if a concurrent winner already landed, return that winner
  - if multiple active records are detected, fail fast as governance corruption

#### 3. Conflict detection no longer depends on `Limit: 8`

- Single-valued canonical keys now use precise active-record loading by `(user, scope, canonical_key)`.
- Multi-valued merge/dedup logic no longer relies on "recent 8 records are probably enough".
- A reusable active-conflict detection query was added for single-valued canonical keys.

#### 4. Request-scope cache fallback is now closed

- In long-term memory recall, fallback paths caused by:
  - scope version unavailable
  - Redis disabled
  - Redis fallback
  now still write back into request-scope cache.
- Result: the system keeps `fail-open`, but no longer recomputes the same recall/ranking work repeatedly inside one request when cache infrastructure degrades.

### P1 Completed

#### 1. Rule-memory ordering now reflects governance intent

- Rule memories are no longer effectively "latest updated first".
- Ordering is now stabilized by:
  1. `scope priority`
  2. `importance`
  3. `last_confirmed_at`
  4. `update_time`
- The same order is preserved on live load, request-cache hit, and Redis-cache hit.

#### 2. Memory equivalence is stricter and safer

- `memoryItemsEquivalent(...)` no longer over-relies on `display_value`.
- Comparison now prefers:
  - structured value
  - then `content`
  - only then `display_value`
- This reduces accidental merges where two memories share a short label but represent different facts.

#### 3. Fact-memory lexical prefilter now uses tokenized candidate expansion

- Fact recall no longer depends only on raw full-query `SearchText`.
- `MemoryItemListFilter` now supports:
  - `SearchText`
  - `SearchTokens`
- The recall layer now builds query-aware tokens for:
  - ASCII words
  - continuous CJK bigrams
  - mixed-language queries
- Repository SQL now uses:
  - raw query fallback
  - token-based `OR` prefilter over `summary / content / display_value / canonical_key`

#### 4. Token noise filtering and scoring alignment are now in place

- Added lightweight lexical denoising for low-value tokens such as:
  - English function words like `how`, `should`, `the`, `please`
  - Chinese filler/functional phrases like `请问`, `这个`, `可以`, `怎么`, `了吗`
- `scoreMemoryText(...)` now reuses the same filtered token logic as DB prefilter construction, so ranking and candidate filtering no longer drift apart.

### Validation

Validated on 2026-05-24:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service/longtermmemory ./internal/adapter/repository/postgres/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service ./internal/bootstrap/rag ./internal/adapter/http/rag/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/... ./internal/bootstrap/rag ./internal/adapter/http/rag/... ./internal/adapter/repository/postgres/rag -count=1
```

Current result:

- `internal/app/rag/service/longtermmemory` PASS
- `internal/adapter/repository/postgres/rag` PASS
- `internal/app/rag/service` PASS
- `internal/bootstrap/rag` PASS
- `internal/adapter/http/rag/...` PASS
- `internal/app/rag/...` PASS

### Current Conclusion

As of 2026-05-24, the memory module is still best described as `Phase 4+`, not a new phase:

- `Phase 4` cache closure remains the main completed milestone
- today’s work hardened the governance path and improved recall quality inside that phase
- the current focus should now move to:
  - lifecycle cleanup / maintenance
  - metrics refinement / diagnostics hardening
  - structural cleanup of long-term-memory recall code

## Additional Update: 2026-05-23 Memory Phase 4 Cache Closure Completed

### What Changed

`memory V1` 的 Phase 4 已经从 “cache skeleton” 推进到完整闭环，当前新增能力包括：

- `request-scope L1`
  - 在 `prepareChat(...)` 生命周期内创建共享请求级缓存
  - 长期记忆 recall、`SearchFacts(...)`、`session recall` 复用同一份 request cache
- `conversation-scope L1`
  - 新增进程内 `TTL + LRU` 会话级缓存
  - 仅用于 `session recall`
- `Redis L2`
  - 继续承载长期记忆的：
    - `rule memories`
    - `fact ranking result`
    - `query embedding`
  - 保留 `global / kb` scope version 失效语义

### Long-Term Memory Cache Closure

当前 `RecallMemories(...)` 与 `SearchFacts(...)` 已统一走：

1. request-scope `L1`
2. Redis `L2`
3. DB / vector recompute

已完成的关键点：

- `rule memories / fact rankings / query embeddings` 三类缓存全部接通
- `RecallMemories(...)` 与 `SearchFacts(...)` 可命中同一份 `fact ranking result`
- cache hit 下仍保留 `TouchLastUsed(...)` 语义
- 未缓存以下粗粒度对象：
  - 最终 `MemoryContext` 文本
  - 最终 `KnowledgeContext` 文本
  - 整包 `RetrievedChunk`
  - 整个 `prepareChat(...)` 结果

### Session Recall Cache

本轮同时把 `session recall` 纳入 Phase 4：

- `SessionChunkRepository` 已新增 recall fingerprint 读取能力
- fingerprint 包含：
  - recallable chunk 是否存在
  - recallable chunk 数量
  - 最新更新时间
  - 最新 chunk / message 标识
- `session recall` 的 cache key 已固定包含：
  - `userID`
  - `conversationID`
  - `rewrittenQuery`
  - `excludeMessageID`
  - recall fingerprint
  - recall 参数集
- `session recall` 最终结果已进入 conversation-scope cache
- 空结果允许短 TTL 缓存，用于抑制重复 miss 风暴

### Observability and Runtime Wiring

Phase 4 这次不只是加 cache，也把观测和装配收完整了：

- 新增 `rag.memory.cache.*` 配置项：
  - `request-scope-enabled`
  - `conversation-scope-enabled`
  - `session-recall-enabled`
  - `request-max-entries`
  - `conversation-max-entries`
  - `conversation-ttl-seconds`
  - `empty-session-ttl-seconds`
  - `metrics-enabled`
  - `redis-key-prefix`
- 新增 `GET /rag/memory/metrics`
- `long_term_memory` trace 已补：
  - `cacheEnabled`
  - `ruleCacheLayer`
  - `factCacheLayer`
  - `embeddingCacheLayer`
  - `scopeVersions`
  - `recomputeReason`
- `session_recall` trace 已补：
  - `cacheEnabled`
  - `cacheLayer`
  - `recallFingerprint`
  - `embeddingCacheLayer`
  - `recomputeReason`
- 所有缓存异常保持 `fail-open`
  - Redis 异常
  - 反序列化异常
  - fingerprint 查询异常
  - local cache 溢出 / evict
  都只允许降级，不阻断 chat / retrieve 主链路

### Validation

Validated on 2026-05-23:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/... ./internal/bootstrap/rag ./internal/adapter/http/rag ./internal/adapter/repository/postgres/rag ./cmd/server -count=1
```

Current result:

- `internal/app/rag/...` PASS
- `internal/bootstrap/rag` PASS
- `internal/adapter/http/rag` PASS
- `internal/adapter/repository/postgres/rag` PASS
- `cmd/server` PASS

### Current Conclusion

As of 2026-05-23, the memory track has moved past “Phase 4 design / skeleton”:

- `Phase 4` cache closure is now implemented
- `session recall` has entered the same cache/observability framework
- memory infrastructure work should now shift to:
  - lifecycle cleanup / maintenance
  - metrics refinement and production observation
  - only then further `memory_fact` tuning
