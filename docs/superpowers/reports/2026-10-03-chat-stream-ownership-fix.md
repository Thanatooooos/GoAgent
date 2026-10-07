# Chat / Work 流归属 P0 修复

日期：2026-10-03。交付核对：**aligned（仅限本报告的 P0 范围）**。代码已接入正式入口，未提交或部署。

## 范围与行为

修复普通聊天续流和停止接口按 task ID 直接访问共享流缓存、取消执行的归属缺口。入口仍为 `/rag/v3/chat/continue` 和 `/rag/v3/stop`。

- 两个接口均先检查登录身份，并调用同一个服务端授权方法；授权成功后才允许读取缓存、调用 journal replay、取消本地执行或追加 stop 控制事件。
- 授权基于 PostgreSQL 的 runtime session、会话和接受的用户消息。必须属于当前用户，且会话和该用户消息未删除；普通 Chat 接口拒绝 Work 会话的执行，即使请求者是 Work 的所有者。
- 未找到、其他用户、已删除和 Work 执行统一返回 HTTP 404。授权查询失败返回服务错误，不能通过缓存继续访问。
- 首次普通聊天在返回 meta / task ID 前，同步保存用户消息和 runtime session。随后异步运行复用同一用户消息与 session。已接受但模型尚未开始的窗口具有持久归属。
- 异步运行前复查已接受执行的访问权限；会话删除后不能重新创建会话或开始该次模型执行。即使仅恢复会话行，已删除的用户消息也不能使旧流重新可读。
- Work 继续使用自己的 GetTurn / CancelTurn 及主题、用户、会话校验。普通入口不能绕过 Work 的边界。
- 有效的已结束执行仍可由所有者续读保留的流 / journal；停止保留原有控制事件语义，不改写已保存消息或运行结果。数据库尚未存在执行身份的旧残留缓存不单独作为授权依据。

## 正式装配

`internal/bootstrap/rag/runtime.go` 将 `Store.CanAccessChatTask` 装配到正式 `ChatService`。`RegisterRoutes` 继续传入该服务；HTTP handler 的服务接口要求支持 admission 与授权，未提供依赖时不允许访问缓存。

修改集中于普通 Chat handler、ChatService、Runtime admission、PostgreSQL 归属查询和 bootstrap。复用现有表与 session 幂等创建机制，没有新增数据库迁移。

## 回归证据

稳定测试文件：

- `internal/adapter/http/rag/chat_access_test.go`：缓存热冷 × 续流/停止 × 所有者/其他用户/删除/Work/未知/授权查询失败/未登录。未授权路径不调用缓存、replay 或取消；接受失败不暴露流。
- `internal/app/runtime/chat_admission_test.go`：接受时保存身份而不执行模型，实际运行复用用户消息；接受后失去访问权限时不开始模型或继续写消息。
- `internal/adapter/repository/postgres/work/chat_stream_access_integration_test.go`：实际 PostgreSQL、正式 Chat 与 Work HTTP handler、共享内存流缓存和可控模型。覆盖跨用户、跨入口、缓存热冷、Work accepted 窗口、会话删除、用户消息撤销、合法所有者读取与停止。首次 Chat 在 meta 写入前即时查询并断言持久身份已存在；阻塞模型不受未授权停止影响，所有者停止实际结束执行；成功请求只创建一条用户消息和一个 runtime session。

数据库测试沿用独立运行迁移且检查库名的 `WORK_TEST_DSN` 测试入口；本轮实际数据库为 `codex_work_20261001`。仅创建及处理本轮唯一用户的测试对象，没有连接原有 `ragent`，没有启动后台 worker。

最终通过：

```powershell
# 先将 WORK_TEST_DSN 设置为 codex_work_ 专用库，勿在文档保存凭据。
go test -p 1 ./internal/app/runtime/... ./internal/adapter/http/rag/... ./internal/adapter/runtime/... ./internal/bootstrap/rag/... ./internal/bootstrap/work/... ./internal/adapter/repository/postgres/work/... ./cmd/server -count=1

go test ./cmd/... ./internal/... -run '^$'
```

第一条包含实际隔离库的整个 Work 仓储回归；未设置 DSN 时跳过不能计为数据库验收。第二条仅是全仓库相关入口的编译检查，不是全量功能测试。

## 保留边界

本轮不解决聊天答案的幂等发布、进程重启后的执行恢复、ID 环境唯一性或 SSE 缓存恢复后的最终 message ID / sources。这些仍按工程评审 P1 接续。接受后崩溃或异步执行前失败，可能留下需要后续恢复协议处理的 running session；不能将提前持久化身份解释为可恢复执行已经交付。

模型为注入的可控模型，数据库及 HTTP / stream / runtime 是实际链路。未执行真实模型或浏览器复验，未验证多实例真实滚动重启，也未修改前端。当前证据针对服务端授权性质，不宣称完成全部七项工程保证。
