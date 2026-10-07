# Chat / Work 最终答案发布与恢复

日期：2026-10-03。对应工程评审第 2 项，以及本轮 P1 第一项。代码已接入正式服务，未提交或部署；迁移仅在专用测试库验证，未操作原业务库。

## 已交付范围

采用评审方案 A：最终答案准备完成后，持久化 pending publication，再幂等发布已有答案。恢复不会重新执行回答模型或工具。

- `ChatService` 选择发布协议，普通 Chat 与 Work 的正式装配均已启用。直接运行内核的评测、任务域发布和测试替身保留原有边界。
- `ReadyAnswer` 在一个事务中保存 `answer_final`、session completed 和 pending publication，避免已完成状态与最终答案分离。
- 新表 `t_runtime_chat_publication` 以 runtime session 为主键，唯一关联助手消息和最终 journal 事件。状态为 pending、published、abandoned，记录尝试次数、下次重试时间和错误。
- 内容处理与 session chunk embedding 在发布事务外完成。短事务检查资源仍有效，写入助手消息、分块和向量、episode 关联、Work turn 完成状态、包含 messageId / sources 的最终 journal，并推进 published。
- 并发发布者通过行锁收敛到同一消息 ID 和最终事件。已提交发布直接加载结果，不再处理内容或嵌入向量。
- 普通 Chat 发布锁住会话和用户输入；Work 还先锁 topic、检查 turn，遵循删除和取消的锁顺序。删除或撤销输入、取消 Work turn、归档 topic 会阻止待发布结果。普通停止入口也可撤销 pending publication。撤销保存可重放的 cancelled 事件，episode 不会变成 ready。
- 普通 Chat 续流和 Work 重连均可恢复发布。冷缓存由持久 journal 重建；热缓存修复遗漏的 finish / done。旧连接先前因发布失败写入的 done 不再挡住恢复后的 finish，超出缓存长度的 offset 也能拿到稳定结果。
- RAG bootstrap 在启动时扫描 pending，之后每 10 秒扫描，单批最多 20 条，失败延迟重试；关闭时停止扫描。此扫描同时覆盖已生成最终答案的 Work turn，成功后补齐现存流缓存。没有缓存时保留 journal 供授权重连读取。
- Work 已成功保存的文档或进展不因回答发布失败而回滚，恢复也不会再次执行这些工具写入。

迁移：`internal/adapter/repository/postgres/migrations/20261003110000_chat_publication.sql`。主路径为 `cmd/server` → RAG / Work bootstrap → ChatService → ReadyAnswer → ChatPublisher；恢复从同一 RAG bootstrap 以及原有 Chat / Work HTTP 重连入口可达。P0 的授权检查仍在缓存和恢复操作之前。

## 验证证据

使用本机真实 PostgreSQL 的专用 `codex_work_20261001` 数据库，测试入口验证数据库名，执行正式迁移。模型、检索结果及 embedding 使用可控替身，没有调用真实模型或外部网络。

- 注入内容准备失败、消息插入失败、消息与 episode 已写入后最终 journal 插入失败，验证事务回滚、pending 保留、episode 不提前 ready。
- 注入最终通知发送失败，验证已提交消息保留且重连返回同一 ID 与 sources。
- 8 个独立发布者并发恢复，验证只有一条助手消息、一条完成事件和唯一 episode 关联。
- 实际子进程分别在答案 ready 后、消息插入后但事务提交前调用 `os.Exit(73)`，父进程使用新服务实例恢复。验证 PostgreSQL 回滚未提交投影，已持久化答案可发布，模型 turn 和工具调用不增加。
- 正式普通 Chat HTTP 热 / 冷续流验证 finish、sources、终止顺序。原 Work HTTP 故障回归恢复助手答案和 turn，同时保持已保存文档与版本不重复。
- 验证会话删除、用户输入撤销、普通停止及 Work turn 取消，不能复活待发布助手消息。
- 长消息处理验证摘要字段、原文、sources、分块与向量保留；准备 embedding 时另一事务能取得 session 锁，提交失败后分块一并回滚。
- 启动扫描测试实际调用 bootstrap recovery loop，验证首次扫描发布并通知、Close 停止循环。
- P0 的共享缓存、双用户、Work 路由隔离、删除、admission 与停止回归通过。历史没有发布标记的 completed journal 保留原有终止行为，避免兼容性悬挂。

通过命令（设置专用 WORK_TEST_DSN、GOCACHE 与 GIN_MODE 后）：

```powershell
go test -p 1 ./internal/app/runtime/... ./internal/adapter/runtime/... ./internal/adapter/http/rag/... ./internal/adapter/http/work/... ./internal/bootstrap/rag/... ./internal/bootstrap/work/... ./internal/app/rag/service/conversation/... ./internal/adapter/repository/postgres/work/... ./cmd/server -count=1 -timeout 90s
go test ./cmd/... ./internal/... -run '^$'
```

## 交付审计与边界

本轮承诺面是**已生成最终答案的发布收敛**，核对为 aligned：module 有原子 ready 与幂等 publisher；integration 在普通 Chat、Work 与恢复扫描的正式装配启用；delivery 从原 HTTP 入口发出真实 messageId / sources；默认迁移和恢复循环存在，测试不依赖真实模型配置。

边界仍然明确：

1. 未提交 answer-ready 的 admitted / running 执行不由此扫描接管或重新执行；模型完成之前的崩溃、执行租约、跨节点运行所有权和一般 interrupted 收敛不属于本次发布保证。
2. 历史执行没有唯一 publication / assistant 关联，不能安全猜测消息归属，因此不自动回填或重新生成。历史空 completed 仍能结束重放，但其缺失的旧 finish / sources 不保证修复。
3. 保证数据库中消息及最终事件幂等；SSE 是可重放传输，跨节点并发重连可能重复通知同一个稳定消息 ID，不声称网络 exactly-once。画像等原有 best-effort 后置 hook 不在原子提交保证内。
4. 未做真实模型、浏览器联合验收或原库迁移。其余 P1 项目不因此完成。
