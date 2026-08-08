# GoAgent 项目当前快照

> 本文档描述当前工作区的项目状态，不记录按日期追加的变更日志。

## 项目定位

GoAgent 是一个 Go 实现的企业知识库与智能体平台：覆盖文档入库、RAG 对话、多模型调用、工具/Agent 编排、会话记忆、权限鉴别和运营级可观测性基础能力。

当前阶段是“主链路已形成、持续补齐可靠性与质量”的状态。系统可以运行并完成主要业务闭环；部分高级能力仍以配置开关、后端接口或定向测试验证为主，尚未全部形成产品化 UI。

## 代码结构

- `cmd/`：服务端和离线评估/诊断命令入口。
- `configs/`：应用与模型配置。
- `frontend/`：React 前端，包含聊天、知识库、审批和日报等页面。
- `internal/adapter/`：HTTP、PostgreSQL、对象存储与 pgvector 适配器。
- `internal/app/`：业务领域。
  - `agent/`：新 Agent runtime、capability、审批、恢复和 pattern。
  - `core/`：通用 parser、chunk 能力。
  - `ingestion/`：可编排文档入库流水线及耐久任务队列。
  - `knowledge/`：知识库、文档、chunk 与调度。
  - `rag/`：会话、改写、检索、提示词、记忆、Trace 和稳定 Tool 体系。
  - `dailybrief/`：日报及趋势洞察能力。
- `internal/bootstrap/`：各领域运行时组装。
- `internal/infra-ai/`：chat、embedding、rerank、多模型选择与熔断降级。
- `internal/middleware/`：请求 ID、日志上下文、访问日志、错误处理和用户上下文。

## 已落地的核心能力

### 文档与知识库

- 知识库、文档、chunk 的 CRUD、分页、启停、删除及处理日志。
- 文件、URL、飞书来源；支持直接切块或转入 ingestion pipeline。
- Markdown/Tika 解析、固定长度与 Markdown 结构化切块。
- pgvector 正文向量、关键词/BM25 与元数据检索。
- 文档处理状态、chunk 日志、定时调度与调度执行记录。

### 文档增强入库

入库链路支持 `fetcher -> parser -> enhancer -> chunker -> enricher -> indexer` 节点编排。

- 父子切块：父块仅持久化，子块负责向量检索；子块保留 `parent_chunk_id`。
- LLM 摘要和问题预测：运行时使用 `aiRuntime.Chat`；每个子块可生成预测问题。
- 问题独立向量化，携带 `source_chunk_id` 与源子块内容；命中问题后回链原子子块。
- 检索可返回父块上下文，但引用 ID 保持子块 ID，兼顾上下文完整性与引用精度。
- 摘要及状态写入文档的 `summary`、`summary_status`、`summary_error_message`。
- LLM 调用或摘要回写失败采用降级策略：正文子块和正文向量仍继续写入，不阻断文档可检索性。
- 新增迁移 `20260711100000_add_document_enrichment.sql`；旧数据默认按普通 `child` chunk 兼容。

### Ingestion 与可靠性

- Pipeline、Task、TaskNode 的 PostgreSQL 持久化与节点执行日志。
- 节点重试、退避、失败补偿清理、reconcile 与基础 metrics。
- Redis/Asynq 耐久队列：任务先落库再入队，进程重启时恢复待执行任务。
- 语义为至少一次投递；任务和索引层具备幂等/清理逻辑，避免服务宕机导致已持久化任务静默丢失。

### RAG 与会话

- 多轮会话、消息、反馈、会话摘要和短期滑动窗口。
- LLM 问题重写、术语归一化、子问题拆分、规则/原问题兜底。
- `semantic / keyword / hybrid / auto` 检索模式；向量、关键词和元数据标题多通道融合。
- Rerank、低置信度降级、Token 预算与上下文压缩。
- 长期记忆包含事实/偏好/会话片段召回。
- PostgreSQL RAG Trace Run/Node：记录检索、聊天、Agent round、工具调用和观测节点；Trace 查询与诊断工具已存在。

### Agent、Tool 与外部证据

- 稳定生产路径：`internal/app/rag/tool`，含注册、规划、执行、结果消费与工具上下文渲染。
- 工具覆盖文档/任务/Trace 查询诊断、网页搜索、网页抓取、外部证据整合和 MCP 调用。
- 新 Agent runtime：capability registry、scheduler、`Plan -> Act -> Observe`、并行调用、reactive 与 plan_execute pattern。
- 支持审批、暂停恢复、前置条件、风险等级、幂等性和 mixed-capability 执行。

### AI 调用与高可用

- 统一 chat、embedding、rerank 服务抽象。
- 多候选模型选择、优先级降级、三态熔断、流式首包探测。
- 模型异常不会直接中断可降级场景；入库增强同样遵循“增强失败不影响正文索引”。

### 身份、日志与可观测性

- 当前认证是不透明 UUID Token + PostgreSQL 服务端 Session；前端通过 `Authorization` 发送 Token，Cookie 仅保留兼容读取路径。
- 用户上下文中间件解析身份；路由通过 `RequireLogin`、`RequireRole` 区分公开、登录和管理员接口。
- 请求级结构化访问日志：request ID、方法、路由模板、状态码、耗时、响应大小、客户端 IP，以及已认证用户上下文。
- 访问日志包裹错误处理中间件，可记录最终 HTTP 状态；日志不记录请求体、Cookie、Authorization 或 query 参数。
- Ingestion workflow 日志带 task/pipeline 上下文；RAG/Agent 有持久化 Trace，异步任务 Trace 传播仍可作为后续增强项。

### 前端与日报

- 聊天流式响应、Tool 事件、Agent 结果、审批 pending 恢复与审批卡片展示。
- 知识库、文档、入库任务和流水线管理界面。
- 日报（Daily Brief）及趋势洞察相关模块已在当前工作区中，包含后端领域、HTTP、前端页面与数据迁移。

## 数据与基础设施

- PostgreSQL：业务数据、会话、Trace、入库任务、知识块、摘要状态等。
- pgvector / pg_search：向量、BM25/关键词和元数据检索。
- RustFS/S3：文档对象存储。
- Redis：Asynq 耐久队列及相关运行时能力。
- Tika：文档解析服务。

## 验证状态

文档增强相关定向测试已覆盖并通过：

```text
go test ./internal/app/core/chunk/test \
  ./internal/app/ingestion/service/runner \
  ./internal/app/rag/core/retrieve \
  ./internal/adapter/repository/postgres/knowledge \
  ./internal/bootstrap/ingestion -count=1
```

当前工作区包含日报、Agent、改写评估、耐久队列、请求日志和文档增强等多组未提交改动。全仓 `go test ./...` 仍可能受到根目录临时命令文件和 `scripts` 中多个 `main` 包影响；这不是文档增强定向测试的失败。

## 当前边界与后续重点

- 父子切块、问题向量与摘要能力已接入后端主链路；尚未提供完整的切块预览、问题/摘要管理和开关配置 UI。
- 预测问题向量为保证检索回链携带源子块/父块上下文，会增加向量 metadata 体积；后续可改为批量仓储回查以降低冗余。
- 文档增强与检索已完成定向测试；仍建议在真实 PostgreSQL、Redis、对象存储和模型服务环境执行端到端入库验证。
- Agent 新 runtime 与稳定 `rag/tool` 路线并存，后续重点是统一入口、Trace 贯通和产品层收口。
- 工作区处于持续开发状态；提交前应按功能切分并运行对应定向测试，避免把无关变更混入同一提交。
