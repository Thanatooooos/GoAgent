# Project Progress Context

Latest incremental update: `2026-06-02`

更新时间：2026-05-30

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
   已完成两条并行路线：旧 `internal/app/rag/tool` 继续作为稳定生产路径；新 `internal/app/agent` 已完成 `M1 -> capability V2 -> planner/handoff -> runtime approval/resume` 的主链路闭环。当前重点已经从“先把 reactive runtime 跑起来”转向“稳定 capability/runtime/approval 边界，并为第二种 pattern 和后续正式接入上层 chat 流程做准备”。

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

## Additional Update: 2026-05-25 Memory Structural Refactor Landed

### Status Update

As of `2026-05-25`, the previously planned code-structure cleanup for long-term memory has landed.

This round did not introduce a new product phase. It focused on package boundaries, dependency direction, and keeping the public import path stable while reducing internal coupling.

So the current memory status should now be read as:

- `Phase 4` cache closure: implemented
- `Phase 4+` correctness hardening and first-pass retrieval-quality tuning: implemented for the current round
- structural cleanup of long-term-memory governance / recall / cache code: implemented

### What Changed

#### 1. `longtermmemory` is now split by responsibility instead of remaining a flat package

The public entrypoint stays at:

- `internal/app/rag/service/longtermmemory`

But its internal structure is now organized as:

- root package
  - keeps `MemoryService`
  - keeps the public import path stable for callers
  - owns compatibility exports for public memory input / output / option types
- `internal/app/rag/service/longtermmemory/governance`
  - owns normalize / gate / schema / conflict detection / save / lifecycle rules
- `internal/app/rag/service/longtermmemory/recall`
  - owns recall / ranking / token building / context rendering / retrieval projection / cache support
- `internal/app/rag/service/longtermmemory/types`
  - is kept as a small leaf package for shared public DTOs and options so the root package and subpackages can reuse them without creating Go import cycles

This means the root package is still the stable application entry, but the write-path governance code and read-path recall code are no longer mixed together in one large flat directory.

#### 2. Cross-layer contracts were moved out of the service package

Two memory contracts were moved into:

- `internal/app/rag/port/memory_recall_cache.go`
- `internal/app/rag/port/memory_mutation_transaction.go`

The key changes are:

- `RecallCache` now comes from `port.MemoryRecallCache`
- mutation transaction wiring now comes from `port.MemoryMutationTransaction`
- Redis cache adapter and Postgres transaction adapter no longer reverse-depend on the `longtermmemory` business package

This fixes the earlier "adapter imports service internals" direction problem and makes later adapter or service refactors safer.

#### 3. Recall internals were further broken into narrower files

The `recall` package is not just a directory move. It is also split into clearer implementation units such as:

- `service.go`
- `ranking.go`
- `tokens.go`
- `projection.go`
- `context_renderer.go`
- `cache_support.go`
- `cache_keys.go`
- `cache_mappers.go`

This was important because the previous recall/cache implementation had already become the densest part of the memory codebase.

#### 4. Session recall and runtime wiring now depend on `port` contracts

The structural refactor also propagated to integration points:

- `internal/app/rag/service/session_recall_service.go`
- `internal/app/rag/service/session_recall_cache.go`
- `internal/bootstrap/rag/runtime.go`
- `internal/adapter/cache/redis/rag_memory_cache.go`
- `internal/adapter/repository/postgres/rag/memory_item_transaction.go`

So the runtime wiring still behaves the same, but the dependency direction is cleaner:

```text
adapter -> port <- service
```

instead of adapter depending on a concrete service package for shared cache / transaction contracts.

### Validation

Validated on `2026-05-25`:

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/longtermmemory -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service ./internal/bootstrap/rag ./internal/adapter/repository/postgres/rag -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/... ./internal/adapter/cache/redis ./internal/adapter/repository/postgres/rag ./internal/bootstrap/rag -run Test^$ -count=1
```

Current result:

- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/service` PASS
- `internal/bootstrap/rag` PASS
- `internal/adapter/repository/postgres/rag` PASS
- `internal/adapter/cache/redis` PASS
- `internal/app/rag/service/longtermmemory/governance` PASS

### Current Conclusion

As of `2026-05-25`, the memory track should no longer describe "long-term-memory recall/cache structural cleanup" as pending work.

The more accurate next priorities are now:

1. lifecycle cleanup / maintenance jobs
2. metrics refinement / diagnostics hardening
3. direct unit-test coverage around `recall` package internals
4. only after that, further `memory_fact` weighting / policy tuning

## Additional Update: 2026-05-25 Memory Phase 4 Maintenance and Metrics Closure

### Status Update

As of `2026-05-25`, the previously pending `Phase 4` follow-up items around lifecycle maintenance and first-pass production metrics are no longer only planned work. The main closure path has landed.

This round still stays within the current `memory V1` scope. It does not introduce automatic preference extraction or a new memory write path. It closes the existing runtime/operations loop around the current long-term-memory implementation.

So the current memory status should now be read as:

- `Phase 4` cache closure: implemented
- `Phase 4+` correctness hardening and retrieval-quality tuning: implemented
- structural cleanup of governance / recall / cache code: implemented
- lifecycle maintenance runtime loop: implemented
- first-pass maintenance / fail-open metrics: implemented

### What Changed

#### 1. Long-term-memory maintenance is now wired into `RAG runtime`

- `MemoryService.RunMaintenance(...)` is no longer only a service-layer capability
- `internal/bootstrap/rag/runtime.go` now starts a background maintenance loop when enabled by config
- loop behavior follows the same engineering pattern already used by `knowledge` background jobs:
  - immediate first run
  - ticker-based periodic execution
  - per-iteration timeout
  - panic recovery
  - graceful shutdown on `Runtime.Close()`

This means memory lifecycle cleanup is now a real runtime concern rather than a dormant helper method.

#### 2. `rag.memory.maintenance.*` config boundary has landed

The memory config now includes:

- `enabled`
- `scan-delay-ms`
- `run-timeout-ms`
- `expire-batch-size`
- `delete-batch-size`
- `delete-retention-days`

Default behavior stays conservative:

- maintenance is off unless explicitly enabled
- default retention and batch behavior still align with the existing service defaults

#### 3. Memory metrics are no longer cache-only

`GET /rag/memory/metrics` continues to keep the existing cache metrics contract, but now exposes additional additive counters for:

- maintenance runs / failures
- total expired / deleted rows
- embedding generation failures
- embedding persistence failures
- `touchLastUsed(...)` fail-open events
- scope-version lookup fail-open events

This means the memory metrics endpoint has moved from "cache observability only" to "first-pass operational observability for the memory subsystem."

#### 4. Fail-open paths now have explicit counters

Several existing fail-open branches were already present in code, but were only visible through logs.

This round made them measurable:

- query embedding generation failure
- embedding persist failure
- `touchLastUsed(...)` write-back failure
- scope-version lookup fallback
- maintenance execution failure

So the current memory behavior is still availability-first, but it is no longer silent from an operations perspective.

### Validation

Validated on `2026-05-25`:

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; $env:GOMODCACHE='D:\goagent\.gomodcache'; go test ./internal/app/rag/service/longtermmemory ./internal/app/rag/cachemetrics -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; $env:GOMODCACHE='D:\goagent\.gomodcache'; go test ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
```

Current result:

- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/cachemetrics` PASS
- `internal/adapter/http/rag` PASS
- `internal/bootstrap/rag` PASS

### Current Conclusion

As of `2026-05-25`, the memory track should no longer treat "lifecycle cleanup / maintenance" and "basic metrics refinement" as pending top-level gaps.

The more accurate next priorities are now:

1. diagnostics / trace-level hardening beyond metrics counters
2. continued direct unit-test coverage and test-file downsizing around long-term-memory internals
3. only after the operations surface is stable, auto-extraction of user preference / habit memories
4. later-stage `memory_fact` weighting / policy tuning

## Additional Update: 2026-05-29 Eino-Native Agent Fetch PoC Landed

### Status Update

As of `2026-05-29`, the agent/tool track now has a second implementation line beyond the existing self-built `internal/app/rag/tool` runtime:

- the existing production-oriented self-built `AgentLoop V1`
- a new independent `internal/app/agent` Eino-native experiment package

This new package is intentionally **not** a compatibility layer over the old tool/runtime contracts. It is a parallel PoC that validates whether a future agent loop can be organized around Eino-native tool, workflow, and session-state boundaries from the start.

So the current agent/tool status should now be read as:

- self-built `AgentLoop V1`: still the main production path
- Eino graph/workflow inside existing tools: already in use
- Eino-native outer agent/runtime exploration: now implemented as an executable PoC skeleton

### What Changed

#### 1. A new independent `internal/app/agent` package now exists with explicit package boundaries

The current structure has stabilized into:

- `internal/app/agent/search`
  - search business kernel
- `internal/app/agent/websearch`
  - Eino-native `web_search` tool
- `internal/app/agent/fetch`
  - page-fetch business kernel
- `internal/app/agent/webfetch`
  - Eino-native `web_fetch` tool
- `internal/app/agent/workflow`
  - Eino-native workflow/runtime assembly
- `internal/app/agent`
  - public `Request / Response / Service`

This means the experiment is no longer only "one tool can run under Eino". It already has a first-pass package structure that can continue growing without depending on the old `rag/tool` contracts.

#### 2. The Eino-native workflow is no longer search-only

The first search-only loop has been extended into:

- `plan -> web_search -> web_fetch -> observe`

More specifically:

- `search_agent.go`
  - invokes Eino-native `web_search`
- `fetch_agent.go`
  - invokes Eino-native `web_fetch`
- `observe_agent.go`
  - combines search result state and fetch result state
  - decides degraded/final status
  - emits unified `FinalState`

So the PoC has moved from "single tool demo" into "minimal external-evidence loop".

#### 3. `fetch / webfetch` is now implemented using the same dual-layer discipline as `search / websearch`

The new `fetch` side owns:

- URL normalization
- concurrent fetching
- HTML text extraction
- truncation
- page-level output aggregation

The new `webfetch` side owns:

- Eino tool schema
- input/output mapping
- degraded-output normalization

This was an important structural checkpoint because it confirms the dual-layer split is still maintainable when the tool is no longer only a simple provider call.

#### 4. The new agent service can now return both search and fetched-page outputs

`internal/app/agent.Service` no longer only returns search result summaries.

Its response now includes:

- search results
- fetched pages
- combined readable fetched text
- final degraded/degrade-reason state

This means the PoC has crossed from "tool invocation testbed" into a small but real application-layer entrypoint.

### Validation

Validated on `2026-05-29`:

```powershell
go test ./internal/app/agent/... -count=1
go test ./internal/app/rag/tool/invokers/web -count=1
```

Current result:

- `internal/app/agent` PASS
- `internal/app/agent/fetch` PASS
- `internal/app/agent/search` PASS
- `internal/app/agent/webfetch` PASS
- `internal/app/agent/websearch` PASS
- `internal/app/agent/workflow` PASS
- `internal/app/rag/tool/invokers/web` PASS

### Current Conclusion

As of `2026-05-29`, the framework-based agent exploration should no longer be described as only an idea or directory plan.

The more accurate status is:

1. the Eino-native experiment line is now executable
2. it already supports a minimal `search -> fetch -> observe` loop
3. it is still intentionally isolated from the current production chat/runtime path
4. the next meaningful work is to improve:
   - URL selection between `web_search` and `web_fetch`
   - fetch-result quality assessment and stop conditions
   - additional native tools only after the current loop semantics are clearer

This also means that if the team later wants to compare "self-built loop vs framework-built loop", there is now a real second implementation line to evaluate instead of only a paper design.

## Additional Update: 2026-05-30 Agent Runtime M0-M1 Closure Progress

### Status Update

As of `2026-05-30`, the Eino-native `internal/app/agent` line is no longer best
described as only a PoC with a separate ADK workflow entrypoint.

The codebase now has a first runtime-native execution path built around:

- `internal/app/agent/state`
- `internal/app/agent/runtime`
- `internal/app/agent/kernel`
- `internal/app/agent/pattern/reactive`

and the public `internal/app/agent/service.go` entrypoint has been switched to
that new path.

### What Changed

#### 1. M0 state/runtime abstractions are now consumed by real execution code

The following abstractions are no longer "definition only":

- `RuntimeSession`
- `StateSnapshot`
- `RuntimeEvent`
- `StateDelta`
- `Reducer`
- `NodeResult`
- `DecisionArtifact`

They are now used by the kernel and the first runtime-native reactive pattern.

#### 2. Answer state and reducer closure landed

The previously missing answer-side state path is now present:

- `StateSnapshot.Answer`
- `AnswerState`
- `AnswerDelta`
- reducer support for answer writes

This means the minimal `evaluate -> branch -> answer|degrade` path now has a
real state target instead of depending on ad-hoc final-state structs.

#### 3. Checkpoint serialization preconditions were wired in

The runtime/state types needed by M1 checkpoint usage now have
`schema.RegisterName(...)` registration.

This closes the spike-discovered gap where typed state could be designed
correctly but still fail at checkpoint persistence time if types were not
registered.

#### 4. A formal M1 kernel skeleton now exists

`internal/app/agent/kernel` now contains:

- runtime-native node protocol
- graph builder
- compiled runner
- memory checkpoint store
- kernel tests for branch/reducer flow and checkpoint resume

So M1 is no longer only "planned around the spike"; a first formal kernel layer
now exists in production code.

#### 5. The first runtime-native reactive pattern landed

`internal/app/agent/pattern/reactive` now contains the first real execution
chain:

- `prepare`
- `search`
- `fetch`
- `observe`
- `answer`
- `degrade`

This chain directly uses:

- `search.Service`
- `fetch.Service`

and writes its outputs back through typed state and reducer merge, rather than
through ADK session-value keys.

#### 6. The old `internal/app/agent/workflow` PoC path was retired

The earlier ADK-based PoC package using:

- `SequentialAgent`
- `LoopAgent`
- session values

has now been removed from `internal/app/agent`.

The public `agent.Service` entrypoint now runs through the runtime-native
reactive path instead.

### Validation

Validated on `2026-05-30`:

```powershell
go test ./internal/app/agent/... -count=1
```

Current result:

- `internal/app/agent` PASS
- `internal/app/agent/fetch` PASS
- `internal/app/agent/kernel` PASS
- `internal/app/agent/kernel/spike` PASS
- `internal/app/agent/pattern/reactive` PASS
- `internal/app/agent/search` PASS
- `internal/app/agent/state` PASS
- `internal/app/agent/webfetch` PASS
- `internal/app/agent/websearch` PASS

### Current Conclusion

As of `2026-05-30`, the most accurate agent/runtime status is:

1. M0 abstractions are no longer just design-time structures; they are now used
   by executable code
2. an M1 kernel skeleton exists and is tested
3. the first runtime-native reactive pattern is landed and wired into
   `agent.Service`
4. the previous `workflow/` PoC line inside `internal/app/agent` has been
   retired
5. the next meaningful work should move to:
   - richer runtime event / replay surfaces
   - more operational checkpoint metadata and inspection
   - additional patterns or more capable answer-generation logic only after the
     current runtime path is further hardened

## Additional Update: 2026-05-30 Agent Runtime M1 Observability Closure

After the `M0-M1` runtime skeleton was wired into `agent.Service`, the agent
runtime line continued with a focused `M1` hardening pass aimed at inspection,
checkpoint lifecycle clarity, and replay quality.

### What Changed

#### 1. Runtime replay now has a formal inspection view

`internal/app/agent/runtime/replay.go` now provides a minimal `ReplayView`
projection from `RuntimeSession`.

The replay surface now exposes:

- node execution summaries
- structured decision summaries
- checkpoint summary
- final answer / degrade reason
- evidence sufficiency summary
- event timeline summary

This means runtime inspection no longer depends only on manually reading the
raw journal.

#### 2. Checkpoint / resume metadata is now written back into `RuntimeSession`

`internal/app/agent/kernel/runner.go` now updates session-level metadata when a
checkpointed run interrupts or resumes.

This includes:

- `RuntimeSession.Checkpoint`
- `SessionMetadata.ResumedFrom`
- `SessionMetadata.ResumeCount`
- journal events for interrupt / resume completion

So checkpointing is no longer only a lower-level compose capability; the
runtime session itself now records the lifecycle in a directly inspectable way.

#### 3. Runtime events are now more structured

`RuntimeEvent` was extended so journal entries can carry:

- `Sequence`
- structured `Decision`
- structured `Checkpoint`

Kernel journal appends now consistently assign event sequence numbers and write
decision/checkpoint structure alongside the existing textual summary.

This reduces replay dependence on string parsing and gives later tooling a more
stable inspection surface.

#### 4. Delta-backed state projection is now available

The runtime now records `state_applied` events after reducer merge, including
the applied `StateDelta`.

`internal/app/agent/runtime/projection.go` now provides:

- `ProjectSnapshotAt(...)`
- `BuildProjectionTimeline(...)`

These rebuild state from:

- `RuntimeSession.InitialSnapshot`
- ordered `state_applied` delta events
- the existing reducer rules

This means the runtime can now answer not only:

- "what events happened?"

but also:

- "what did `StateSnapshot` look like at event sequence `N`?"

#### 5. Snapshot / delta cloning support was added for safe replay

`internal/app/agent/state/clone.go` now provides deep-copy helpers for
`StateSnapshot` and `StateDelta`.

This keeps replay/projection from accidentally sharing backing slices with the
live session state.

### Validation

Validated on `2026-05-30`:

```powershell
go test ./internal/app/agent/runtime ./internal/app/agent/kernel ./internal/app/agent/state -count=1
go test ./internal/app/agent/... -count=1
```

Current result:

- `internal/app/agent/runtime` PASS
- `internal/app/agent/kernel` PASS
- `internal/app/agent/state` PASS
- `internal/app/agent` PASS
- `internal/app/agent/pattern/reactive` PASS

### Updated Conclusion

As of the end of `2026-05-30`, the most accurate `M1` status is:

1. the kernel skeleton is implemented and wired into the public agent entry
2. checkpoint / resume is not only runnable, but session-visible
3. replay now has a formal summary view
4. journal events are structurally richer and sequence-aware
5. state can now be projected at arbitrary event offsets using
   `InitialSnapshot + applied deltas + reducer`

So the runtime is no longer only "able to run and resume"; it now has a first
real inspection and state-reconstruction surface, which materially lowers the
risk of continuing into broader `M2` work.

## Additional Update: 2026-05-30 Agent Runtime M1.5 Hardening and Capability Registry Closure

### Status Update

As of `2026-05-30`, the `internal/app/agent` line should no longer be described
only as:

- `M0` abstractions landed
- `M1` kernel skeleton landed
- one minimal `search -> fetch -> observe -> answer|degrade` path landed

That description is still directionally true, but it now misses a substantial
`M1.5` hardening pass that has already landed in code.

The current runtime line now also includes:

- normalized runtime event semantics
- richer replay / checkpoint inspection
- more coherent execution-state writeback
- a typed capability seam
- a minimal capability registry
- capability-spec-driven approval interrupt wiring
- reducer-based error-path state updates

So the more accurate status is:

- `M0`: implemented
- `M1`: implemented
- first `M2`-style reactive path: implemented
- `M1.5` hardening and registry closure around that path: implemented

### What Changed

#### 1. Runtime event and journal semantics were normalized

`internal/app/agent/state/event.go` now centralizes the runtime event
vocabulary, and `internal/app/agent/kernel/journal.go` now normalizes journal
append behavior.

The current event surface now explicitly includes:

- `node_start`
- `node_finish`
- `node_error`
- `reducer_error`
- `state_applied`
- `decision_emitted`
- `branch_selected`
- `capability_start`
- `capability_result`
- `capability_skipped`
- `answer_finalized`
- `degraded`
- `interrupt`
- `resume_completed`

This means the runtime now has a more stable internal event contract rather
than a loosely accumulated set of event strings.

#### 2. Replay / checkpoint inspection is now materially richer

`internal/app/agent/runtime/replay.go` no longer exposes only a minimal
timeline projection.

It now also provides:

- checkpoint lifecycle summary
- event type counts
- capability summaries
- branch summaries
- last decision summary
- more readable `state_applied` delta summaries

So runtime inspection can now answer not only "what happened?" but also:

- "which capability ran?"
- "which branch was selected?"
- "is this checkpoint still active or already resumed?"
- "what state domains changed at this step?"

#### 3. Execution-state writeback was tightened up

`internal/app/agent/pattern/reactive/execution_state.go` now centralizes the
reactive pattern's execution-state delta helpers.

At the same time, `internal/app/agent/state/reducer.go` now de-duplicates:

- `ScheduledActions`
- `CompletedActions`
- `FailedActions`

This means execution-state history is now less prone to duplicate growth across
repeated reducer merges.

#### 4. Interrupt / resume now update both journal and snapshot execution state

Earlier work already made checkpoint lifecycle visible through:

- `RuntimeSession.Checkpoint`
- `SessionMetadata.ResumedFrom`
- `SessionMetadata.ResumeCount`
- interrupt / resume journal events

This round also made the execution snapshot itself align better with those
events:

- interrupt writes `Execution.CurrentNode`
- interrupt sets `Execution.Interrupted=true`
- interrupt records `Execution.InterruptReason`
- resume clears the interrupt flags back out

So runtime state and runtime journal are now more coherent on checkpointed
runs.

#### 5. Search / fetch now run through a typed capability seam

A new `internal/app/agent/capability` package now provides:

- `SearchCapability`
- `FetchCapability`
- `Spec`
- capability construction options

The reactive runtime path no longer directly treats `search.Service` /
`fetch.Service` as its execution-unit abstraction.

Instead:

- services are adapted into typed capabilities
- reactive nodes invoke typed capabilities
- capability-level eventing continues to flow into the runtime journal

#### 6. A minimal capability registry is now real and used by the public path

`internal/app/agent/capability/registry.go` now provides a first registry
layer, and `internal/app/agent/service.go` now assembles:

- search service
- fetch service
- typed capabilities
- capability registry

`pattern/reactive.Compile(...)` now resolves capabilities from the registry
instead of receiving capability instances directly.

So the public runtime path has already crossed into registry-mediated
capability assembly.

#### 7. Approval-gated capability specs now drive compile-time interrupts

Capability spec metadata now includes:

- `RiskLevel`
- `RequiresApproval`
- `SupportsParallel`
- `SupportsResume`

The reactive builder now derives `InterruptBeforeNodes` from those specs.

So when a registered capability is marked `RequiresApproval=true`, the current
runtime automatically inserts a compile-time interrupt before the corresponding
node.

This is the first real closure of:

- capability metadata
- runtime compile configuration
- approval / interrupt behavior

in one execution path.

#### 8. Builder error-path state updates now go through the reducer

One structural gap remained after the earlier runtime closure:

- successful node execution wrote state through reducer
- failed node execution still mutated `session.Snapshot.Execution` directly

That gap is now closed.

`internal/app/agent/kernel/builder.go` now constructs an execution error delta
for node failures and applies it through the reducer before completing the
error-path journal updates.

This means reducer-mediated state writes are now used in both:

- success path
- failure path

which is an important consistency improvement for the runtime model.

### Validation

Validated on `2026-05-30`:

```powershell
go test ./internal/app/agent/runtime ./internal/app/agent/kernel ./internal/app/agent/pattern/reactive ./internal/app/agent -count=1
go test ./internal/app/agent/capability ./internal/app/agent/pattern/reactive ./internal/app/agent ./internal/app/agent/runtime ./internal/app/agent/kernel -count=1
go test ./internal/app/agent/... -count=1
```

Current result:

- `internal/app/agent` PASS
- `internal/app/agent/capability` PASS
- `internal/app/agent/fetch` PASS
- `internal/app/agent/kernel` PASS
- `internal/app/agent/kernel/spike` PASS
- `internal/app/agent/pattern/reactive` PASS
- `internal/app/agent/runtime` PASS
- `internal/app/agent/search` PASS
- `internal/app/agent/state` PASS
- `internal/app/agent/webfetch` PASS
- `internal/app/agent/websearch` PASS

### Updated Conclusion

As of the end of `2026-05-30`, the most accurate agent/runtime status is:

1. the typed `M0` runtime abstractions are fully implemented and actively used
2. the `M1` kernel skeleton is implemented, tested, and no longer only a spike
3. the first `M2`-style reactive path is executable through the public service
4. `M1.5` hardening has already improved:
   - event consistency
   - replay/checkpoint inspection
   - execution-state coherence
   - capability assembly boundaries
   - approval-driven interrupt wiring
   - failure-path state consistency
5. the next meaningful work should move to:
   - real reactive loop / `continue` semantics
   - broader capability families beyond search/fetch
   - richer capability metadata (`Dependencies`, schemas, etc.)
   - later planner / observer / policy-layer upgrades

## Additional Update: 2026-05-31 Agent Runtime Planner, Handoff, and Policy Projection Closure

### Status Update

As of `2026-05-31`, the `internal/app/agent` runtime line has moved beyond:

- a typed reactive loop with `continue`
- a rule-based observe policy
- a service-end handoff projection

The runtime now has:

- an `observe`-after structured LLM planner seam
- planner-guided next-round search/fetch inputs
- `handoff` as a first-class runtime terminal action
- an explicit `OutputMode` split between `handoff` and `final_answer`
- a dedicated `handoff/` projection package
- capability/profile-derived `WorkflowPolicy` instead of hardcoded prompt text

This means the new runtime is no longer only "able to gather evidence and stop";
it now has a clearer answer-boundary model and a more realistic integration seam
for later `RagChatService` wiring.

### What Changed

#### 1. A structured LLM planner now runs after `observe`

The new `internal/app/agent/planner` package now provides a runtime-native
planner seam:

- `Planner` interface
- `LLMPlanner`
- strict planner JSON contract validation

The planner is placed after `observe`, not before the first search round.

Its input is a compressed runtime summary covering:

- request / iteration state
- search summary
- fetch summary
- evidence summary
- progress summary
- baseline rule-policy decision

Its output is a strict structured artifact including:

- `decision`
- `reason`
- `confidence`
- `next_query`
- `preferred_urls`
- `avoid_urls`
- `answer_plan`

The runtime validates planner output before accepting it, including:

- allowed-action checks
- iteration-budget checks
- evidence-required checks for terminal answer/handoff
- "known URL only" validation for preferred/avoid URL guidance
- fallback to the baseline rule policy when validation fails

So planner integration landed as a guarded decision layer rather than an
unbounded free-text override.

#### 2. The reactive loop can now consume planner guidance across rounds

Planner output is no longer only logged or summarized.

The runtime state now persists the next-round guidance needed by the reactive
loop:

- `Context.SearchQuery`
- `Context.PreferredURLs`
- `Context.AvoidURLs`

This means:

- the next `search` round can use a planner-refined query
- `fetch` can prioritize preferred URLs
- `fetch` can skip avoided URLs
- repeated low-value or duplicate fetches are easier to suppress

This is the first closure where:

- `observe`
- planner decision
- next-round execution inputs

are all connected through typed state instead of ad-hoc node-local logic.

#### 3. Fetch text preparation was intentionally kept simple and deterministic

The new planner path needed a cleaner evidence surface, but this round
intentionally did **not** add relevance scoring or passage-ranking complexity.

Instead, `fetch` text preparation now focuses on deterministic cleanup:

- removing script/style/head/comment noise
- preserving paragraph structure
- removing common boilerplate lines
- deduplicating repeated lines
- normalizing whitespace

So planner-facing fetched text is now cleaner than the original raw extraction,
while still avoiding premature LLM-based or scoring-heavy summarization logic.

#### 4. `handoff` is now a first-class runtime terminal, not only a service projection

The reactive pattern no longer treats `answer` as the only positive terminal.

It now supports explicit output-mode-aware terminals:

- `continue`
- `handoff`
- `answer`
- `degrade`

And the public `agent.Service` default path now prefers:

- `OutputModeHandoff`

rather than defaulting to final answer generation.

This is an important architecture clarification:

- the runtime's primary job is now framed as action / evidence / policy
  orchestration
- final user-facing answer generation is no longer assumed to be the default
  runtime responsibility

This also matches the production reality of the old `rag/tool` path more
closely, where agent execution primarily produced context and answer guidance
for `RagChatService`.

#### 5. A dedicated `internal/app/agent/handoff` package now owns the handoff projection

The initial single-file handoff projection has now been split into a dedicated
package with clearer responsibilities:

- `result.go`
- `build.go`
- `tool_context.go`
- `answer_guidance.go`
- `workflow_policy.go`

This means the runtime now has a cleaner boundary between:

- execution-time typed state
- post-run prompt-ready handoff projection

The public `agent` package still exposes `HandoffResult`, but the actual
projection logic is no longer mixed into the top-level service package.

#### 6. `WorkflowPolicy` is now derived from capability metadata and runtime state

The handoff layer no longer hardcodes policy text such as:

- `capability: search`
- `execution_mode: read_only`
- `risk_level: low`

Instead, `WorkflowPolicy` is now built from:

- runtime options
- output mode
- max iterations
- actual capability usage
- capability profile metadata
- approval requirements
- highest observed capability risk

This is an important quality step because the handoff prompt surface is now
closer to the real runtime state, rather than a placeholder string template.

### Validation

Validated on `2026-05-31` with focused package suites:

```powershell
go test ./internal/app/agent/planner -count=1
go test ./internal/app/agent/pattern/reactive -count=1
go test ./internal/app/agent/handoff -count=1
go test ./internal/app/agent -count=1
```

Current focused result:

- `internal/app/agent/planner` PASS
- `internal/app/agent/pattern/reactive` PASS
- `internal/app/agent/handoff` PASS
- `internal/app/agent` PASS

In addition, an earlier same-day end-to-end `go test ./internal/app/agent/... -count=1`
pass was completed before the final handoff-policy derivation cleanup.

### Updated Conclusion

As of the end of `2026-05-31`, the most accurate agent/runtime status is:

1. the reactive runtime now has a guarded post-observe LLM planner seam
2. planner output can influence the next search/fetch round through typed state
3. `handoff` is now a first-class runtime terminal and the default public output mode
4. the handoff projection has been split into a dedicated package with clearer boundaries
5. `WorkflowPolicy` is no longer placeholder text; it is now derived from capability/runtime metadata
6. the next meaningful work should move to:
   - integrating the handoff contract into the future `RagChatService` bridge
   - deciding how much final-answer generation should remain optional inside `agent`
   - broadening capability families beyond search/fetch while preserving the same runtime boundaries

## Additional Update: 2026-05-31 Agent Runtime Capability Registry and Pattern Assembly Closure

### Status Update

As of `2026-05-31`, the `internal/app/agent` line has continued past the
earlier planner / handoff closure and completed a smaller but important
architecture-hardening pass around:

- capability definition boundaries
- unified capability registration
- pattern-facing assembly contracts
- metadata-driven handoff/profile projection

This work did **not** add a second pattern yet. Instead, it intentionally
finished the structural cleanup that should happen before adding another
pattern or wiring the new runtime into `RagChatService`.

### What Changed

#### 1. Capability spec is no longer only a thin search/fetch seam

`internal/app/agent/capability/capability.go` now defines a more complete V1
capability descriptor.

The runtime capability model now explicitly includes:

- `Name`
- `Kind`
- `Family`
- `Roles`
- `RiskLevel`
- `RequiresApproval`
- `SupportsParallel`
- `SupportsResume`
- `Dependencies`

The current kind taxonomy is now:

- `tool`
- `workflow`
- `sub_agent`

And the first family / role vocabulary is now formalized in code, including:

- `external_evidence`
- `document_investigation`
- `trace_investigation`
- `discovery`
- `meta`

plus role constants such as:

- `search`
- `fetch`

This means capability is now better defined as a runtime-governed execution
unit, not only "whatever search/fetch adapter happened to exist first."

#### 2. The capability registry is now unified instead of search/fetch-specialized

`internal/app/agent/capability/registry.go` no longer uses separate dedicated
registration maps as the primary model.

The registry now:

- registers a common `Handle`
- stores normalized `Spec`
- indexes capabilities by `name`
- indexes capabilities by `role`
- indexes capabilities by `family`
- resolves typed search/fetch handles from the unified catalog

Registration now also validates:

- required `Name / Kind / Family / Roles`
- duplicate names
- self-dependencies
- missing declared dependencies
- role/interface consistency

This is an important closure because runtime assembly is now based on one
capability catalog rather than a hardcoded "search registry + fetch registry"
split.

#### 3. Role binding is now a reusable capability-layer concept

`internal/app/agent/capability/bindings.go` now provides a reusable
`RoleBindings` model.

Patterns can now:

- bind a role explicitly to a capability name
- validate explicit bindings against the registry
- fall back to automatic resolution when a role has exactly one registered
  candidate
- reject ambiguous resolution when multiple capabilities implement the same role

This means pattern assembly is no longer coupled to one-off config fields like
`SearchCapabilityName` and `FetchCapabilityName`.

#### 4. Pattern-facing assembly contract is now explicit

`internal/app/agent/pattern/config.go` now contains the generic pattern
assembly contract:

- `AssemblyContext`
- `RuntimeConfig`

The reactive pattern now consumes that contract instead of carrying its own
mixed assembly/runtime fields.

This is the first real step toward making `internal/app/agent/pattern` a
shared pattern layer rather than a folder that only happens to contain one
reactive implementation.

#### 5. Reactive assembly and handoff projection no longer depend on service-local hardcoding

`internal/app/agent/pattern/reactive` now owns:

- role-to-capability resolution for the reactive pattern
- reactive-specific handoff node bindings

And `internal/app/agent/handoff/profile_projection.go` now owns:

- capability-profile projection from registry metadata
- family to workflow-capability mapping rules

The current mapping is now explicit in one place:

- `external_evidence -> search`
- `document_investigation -> diagnosis`
- `trace_investigation -> diagnosis`
- `discovery -> knowledge`
- default -> `general`

At the same time, `internal/app/agent/service.go` is now closer to a pure
assembly layer:

- register capabilities
- declare role bindings
- compile the reactive pattern
- build the handoff projector

instead of also hand-owning capability/profile mapping logic.

### Validation

Validated on `2026-05-31` with focused package suites:

```powershell
go test ./internal/app/agent/capability ./internal/app/agent/pattern/reactive ./internal/app/agent/handoff ./internal/app/agent -count=1
```

Current focused result:

- `internal/app/agent/capability` PASS
- `internal/app/agent/pattern/reactive` PASS
- `internal/app/agent/handoff` PASS
- `internal/app/agent` PASS

### Updated Conclusion

As of the end of `2026-05-31`, the most accurate additional runtime status is:

1. capability is now better defined as a runtime-managed unit with `kind`,
   `family`, and `role` semantics
2. capability registration is unified and metadata-indexed rather than
   search/fetch-specialized
3. pattern assembly now has a reusable contract instead of reactive-local
   config drift
4. handoff/profile projection is now driven by registry metadata and explicit
   bindings, not service-local manual mapping
5. the next meaningful step is still **not** "add many low-level query
   capabilities," but rather:
   - decide the second pattern target
   - add higher-level task-oriented capabilities later on top of this registry
     model
   - bridge the new runtime into outer chat flows only after these boundaries
     stay stable

## Additional Update: 2026-06-01 Agent Runtime Capability V2 and Approval/Resume Closure

### Status Update

As of `2026-06-01`, the `internal/app/agent` line should no longer be described
as only:

- a reactive runtime skeleton
- a capability registry hardening pass
- a handoff-oriented output mode experiment

Those are still true, but they are now incomplete.

The new runtime has now completed another important closure around:

- capability V2 contract uplift
- metadata-driven runtime behavior
- runtime approval surfaced as an explicit public outcome
- approval resume wired into the runtime/service path

This means the current `internal/app/agent` line has moved from "M1 runtime
with cleaner seams" into "first runnable runtime with capability governance and
approval lifecycle semantics."

### What Changed

#### 1. Capability V2 is now the real runtime contract

The capability layer is no longer best understood as a thin typed seam for only
`search` and `fetch`.

The runtime now uses a richer capability model centered on:

- `Spec`
- `Handle`
- `InvocationRequest`
- `InvocationResult`
- `ActionRecord`
- `ObservationRecord`

The capability spec now carries runtime-facing metadata such as:

- `InputSchema`
- `OutputSchema`
- `Preconditions`
- `ProducesEvidence`
- `Idempotency`
- `SupportsResume`

This is an important milestone because capability metadata is now being used by
runtime policy rather than only by registration and handoff projection.

#### 2. Search/fetch capability implementation has been separated from the capability root package

`internal/app/agent/capability` now acts as the runtime-facing contract and
governance layer, while:

- `internal/app/agent/search`
- `internal/app/agent/fetch`

own their own capability adapters.

At the same time, a higher-level workflow capability sample now exists in:

- `internal/app/agent/external_evidence`

through `external_evidence_collect`.

This means the catalog now carries both:

- low-level tool capability
- higher-level workflow capability

under the same runtime contract.

#### 3. Capability metadata now affects runtime behavior

The reactive runtime is no longer only "calling generic handles."

It now also consumes richer capability semantics for policy decisions such as:

- `Preconditions`
- `ErrorClass`
- `Idempotency`
- `ProducesEvidence`

In practice, this means:

- invalid input can be rejected through declared capability preconditions
- permission/dependency/external failures are no longer all treated the same
- retry/continue behavior now depends on `ErrorClass` and `Idempotency`
- workflow capabilities can be validated as evidence-producing runtime units

So capability metadata has started becoming runtime policy input, not only
description metadata.

#### 4. Runtime approval is now a first-class public outcome

The new runtime no longer treats approval-related interruption as only an
internal execution detail.

`agent.Service` now exposes detailed run outcomes that distinguish:

- `completed`
- `degraded`
- `awaiting_approval`

The service layer now also exposes approval-aware APIs such as:

- `RunDetailed`
- `RunHandoffDetailed`
- `ResumeAfterApproval`
- `ResumeHandoffAfterApproval`

This is the first time the new runtime has a clean outward semantic for
"execution paused pending approval" rather than only a lower-level interrupt
flag.

#### 5. Approval resume is now wired through session store + checkpoint + reactive approval gate

Compile-time approval and runtime permission-triggered approval are now both
handled through a shared public lifecycle:

- run reaches an approval boundary
- outcome becomes `awaiting_approval`
- service stores the resumable runtime session
- caller later supplies approval decision
- runtime resumes and either:
  - reruns the gated capability path
  - or ends in degrade when approval is rejected

This closure uses:

- checkpoint persistence in the kernel path
- runtime session persistence through a dedicated session store
- a dedicated `ApprovalState`
- a reactive `approval` gate node that can route resumed execution

Architecturally, this is a major step because the new runtime now supports a
real human-in-the-loop lifecycle rather than only static interrupt points.

### Validation

Validated on `2026-06-01`:

```powershell
go test ./internal/app/agent/... -count=1
```

Current result:

- `internal/app/agent` PASS
- `internal/app/agent/capability` PASS
- `internal/app/agent/external_evidence` PASS
- `internal/app/agent/fetch` PASS
- `internal/app/agent/handoff` PASS
- `internal/app/agent/kernel` PASS
- `internal/app/agent/pattern/reactive` PASS
- `internal/app/agent/runtime` PASS
- `internal/app/agent/search` PASS
- `internal/app/agent/state` PASS

### Updated Conclusion

As of the end of `2026-06-01`, the most accurate `internal/app/agent` status is:

1. capability has been lifted to a richer V2 runtime contract rather than a
   typed search/fetch seam
2. capability metadata has begun driving runtime retry/degrade/approval
   behavior
3. runtime approval is now exposed as a public run outcome instead of only an
   internal interrupt detail
4. approval resume has a working service/runtime closure based on checkpoint +
   session store + approval gate routing
5. the next best steps remain:
   - do **not** rush to wire this into `RagChatService` yet
   - first stabilize the outward approval contract and approval persistence
     model
   - then add a second pattern to validate that the runtime is not reactive-only

## Additional Update: 2026-06-02 Capability Selection LLM Closure for Plan-Execute

### Status Update

As of `2026-06-02`, the most important agent/runtime increment is **not**
"pattern router landed."

That is intentionally still deferred.

Instead, the runtime line has now completed the first meaningful closure around:

- LLM-driven capability exposure
- LLM-driven capability selection
- selector-to-registry resolution and validation
- selector-driven `plan_execute` step generation

This is an important sequencing decision.

The current direction is now:

- keep `pattern` selection explicit for now
- first make capability composition and capability choice intelligent
- only later decide whether request-time pattern routing should also become
  LLM-driven

### What Changed

#### 1. The runtime now has an explicit capability-selection stack

The capability layer is no longer only:

- `Spec`
- `Registry`
- `Handle`

It now also has three new runtime-facing sublayers:

- `capability/catalog`
- `capability/select`
- `capability/resolve`

Their roles are intentionally separated:

- `catalog`
  - builds LLM-facing capability cards from registry metadata
- `select`
  - runs structured LLM capability selection
- `resolve`
  - turns selector output back into one concrete executable capability

This means capability choice is no longer forced to live:

- inside one pattern node
- inside service-local wiring
- or inside hardcoded capability-name conditionals

#### 2. Capability metadata is now being exposed to LLM as a constrained catalog

The registry is still the source of truth, but the LLM no longer has to infer
everything from raw implementation identity.

The new catalog layer now exposes a reduced capability card containing fields
such as:

- `name`
- `kind`
- `family`
- `roles`
- `summary`
- `input_hints`
- `requires_approval`
- `supports_resume`
- `produces_evidence`

This is a key architecture step because the runtime now has a cleaner bridge
between:

- governed registry metadata
- model-visible capability semantics

rather than forcing the planner/pattern layer to hardcode capability names.

#### 3. LLM capability selection now follows the same guarded JSON pattern as the planner

The new `LLMSelector` uses the same general runtime style already proven by the
planner path:

- strict JSON mode
- structured response decoding
- post-response validation

Selector output is constrained to structured capability choices rather than
free text.

The current selection object can identify a capability through:

- `name`
- `family`
- `role`
- optional `kind`
- structured `input`

This matters because the runtime is starting to move from:

- "pattern chooses a concrete implementation name"

toward:

- "model chooses a capability by declared semantics, and runtime resolves it"

#### 4. A resolver/validator layer now closes the selector-to-execution gap

One of the main practical risks in LLM-driven capability selection is that a
model can emit:

- an unknown capability
- an ambiguous family/role match
- invalid structured input

That gap is now explicitly handled by the new resolver layer.

It is responsible for:

- matching selector output to registry entries
- rejecting ambiguous matches
- normalizing structured input
- validating preconditions before execution

So the new execution path is now better described as:

`LLM selection -> registry resolution -> input normalization -> capability validation -> execution`

rather than:

`LLM text -> direct invocation`

#### 5. Capability input is no longer limited to already-typed caller-owned structs

To make selector-driven execution actually usable, capability implementations
now support a shared input-normalization seam.

The capability layer now exposes:

- `InputNormalizer`
- `DecodeStructuredInput`

And the current concrete capabilities have been upgraded so they can accept
JSON-like structured input and normalize it into typed runtime input:

- `search`
- `fetch`
- `external_evidence`
- `document_investigation`

This is a very important closure because without it, capability selection would
only be able to choose names, but not reliably execute LLM-produced inputs.

#### 6. `plan_execute` is now the first pattern to consume selector-driven capability choice

The new selector stack is not only registered in service assembly; it is now
actually used by the second pattern.

`plan_execute` now supports:

- building a plan from selector output
- storing selector semantics inside `PlanStep`
- resolving selected capability at execution time

`PlanStep` has been uplifted so it can carry capability-selection semantics
instead of only `search/fetch`-specific fields, including:

- `CapabilityKind`
- `CapabilityFamily`
- `CapabilityRole`
- `CapabilityInput`

This means `plan_execute` is now meaningfully less coupled to:

- fixed `search -> fetch` planning
- concrete capability name hardcoding

even though legacy fallback behavior is still retained.

#### 7. `document_investigation` is now the first selector-driven high-level capability path

The first concrete validation target for this new stack is:

- `document_investigation`

The runtime can now support a path where:

- the request remains on explicit `plan_execute`
- the selector sees the capability catalog
- the selector chooses `document_investigation`
- the runtime resolves it
- the plan becomes a single selected capability step

This is exactly the kind of higher-level workflow capability path the project
intended to validate before building a pattern router.

It proves that a new capability can now be:

- registered
- surfaced to the model
- selected by semantic metadata
- executed through the same runtime contract

without first inventing a new pattern.

### Validation

Validated on `2026-06-02`:

```powershell
go test ./internal/app/agent/pattern/planexecute -count=1
go test ./internal/app/agent/capability/... -count=1
go test ./internal/app/agent/... -count=1
```

Current result:

- `internal/app/agent` PASS
- `internal/app/agent/capability` PASS
- `internal/app/agent/document_investigation` PASS
- `internal/app/agent/external_evidence` PASS
- `internal/app/agent/fetch` PASS
- `internal/app/agent/handoff` PASS
- `internal/app/agent/kernel` PASS
- `internal/app/agent/pattern/planexecute` PASS
- `internal/app/agent/pattern/reactive` PASS
- `internal/app/agent/planner` PASS
- `internal/app/agent/runtime` PASS
- `internal/app/agent/search` PASS
- `internal/app/agent/state` PASS

### Updated Conclusion

As of the end of `2026-06-02`, the most accurate current `internal/app/agent`
status is:

1. capability is no longer only a governed registry/runtime contract; it now
   also has an explicit model-facing selection stack
2. LLM-driven capability choice is now validated on top of the registry rather
   than bypassing it
3. `plan_execute` has become the first pattern that can consume selector-driven
   capability semantics instead of only fixed `search/fetch` planning
4. `document_investigation` is now the first high-level workflow capability
   that can be selected and executed through this new path
5. the most appropriate next steps are:
   - keep `pattern` routing explicit a little longer
   - broaden selector-driven capability usage to more scenarios and families
   - later decide how much of pattern routing should also become LLM-driven

## Additional Update: 2026-06-02 Approval Contract P0 Closure

### Status Update

As of the end of `2026-06-02`, the `internal/app/agent` line should no longer
be described as only having:

- an internal approval pause/resume mechanism
- a service-level `awaiting_approval` outcome
- a partially formed approval lifecycle

Those are still true, but they are now incomplete.

The runtime/service line has now completed a first meaningful **P0 closure**
around:

- outward approval payload definition
- outward approval resume contract
- typed service-level error contract
- handoff approval resume coverage
- approval persistence and cleanup semantics
- approval audit metadata verification

This means the current approval work is no longer just "runtime can pause and
resume." It now has a more explicit public contract and a better-tested service
lifecycle.

### What Changed

#### 1. Approval pending is now a richer public payload rather than a minimal interrupt marker

`internal/app/agent/request_response.go` no longer exposes approval to callers
as only:

- `reason`
- `node`
- `capability`
- `checkpoint_id`

The outward `ApprovalPending` payload now carries a more useful approval-facing
projection, including:

- approval status
- reason code + human-readable reason message
- trigger type
- rerun node
- capability metadata such as:
  - `kind`
  - `family`
  - `risk_level`
  - `supports_resume`
  - `idempotency`
- request / execution context such as:
  - question
  - search query
  - current plan step
  - candidate URLs
- action semantics such as:
  - can approve
  - can reject
  - reject outcome

This outward projection is now assembled in a dedicated service-layer file:

- `internal/app/agent/service_approval.go`

rather than being hand-assembled inline in `service_run.go`.

#### 2. Approval resume now has an explicit outward decision contract

`ResumeApprovalRequest` no longer needs to be understood only through the old
`Approved bool` convention.

The public contract now supports an explicit:

- `Decision = approved | rejected`

while still tolerating the previous boolean path as a compatibility fallback.

Decision parsing and validation has been split into:

- `internal/app/agent/service_approval_resume.go`

This matters because approval resume is now less ambiguous for outer callers,
and invalid decisions now fail through a stable service contract rather than
only through ad hoc string checks.

#### 3. Agent service errors now have a typed outward contract

The approval work exposed a practical gap:

outer callers previously had to infer behavior from free-form error strings.

That gap has now been reduced through:

- `internal/app/agent/service_error.go`

The service layer now exposes:

- stable error codes
- error kind classification
- retryability metadata
- `DescribeServiceError(...)`

The current error kinds now distinguish categories such as:

- `invalid_request`
- `not_found`
- `failed_precondition`
- `unavailable`
- `internal`

This means outer layers no longer need to depend only on `err.Error()` string
matching to understand approval-related failures such as:

- invalid decision
- missing approval session
- approval not pending
- missing question

#### 4. Approval persistence lifecycle is now better defined and tested

The latest P0 closure also hardened the session persistence path used by
approval pause/resume.

One concrete issue was fixed:

- pending approval sessions were stored under both:
  - `checkpoint_id`
  - `session_id`
- but cleanup previously removed only the checkpoint key

`deletePendingSession(...)` now clears both entries, which avoids stale
approval-session leftovers after approve / reject / completion paths.

This is important because approval persistence should now be read as a real
lifecycle contract, not merely "something happened to be stored in memory."

#### 5. Handoff approval resume is now covered alongside final-answer resume

Earlier approval work primarily validated:

- `RunDetailed`
- `ResumeAfterApproval`

The latest closure also added explicit coverage for:

- `RunHandoffDetailed`
- `ResumeHandoffAfterApproval`

This means approval semantics are now better aligned across both public output
modes:

- final answer
- handoff

rather than being validated only on the final-answer path.

#### 6. Approval audit metadata is now explicitly verified

The latest tests now assert that approval review state is actually preserved in
runtime session data, including:

- `Approval.RequestedAt`
- `Approval.ReviewedAt`
- `Approval.DecisionNote`
- `Metadata.ApprovalDecision`
- `Metadata.ApprovalNote`
- `Metadata.ResumeCount`
- `Metadata.ResumedFrom`

This is a useful closure because approval is now better represented not only as
"pause happened / resume happened," but also as an auditable lifecycle with
review metadata.

### Validation

Validated on `2026-06-02`:

```powershell
go test ./internal/app/agent -count=1
```

Current focused result:

- `internal/app/agent` PASS

### Updated Conclusion

As of the end of `2026-06-02`, the most accurate additional `internal/app/agent`
approval/runtime status is:

1. approval now has a richer outward pending payload rather than only a thin
   interrupt projection
2. approval resume now has an explicit outward decision contract instead of
   relying only on a boolean convention
3. service-level approval failures are now described through a typed outward
   error contract with code/kind/retryable semantics
4. approval persistence cleanup is more correct because both checkpoint and
   session aliases are now removed on terminal paths
5. approval audit fields are now explicitly covered by tests, so the current
   service lifecycle is better defined end-to-end
6. the next best steps after this P0 closure are no longer inside approval P0
   itself, but instead move to:
   - bridging `RunOutcome + ApprovalPending + ServiceError` into outer chat /
     transport layers
   - broadening selector-driven capability usage
   - continuing post-P0 runtime/pattern expansion
