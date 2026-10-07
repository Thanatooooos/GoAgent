# 文档 chunk 执行所有权与恢复

日期：2026-10-04。对应工程评审第 3 项、本轮 P1 第二项。交付核对为 **aligned（限本文范围）**。已接入正式入口，未提交或部署；数据库迁移仅在专用测试库验证。

## 承诺面与实现

保留本地 goroutine worker，为文本处理增加持久任务、owner、epoch、租约、心跳及发布校验。进程退出后，已接受的任务不能无声丢失；旧执行不能覆盖更新一轮的 chunk、向量、日志或状态。

- 新表 `t_document_chunk_job` 保存任务身份、触发者、文档快照、原文件地址、epoch、owner、租约和结果。文档保存 `chunk_epoch` 与 `current_chunk_job_id`，每个文档最多有一个 pending / running 任务。
- `StartChunk` 锁住文档，原子写入 pending 和文档 running，再通知本地队列。并发 admission 会在数据库中收敛；通知失败或本地并发槽已满时，已接受意图仍在数据库，接口返回接受成功。
- 执行者领取任务时固定 owner 和快照；重复投递不再调用模型。默认租约 3 分钟，每分钟续租；失去续租权后取消执行 context。数据库时间决定租约，过期 owner 不能续期复活。
- worker 启动即扫描，之后每 5 秒扫描。普通 pending 任务可重新投递；已领取且租约过期的任务显式变为 interrupted，当前文档及该次运行日志收敛为 failed，用户可重新触发。不会自动重跑已经开始的模型调用。
- 发布事务按文档、任务的顺序加锁，检查 owner、epoch、当前任务、有效租约、启用及删除状态、原文件和处理配置。慢解析、embedding 和增强在事务外完成；chunk、向量、摘要、数量、处理日志、文档状态及任务 completed 在同一事务提交。终结写入再次检查实际时钟，租约若在提交过程中到期则整笔回滚。
- 日志使用任务 ID，创建日志也检查当前执行，失效执行不能在恢复后重新插入 running 日志。失败清理仅更新仍有效的本次 owner；旧成功或失败都不能修改新任务的结果。
- 远程刷新在同一文档锁内获取明确的前置占用快照，随后创建独立 refreshed 任务。新文件元数据随结果事务切换，调度器不再单独覆盖处理状态。旧占用不能冒领更新的 running，迟到清理也不能修改新占用或删除已经引用的文件。
- 远程 refreshed pending 不交给普通扫描器接管，避免同步调度与扫描竞争、提前清理临时来源。接受后尚未领取超过 3 分钟会显式 interrupted；下一次调度或人工操作按正常入口重新开始。
- 原有仅按文档更新时间的恢复和无 owner 状态写入跳过 durable 任务。旧图片 worker 不在 pending / running 文本任务期间改写文档处理状态；新文本事务完成后保留既有图片状态收敛。

迁移为 `internal/adapter/repository/postgres/migrations/20261004090000_document_chunk_jobs.sql`，未改历史迁移。

## 正式入口核对

`cmd/server` 先运行正式迁移，再创建 knowledge runtime。runtime 给文档服务和处理器装配 PostgreSQL ChunkJobs，使用持久扫描队列；原有 chunk HTTP 路由和远程调度共用这一处理器。关闭 runtime 会取消并等待扫描与执行，数据库意图不依赖原 goroutine 存活。

module：领取、心跳、失败回收、快照和事务校验存在。integration：手动处理及远程刷新均在正式 bootstrap 启用。delivery：原 chunk HTTP 返回前已有持久意图，真实 bootstrap 可以处理并恢复任务；用户仍从原文档状态和运行日志观察处理结果，无需新增消息系统。

## 验证证据

实际数据库为本机独立 `codex_work_20261001`。测试入口检查库名前缀并运行正式迁移；未操作业务库 `ragent`。模型与文件存储为可控替身，仓储、pgvector、事务、进程退出、队列、HTTP handler 和 bootstrap 为实际实现。

稳定测试在 `internal/adapter/repository/postgres/work/document_chunk_*_integration_test.go`，复用隔离库及身份 fixture：

1. A 阻塞在 embedding；租约失效并被回收，B 完成；再释放 A，分别迟到成功和失败。B 的 chunk、向量、状态及日志保持不变。
2. 8 路并发领取只有一个 owner；错误 owner、过期 owner 无法续租。阻塞超过两轮租约时正常心跳保留所有权；重复投递不增加 embedding 调用。
3. 最终日志写入故障回滚 chunk / 向量；在最终写入阶段故意拖过租约期限，整笔发布回滚并保留上一轮投影。
4. 配置变更、文件替换、禁用及软删除阻止旧结果发布；旧图片状态刷新不能撤销正在运行的文本任务。
5. 远程刷新成功时文件元数据与结果一起切换；失败保留原文件。验证在途 / 已发布文件不能被错误清理，旧占用及其失败清理不能覆盖新占用。
6. 真实子进程在接受 pending 后及 running 的 embedding 阶段分别 `os.Exit(77)`；新 worker 恢复 pending，过期 running 显式 interrupted 后可由新任务完成。
7. 使用正式 knowledge bootstrap 和原 chunk HTTP 路由验证返回前意图持久化、实际队列处理，以及关闭后重新启动 runtime 恢复丢失本地通知的任务。
8. 知识处理、远程调度、图片相关常规套件，P0 Chat / Work 归属、P1 最终答案发布和整个 Work 仓储隔离数据库回归通过；全仓编译检查通过。

通过命令（设置专用 `WORK_TEST_DSN`，不在文档保存凭据）：

```powershell
go test -p 1 ./internal/app/knowledge/... ./internal/adapter/repository/postgres/knowledge/... ./internal/adapter/taskqueue/goroutine/... ./internal/bootstrap/knowledge/... ./internal/app/runtime/... ./internal/adapter/runtime/... ./internal/adapter/http/rag/... ./internal/adapter/http/work/... ./internal/bootstrap/rag/... ./internal/bootstrap/work/... ./internal/app/rag/service/conversation/... ./internal/adapter/repository/postgres/work/... ./cmd/server -count=1 -timeout 90s

go test ./cmd/... ./internal/... -run '^$'
```

第二条只验证编译。未设置 DSN 的跳过不算数据库验收。

## 保留边界

- 恢复语义为普通 pending 可投递、已领取的失效执行显式中断。没有持久模型检查点，不从半次 embedding / 增强继续，也不保证外部模型调用只发生一次。
- 部分现有 embedding API 不接收 context，失效后外部调用可能仍返回；数据库校验保证它没有发布权，不能将取消理解为物理停止。
- 远程文件获取尚未创建 durable chunk job 的前置阶段仍受调度租约与占用快照约束；崩溃留下的历史无任务 running 仍按既有超时恢复为 failed，不自动推断 owner。
- 图片任务已有独立版本和租约；本次补齐与文本任务的状态边界，没有重写其全部恢复机制。embedding profile 兼容性及 ID 环境唯一性仍属于后续 P1。
- 未验证真实模型、浏览器联合流程或多节点滚动重启，未对原业务库运行新迁移。工作区原有大量修改保留，未提交、清理或部署。
