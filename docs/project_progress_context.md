# Project Progress Context

更新时间：2026-05-11

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
   已完成第一阶段基础设施：自研 tool 抽象、tool registry、tool executor、AgentLoop V1（支持并行执行）、LLMPlanner、LLMObserver（支持多 hint + think 隔离 + 解析失败日志），以及接入 `RagChatService` 的扩展点。工具集已从 8 个诊断工具扩展为 10 个（+ document_list / task_list / web_search / think）。当前处于“LLM 主决策 + diagnose 一步到位 + 规则 fallback/guardrail”的稳定 agent 化阶段。

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
- 已实现 tool（10 个）
  - 诊断类：`document_ingestion_diagnose`（含 live task node 补齐）、`task_ingestion_diagnose`、`trace_retrieval_diagnose`
  - 查询类：`document_query`、`document_chunk_log_query`、`ingestion_task_query`、`ingestion_task_node_query`、`trace_node_query`
  - 发现类：`document_list`（按 status/query 分页）、`task_list`（按 status/pipelineId 分页）
  - 外部类：`web_search`（DuckDuckGo API）
  - 元工具：`think`（推理记录，无副作用）
- 已实现能力
  - LLM planner（含 retrieve/rewrite 上下文注入）
  - LLM observer（主 observer，支持多 hint、think 隔离、taskNodeSummary 强制下钻）+ RuleObserver fallback（跳过 think）
  - AgentLoop V1（Plan -> Act -> Observe，支持并行执行 + 墙钟/累计耗时观测）
  - 接入 `RagChatService`
  - 诊断回答引导（深度证据升级 + 状态冲突归一）
  - baseRules 开放问题处理（"最近哪些文档失败了？" → `document_list`）
  - SSE `tool / tool_start / tool_result / agent_think` 事件
  - trace `agent_round / tool_call / agent_observation` 落库

## 最新进展

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

截至 2026-05-11，以下增量验证已通过（测试数 49 → 71）：

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

## 当前已知问题与风险

1. `tool / agent` 决策已进入 “LLMObserver + LLMPlanner + 规则兜底” 的混合版本  
   `LLMObserver` 已成为默认主 observer，`RuleObserver` 已退为 fallback/guardrail。`doc_fail_01` 的标准失败样例可稳定走通，且 `diagnose` 一步到位大幅减少了对 LLM Observer 下钻决策的依赖。当前剩余主要风险已从”基础交互是否可用”转为”LLM 决策质量是否足够稳定”，在 diagnose 覆盖不到的场景下仍需防范参数幻觉和重复调用。

2. 运行中场景已回答一致性收口  
   `ID` 误识别、同轮过深规划、状态冲突归一已完成首轮修复。`answer_guidance` 已支持当 diagnose 结论与 task/node 实际状态冲突时以 task/node 为准，并显式标注不一致。

3. 多通道检索目前仍是”无 intent 依赖”版本  
   已完成 `vector_global + keyword + metadata_title` 通道化重构，但还没有接入 `intent_directed`、metadata 字段扩展策略或更细的 route hint。

4. 工具集已扩展但仍缺少写操作和更多外部工具  
   已从 8 个扩展到 10 个（含 web_search），但仍无写操作工具（如创建文档）或更多外部系统对接。

5. `ingestion` 生产化仍未完全收口  
   已补上 reconcile 结果留痕与基础统计，但修复结果沉淀、异常统计、超时治理和更系统的恢复策略仍未完成。该项已转入”中期待办”。

6. trace 可观测性具备首轮闭环，但仍需继续产品化  
   round / observation extraData 中已补充 nextHintCalls、executionMode、toolCallCount 等字段，但列表摘要、聊天到 trace 联动、异常筛选仍需继续补齐。

7. AgentLoop V1 已从”纯诊断”扩展为”诊断 + 发现 + 搜索”，但仍是单 Agent  
   对模糊问题、多意图问题的处理有所提升（开放发现 + web 搜索），但还不是多 Agent 协作系统。

## 下一步计划

### P0

- 联调验证近期改动
  - 端到端验证 `doc_fail_01`：确认 diagnose 一步到位返回 indexer 错误
  - 端到端验证 `doc_run_01`：确认状态冲突归一后回答不再被浅层结论带偏
  - 验证开放问题："最近有哪些文档导入失败了？"

- 多通道检索优化
  - 基于 `retrieve-eval` 持续扩充真实 query 样本集
  - 评估是否接入 `intent_directed` 检索策略

### P1

- 继续扩展工具集
  - 评估是否需要更多外部工具（如 HTTP fetch、数据库查询等）
  - 评估是否需要写操作工具（需配合确认机制）

- trace 产品化
  - 列表摘要、聊天到 trace 联动、异常筛选

### P2

- ingestion 收口
  - pending/running 超时治理
  - reconcile 结果沉淀与更系统恢复策略

- Planner/Observer 合并探索
  - 减少 LLM 调用次数
