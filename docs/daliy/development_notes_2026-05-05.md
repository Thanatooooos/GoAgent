# Development Notes - 2026-05-05

## 背景

今天的工作围绕两条主线展开：

1. **Ingestion 观测增强**：把此前仅停留在结构化日志层的执行观测，升级为可直接查询和可在后台展示的 metrics 能力。
2. **RAG 智能化增强**：让 rewrite 和 retrieve 两段链路真正联动起来，使系统可以根据问题类型自动选择更合适的检索策略，并把最终使用的策略透出给前端。

## 本次新增

### 1. Ingestion Metrics API 落地

新增能力：

- 新建 `internal/app/ingestion/service/metrics_service.go`
  - `MetricsService`：进程内实时聚合 ingestion 指标
  - `MetricsObserver`：通过 `TaskObserver` 链路采集 task / node 执行事件
  - `MetricsSnapshot`：统一返回运行指标快照
- 扩展 `TaskObserver`
  - 新增 `OnNodeRetry(...)` 事件
  - `MultiTaskObserver`、`RepositoryTaskObserver`、knowledge 侧 bridge observer 全部补齐实现
- `internal/app/ingestion/service/executor_service.go`
  - 新增 `Metrics *MetricsService` 装配项
  - 在 task 进入执行器时记录 `submitted`
  - 在节点重试前记录 retry 事件
  - 暴露 `MaxConcurrent()`，供 metrics 同步当前执行器并发配置
- `internal/bootstrap/ingestion/runtime.go`
  - 装配 `repository observer + metrics observer`
  - runtime 暴露 `Metrics` 服务
- `internal/adapter/http/ingestion/metrics_handler.go`
  - 新增 `GET /ingestion/metrics`
- `cmd/server/main.go`
  - ingestion 路由新增 metrics 注册

当前指标覆盖：

- task 维度
  - `submitted`
  - `started`
  - `succeeded`
  - `failed`
  - `canceled`
- 并发维度
  - `runningTasks`
  - `maxConcurrent`
  - `usedSlots`
- node 维度
  - `runs`
  - `successes`
  - `failures`
  - `retries`
  - `avgDurationMs`
  - `maxDurationMs`

### 2. Ingestion 管理页接入 Metrics 面板

前端新增：

- `frontend/src/services/ingestionService.ts`
  - 新增 `IngestionMetricsSnapshot`
  - 新增 `IngestionNodeMetrics`
  - 新增 `getIngestionMetrics()`
- `frontend/src/pages/admin/ingestion/IngestionPage.tsx`
  - 任务 tab 顶部新增运行指标总览卡片
  - 新增节点指标表
  - 支持手动刷新
  - 支持 `10s` 自动轮询

页面现可直接展示：

- 运行中任务数
- 并发占用 / 最大并发
- 任务成功率
- 失败 / 取消数
- 累计重试次数
- 各节点类型的运行次数、成功次数、失败次数、重试次数、平均耗时、最大耗时

### 3. RAG Rewrite 增加检索偏好推断

增强文件：

- `internal/app/rag/core/rewrite/rewrite.go`
  - `Result` 新增 `PreferredSearchMode`
  - 新增 `InferSearchModePreference(question)` 本地推断逻辑
- `internal/app/rag/core/rewrite/llm_rewrite_service.go`
  - rewrite prompt 扩展为同时输出：
    - `rewritten`
    - `sub_questions`
    - `preferred_search_mode`
  - 当 LLM 未返回该字段时，回退到本地推断
  - fallback 结果也会自动补齐 `PreferredSearchMode`

当前启发式规则：

- `semantic`
  - 概念解释、原理、区别、总结类问题
- `keyword`
  - 名称、标题、包含、精确短语匹配类问题
- `hybrid`
  - 代码、报错、配置、参数、接口、路径、术语定位类问题

### 4. RAG Retrieve 增加 `auto` 决策并接回 Chat 主链

增强文件：

- `internal/app/rag/core/retrieve/service.go`
  - 新增 `SearchModeAuto = "auto"`
  - 新增 `resolveSearchMode()` 与 `inferSearchModeFromQuery()`
  - 未显式传入或传入非法模式时，自动根据 query 选择 `semantic / keyword / hybrid`
- `internal/app/rag/service/rag_chat_service.go`
  - `runRetrieveStage()` 新增 `resolveRetrieveSearchMode()`
  - chat 主链现在优先使用：
    1. 用户显式指定的 `searchMode`
    2. rewrite 产出的 `PreferredSearchMode`
    3. 最终回退到 `auto`
  - trace node 额外记录本次实际使用的 `searchMode`

效果：

- 前端即使不传 `searchMode`，系统也不再默认总走纯 semantic
- rewrite 和 retrieve 不再割裂，而是形成“改写结果驱动检索模式”的闭环

### 5. Chat 前端展示本次检索策略

增强文件：

- `internal/app/rag/service/rag_chat_service.go`
  - `RagChatMeta` 新增 `searchMode`
  - retrieve 阶段会把本次实际使用的模式写入 SSE `meta`
- `frontend/src/types/index.ts`
  - `StreamMetaPayload` 新增 `searchMode`
  - `Message` 新增 `retrievalMode / retrievalModeLabel`
- `frontend/src/stores/chatStore.ts`
  - 在 `onMeta` 中把 `searchMode` 挂到当前 assistant 消息
- `frontend/src/components/chat/MessageItem.tsx`
  - assistant 消息顶部新增“检索策略：语义检索 / 关键词检索 / 混合检索”标签

## 当前验证状态

后端测试通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/ingestion/service ./internal/adapter/http/ingestion ./cmd/server -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/core/rewrite ./internal/app/rag/core/retrieve ./internal/app/rag/service ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/service ./internal/adapter/http/rag ./cmd/server -count=1
```

新增测试点：

- `internal/app/ingestion/service/metrics_service_test.go`
- `internal/adapter/http/ingestion/metrics_handler_test.go`
- `internal/app/rag/core/rewrite/llm_rewrite_service_test.go`
- `internal/app/rag/core/rewrite/rewrite_test.go`
- `internal/app/rag/core/retrieve/service_test.go`
- `internal/app/rag/service/rag_chat_service_test.go`

前端验证状态：

- `frontend` 已执行：
  - `prettier --write`
  - `tsc --noEmit`
- `vite build` 未能在当前环境完成，错误为 `esbuild spawn EPERM`
  - 判断为当前环境的 Node / 子进程权限限制
  - 不是本次 TS 类型或语法错误导致

## 后续建议

1. **P1**：前端接入 `fallback` SSE 事件，补齐知识库未命中告警横幅
2. **P1**：在 trace 详情页展示本次 `searchMode`、rewrite 结果和 sub-questions
3. **P1**：继续增强 retrieval 自动决策，例如结合反馈数据调优模式选择
4. **P1**：为 ingestion metrics 增加历史趋势、持久化与多实例聚合能力
