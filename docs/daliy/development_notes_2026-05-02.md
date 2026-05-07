# Development Notes - 2026-05-02

## 背景

本次修改不属于新功能推进，主要是补齐前一轮代码评估中已经识别出的遗留质量问题，目标是先收口 P0 风险，避免已打通链路在一致性、生命周期管理和资源清理上继续带病推进。

对应来源：

- `docs/code_quality_assessment.md`
- 2026-05-02 当日代码审阅结论

## 本次修复

### 1. 修复会话级联删除缺少事务保护

问题：

- `ConversationService.Delete(...)` 原先分三步独立删除：
  - 删除 conversation
  - 删除 conversation messages
  - 删除 conversation summaries
- 任一步失败都可能留下不一致数据。

处理：

- 新增会话删除事务抽象：
  - `internal/app/rag/service/conversation_delete_transaction.go`
- 新增 PostgreSQL 事务实现：
  - `internal/adapter/repository/postgres/rag/conversation_delete_transaction.go`
- 将 `ConversationService.Delete(...)` 改为在事务里统一执行级联删除：
  - `internal/app/rag/service/conversation_service.go`
- 在 RAG runtime 中完成装配：
  - `internal/bootstrap/rag/runtime.go`

结果：

- 会话、消息、摘要删除具备原子性。
- 避免出现“会话已删，但关联消息/摘要残留”的数据一致性问题。

### 2. 修复知识文档调度 goroutine 缺少生命周期管理

问题：

- `KnowledgeDocumentScheduleJob` 默认 dispatcher 直接 `go task()`。
- 调度任务缺少取消和等待退出机制。
- runtime 关闭时无法显式收口这批异步任务。

处理：

- 将默认 dispatcher 改为受控实现，支持：
  - cancel
  - wait group 等待退出
- 为 `KnowledgeDocumentScheduleJob` 新增 `Close()`：
  - `internal/app/knowledge/schedule/knowledge_document_schedule_job.go`
- 在 `Runtime.Close()` 中显式调用 `ScheduleJob.Close()`：
  - `internal/bootstrap/knowledge/runtime.go`
- schedule 锁释放改为使用带超时的后台上下文，避免异步尾清理无限悬挂。

结果：

- 默认异步调度任务具备基础生命周期管理能力。
- 应用关闭时可以更稳妥地回收调度任务。

### 3. 修复上传失败清理使用错误上下文

问题：

- 文档上传和远程文件抓取失败后的存储清理闭包直接使用 `context.Background()`。
- 会丢失原始上下文中的值，也缺少明确超时边界。

处理：

- 在 `KnowledgeDocumentService` 中新增统一 cleanup context 构造：
  - `internal/app/knowledge/service/knowledge_document_service.go`
- 清理逻辑改为：
  - 基于原始 `ctx` 的 `context.WithoutCancel(...)`
  - 再叠加固定超时
- 替换上传文件和远程文件两处回滚清理逻辑。

结果：

- 清理操作不再裸用 `context.Background()`。
- 在保留上下文值的同时，避免清理过程无限阻塞。

## 受影响文件

- `internal/app/rag/service/conversation_delete_transaction.go`
- `internal/adapter/repository/postgres/rag/conversation_delete_transaction.go`
- `internal/app/rag/service/conversation_service.go`
- `internal/bootstrap/rag/runtime.go`
- `internal/app/rag/service/conversation_service_test.go`
- `internal/app/knowledge/schedule/knowledge_document_schedule_job.go`
- `internal/bootstrap/knowledge/runtime.go`
- `internal/app/knowledge/service/knowledge_document_service.go`

## 验证

已执行并通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/rag/service
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/knowledge/schedule
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/app/knowledge/service
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/bootstrap/rag
$env:GOCACHE='D:\goagent\.gocache'; go test ./internal/bootstrap/knowledge
```

## 备注

这次修复优先处理的是“之前已经存在、且会影响稳定性或一致性”的遗留问题，不是新需求开发。

后续仍建议继续推进：

- SSE 背压与慢客户端保护
- 流式回调的 panic 隔离
- 数据库连接池与健康检查配置

## P1 持续修复

### 4. SSE 写入保护与慢客户端基础防护

问题：

- SSE 发送原先直接 `WriteString + Flush`。
- 没有串行化保护，也没有写超时边界。
- 慢客户端可能导致连接长期阻塞。

处理：

- 在 `internal/framework/web/sse_emitter_sender.go` 中增加：
  - 发送互斥锁，避免并发写入混乱
  - 基于 `ResponseController` 的写截止时间控制
  - `rag.default.sse-timeout-ms` 的超时配置回退
- 重构事件序列化逻辑，抽出统一 payload 构造。
- 补充测试：
  - `internal/framework/web/sse_emitter_sender_test.go`

结果：

- SSE 输出具备基础写超时保护。
- 默认发送路径具备更稳妥的串行化约束。

### 5. 流式回调 panic 隔离与终态收口

问题：

- chat 流式回调直接调用 `OnThinking / OnContent / OnComplete / OnError`。
- 如果业务回调 panic，整个流处理 goroutine 会崩掉。
- 错误和完成态缺少统一的终态收口。

处理：

- 在 `internal/infra-ai/chat/openai_style_chat_client.go` 中新增安全回调派发器：
  - 对各类 callback 调用增加 `recover`
  - 使用 `sync.Once` 保证终态只下发一次
  - callback panic 时转为 `OnError(...)`
- 补充测试：
  - `internal/infra-ai/chat/test/openai_style_chat_client_panic_test.go`

结果：

- 回调 panic 不再直接打崩整个流处理。
- 流式终态分发更一致。

### 6. 数据库连接池参数与启动健康检查

问题：

- `NewGormDB(...)` 和 `NewPGXPool(...)` 原先只负责创建连接。
- 没有统一应用连接池参数。
- 启动阶段缺少显式 health check。

处理：

- 在 `internal/adapter/repository/postgres/conn.go` 中补齐：
  - SQL DB 连接池参数应用
  - PGX pool 参数应用
  - 启动后 `Ping / PingContext` 健康检查
  - Hikari 配置向 Go 连接池参数的映射与默认值
- 补充测试：
  - `internal/adapter/repository/postgres/conn_test.go`

结果：

- 启动时能更早暴露数据库连通性问题。
- 连接池配置不再完全依赖驱动默认值。

### 7. UpdatePredicates 重复逻辑下沉为公共实现

问题：

- `knowledge` 与 `rag` 两套 PostgreSQL 更新 helper 基本重复。
- 继续各自维护会增加后续修改成本和漂移风险。

处理：

- 新增公共 helper：
  - `internal/adapter/repository/postgres/common/update_helpers.go`
- 将以下两处改为薄封装，复用公共实现：
  - `internal/adapter/repository/postgres/knowledge/update_helpers.go`
  - `internal/adapter/repository/postgres/rag/update_helpers.go`
- 补充公共 helper 测试：
  - `internal/adapter/repository/postgres/common/update_helpers_test.go`

结果：

- 更新 DSL 的条件拼装与赋值构造只保留一份核心实现。
- 后续若补运算符或调整错误处理，维护面更小。

### 8. 分页魔法数字收口

问题：

- 多个 service 中反复散落 `page=1 / pageSize=10 / max=100` 之类的分页归一化逻辑。
- `user` 业务也单独维护一套分页边界判断。

处理：

- 新增公共分页归一化 helper：
  - `internal/framework/paging/paging.go`
- 补充测试：
  - `internal/framework/paging/paging_test.go`
- 将以下服务改为复用统一 helper：
  - `internal/app/knowledge/service/knowledge_document_service.go`
  - `internal/app/knowledge/service/knowledge_chunk_service.go`
  - `internal/app/user/service/user_service.go`

结果：

- 分页边界逻辑集中，减少重复判断分支。
- 常见页大小“魔法数字”不再散落在多个函数里。

## Ingestion 进展

### 9. ingestion 模块骨架、持久化与最小执行链路落地

本轮新增：

- `docs/ingestion_module_goal.md`
- `docs/ingestion_execution_design.md`
- `internal/app/ingestion/`
- `internal/adapter/repository/postgres/ingestion/`
- `internal/bootstrap/ingestion/`

本轮明确并落地的内容：

- ingestion 作为独立模块存在，不继续塞回 `knowledge`
- 当前推荐的接入关系是：
  - `TaskService -> ExecutorService -> WorkflowBuilder / NodeRunner`
- EINO 的定位是执行编排层，而不是整个 ingestion 业务壳

已完成的代码落地：

- 建立 `Pipeline / Task / TaskNode` 领域模型
- 建立 `pipeline / task / task_node` 三张表及 repository
- 建立 `ExecutionState / WorkflowSpec / NodeRunnerRegistry / TaskObserver`
- 建立四个节点的第一版占位实现：
  - `fetcher`
  - `parser`
  - `chunker`
  - `indexer`
- `ExecutorService` 已能按顺序执行最小 workflow 并回写 `task / task_node`
- ingestion 已接入 `cmd/server/main.go` 的后台管理员路由

额外修正：

- `pipeline / task / task_node` 创建时补上真实 ID 生成
- `task_node` 改为独立主键，并通过 `(task_id, node_id)` 做状态回写
- 避免了“任务显示 running 但实际未执行”的假状态设计

当前边界：

- `fetcher` 还未实现真实远程读取
- `indexer` 还未接真实下游
- 当前最小链路更偏编排验证和观测验证，不是最终生产形态

验证：

```powershell
$env:GOCACHE='D:\goagent\.gocache'; go test ./...
```
