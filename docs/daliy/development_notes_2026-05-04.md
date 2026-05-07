# Development Notes - 2026-05-04

## 背景

项目已从"基础设施搭建期"进入"主链路闭环、联调与质量收口期"。早间通过 `project_progress_context.md` 评估了整体进度后，确定了当日的三个推进方向：

1. **P0**：补齐飞书数据源（fetcher 一直返回 "not implemented yet"）
2. **P0**：ingestion indexer 写入缺乏重试与补偿机制
3. **RAG 增强**：检索置信度不足时回退到通用 LLM 并提醒用户

## 本次新增

### 1. 飞书 Fetcher 落地

之前 `FetcherNodeRunner.fetchSource()` 中 `feishu` 分支直接返回错误。本次补齐完整的飞书文档拉取链路。

新增文件：

- `internal/adapter/feishu/client.go`
  - `Client` 结构体，封装 `tenant_access_token` 缓存/自动刷新
  - `FetchDocumentContent(ctx, documentID)` — 获取 Docx 文档纯文本
  - `ExtractDocumentID(location)` — 从飞书 URL 中提取文档 ID
- `internal/adapter/feishu/client_test.go` — 5 个测试（成功拉取、token 缓存、鉴权失败、API 错误、ID 提取）

修改文件：

- `internal/app/ingestion/service/fetcher_node_runner.go`
  - 新增 `feishuClient` 字段（`feishu.DocumentFetcher` 接口，方便测试替换）
  - 新增 `fetchFeishu()` 方法，支持注入 client 或从 settings 凭据动态创建
- `internal/bootstrap/ingestion/runtime.go` — 从 `feishu.app-id/app-secret` 配置创建 client 注入
- `internal/app/knowledge/domain/knowledge_document.go` — 新增 `KnowledgeDocumentSourceFeishu` 常量
- `internal/app/knowledge/service/knowledge_document_service.go` — 源类型归一化支持 feishu
- `internal/framework/config/config.go` — 新增 `FeishuConfig` 结构体
- `configs/application.yaml` — 新增 `feishu.app-id / feishu.app-secret` 配置节

### 2. Indexer 写入重试与补偿

问题：

- `IndexerNodeRunner.Run()` 采用 delete-then-recreate 模式，本身幂等
- 但下游临时故障（DB 抖动、embedding 超时）直接导致 task 失败
- delete 成功但 create 失败时旧数据已丢失，依赖手动重跑

处理：

- `internal/app/ingestion/service/executor_service.go`
  - `ExecutorServiceOptions` 新增 `MaxRetries`、`RetryBackoffMs` 字段
  - `runWorkflow()` 节点执行循环改为带重试：支持全局默认 + 节点级 `retryCount`/`retryBackoffMs` settings 覆盖
  - 指数退避 backoff = base * 2^(attempt-1)，上限 5 次
  - 每次重试前检查 `ctx.Done()`，关闭/取消时立即中止
- `internal/app/ingestion/service/indexer_node_runner.go`
  - `Run()` 改为命名返回值，增加 `defer` 补偿清理块
  - 失败时自动删除已写入的 chunks/vectors，确保重试从干净状态开始
- `internal/framework/config/config.go` — `RagKnowledgeIngestion` 增加 `MaxRetries`、`RetryBackoffMs`
- `configs/application.yaml` — 增加 `rag.knowledge.ingestion.max-retries: 2` 和 `retry-backoff-ms: 1000`

测试：

- `internal/app/ingestion/service/executor_service_test.go` — 6 个测试（重试成功、重试耗尽失败、节点级覆盖、取消中断、workflow 构建、参数校验）
- `internal/app/ingestion/service/node_runner_test.go` — 新增 5 个测试（飞书拉取 × 3、indexer 补偿 × 2）

### 3. RAG 检索置信度回退

在 `RagChatService.Chat()` 的检索阶段后新增置信度检查：最高 chunk 分数低于阈值时，清空知识上下文并切换到回退提示词，明确告知用户"未检索到相关内容"和"通用模型可能出现幻觉"。

新增文件：

- `internal/app/rag/service/rag_chat_service_test.go` — 8 个测试（`topChunkScore`、`buildFallbackPrompt`、置信度、sink stub 等）

修改文件：

- `internal/app/rag/core/prompt/constants.go`
  - 新增 `FallbackKBTemplate` 回退提示词模板，包含 `{{question}}` 变量插槽
- `internal/app/rag/core/prompt/template_loader.go`
  - 注册 fallback 模板
- `internal/app/rag/service/rag_chat_service.go`
  - `RagChatService` 新增 `confidenceThreshold` 字段 + `SetConfidenceThreshold()` setter
  - `RagChatEventSink` 接口新增 `SendFallback(reason)` 方法
  - `Chat()` 在 `prepareChat` 之后、`runPromptStage` 之前插入置信度检查
  - `runPromptStage` 新增 `systemPromptOverride` 参数
  - 新增 `topChunkScore()`、`buildFallbackPrompt()` 辅助函数
- `internal/adapter/http/rag/handlers.go`
  - `sseChatSink` 实现 `SendFallback` → 发送 `fallback` SSE 事件
- `internal/bootstrap/rag/runtime.go`
  - 读取 `rag.search.channels.vector-global.confidence-threshold` 配置注入

配置项（已接入已有的 config 预留）：

```yaml
rag:
  search:
    channels:
      vector-global:
        confidence-threshold: 0.6  # 低于此值触发回退，0 表示关闭
```

完整行为流：

```
检索结果 → 取最高 chunk 分数
  ├─ score >= 0.6 → 正常走知识库回答
  └─ score < 0.6 →
       1. 清空 KnowledgeContext
       2. SSE 发送 fallback 事件（前端可渲染警告 banner）
       3. 系统提示词注入提醒模板
       4. LLM 在回答开头告知用户"未检索到相关内容"+"可能存在幻觉"
```

## 当前验证状态

全量测试通过，零回归：

```powershell
go test ./internal/app/... ./internal/adapter/... ./internal/bootstrap/... -count=1
```

Go build 零错误。

## 后续建议

1. **P1**：前端对接 `fallback` SSE 事件，渲染可视化警告横幅
2. **P1**：RAG 反馈闭环（利用 message_feedback 优化检索排序）
3. **P1**：Ingestion 观测指标 API
