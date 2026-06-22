# Project Progress Context

这份文档只用于描述 `goagent` 的当前状态，不再记录按日期追加的增量更新。

## 1. 进度

项目已经从“基础能力搭建期”进入“主链路闭环、联调和质量收口期”。

当前整体进度可以概括为：

- `Knowledge` 已具备完整的知识库、文档、chunk、调度和管理能力，重点从继续铺功能转向一致性、状态联动和排障体验。
- `Ingestion` 已跑通 `pipeline -> task -> task_node -> knowledge 回写` 的最小可用闭环，并补齐了重试、补偿、reconcile 和基础 metrics。该模块目前以稳定性维护为主，不再是短期主工作面。
- `RAG` 已形成最小 chat 闭环，支持多轮对话、rewrite、retrieve、prompt、trace、fallback，以及多通道检索、结构化 summary 和记忆压缩、离线评估能力。
- `Agent / Tool` 已形成两条并行路线：
  - 旧 `internal/app/rag/tool` 继续作为稳定生产路径。
  - 新 `internal/app/agent` 已完成 capability、planner、handoff、runtime approval / resume、plan_execute 泛化和 mixed-capability 基础闭环。
- approval 已不再只是后端 runtime 能力，待审批状态已经可以恢复到 chat UI，支持会话级 pending lookup、SSE 事件处理和前端审批卡片展示。

当前项目状态的核心判断是：

- 主链路已经成型，重点不再是“功能有没有”，而是“边界是否稳定、结果是否一致、体验是否完整”。
- 后端重点集中在 `agent / tool / rag` 的协作边界、可解释性和可恢复性。
- `summary` 已进入专项质量收口阶段，当前已经具备结构化 schema、repair、validation、renderer 和离线评估样本体系，工作重点是继续提升评测通过率并压低 dangerous drift。
- 前端已经具备承接 Agent 运行态和审批态的基础能力，但仍以现有链路承接为主，而不是大规模扩展新交互。

## 2. 结构

项目当前结构分为后端、前端、命令行工具和文档四个层面。

### 仓库顶层

- `cmd`
  - 可执行入口与评估/调试工具，当前包含 `server`、`retrieve-eval`、`retrieve-inspect`、`rewrite-eval`、`eval-sample-gen`、`corpus-loader`、`lexical-rebuild`。
- `configs`
  - 运行配置。
- `docker`
  - 容器化相关文件。
- `docs`
  - 项目说明、设计和上下文文档。
- `frontend`
  - 前端应用。
- `internal`
  - 后端核心代码。
- `scripts`
  - 辅助脚本。
- `testdata`
  - 测试数据。

### 后端结构

`internal` 当前按基础设施、应用层、启动装配和接口层组织：

- `internal/adapter`
  - HTTP 等适配层，对外暴露接口。
- `internal/app`
  - 业务应用层，是当前核心模块所在位置。
- `internal/bootstrap`
  - Runtime / service 装配与启动入口。
- `internal/framework`
  - 配置、通用框架能力。
- `internal/infra-ai`
  - 模型调用、embedding、rerank、provider 路由等 AI 基础设施。
- `internal/infra-mcp`
  - MCP 管理与工具调用底座。
- `internal/middleware`
  - Web 中间件。

`internal/app` 当前按业务域拆分为：

- `agent`
  - 新一代 Agent runtime、capability、pattern、approval、resume、plan_execute。
- `core`
  - parser、chunk 等通用核心能力。
- `ingestion`
  - pipeline / task / task_node 及其执行链路。
- `knowledge`
  - knowledge base、document、chunk 管理能力。
- `rag`
  - rewrite、retrieve、prompt、conversation、trace、旧 tool 体系。
- `user`
  - 用户相关领域能力。

其中需要特别注意的两条主线是：

- `internal/app/rag/tool`
  - 当前稳定生产路径，承担诊断、查询、搜索、外部证据整合等 Tool 能力。
- `internal/app/agent`
  - 新 runtime 路线，正在承接 capability 化、审批恢复、pattern 扩展和更通用的 Agent 编排能力。

### 前端结构

`frontend/src` 当前按界面组件、状态管理和服务层拆分：

- `components`
  - 通用 UI 组件与聊天相关组件。
- `hooks`
  - 流式响应、交互等 hooks。
- `pages`
  - 页面入口。
- `services`
  - 前端 API 调用封装。
- `stores`
  - 会话、消息、审批状态等前端状态管理。
- `types`
  - 前端类型定义。
- `utils` / `lib` / `styles`
  - 工具函数、基础库和样式资源。

当前前端已经能够承接聊天消息流、Tool 事件，以及 approval pending 的恢复与展示。

## 3. 已支持的功能

### 基础设施

- `infra-ai`
  - chat
  - embedding
  - rerank
  - provider 路由与候选选择
  - JSONMode `response_format` 支持
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
  - 幂等迁移执行
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
  - chunk / vector 同步
- `DocumentProcessService`
  - 文件读取
  - 文本解析
  - chunk 切分
  - embedding
  - chunk / vector 持久化
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
- 生产化补强
  - 节点重试与指数退避
  - `Indexer` 失败补偿清理
  - `task_node` 重试信息持久化
  - `document` 级活动 task 保护
  - task-scoped chunk log 回写保护
  - 即时 reconcile 与后台 reconcile scan
  - reconcile 结果接入 ingestion metrics

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
- RAG 能力
  - 多轮对话
  - LLM rewrite
  - memory compression
  - structured summary schema / repair / validation / renderer
  - `semantic / keyword / hybrid / auto` 检索模式
  - 多通道检索架构
  - `vector_global / keyword / metadata_title` 三路检索通道
  - 低置信度 fallback
  - SSE `meta` 下发 `searchMode`
  - prompt 注入 `ToolContext`
- 检索评估基础设施
  - `internal/app/rag/evaluation`
  - `cmd/retrieve-eval`
  - `Hit@K / Recall@K / MRR`
  - 离线样本评估与真实 retrieve 回放执行
  - `summary` 离线评估样本、规则校验、field judge 与 downstream equivalence 判定

### Tool

- `internal/app/rag/tool`
  - `Tool`
  - `Definition / Call / Result`
  - `Registry`
  - `Executor`
  - `RenderContext`
  - `Workflow`
  - `Planner + PlanInput / PlanResult`
- 当前已实现工具能力
  - 诊断类
    - `document_ingestion_diagnose`
    - `task_ingestion_diagnose`
    - `trace_retrieval_diagnose`
  - 查询类
    - `document_query`
    - `document_chunk_log_query`
    - `ingestion_task_query`
    - `ingestion_task_node_query`
    - `trace_node_query`
  - 发现类
    - `document_list`
    - `task_list`
  - 外部类
    - `web_search`
    - `web_fetch`
  - 元工具
    - `think`
  - Graph
    - `document_root_cause_diagnosis`
    - `document_diagnose_with_search`
    - `external_evidence_workflow`
- 搜索与外部证据能力
  - `web_search` 支持 DuckDuckGo / Tavily / Tavily MCP
  - 支持 MCP 主路与 API fallback
  - `web_fetch` 支持网页正文提取与并发抓取
  - 支持外部证据工作流整合

### Agent Runtime

- 新 Agent 主链路
  - capability registry
  - planner
  - handoff
  - runtime approval / resume
  - pending approval lookup
  - plan_execute pattern
- 运行时能力
  - `Plan -> Act -> Observe` 循环
  - LLM planner
  - LLM observer
  - RuleObserver fallback / guardrail
  - 并行 tool calls
  - trace 落库
  - 结构化 hint / next step 驱动
- plan_execute 泛化能力
  - 通用 `PlanStep / PlanStepResult`
  - step artifacts
  - completion / failure policy
  - mixed-capability synthesis
  - retry / optional / replan 执行语义

### MCP

- `internal/infra-mcp`
  - stdio `Manager`
  - 懒启动 session
  - `ListTools / CallTool / Close`
  - runtime 生命周期回收
- 当前接入
  - Tavily MCP
  - `web_search` 通过 MCP 与 fallback provider 协同工作

### Chat UI / Frontend Integration

- 聊天基础能力
  - 消息流式响应
  - Tool 事件展示
  - Agent 结果事件处理
- approval 相关能力
  - conversation 级 pending approval 恢复
  - `GET /rag/v3/chat/approval/pending`
  - 前端会话切换时恢复待审批状态
  - SSE 处理 `approval_pending` / `agent_outcome` / `agent_service_error`
  - 审批卡片展示当前步骤、问题、查询、候选 URL、风险等级
- 前端状态管理
  - `chatStore.ts` + `chatStateModel.ts` 承接消息映射、会话合并、审批态合并

## 当前结论

`goagent` 当前已经具备以下项目形态：

- 一个可运行的知识库与文档处理系统
- 一个具备最小闭环的 ingestion 执行系统
- 一个带多通道检索、trace 和评估基础设施的 RAG 系统
- 一套仍可稳定服务生产路径的 `rag/tool` 工具体系
- 一套正在承接通用 Agent 编排、审批恢复和 mixed-capability 扩展的 `agent runtime`
- 一个已经能够承接聊天、Tool 事件和审批恢复展示的前端界面

因此，这个项目当前最准确的定位不是“能力搭建中”，而是“主链路已经形成，正在围绕稳定性、边界、体验和扩展性持续收口”。
