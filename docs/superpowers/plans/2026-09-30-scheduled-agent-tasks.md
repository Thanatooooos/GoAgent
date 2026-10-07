# 定时智能体任务实施计划

**规格：** `docs/superpowers/specs/2026-09-30-scheduled-agent-tasks-design.md`

**目标：** 先交付体育条件监测试点，再把 DailyBrief 迁移到同一任务基座；所有计划运行都调用 `internal/app/runtime`，应用内消息是唯一保证的投递渠道。

**状态：** 通用任务主体已接入，并完成真实模型、HTTP 和浏览器验证；DailyBrief 统一任务关口于 2026-10-06 通过，体育试点及其他未验收项仍开放。逐项证据见首轮审计与 `docs/superpowers/reports/2026-10-06-dailybrief-migration-followup.md`。复选框仅表示该条交付验收已有证据。

## 实施原则

- 保留任务配置、计划运行、技术重试尝试、runtime session、专用会话五种不同身份；不要把长期 `task_id` 直接当成每次 `RunTask` 的运行 ID。
- 聊天 runtime 负责识别并提出任务草案；只有用户确认操作可以写入有效配置。定时 runtime 只运行确认过的固定 prompt 和该任务的私有上下文。
- 服务端强制用户、KB ID、网页来源及工具权限。试点不做“来源是否查全”的在线核验；`no_report` 只表示模型的结论。
- 调度和发布均须幂等。先让运行事实、消息及未读状态在异常恢复后可追溯，再接入后台循环。
- 新旧 DailyBrief 路径不能同时为同一用户、同一日期发布。迁移以前保留现有页面与调度；迁移完成后再关闭旧调度。
- 仓库当前有大量既存未提交改动。实施时只改本计划列出的必要文件；开始每个阶段前核对工作区状态，不清理或覆盖他人改动。

## 建议模块与数据契约

建议新增 `internal/app/scheduledtask/{domain,port,service,schedule}`、`internal/adapter/repository/postgres/scheduledtask`、`internal/adapter/http/scheduledtask` 和 `internal/bootstrap/scheduledtask`。名称可随仓库风格调整，但调度、业务状态、HTTP 和持久化职责应保持分离。

| 对象 | 关键字段与约束 |
| --- | --- |
| task | `id, user_id, status, current_version, conversation_id?, created_at, confirmed_at, next_due_at`；手动删除后不再可调度。 |
| task_version | `task_id, version, prompt, trigger, timezone, allowed_sources, kb_ids, tool_policy, report_policy, confirmed_at`；确认后不可变。 |
| occurrence | `id, task_id, version, scheduled_at, deadline_at, status, result_signal?, published_message_id?`；对 `(task_id, version, scheduled_at)` 唯一。 |
| attempt | `id, occurrence_id, runtime_session_id?, started_at, finished_at, technical_status, error`；同一 occurrence 可有限重试。 |
| draft | `id, user_id, originating_conversation_id?, base_version?, proposed_config, expires_at, status`；确认须校验用户、草案版本与过期状态。 |
| notification | 任务专用会话中的消息及站内未读状态；同一发布事件有稳定幂等键。 |

`report | no_report | uncertain` 是模型结果信号；`failed | cancelled | completed` 是尝试的技术状态。结构化结果最少有 `signal`、`body` 或 `reason`、模型给出的来源引用；日报可附结构化 artifact。任务基座只校验结构、权限、版本、有效期和去重，不把引用或工具调用的存在解释为资料覆盖证明。

## 阶段 0：冻结接口和可修改默认值

**涉及：** `internal/app/scheduledtask/domain`（新建）、`internal/framework/config/config.go`、`configs/application.yaml`、本规格文档。

- [ ] 定义单次提醒、周期汇总、条件轮询三种触发规则的数据结构，以及状态条件/事件条件、首次基线、时区和完成规则。任务种类只影响触发与结果合法性，不建立绕过 runtime 的执行器。
- [ ] 固定 `task_id / version / occurrence_id / attempt_id / runtime_session_id / conversation_id` 的关联与唯一键；明确 `report` 的正文要求、提醒/汇总遇到 `no_report` 的失败规则，以及 `uncertain` 等下次计划运行。
- [ ] 选定可修改的实现默认值并配置化：调度扫描间隔、允许的调度抖动、技术重试次数和退避、各类结果有效期、连续失败通知阈值、低频状态反馈间隔、夏令时缺失/重复本地时间处理、运行记录保留期。默认 09:00 的建议时间进入预览，不直接生成未确认任务。
- [ ] 为确认预览和结果信号写最小请求/响应契约及状态转换表，供后端和前端共享；新增配置项在启动时校验，不能以零值意外关闭有效期或扩大权限。

**验收：** 使用固定时钟覆盖一次性、每日、每周、跨月与夏令时时间；非法时区和无效结果信号被拒绝；默认值可查且可修改。

## 阶段 1：任务配置、版本和运行记录的持久化

**涉及：** `internal/adapter/repository/postgres/migrations/`、`internal/app/scheduledtask/{domain,port,service}`（新建）、`internal/adapter/repository/postgres/scheduledtask`（新建）。

- [ ] 新增 task、不可变版本、draft、occurrence、attempt 与任务会话关联所需表和索引。迁移不修改现有 DailyBrief 数据；建表与回滚方式遵循仓库现有迁移惯例。
- [ ] 实现草案确认事务：校验用户和 `base_version`，写入完整新版本并推进当前版本；重复确认同一草案返回同一结果，版本冲突返回可展示的差异冲突。
- [ ] 实现按用户读取与管理任务、手动暂停/恢复/删除、等价任务候选查询。删除任务不级联删除会话或消息；删除专用会话引发暂停的事务入口留给阶段 4 接入。
- [ ] 实现 occurrence 唯一键、到期扫描与租约/认领、attempt 记录、过期标记和崩溃后的卡住运行恢复。多实例或重复扫描同一计划时间只能认领一个 occurrence。

**验收：** 数据库集成测试覆盖双实例并发认领、重复确认、版本竞争、任务删除后不再调度、重启后重领未完成工作；按用户读取不能跨账号返回任务或运行记录。

## 阶段 2：扩展 runtime 的定时任务入口

**涉及：** `internal/app/runtime/task.go`、`internal/app/runtime/contract.go`、`internal/app/runtime/capability/*`、`internal/adapter/repository/postgres/runtime/task_journal.go`、`internal/bootstrap/rag/runtime_build_conversation_runtime.go`。

- [ ] 在 `TaskRequest` 中显式传入任务配置版本、occurrence/attempt 身份、固定 prompt、任务私有历史、KB ID 与批准的来源/工具策略；继续复用模型/工具循环，不注入普通聊天或专用会话闲聊历史。
- [ ] 为任务结果增加受约束的三态解析，保留可追溯正文、原因和来源引用。模型超时、空输出、非法结构、预算耗尽与 journal 必要写入失败返回技术失败；不把工具局部失败自动误判为 `no_report` 或“已查全”。
- [ ] 任务工具上下文实际携带获准 KB ID，空 KB scope 禁止检索；web 搜索结果与抓取均按任务确认的来源范围过滤/拒绝，不能只在 prompt 中描述权限。任务运行只暴露只读能力，禁用会话归档和可写工具。
- [ ] 每个 attempt 使用独立 runtime session。保留现有 DailyBrief 调用的兼容性或同步修改调用方；不要把当前 `(task_type, task_id)` 的唯一约束扩展成“一个长期任务只用一个 journal”。
- [ ] 修正 `RunTask` 忽略必要 journal 写入与 finish 错误的问题，或明确由任务 attempt 持久化足够的可复盘事实；试点抽样必须能关联一次结果与当次工具调用。

**验收：** runtime 单元/集成测试证明三态输出、KB/web scope、跨任务历史隔离、模型或工具故障、attempt 日志和兼容的 DailyBrief 调用；关闭 journal 存储时不能把缺失运行记录的调用当作正常可复盘运行。

## 阶段 3：通用调度器与结果结算

**涉及：** `internal/app/scheduledtask/schedule`、`internal/app/scheduledtask/service`（新建）、`internal/bootstrap/scheduledtask`（新建）、`cmd/server/main.go`。

- [ ] 调度器按保存的 IANA 时区计算下一触发点，写入 occurrence 并调用 `RunTask`。补扫只接纳仍在有效期内的 occurrence；过时的一次性提醒标记 `missed`，不晚发。
- [ ] 技术错误在同一 occurrence 内按配置有限重试，并在有效期后停止；`uncertain` 不立即重试。提醒/周期汇总返回 `no_report` 记为无效结果；条件监测返回 `no_report` 只记运行事实。
- [ ] 结算时再次核对任务状态、配置版本、权限和结果有效期。旧版本结果标记 `superseded`；任务暂停或删除时保存已完成尝试但不发布。状态条件报告前由本次 runtime 结果判断当前状态；事件条件以确认时间为基线。
- [ ] 以任务私有状态向模型提供必要的上次运行/暂停区间信息，提示尽力回看。不得生成“上次成功检查时间”或来源覆盖声明。完成的条件任务停止调度；重新启用形成新版本/阶段。
- [ ] 给低频无消息反馈、连续技术失败或 `uncertain` 状态通知建立独立于条件达成报告的事件键和节流规则；用户不回复不会暂停任务。

**验收：** 固定时钟和并发测试覆盖重复扫描、超时后模型才返回、配置修改与运行并发、暂停/删除与运行并发、失败重试、过期跳过、每日/每周无消息反馈及恢复后单次尽力回看。

## 阶段 4：专用会话、后台发布与未读

**涉及：** `internal/app/rag/service/conversation/conversation_service.go`、现有会话/消息仓储和迁移、`internal/adapter/http/rag/conversation_handler.go`、`internal/app/scheduledtask/service`、前端会话列表读取接口。

- [x] 增加由任务 publisher 调用的后台会话/消息写入服务。第一次需要发布时创建专用会话，之后复用；消息、未读提示、occurrence 发布状态以事务或 outbox 加幂等键结算。
- [x] 为会话记录任务归属。用户删除专用会话时，在同一可靠边界暂停所属任务并清除会话关联；删除时已开始的 attempt 可保存结果，但发布门槛拒绝写入旧会话。
- [ ] 用户恢复任务后保持会话关联为空，直到下一次真正需要汇报时创建新专用会话。手动删除任务仍保留历史会话消息；已完成任务重新启用且原会话存在时继续使用它。
- [x] 增加可查询的站内未读数量/状态，前端现有会话列表可在没有打开聊天请求的情况下看到新消息；用户阅读会话后按既有用户身份消除未读。

**验收：** 测试首次懒建会话、后续复用、重复发布至多一条消息、发布任一写入失败后恢复、删会话与发布竞态、删任务保留会话、浏览器未连接时仍能在重开应用后看到消息和未读。

## 阶段 5：聊天草案、显式确认与管理 API

**涉及：** `internal/app/runtime/capability`、聊天 runtime 装配、`internal/adapter/http/rag/chat_handler.go`、`internal/adapter/http/scheduledtask`（新建）、`cmd/server/main.go`。

- [ ] 在聊天 runtime 中增加“提出定时任务草案”的无外部副作用能力。仅当用户同时表达未来动作和触发时间/条件时提出；缺少条件轮询频率先追问。草案生成不启用任务，且不直接调用定时任务的运行入口。
- [ ] 生成可读预览与完整 prompt 展开内容，包含保存的浏览器时区、触发、来源权限、报告规则。浏览器时区从客户端明确传给服务器并写入草案；确认接口校验草案所有权、时效和版本，再原子写入任务。
- [ ] 增加列表、详情、运行记录、创建草案、确认、修改草案、暂停、恢复、手动删除 API。普通聊天和专用会话都能按用户管理任务；修改 prompt 必须重新分析派生规则并展示版本差异。等价任务默认提示现有任务，用户明确另建可越过该提示。
- [ ] 所有读写 API 校验当前用户；来源/工具权限以服务端数据为准，客户端和模型不能提交未经确认的扩权。管理界面删除是唯一永久删除入口；聊天“停止提醒”只产生暂停操作。

**验收：** API/聊天契约测试覆盖事实提问不建任务、缺频率追问、无确认不生效、重复确认幂等、跨账号拒绝、prompt 编辑后差异确认、直接请求绕过确认失败，以及普通聊天/专用会话管理同一任务。

## 阶段 6：前端任务管理与聊天确认体验

**涉及：** `frontend/src/router.tsx`、`frontend/src/pages/ChatPage.tsx`、`frontend/src/components/chat/*`、`frontend/src/components/layout/Sidebar.tsx`、`frontend/src/pages/ScheduledTasksPage.tsx`（新建）、`frontend/src/services/scheduledTaskService.ts`（新建）、前端类型定义。

- [x] 在聊天结果中展示任务草案卡片和明确确认/修改入口，显示触发时间、时区、来源范围、报告条件，并可展开 prompt；缺失时间或频率时继续提问。
- [x] 增加定时任务管理页：列表、状态、触发频率、prompt 编辑、配置差异确认、运行记录、专用会话入口、暂停/恢复与手动删除。查看运行记录不得把 `no_report` 渲染成“检查完成”。
- [x] 使用 `Intl.DateTimeFormat().resolvedOptions().timeZone` 获取浏览器时区，保存到草案；切换设备只展示已保存的任务时区，不自动覆盖。沿用现有 DailyBrief 的浏览器时区获取习惯。
- [ ] 在侧栏/会话列表展示应用内未读和后台新增的专用会话；确保普通聊天的 SSE 体验不被后台任务消息路径替换。

**验收：** 前端构建通过；浏览器走通聊天创建、确认、修改、暂停/恢复、删除会话导致暂停、后台消息出现与未读清除，以及不同设备时区下计划时间不漂移。

## 阶段 7：体育条件监测试点与离线评估

**涉及：** 任务模板/默认 prompt、runtime 已有 web 能力、`internal/app/scheduledtask` 的运行详情与评估导出；必要时增加一个只读体育官方来源能力。

- [ ] 配置至少一个体育事件任务和一个当前状态任务，检验创建时的事件基线、官方证据提示、频率、`report/no_report/uncertain` 和专用会话汇报。若现有 web 工具无法读到所选官方来源，再新增只读能力；不得为试点另建一条绕过 runtime 的采集流水线。
- [ ] 对纳入试点的任务保存模型结果、工具调用状态、计划/实际时间、配置版本及可追溯来源；离线抽样 `no_report`、`uncertain` 和已报告结果，对照官方赛程/公告标记漏报与误报。
- [ ] 试点评估不在线阻断报告，也不引入必查清单或“检查完成”状态。汇总运行成功率、模型不确定比例、技术失败、漏报、误报和重复发布；再决定是否需要下一版检查核验。

**验收：** 受控样例覆盖旧公告不触发、新公告触发、官方来源暂不可用产生可复盘 `uncertain`、状态曾满足但发布时不满足不发当前状态报告；抽样结果能追到具体 occurrence 和 attempt。

## 阶段 8：DailyBrief 迁移到通用任务

2026-10-03 历史：迁移代码及真实数据库/正式 worker 注入 runtime 回归已补齐，当时原库未切换；实现、交接协议与旧 E2E 失败见 `docs/superpowers/reports/2026-10-03-dailybrief-scheduled-task-migration.md`。

2026-10-06 完成：审批阻塞解除，修复订阅关闭、信号/配额及研究收尾提示和时间显示。最终正式服务真实模型到点、网页工具、双视图、重启去重、历史、未读、关闭订阅与浏览器验收通过，实库回归通过。原库 20 条订阅已交接，9090 服务以 scheduled 停用旧链路；历史/偏好未变，唯一今日失败保留并显式 missed，明天原时间运行。DailyBrief 范围 aligned，体育试点仍独立开放。前序失败、备份、部署与限制见 `docs/superpowers/reports/2026-10-06-dailybrief-migration-followup.md`。

**涉及：** `internal/bootstrap/dailybrief/runtime.go`、`internal/app/dailybrief/service/generation_orchestrator.go`、`internal/app/dailybrief/service/runtime_generator.go`、`internal/app/dailybrief/schedule/*`、DailyBrief 读写仓储、`frontend/src/pages/DailyBriefPage.tsx`。

- [x] 将现有订阅映射为每天触发的任务配置版本，保留用户时区、交付时刻、主题/来源偏好和历史 issue。建立旧订阅与新任务的稳定关联，不在迁移时给用户重复建任务或空会话。
- [x] 让 DailyBrief 的通用任务到点直接调用 `RunTask` 获取资料和生成结构化 artifact；移除本任务路径上的 `SourceCollector -> CandidatePipeline -> RuntimeBriefGenerator` 前置采集。保留页面所需 `headline/topSummary/sections/items` 输出与校验，用 runtime 产出的来源引用替代依赖候选列表的对齐规则。
- [x] 将同一次成功发布同时投影到 DailyBrief issue 页面与任务专用会话，建立日期/occurrence 幂等键。保留旧 issue 查询 API 和历史页面；新旧路径灰度时每位用户、每个日期只能有一个调度及一个发布所有者。
- [x] 在迁移验收完成后停用独立 DailyBrief schedule loop 和独立重试策略，将失败重试交给通用 occurrence/attempt 流程；删除旧链路之前先确认没有生产代码仍依赖它。

scheduled 部署不再装配旧采集器/生成器/loop/重试；checked-in 默认 mixed 和兼容代码仍保留供其他未交接环境使用，不在本轮删除。

**验收：** 相同订阅在新路径只产生一份 issue 与一条对应会话汇报；页面结构正常、历史可读、部分来源故障可说明覆盖缺口；关闭旧调度后 `cmd/server` 实际运行仍能到点生成，而不是仅单元测试调用生成器。

## 阶段 9：端到端交付审计

- [x] 依序验证：聊天提出与确认 → 数据库存储版本 → 后台扫描/认领 → `RunTask` 工具调用 → 三态结算 → 懒建/复用会话 → 未读与管理界面。至少一次测试在浏览器关闭时完成后台投递。
- [ ] 验证故障与竞态：进程重启、模型超时后的迟到结果、重复调度、配置更新、删会话、暂停/恢复、跨账号请求、来源权限撤销、发布中断后恢复。
- [ ] 验证体育试点复盘数据完整；确认运行记录中没有未经验证的“已检查全部来源”声明。DailyBrief 完成迁移时另做一次页面和调度真实装配审计。
- [ ] 运行受影响 Go 包测试、`go test ./cmd/... ./internal/... -run '^$'` 编译检查、`frontend` 的 `npm run build`，并在具备数据库与模型配置的环境执行浏览器端到端场景。只因具体失败扩大测试范围。
- [ ] 更新配置说明和用户可见的任务说明；记录最终选定的可修改默认值和试点指标。不要以文档、模块测试或未启动的后台循环宣称交付。

## 交付关口

**体育试点关口：** 阶段 0–7 与阶段 9 中对应的端到端验收全部完成。用户能从聊天确认体育监测任务，关闭浏览器后后台运行，并在应用内专用会话收到一次且仅一次的报告；管理界面能解释 `no_report`、`uncertain` 和技术失败。离线复盘可用。

**统一任务关口：** 阶段 8 及 DailyBrief 专项端到端验收完成。旧 DailyBrief 独立调度已停用，页面与会话读取同一次通用任务发布，历史仍可访问。届时才能说 DailyBrief 已成为普通的每日定时任务。

2026-10-06：统一任务关口已通过；体育试点关口仍未通过，不能外推为通用任务整体完成。
