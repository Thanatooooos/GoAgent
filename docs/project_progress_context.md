# Project Progress Context

更新时间：2026-05-09

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
   已完成第一阶段基础设施：自研 tool 抽象、tool registry、tool executor、本地 workflow runner、LLMPlanner，以及接入 `RagChatService` 的扩展点。当前处于“LLM 决策 + 规则 fallback”的混合规划阶段。

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
  - `vector_global / keyword` 两路检索通道
  - 低置信度 fallback
  - SSE `meta` 下发 `searchMode`
  - prompt 支持单独注入 `ToolContext`

### Agent / Tool

- `internal/app/rag/tool`
  - `Tool`
  - `Definition / Call / Result`
  - `Registry`
  - `Executor`
  - `RenderContext`
  - `Workflow`
  - `Planner + PlanInput / PlanResult`
- 已实现 tool
  - `document_query`
  - `document_chunk_log_query`
  - `ingestion_task_query`
  - `ingestion_task_node_query`
  - `trace_node_query`
  - `document_ingestion_diagnose`
  - `task_ingestion_diagnose`
  - `trace_retrieval_diagnose`
- 已实现能力
  - LLM planner
  - 本地 workflow runner
  - 接入 `RagChatService`
  - 诊断回答引导
  - SSE `tool` 事件与 trace 落库

## 最新进展

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

截至 2026-05-09，以下增量验证已通过：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service ./internal/adapter/http/ingestion -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/adapter/http/ingestion ./internal/bootstrap/ingestion ./internal/app/knowledge/service -count=1
```

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS
- `internal/adapter/http/ingestion` PASS
- `internal/bootstrap/ingestion` 可正常编译

历史验证保持有效：

- `internal/app/rag/core/retrieve` PASS
- `internal/app/rag/service` PASS
- `internal/app/rag/tool/...` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）

## 当前已知问题与风险

1. `tool` 决策仍是本地规则 + LLM planner 双轨  
   LLMPlanner 已落地，但 planner 与 rule fallback 的边界仍需继续打磨。

2. 多通道检索目前仍是“无 intent 依赖”版本  
   已完成 `vector_global + keyword` 通道化重构，但还没有接入 `intent_directed`、标题检索或 metadata 定向检索。

3. 诊断能力已形成最小闭环，但准确率和可信度仍需继续提升  
   当前已具备 `document / task / trace` 三类诊断入口，但更多仍是基于状态和规则的高概率判断。

4. `RagChatEventSink` 接口扩展后需要持续关注实现一致性  
   已更新 `sseChatSink` 和 `fallbackSinkStub`，后续若新增实现需同步。

5. `ingestion` 生产化仍未完全收口  
   已补上 reconcile 结果留痕与基础统计，但修复结果沉淀、异常统计、超时治理和更系统的恢复策略仍未完成。  
   该项已转入“中期待办”，短期仅保留必要修复。

6. trace 可观测性具备首轮闭环，但仍需继续产品化  
   trace 详情页已经较完整，但列表摘要、聊天到 trace 联动、异常筛选还需继续补齐。

## 下一步计划

### P0

- 稳定多通道检索基础架构
  - 继续细化 `auto` 模式下的 channel 启停策略
  - 继续验证精确匹配类 query 的召回质量提升
  - 评估是否补充 `title / metadata` 等无 intent 依赖通道

- 提升诊断结果质量
  - 继续细化 `document / task / trace` 诊断证据提取
  - 继续让结论、事实、推断、风险提示、建议边界更清晰
  - 继续稳定置信度判断逻辑

- 完善 trace / tool / fallback 的消费闭环
  - 继续把 `rag_trace_run.extraData`、`rag_trace_node.extraData` 用于列表摘要、聊天/trace 联动和异常筛选
  - 进一步区分 `tool_workflow` 汇总节点与 `tool_call` 子节点的展示语义

### P1

- 完善 LLMPlanner
  - planner 识别真实 ID 的能力
  - planner 与 rule fallback 的可观察性对比

- 继续扩展 diagnose 闭环价值
  - 对常见失败模式给出更具体的排障建议
  - 逐步沉淀结构化错误归因

- 巩固 integration test 日常执行流程
  - 明确 PR 必跑范围
  - 固化 compose 场景

### P2

- 中期再回到 ingestion 收口
  - pending/running 超时治理
  - reconcile 结果沉淀与更系统恢复策略
  - 更完整的 `document / chunk_log / task` 不一致规则矩阵

- 评估 EINO 在 tool workflow 编排层的接入
  - 不改写 `RagChatService` 业务外壳
  - 只替换 tool workflow 的执行层
