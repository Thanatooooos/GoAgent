# Chat / Work 最终答案之前的执行所有权与中断收敛

日期：2026-10-04。补齐工程评审第 2 项、P1 第一项中“尚未保存最终答案的执行”的恢复协议。交付核对为 **aligned（限新受管执行与本文范围）**。未提交或部署；新迁移只在专用测试库验证。

## 承诺面

服务在接受请求后、模型运行中或工具已保存结果后退出，不能让对应执行永久保持 running。恢复应提供明确终态，保留已保存的 Work 内容，不自动重跑模型或工具；旧执行即使迟到返回，也不能追加执行事实、写入 Work 内容或发布最终答案。已有 answer-final / pending publication 继续使用先前的发布恢复协议。

## 实现

- 新表 `t_runtime_chat_execution` 保存 task、用户、会话、用户消息、pending / running / 终态、owner、epoch、lease_until 与 heartbeat_at。迁移为 `internal/adapter/repository/postgres/migrations/20261004100000_chat_execution_leases.sql`，未改历史迁移。
- 普通 Chat 在 admission 创建 runtime session 时原子预留执行，发生在暴露 stream ID 之前。Work 在接受 turn、用户消息的同一事务预留执行，覆盖尚未创建 runtime session 的 accepted 窗口。
- pending 的接受期限为 3 分钟。模型开始前 CAS 领取，重复运行无法获得所有权。运行租约默认为 3 分钟，每分钟续租；数据库实际时间决定有效性，过期 owner 不能续租复活。续租失败会取消执行 context。
- 执行 token 随 context 进入 journal、工具状态、episode 和 Work 写入。事务进入及结束时检查所有权和有效期；最终 ready、失败收尾也再次检查实际时钟。Work 操作已保存的结果允许幂等读取，新写入仍要求有效所有权。
- answer-final、session completed、pending publication 与 execution completed 原子提交。发布事务失败或提交过程中租约过期会整笔回滚，没有 final answer 的执行不能被发布器恢复为成功。
- 普通失败、取消和 panic 原子关闭 session / execution / Work turn，收敛未结算工具并丢弃待发布 episode。重复收尾不会产生第二条终态事实。
- RAG runtime 启动即扫描，随后每 5 秒扫描一批 20 条；关闭 runtime 会取消并等待扫描结束。租约过期或 Work 总期限过期的未完成执行变为 interrupted；取消、删除或归属资源撤销保留 cancelled 语义；模型启动前已有的 Work failed 保留失败语义。
- 恢复在同一事务中创建必要的 runtime session、结算未完成工具、丢弃待发布 episode、保存终态 journal 并关闭 Work turn / execution。8 路并发恢复得到同一个终态 journal ID。已经 ready 的答案不进入此中断分支。
- 普通续流与 Work turn 读取 / 重连也触发恢复检查。热缓存追加终态，冷缓存从 journal 重建；中断使用既有 SSE error + done，包含 `status=interrupted` 与稳定 `runtimeEventId`。过大的旧 offset 也可取得终态。Work HTTP 的旧 worker 失去租约时由恢复协议收尾，不能另写 failed 覆盖 interrupted。

默认恢复上限受最后一次续租时间和扫描批量影响；重启不会把另一台仍在正常续租的执行立即判为中断。

## 正式路径与交付核对

`cmd/server` 运行迁移后创建 RAG runtime；`rag.NewRuntime` 启动执行恢复循环并把新表加入必需表检查。普通 Chat 的原 admission / runtime / publication 路径采用该协议。Work 的正式 `ConfigureChat` 复用持久 lifecycle、publication 与共享 stream manager，`AcceptTurn` 的预留直接落在 Work 仓储事务中。共享 RAG 扫描器也负责 Work 执行。

module：持久身份、领取、心跳、写入校验和中断事务存在并有实际数据库测试。integration：正式 Chat / Work 装配可达，无需启用新开关。delivery：普通 ContinueChat 和 Work stream 路由可返回明确 error / done，Work 的已保存版本和 Outputs 仍能读取。前端既有 error 处理消费 error 文本，未新增 UI 协议分支。

默认路径沿用现有 PostgreSQL 与 stream 配置，没有新模型、文件或样本依赖。此次未将整个 P1 或其他评审项标为完成。

## 验证证据

数据库为本机独立 `codex_work_20261001`，测试核对库名前缀并运行正式迁移，未操作业务库。仓储、pgvector、事务、进程退出、HTTP handler 和恢复循环为实际实现；模型为可控替身。

新增证据位于 `internal/adapter/repository/postgres/work/chat_execution_integration_test.go` 和 `internal/bootstrap/rag/execution_recovery_integration_test.go`：

1. 8 路领取仅一个 owner，错误 epoch 和过期 owner 不能续租；8 路恢复只产生一条稳定终态事实。
2. 模型阻塞超过三个测试租约仍由心跳保持所有权；独立 ChatService 无法重复启动模型。失效后释放旧模型，迟到成功、迟到失败均不能发布助手消息或改变 interrupted。
3. 未结算工具变为失败、episode 被丢弃；旧 token 不能结算工具、写最终答案或用迟到失败覆盖中断。
4. ready 创建故障与事务内实际时钟过期均回滚答案、状态和发布意图。自然运行的 ready 失败明确收敛为 failed，panic 也留下持久 failed。
5. 实际子进程在普通 Chat 接受后和模型执行中分别 `os.Exit(74)`；新服务在租约失效后恢复 interrupted，不调用模型。
6. Work accepted 尚无 session，以及已领取并保存文档后的失效，均收敛为 interrupted。已保存版本与 Outputs 保留，重复操作只读重放；旧执行的新进展写入被拒绝，之后可接受同会话的新请求。
7. 正式 Work HTTP 中，模型在保存文档后阻塞，租约过期后迟到返回；原请求收到 interrupted / done，文档仍保存，旧 HTTP 收尾不能把 turn 改成 failed。
8. 普通热 / 冷 HTTP 重连、重复重连及越界 offset 得到稳定中断提示；跨用户访问被拒绝，重连不调用模型。取消、输入删除、Work 总期限和已有 ready 分支分别验证。
9. 实际恢复循环启动即回收第一条过期意图，周期扫描回收随后过期的第二条；未过期意图不被中断，Close 等待循环停止。
10. P0 流归属、最终答案发布、文档 chunk 所有权以及相关知识处理 / Chat / Work 回归通过；全仓编译检查通过。

通过命令（设置专用 `WORK_TEST_DSN`，不在文档保存凭据）：

```powershell
go test -p 1 ./internal/app/knowledge/... ./internal/adapter/repository/postgres/knowledge/... ./internal/adapter/taskqueue/goroutine/... ./internal/bootstrap/knowledge/... ./internal/app/runtime/... ./internal/adapter/runtime/... ./internal/adapter/http/rag/... ./internal/adapter/http/work/... ./internal/bootstrap/rag/... ./internal/bootstrap/work/... ./internal/app/rag/service/conversation/... ./internal/adapter/repository/postgres/work/... ./cmd/server -count=1 -timeout 90s

go test ./cmd/... ./internal/... -run '^$'
```

最后补充 panic / 前置失败测试及失败收尾去重后，重新通过 runtime、HTTP、RAG bootstrap、整个 Work 仓储与 cmd/server 套件。第二条只验证编译；未设置 DSN 的跳过不算数据库验收。

## 保留边界

- 没有模型检查点，不从半次模型或工具调用续跑，也不自动重新领取 interrupted。用户查看已保存内容后发起新请求。取消 context 不等于外部调用已物理停止；校验限制其持久写入权，已发生的外部副作用不能凭 journal 撤销。
- 新协议覆盖正式 PostgreSQL publication 装配下的新 Chat / Work。历史无 execution 关联的 Chat running 不自动猜测 owner 或回填；旧 Work 无关联 turn 仍使用原期限恢复。这些历史数据需独立审计。
- 内部手动 Work 命令兼容尚未领取的有效 pending 意图；正式模型执行必须领取 token。直接 RunTask、独立不带 publication 的旧 kernel 路径没有改为 Chat 执行租约。
- 中断记录和已保存内容以 PostgreSQL 为准；缓存通知失败可经重连从 journal 恢复。没有新增 SSE 通知 outbox 或跨节点缓存原子投影协议。
- 完整 `cmd/server` 启动为装配代码核对与编译验证，恢复循环有实际启动 / 周期 / 关闭测试。未验收真实模型、浏览器联合流程或多节点滚动重启，未在原业务库运行迁移，未提交、清理或部署。
- embedding profile 兼容性、ID 环境唯一性及其他工程评审项仍属后续工作。
