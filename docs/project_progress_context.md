# Project Progress Context

更新时间：2026-05-02

这份文档用于维护 `goagent` 当前项目进度，帮助后续继续开发时快速对齐：

- 当前阶段
- 已完成能力
- 今日新增进展
- 当前验证状态
- 当前已知问题与风险
- 下一步计划

## 开发约定

- 从 2026-04-30 开始，新增代码需要补充必要的中文注释。
- 在函数声明上，使用简短中文注释说明这个函数的功能。
- 在关键步骤或相对复杂的步骤上，使用简短中文注释说明这一步是在做什么。
- 注释以“必要、准确、易读”为原则，避免无信息量的逐行翻译式注释。

## 当前阶段

项目已经从“基础设施搭建期”进入“业务模块联调与质量收口期”。

当前可以分成三条主线来看：

### 1. Knowledge 主链路已进入稳定化阶段

- `knowledge` 已形成完整的 `domain / port / service / schedule / adapter / http` 结构
- 后台已能完成登录、知识库管理、文档上传、文档分块、chunk 管理、调度相关联调
- 当前重点已从“继续铺功能”转向：
  - 主链路一致性
  - 状态流转
  - 排障与日志
  - chunk 质量基线

### 2. RAG 一期最小 chat 闭环已打通

- `conversation / message / summary / feedback / trace` 已落地
- `RagChatService`、HTTP handler、runtime、trace 查询接口都已接通
- 当前重点不再是“能不能跑”，而是：
  - 稳定性
  - 观测
  - 多轮对话一致性
  - 后续增强能力的演进空间

### 3. Ingestion 已从“方向澄清”进入“模块落地期”

- 已明确 `ingestion` 是独立业务模块，而不是继续塞进 `knowledge`
- 已明确交互模型是：
  - `配置 pipeline`
  - `发起 task`
  - `查看 task / task_node 日志`
- 已完成第一轮模块骨架、落表、repository、最小执行链路和路由接入
- 当前重点已转为：
  - 接入 EINO 作为执行编排层
  - 把最小占位链路升级为真实可消费链路
  - 最终接回 `knowledge processMode = pipeline`

## 已完成能力

### 基础层

- `internal/infra-ai`
  - chat
  - embedding
  - rerank
  - provider 路由与候选选择
- `internal/app/core/parser`
  - Markdown parser
  - Tika parser
- `internal/app/core/chunk`
  - fixed size chunker
  - markdown chunker
  - chunk selector
- Web 基础设施
  - Gin
  - request id
  - global error handler
  - user context middleware
  - Viper 配置加载

### Knowledge 业务层

核心目录：

```text
internal/app/knowledge/
  domain/
  port/
  service/
  schedule/
```

当前已完成：

- `KnowledgeBaseService`
  - create / get / update / delete / page
  - chunk strategies 查询
  - embedding model 更新校验
- `KnowledgeDocumentService`
  - upload
  - get
  - page
  - search
  - update
  - enable
  - delete
  - start chunk
  - chunk log page
  - schedule exec page
  - 支持 `sourceType=file`
  - 支持 `sourceType=url`
- `KnowledgeChunkService`
  - page
  - create
  - update
  - delete
  - enable
  - batch toggle enabled
  - 支持 chunk/vector 同步
- `DocumentProcessService`
  - 文件读取
  - 文本解析
  - chunk 切分
  - embedding
  - chunk 持久化
  - vector 持久化
  - chunk log 写入
  - 文档状态流转
- `KnowledgeDocumentScheduleService`
  - schedule 同步
  - 按文档删除 schedule / exec
- `KnowledgeDocumentScheduleJob`
  - 扫描到期任务
  - 恢复 stuck running document

### Repository / Adapter

当前已完成：

- PostgreSQL repository
  - knowledge
  - rag
  - user
  - ingestion
- 条件更新 DSL
  - 已沉淀为公共 helper，供 `knowledge` 与 `rag` 仓储更新复用
- FileStorage
  - `internal/adapter/storage/s3`
  - `Upload / Open / Delete`
- VectorStore
  - `internal/adapter/vectorstore/pgvector`
  - chunk upsert / delete / batch delete
  - 已补充检索能力，供 `rag` 一期复用
- TaskQueue
  - `internal/adapter/taskqueue/rocketmq`
  - chunk document task
  - refresh remote document task
  - chunk document consumer

### RAG 一期能力

#### Domain / Repository

- 已落地 `conversation`
- 已落地 `conversation_message`
- 已落地 `conversation_summary`
- 已落地 `message_feedback`
- 已落地 `rag_trace_run`
- 已落地 `rag_trace_node`
- `rag` repo 更新操作已适配 `UpdateWhere` 风格

#### Core

- `core/rewrite`
  - 最小可用 query rewrite 抽象与默认实现
- `core/prompt`
  - 中文默认提示词
  - prompt template loader
  - prompt service
- `core/retrieve`
  - embedding + vector search + 可选 rerank 插槽
- `core/vector`
  - 面向未来扩展的向量抽象
- `core/memory`
  - `Store / SummaryService / Service` 三层抽象
  - `DefaultService`
  - `RepositoryStore + RepositorySummaryService`
  - `MessageServiceStore + SummaryServiceAdapter`

#### Service / HTTP

- `ConversationService`
  - 列会话
  - 创建或更新时间
  - 重命名
  - 删除会话并级联删消息/摘要
- `ConversationMessageService`
  - 新增消息
  - 查询消息列表
  - 写入摘要
  - 读取最新摘要
  - 聚合 assistant 消息 vote 信息
- `RagChatService`
  - 最小 chat 闭环
  - 多轮对话落同一会话
  - stop 能力
  - trace run / trace node 收口
- Trace 查询接口
  - `GET /rag/traces/runs`
  - `GET /rag/traces/runs/:traceId`
  - `GET /rag/traces/runs/:traceId/nodes`

### Ingestion 第一阶段已完成能力

核心目录：

```text
internal/app/ingestion/
  domain/
  port/
  service/
```

当前已完成：

- 目标与实施设计文档
  - `docs/ingestion_module_goal.md`
  - `docs/ingestion_execution_design.md`
- 领域模型
  - `Pipeline`
  - `PipelineNode`
  - `Task`
  - `TaskNode`
- PostgreSQL 持久化
  - `pipeline / task / task_node` 三张表
  - GORM model
  - repository 实现
- HTTP 接口骨架
  - pipeline CRUD
  - task 创建 / 分页 / 详情 / 节点日志
- Runtime 装配
  - 独立 ingestion runtime
  - 已接入主程序路由
- 执行层最小骨架
  - `ExecutionState`
  - `WorkflowBuilder`
  - `NodeRunnerRegistry`
  - `TaskObserver`
  - `ExecutorService`
- 最小顺序执行链路
  - `fetcher`
  - `parser`
  - `chunker`
  - `indexer`

## 今日新增进展

### 1. ingestion 模块从设计阶段推进到可运行骨架

- 新增 `docs/ingestion_execution_design.md`
- 明确 ingestion 的模块定位、用户交互、与 `knowledge / rag` 的关系
- 确认 EINO 适合作为执行编排层，而不是替代整个 ingestion 模块

### 2. ingestion 落表与 repository 已完成

- 新增 migration：
  - `t_ingestion_pipeline`
  - `t_ingestion_task`
  - `t_ingestion_task_node`
- 完成 ingestion 的 GORM model、JSON 映射与 PostgreSQL repository
- ingestion runtime 已接入这些 repository

### 3. ingestion 最小执行链路已打通

- 建立 `ExecutionState / WorkflowSpec / NodeRunner / TaskObserver` 骨架
- 完成四个节点的第一版占位实现：
  - `fetcher`
  - `parser`
  - `chunker`
  - `indexer`
- `ExecutorService` 已能按顺序执行 workflow 并回写 `task / task_node`

### 4. ingestion 已接入主程序后台路由

- 在 `cmd/server/main.go` 中完成 ingestion runtime 初始化
- ingestion 路由已注册到后台管理员路由组
- 当前可通过后台接口真实创建和执行最小 ingestion task

### 5. 已顺手修复若干 ingestion 基础问题

- `pipeline / task / task_node` 创建时改为生成真实 ID
- `task_node` 不再依赖拼接主键，而是使用独立主键与 `(task_id, node_id)` 查询更新
- 避免了“task 显示 running 但实际上未执行”的错误状态设计

## 当前验证状态

最近已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./...
```

本轮验证重点包括：

- `internal/app/ingestion/service`
- `internal/bootstrap/ingestion`
- `cmd/server`
- 以及全仓关键模块回归编译

当前人工确认的事实：

- ingestion 已能从主程序完成 runtime 装配
- ingestion 后台路由已可注册
- 最小 task 提交后会进入执行流程并回写状态
- 但 fetcher 仍是占位能力，当前要跑通最小链路，需要通过 metadata 或节点 settings 提供 inline content

## 当前已知问题与风险

### 1. ingestion 已可运行，但仍是“最小占位执行链路”

- `fetcher` 还不做真实远程读取
- `indexer` 还不写真实下游
- 当前链路更适合用于验证编排、状态流转和观测，而不是生产可用 ingestion

### 2. EINO 还未真正接入执行层

- 当前 `WorkflowBuilder + NodeRunner + ExecutorService` 是为 EINO 预留的本地骨架
- 还没有把 workflow 转成 EINO graph/workflow
- callback 还没有用 EINO 的标准机制来驱动 `task_node` 观测

### 3. `knowledge processMode = pipeline` 仍未接回 ingestion

- `knowledge document` 已允许配置 `processMode = pipeline`
- ingestion 已有独立模块和最小执行能力
- 但两者尚未真正闭环

### 4. chunk 质量仍停留在“工程可用”阶段

- `markdown` chunker 虽比固定大小更好，但离更接近语义级切分仍有差距
- 缺少真实问答样本下的召回和上下文完整性评测基线

### 5. 异步链路与资源稳定性仍需继续验证

- `knowledge` 的 MQ 路径仍依赖 RocketMQ 端到端稳定性
- ingestion 目前是单进程最小执行模式，后续如果演进到 MQ / worker，需要重新验证状态一致性与恢复机制

## 下一步计划

### P0：把 EINO 接入 ingestion 执行编排层

目标：

- 设计并落地 ingestion 的 EINO adapter
- 将 `WorkflowSpec` 转为 EINO workflow / graph
- 将 callback 事件接到 `TaskObserver / task_node`

### P0：把最小占位执行链路升级成真实可消费链路

目标：

- 让 `fetcher` 支持真实来源读取
- 让 `indexer` 接入真实下游目标
- 让最小 ingestion task 不再依赖 inline content 占位

### P1：把 pipeline 真正接回 knowledge

目标：

- 让 `knowledge document processMode = pipeline` 有真实执行入口
- 避免继续停留在“只保存配置”的状态

### P1：继续完善观测与排障

目标：

- 保持 ingestion task 从第一天起就具备最小排障能力
- 评估后续是否抽象通用 trace / task 观测模型

### P1：建立 chunk 质量评测样本

目标：

- 选取 Markdown 文档、说明文档、FAQ 文档做基准样本
- 评估不同 chunk 策略下的召回和上下文完整性

## 维护建议

后续每完成一类能力后，建议同步更新这份文档的几个部分：

1. `今日新增进展`
2. `当前验证状态`
3. `当前已知问题与风险`
4. `下一步计划`
