# Development Notes - 2026-05-08

## 补充更新（本轮）

### A. 前端补齐了 `fallback` SSE 事件消费

- `frontend/src/hooks/useStreamResponse.ts`
  - 新增 `fallback` event 分支与 `onFallback`
- `frontend/src/types/index.ts`
  - `Message` 新增 `fallbackReason`
  - 新增 `FallbackPayload`
- `frontend/src/stores/chatStore.ts`
  - 在流式会话中把 `fallback` 事件绑定到当前 assistant 消息
- `frontend/src/components/chat/MessageItem.tsx`
  - 聊天消息中新增琥珀色提示条，明确提示“已回退到通用模型，需要注意核验”

### B. 固化了 integration test 的统一入口与 CI 骨架

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
- 调整 integration test 建表路径
  - `knowledge_document_pipeline_integration_test.go`
  - `knowledge_document_pipeline_failure_integration_test.go`
  - 把 `AutoMigrate` 改成 `postgresrepo.RunMigrations(db)`，让测试 schema 路径与 runtime 更一致

### C. 收口了一轮 ingestion 生产化一致性问题

- `KnowledgeDocumentService.OnIngestionTaskCompleted(...)`
  - 不再静默吞掉 `document / chunk_log` 回写错误
  - 改为返回聚合错误，便于上层日志和后续对账发现问题
- `finishPipelineChunkLogWithRecord(...)`
  - 优先按 `taskId` 精确命中 chunk log
  - 仅在按 task 未命中时才回退到 document 最新记录
  - 新增 task/document 不匹配保护，避免错误覆盖其他任务的 chunk log
- `MultiTaskObserver`
  - 从“某个 observer 失败就短路”调整为“所有 observer 尽量执行完，再聚合错误返回”
- `ExecutorService`
  - task completion observer 失败时显式记录 error log
  - 修正成功日志中的 chunk 统计为 `current.Chunks`
- 新增测试
  - `internal/app/ingestion/service/multi_task_observer_test.go`
  - `internal/app/knowledge/service/knowledge_document_service_test.go` 中补齐 task-scoped chunk log 回写与 mismatch 场景

### D. 本轮补充验证

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service -count=1
```

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS

### E. 下一步建议补充

- 设计 `document / chunk_log / task` 不一致状态的对账规则
- 评估是否落地后台 reconcile loop，定时自动修复知识侧回写异常

### F. 落地了 ingestion 对账规则与后台 reconcile 入口

- `internal/app/knowledge/service/knowledge_document_ingestion_reconcile.go`
  - 新增 `ReconcileIngestionTaskCompletion(...)`
  - 以 ingestion task 终态为准，对 `document / chunk_log` 做自动对账修复
  - 新增 task metadata 中 `documentId` 与输入 `documentId` 的 mismatch 保护
  - 支持在 `chunk_log` 缺失时按 `taskId` 自动补建最小可用记录
- `KnowledgeDocumentService.OnIngestionTaskCompleted(...)`
  - 在原有“即时回写”之后追加 reconcile，形成“完成回调修复 + 兜底修复”的双层机制
- `KnowledgeDocumentService.ScanAndReconcileIngestionTasks(...)`
  - 新增按文档分页扫描的 reconcile 入口
  - 当前只处理 `processMode=pipeline` 文档
  - 基于最新 `chunk_log.taskId` 反查 ingestion task 并执行修复
- `internal/bootstrap/knowledge/runtime.go`
  - 将 ingestion reconcile scan 挂入现有 knowledge schedule loop，复用同一套后台 ticker 与生命周期
- 新增测试
  - 覆盖 task 终态与 `document/chunk_log` 状态漂移时的自动修复
  - 覆盖 `chunk_log` 缺失时的自动补建
  - 覆盖 scan 入口按最新 `chunk_log.taskId` 触发 reconcile 的场景

### G. 补齐了 trace / tool / fallback 可观测性展示链路

- 后端 trace 数据补齐
  - `internal/app/rag/service/chat_tracer.go`
    - 新增 trace run `extraData` 追加能力
  - `internal/app/rag/service/rag_chat_service.go`
    - 在低置信度 fallback 触发时写入 `trace run.extraData.fallback`
    - 额外落一条 `fallback` trace node
  - `internal/adapter/http/rag/trace_handlers.go`
    - trace 详情接口开始透出 `rag_trace_run.extraData`
- 前端 trace 详情页增强
  - `frontend/src/pages/admin/traces/RagTraceDetailPage.tsx`
    - 新增 `Retrieve Observability` 面板
    - 展示 `searchMode / chunkCount / topScore / searchChannels / channelStats / searchDecisions`
    - 新增 `Tool Workflow` 面板，展示 `used / toolCallCount / toolNames / degraded / degradeReason`
    - 展示每次 `tool_call` 的状态、耗时、summary、error
    - 顶部新增 `Fallback` 风险提示
  - `frontend/src/services/ragTraceService.ts`
    - 补齐 `run.extraData` 与 `node.extraData` 类型字段
- 最终效果
  - trace 详情页从“仅时间线”升级为“检索 + 工具 + fallback”三段式可解释观察面板

### H. 本轮补充验证（追加）

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/knowledge/service ./internal/app/rag/service ./internal/adapter/http/rag -count=1
```

- `internal/app/knowledge/service` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）

### I. 当前判断更新

- `ingestion`
  - 已从“有回写保护”推进到“有即时 reconcile + 后台 reconcile scan”
  - 但修复结果沉淀、异常统计和更系统的恢复策略仍未完全做完
- `trace / tool / fallback`
  - 已从“后端有元数据”推进到“前端详情页可直接消费并解释”
  - 后续仍可继续补列表摘要、聊天到 trace 的联动与异常筛选

## 背景

今天的工作聚焦在 `RAG` 主链路里的检索层重构，目标不是直接引入 `intent`，而是先把当前 `semantic / keyword / hybrid` 的模式分支，升级成一个无 `intent` 依赖、但可为后续 `intent_directed channel` 留口子的多通道检索基础架构。

这次的原则很明确：

1. 先不引入 `intent` 领域模型和路由逻辑
2. 先把 retrieve 层做成 `channel + processor + context`
3. 先落地最有价值、现有能力最成熟的两条通道：`vector_global` 和 `keyword`
4. 先补齐 trace 元数据，让后续质量评估和排障更可解释

## 本次新增

### 1. 重构了 retrieve 内核

原先 `internal/app/rag/core/retrieve/service.go` 中的检索逻辑是分支式实现：

- `semantic`
- `keyword`
- `hybrid`

今天把它重构为多通道检索基础架构，新增文件：

```text
internal/app/rag/core/retrieve/
├── channels.go
├── post_processors.go
├── search_types.go
└── service.go
```

新增的核心抽象包括：

- `SearchContext`
- `SearchChannel`
- `SearchChannelResult`
- `SearchResultPostProcessor`
- `SearchProcessInput`
- `ChannelStat`

这样当前检索链已经从“写死的模式分支”变成了“通道执行 + 后处理链”的结构。

### 2. 落地了两条无 intent 依赖的检索通道

#### `vector_global`

职责：

- 执行全局向量语义检索
- 在 `semantic` 和 `hybrid` 模式下启用

实现要点：

- 复用现有 embedding 能力
- 复用现有 `corevector.Searcher.Search(...)`
- 默认把通道内部 `TopK` 放大到 `2 * topK`，给后续融合与去重留余量

#### `keyword`

职责：

- 执行关键词检索
- 在 `keyword` 和 `hybrid` 模式下启用

实现要点：

- 复用现有 `corevector.Searcher.SearchByKeyword(...)`
- 与 `vector_global` 保持对称的 `TopK` 放大策略

### 3. 新增了检索后处理链

为了避免继续把“融合 / 去重 / 重排”写死在 `hybrid` 分支里，这次把后处理拆成独立 processor：

#### `fusion`

- 多通道场景下，使用 RRF 方式对结果做统一融合
- 单通道场景下，直接透传该通道结果

#### `dedup`

- 以 chunk ID 为主键去重
- 保留高分版本
- 最终按 score 降序排序，并裁剪到 `topK`

#### `rerank`

- 如果配置了 rerank 服务，则在融合去重后统一重排
- 无 rerank 或 rerank 失败时自动降级，不中断主链

### 4. 保留了现有对外检索模式语义

虽然内部已经通道化，但对外仍兼容原有模式：

- `semantic` → 只开 `vector_global`
- `keyword` → 只开 `keyword`
- `hybrid` → 同时开 `vector_global + keyword`
- `auto` → 仍先根据 query 特征推断模式，再映射到具体通道组合

同时修正了一处兼容性细节：

- 默认兜底模式保持保守，回到 `semantic`
- 避免普通自然语言问句被无脑升级到双通道，导致噪音上升

### 5. 给 retrieve 结果补齐了多通道元数据

`retrieve.Result` 新增字段：

- `SearchChannels`
- `ChannelStats`

其中 `ChannelStats` 当前包含：

- `name`
- `chunkCount`
- `latencyMs`
- `error`
- `metadata`

这一步的意义是：

1. 当前就能增强可解释性
2. 后续如果加 `intent_directed channel`，可以直接把 `intentCode / intentScore / routeReason` 放进 `metadata`
3. 不需要再次重构 `retrieve.Result`

### 6. 调整了 `RagChatService` 中多子问题检索结果的聚合逻辑

之前 `runRetrieveStage()` 的行为是：

- 对每个子问题单独检索
- 把所有 chunk 拉平
- 仅按 chunk ID 去重

这会丢失每个子问题命中的检索通道元信息。

今天改成：

- 保留每个子问题的完整 `retrieve.Result`
- 调用 `ragretrieve.MergeResults(...)` 做统一聚合

聚合内容包括：

- chunks
- `SearchChannels`
- `ChannelStats`
- `KnowledgeContext`

这样后续接入更多 channel 或 route hint 时，不会丢子问题级别的检索证据。

### 7. retrieve trace 已可记录通道级信息

`RagChatService.runRetrieveStage()` 写入的 retrieve trace `extraData` 现在除了原有：

- `chunkCount`
- `searchMode`
- `topScore`

还新增：

- `searchChannels`
- `channelStats`

当前可以支持后续展示：

- 本次实际启用了哪些检索通道
- 每个通道返回了多少 chunk
- 每个通道耗时多少
- 某一路失败但整体是否成功降级

### 8. 补齐和更新了测试

更新文件：

- `internal/app/rag/core/retrieve/service_test.go`

新增覆盖点：

- `semantic` 模式下的 channel 输出
- `keyword` 模式下的 channel 输出
- `hybrid` 模式下的双通道输出
- `hybrid` 一路失败、另一路成功的降级场景
- `MergeResults` 对 `SearchChannels / ChannelStats` 的聚合

同时验证：

- `internal/app/rag/service` 相关测试保持通过

### 9. 给 `retrieve auto` 决策补了样本回放工具和规则校准闭环

为了让 `auto` 模式不只停留在“规则看起来合理”，今天又补了一轮可回放、可校准的验证基础设施。

新增内容：

- 新增 `internal/app/rag/core/retrieve/search_mode_decision.go`
  - 把 `auto` 模式决策统一收敛为 `AnalyzeSearchMode(...)`
  - 输出：
    - `RequestedMode`
    - `ResolvedMode`
    - `Source`
    - `Reason`
    - `Signals`
- 新增命令行回放工具：
  - `cmd/retrieve-debug`
  - 支持：
    - 从 JSON 样本文件批量读取 query
    - 文本模式输出 `query -> mode -> reason -> signals`
    - `-json` 输出结构化结果
    - 带 `expectedMode` 时输出 `PASS / FAIL`
- 新增样本集：
  - `testdata/retrieve_search_mode_samples.json`
  - 覆盖：
    - 概念问答
    - 精确匹配
    - `document / task / trace` 资源 ID 查询
    - 报错排障
    - 代码符号定位
    - SSE / fallback 事件定位

在这轮回放里，工具帮助发现并修正了几类误判：

- 代码符号说明类 query
  - 新增 `code_symbol_shape`
- 资源 ID 查询类 query
  - 新增 `resource_id_lookup`
- SSE / fallback 事件定位类 query
  - 新增并收窄 `event_or_protocol_locator`

最终样本回放结果：

- `18 / 18 PASS`

验证命令：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go run ./cmd/retrieve-debug -input testdata\retrieve_search_mode_samples.json
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go run ./cmd/retrieve-debug -input testdata\retrieve_search_mode_samples.json -json
```

### 10. 补了一轮 diagnose 结构化增强

今天还继续收口了 `diagnose` 模块，让它从“能给出结论”进一步走向“能区分事实、推断和风险提示”。

本次改动范围：

- `internal/app/rag/tool/builtin/diagnose_helpers.go`
- `internal/app/rag/tool/builtin/document_ingestion_diagnose_tool.go`
- `internal/app/rag/tool/builtin/task_ingestion_diagnose_tool.go`
- `internal/app/rag/tool/builtin/trace_retrieval_diagnose_tool.go`
- `internal/app/rag/tool/answer_guidance.go`

新增能力：

- 三类 diagnose tool 统一补齐结构化字段：
  - `diagnosisScope`
  - `facts`
  - `inferences`
  - `riskHints`
  - `nextActions`
- `confidence` 统一收口为固定口径：
  - `high`
  - `medium`
  - `low`
- `answer_guidance` 不再只消费一串 `evidence`
  - 现在会明确要求回答区分：
    - 结论
    - 事实证据
    - 推断
    - 风险提示
    - 下一步建议

`trace_retrieval_diagnose` 还补了一条新的判断分支：

- 当 `retrieve.chunkCount > 0`，但 `retrieve.topScore` 很低时
  - 不再直接归类为“执行成功”
  - 会输出“命中存在，但 grounding 质量偏弱”的中置信度诊断

新增测试覆盖：

- diagnose payload 结构字段校验
- degraded tool workflow 风险提示
- weak topScore 检测
- 更新后的 answer guidance 断言

## 当前验证状态

已通过：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/core/retrieve ./internal/app/rag/service -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service ./internal/app/rag/... -count=1
```

结果：

- `internal/app/rag/core/retrieve` PASS
- `internal/app/rag/service` PASS
- `internal/app/rag/tool/...` PASS
- `internal/app/rag/...` PASS

## 当前状态判断

这次改造完成后，系统已经具备了“无 intent 依赖的多通道检索基础设施”：

- 质量层面：
  - 精确匹配类 query 的召回能力有了更好的工程基础
  - `hybrid` 不再是写死逻辑，而是可扩展的通道组合
- 架构层面：
  - 后续接入 `intent_directed channel` 时，不需要再回头重拆 retrieve 主流程
  - 只需要新增 channel 和少量 route/context 信息即可
- 观测层面：
  - 已经具备通道级 trace 元数据，后续可以进一步在 trace 页面或 chat meta 中可视化

同时，这一天的工作也把两条“后续能持续演进”的基础铺起来了：

- `retrieve`
  - 已从“有规则”推进到“有规则 + 有样本回放 + 可校准”
- `diagnose`
  - 已从“给出结论 / 证据 / 建议”推进到“区分事实 / 推断 / 风险提示 / 下一步行动”

## 下一步建议

### P0

- 继续细化 `auto` 模式下的 channel 启停规则
- 继续扩大 `retrieve` 真实 query 样本集
- 评估是否追加 `title / metadata` 类无 intent 通道
- 继续细化 diagnose 的冲突证据识别与置信度规则

### P1

- 在 trace 详情页展示 `searchChannels` 和 `channelStats`
- 在 trace / chat 侧逐步消费 `searchDecisions`
- 给检索层增加更细的后处理能力：
  - 文档级去重
  - section/source 优先级控制
  - 版本过滤

### P2

- 在保持当前架构不变的前提下，评估后续 `intent_directed channel` 的最小接入方案
## 补充更新（本轮）

### A. 前端补齐了 `fallback` SSE 事件消费

- `frontend/src/hooks/useStreamResponse.ts`
  - 新增 `fallback` event 分支与 `onFallback`
- `frontend/src/types/index.ts`
  - `Message` 新增 `fallbackReason`
  - 新增 `FallbackPayload`
- `frontend/src/stores/chatStore.ts`
  - 在流式会话中把 `fallback` 事件绑定到当前 assistant 消息
- `frontend/src/components/chat/MessageItem.tsx`
  - 聊天消息中新增琥珀色提示条，明确提示“已回退到通用模型，需要注意核验”

### B. 固化了 integration test 的统一入口与 CI 骨架

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
- 调整 integration test 建表路径
  - `knowledge_document_pipeline_integration_test.go`
  - `knowledge_document_pipeline_failure_integration_test.go`
  - 把 `AutoMigrate` 改成 `postgresrepo.RunMigrations(db)`，让测试 schema 路径与 runtime 更一致

### C. 收口了一轮 ingestion 生产化一致性问题

- `KnowledgeDocumentService.OnIngestionTaskCompleted(...)`
  - 不再静默吞掉 `document / chunk_log` 回写错误
  - 改为返回聚合错误，便于上层日志和后续对账发现问题
- `finishPipelineChunkLogWithRecord(...)`
  - 优先按 `taskId` 精确命中 chunk log
  - 仅在按 task 未命中时才回退到 document 最新记录
  - 新增 task/document 不匹配保护，避免错误覆盖其他任务的 chunk log
- `MultiTaskObserver`
  - 从“某个 observer 失败就短路”调整为“所有 observer 尽量执行完，再聚合错误返回”
- `ExecutorService`
  - task completion observer 失败时显式记录 error log
  - 修正成功日志中的 chunk 统计为 `current.Chunks`
- 新增测试
  - `internal/app/ingestion/service/multi_task_observer_test.go`
  - `internal/app/knowledge/service/knowledge_document_service_test.go` 中补齐 task-scoped chunk log 回写与 mismatch 场景

### D. 本轮补充验证

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service -count=1
```

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS

### E. 下一步建议补充

- 设计 `document / chunk_log / task` 不一致状态的对账规则
- 评估是否落地后台 reconcile loop，定时自动修复知识侧回写异常
