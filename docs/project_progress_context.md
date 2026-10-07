# 项目进展上下文

更新时间：2026-10-07。本文件供后续开发接手，记录当前代码、已验证行为和未完成工作，不代表整项功能已全部交付。

Git 整理（2026-10-07）：按用户授权将累积的代码、迁移、测试、规格与报告整理为提交，并单独记录仓库清理。停止跟踪 `frontend/node_modules`、Python 缓存、TypeScript 构建缓存、生成的 Vite 配置及 `.claude` 本地配置，保留磁盘副本并补齐忽略规则；临时验证证据仍留在忽略的 `tmp`。本次 `go test -p 1 ./cmd/... ./internal/... -count=1`、显式使用 `vite.config.ts` 的前端构建和 8 项聊天行为测试通过，未配置专用 DSN 的数据库用例不计入数据库验收。下文及历史报告的“未提交”描述保留当时状态，不再代表本次整理后的 Git 状态；提交与远程同步不代表部署或补齐尚未完成的验收关口。

最近优化：普通 Chat 执行过程改为轻量时间线。前端按既有 SSE 到达顺序记录多段 Thinking / Tool Call / Tool Result / 中间正文，思考执行时展开、完成后折叠；工具结果弱化且完整输出可展开，长执行收起较早记录但保留失败步骤。最终正文保持完成事件校准、复制及引用语义。流式自动跟随尊重用户上滚位置，明暗与 390px 窄屏、8 思考 / 15 工具长执行浏览器验收通过；8 项真实 SSE reader / 状态回归和前端 build 通过，类型基线仍有 42 项诊断，无新增。详见 [执行过程视觉报告](superpowers/reports/2026-10-07-agent-execution-visual.md)。仅前端改造，未提交或部署，未追加真实模型调用；执行段暂不持久化，Work 独立界面未改造。

最近修复：普通 Chat 深度思考开关已接入后端。每次请求的 `deepThinking` 通过 HTTP、ChatService、runtime 传至各模型轮次，硅基流动显式接收 `enable_thinking=true/false`，共享实例不修改全局开关；开启时 reasoning 经独立 thinking SSE 展示。HTTP 参数、admission、多轮及并发隔离测试，专用库正式 HTTP 到本地模拟模型协议和 SSE 全链路验证、发布恢复回归及全仓编译通过，前端行为测试 4 项通过。随后按用户授权完成一次真实 SiliconFlow DeepSeek-V4-Flash / 正式 Chat 浏览器联调：开启请求产生 122 段思考，结束折叠可展开；110 段正文、finish、数据库与刷新页面正文一致。使用隔离库并关闭定时任务及画像观察，未重启原应用。详见 [深度思考开关报告](superpowers/reports/2026-10-07-chat-thinking-switch.md)。未提交或部署；thinking 历史展示、Work 开关仍未实现。

最近修复：Chat / Work 成功执行的完整可见正文已统一保存与展示。runtime 按 journal 顺序汇总各轮 `answer_delta`，包含工具调用前后的正文，thinking 和工具结果独立保留；发布完成事件携带实际保存的完整正文，前端用它校准流式结果，旧完成事件兼容。长消息原文用于页面展示与来源提取，模型历史仍使用摘要；Work 列表及原始讨论依据同步处理。修复普通 Chat 断线重连 URL 的双问号。新建专用 PostgreSQL 库的持久化、发布恢复、重复发布、正式 HTTP 重载，Work 完整套件及相关 Go/前端行为回归通过，全仓编译和前端 build 通过。TypeScript 既存基线仍失败，本轮新增诊断为 0。详见 [正文一致性报告](superpowers/reports/2026-10-07-chat-visible-body-consistency.md)。未提交或部署，无数据库迁移或历史批量回填；本次模型为可控测试模型，未做真实模型/浏览器验收。

最近完成：DailyBrief 已完成原库迁移。20 条订阅全部显式绑定，ragent 服务在 9090 以 scheduled 模式运行，旧采集、独立调度及旧重试不再装配；全部安排在保存时区的 2026-10-07 原时刻。140 ready / 320 failed issue、700 条目及订阅偏好摘要保持一致，未因交接新建会话。按用户选择保留唯一今日失败，并记录 2026-10-06 missed。修复订阅关闭、输出信号/配额与研究收尾提示、生成时间读取映射；最终真实模型到点、正式 HTTP/CLI、浏览器及真实库回归通过。此前审批阻塞已解除，前序失败证据保留。DailyBrief 范围 aligned，体育试点仍开放。启动、备份与限制见 [迁移完成报告](superpowers/reports/2026-10-06-dailybrief-migration-followup.md)。未提交或重置工作区。

最近修复：Chat / Work 尚未保存最终答案的新受管执行已有持久所有权与中断收敛。普通 admission 和 Work accepted 原子预留执行，模型开始前领取 owner / epoch，默认租约 3 分钟、每分钟续租；启动 / 每 5 秒扫描及重连回收失效执行为 interrupted，保留已保存 Work 内容并阻止旧执行迟到写入。取消 / 删除与已有 ready 答案分别保留原语义，不自动重跑模型或工具。专用 PostgreSQL 的并发领取 / 恢复、心跳、迟到成功 / 失败、事务故障 / 过期、panic、实际子进程退出、Chat / Work HTTP 和恢复循环测试通过；相关回归及全仓编译通过。详见 [Chat 执行所有权报告](superpowers/reports/2026-10-04-chat-execution-ownership.md)。未提交或部署；历史无执行关联的数据回填、真实模型 / 浏览器及多节点验收仍待处理。

最近修复：P1 第二项文档文本 / chunk 执行所有权已接入正式入口。处理意图先持久化，再由本地 worker 领取；owner、epoch、租约与心跳限定发布权，chunk / 向量 / 摘要 / 日志 / 状态原子提交。启动扫描恢复普通 pending，已领取的失效执行显式 interrupted，旧执行迟到成功或失败都不能覆盖新结果。远程刷新文件元数据随结果事务切换，旧占用和文件清理不能影响新结果。专用 PostgreSQL 的并发、心跳、提交故障 / 租约过期、配置与资源撤销、真实子进程退出、正式 HTTP / bootstrap 恢复以及 Chat / Work 回归通过，全仓编译通过。详见 [文档执行所有权报告](superpowers/reports/2026-10-04-document-chunk-ownership.md)。未提交或部署；已开始的模型步骤不自动续跑，真实模型 / 浏览器 / 多节点滚动重启未验收，embedding profile 与 ID 等后续项仍待处理。

最近修复：P1 第一项的 Chat / Work 最终答案发布收敛已接入正式入口。answer-final、执行完成与 pending publication 原子保存；助手消息、分块、episode、Work turn 与带 messageId / sources 的完成 journal 原子发布。启动扫描和热冷重连恢复已有答案，不重跑模型或工具。专用 PostgreSQL 下故障注入、8 路并发、真实子进程退出、启动恢复、删除/取消及 P0 回归通过，全仓编译通过。详见 [发布恢复报告](superpowers/reports/2026-10-03-chat-publication-recovery.md)。未提交或部署；最终答案之前的新受管 running 已由上述后续修复补齐，历史无关联数据回填、真实模型/浏览器验收仍未完成。

最近修复：工程评审 P0 的 Chat / Work 跨入口流归属缺口已关闭。普通续流/停止先查持久用户、会话及用户消息，删除或 Work 执行不能凭热缓存绕过；普通 Chat 在 meta 前保存执行身份，异步运行复用并复查访问。实际 `codex_work_20261001` + 正式 HTTP + 共享流/可控模型验证双用户、缓存热冷、Work accepted、删除、实际停止及单条用户消息/单个 session，通过相关套件和全仓编译检查。交付边界见 [P0 修复报告](superpowers/reports/2026-10-03-chat-stream-ownership-fix.md)。未提交或部署；embedding profile、ID 等仍待处理，聊天发布恢复和文档执行所有权状态见上文。

首次迁移实现（2026-10-03 历史）：显式绑定、直接 RunTask、issue/消息/未读原子发布、日期去重及新旧数据库门槛已接入；当时只有注入 runtime 的实库证据，原库尚未切换。2026-10-06 的实际完成结果见上文。checked-in 默认仍为 mixed，原库部署明确使用 scheduled；其他未交接环境不会因启动自动迁移。旧 DailyBrief 两个 E2E 的既存 fixture/fallback 断言失败未修改。历史实现与安全交接协议见 [首次迁移审计](superpowers/reports/2026-10-03-dailybrief-scheduled-task-migration.md)。

最近完成：Work 第一版六阶段实施与相关原入口回归已完成，业务承诺交付核对为 **aligned**；最终补齐建议去重、文档建议按接受版本重放、管理页配置审核模式、混合向量维度检索，以及私有范围和画像观察回归。证据见 [Work 交付报告](superpowers/reports/2026-10-02-work-delivery-audit.md)，勾选状态见 [实施计划](superpowers/plans/2026-10-01-work-topics.md)。全量前端类型基线、既有定时任务后续范围和原有数据库事件的数据处置仍未完成。本轮未提交、合并或部署。

## 工作区与开发约束

仓库为 `D:\goagent`，服务主入口为 `cmd/server`。本轮开始前已有大量未提交修改和未跟踪文件；最近核对 `git status --short` 约 808 项。不能把现有脏文件都视为本轮改动，也不能清理、重置或覆盖其他工作。本轮未提交代码。

继续开发前先阅读本文件、定时任务规格和实施计划，对照相关代码及 git 状态。不要重新设计已确定的任务基座。除用户另有指示，不擅自提交、清理或重置工作区。

## 知识库文档处理的现状

知识库文档入库已经统一为 `DocumentProcessService` 的 chunk 任务。`internal/app/ingestion` 及其 HTTP 路由、运行时装配、队列、仓储和前端管理页已移除。迁移 `20260909110000_remove_ingestion.sql` 将遗留文档、分块日志的 `process_mode` 归一为 `chunk`，删除 `pipeline_id` 及历史 ingestion 表。

当前链路：

1. 上传文件或登记 URL，创建待处理文档。
2. 用户触发分块，原子保存 durable chunk job 和文档 running，本地队列或启动扫描调用 `DocumentProcessService.ExecuteChunk` 领取带 owner / epoch 的任务并续租。
3. 读取对象存储内容，按文件类型解析为文本。
4. 按配置执行父子分块（默认）或普通分块，为子块生成内容向量，父块用于检索回溯。
5. 同一处理链路调用 LLM 生成文档摘要、chunk 摘要和每个 chunk 最多两个预测问题。
6. 为预测问题生成向量，保存来源 chunk、原文和父块关联；提取关键词并写入向量元数据。
7. 在持久化事务中校验当前任务、租约和文档快照，替换文档 chunk 与向量，原子写回摘要、处理日志、文档状态与任务 completed；失效执行不能提交。已领取的过期任务显式 interrupted，普通 pending 可由新 worker 恢复。

LLM 不可用时，内容分块与内容向量仍可完成，文档摘要标记为 `degraded`；单个 chunk 的问题生成或问题向量失败不阻断主流程。自动 wiki 生成不在当前入库范围，既有 wiki 浏览能力保留。

## 通用定时任务：目标与交付状态

相关文档：

- 规格：`docs/superpowers/specs/2026-09-30-scheduled-agent-tasks-design.md`。
- 实施计划：`docs/superpowers/plans/2026-09-30-scheduled-agent-tasks.md`。
- 首轮交付审计：`docs/superpowers/reports/2026-10-01-scheduled-task-delivery-audit.md`。

通用任务主体、聊天管理、管理页、后台运行和应用内发布已接入，并完成多项真实模型、HTTP、浏览器及数据库验证。用户已验证简化后的界面通过。整体仍为 **partially aligned**：体育事件试点的离线评估及部分异常全链路场景尚未完成；DailyBrief 的统一任务关口已在 2026-10-06 通过。审计报告记录首轮及名称/编辑界面的证据；其后增加的详情布局和显式状态按钮见本文件。

### 已确定的产品语义

- 聊天是创建和管理任务的主要入口，另有专门管理页。只有明确的未来动作与触发时间或条件才进入创建；条件监测缺少频率等必要信息时先追问。
- 创建和修改先生成完整草稿，用户明确确认后生效。名称、固定 prompt、频率、时区、来源及工具范围等保存为不可变配置版本。模型识别结果不能静默修改配置。默认检查重复任务，用户可明确确认另建一份。
- 创建使用浏览器 IANA 时区，之后按保存的时区执行，旅行或换设备不自动改时区。
- 每次到点将任务固定 prompt 注入 `internal/app/runtime.RunTask`，使用任务隔离历史；普通聊天和专用会话中的闲聊不进入任务执行历史。通用任务不预先启动独立资料采集器。
- 模型使用已确认范围内的只读工具获取资料，服务端执行知识库、网页域名、工具及重定向限制，默认优先官方或一手证据。
- 模型返回 `report`、`no_report` 或 `uncertain`。提醒和周期汇总不能使用 `no_report`。事件监测以确认版本的基线区分新旧事件，状态监测检查当前状态。试点不设置在线“模型是否真正检查完”判定；`no_report` 不能解释为已查全或事件不存在。
- 首次需要报告或状态反馈时才创建专用会话，之后复用。删除专用会话会暂停任务并清除关联及未读，保留配置和运行历史；在途结果可以保存，但不能发布到被删除会话。恢复后按需创建新会话。
- 已完成任务可重新启用，创建新配置版本和确认基线，复用仍存在的会话；暂停任务恢复沿用当前版本。过期的一次性任务需先编辑执行时间。只有管理页手动删除才真正删除任务，原汇报会话不随任务删除。
- 自动运行只允许写应用内任务状态、运行记录、会话消息和未读，不发送邮件或执行外部写入。条件监测长期无实质消息可低频反馈，用户不回复不会自动暂停。
- 技术失败只在本次有效窗口内有限重试；多实例领取和发布去重；旧配置版本结论不发布；过时一次性任务跳过，不复杂补跑。

### 后端已实现

- `internal/app/scheduledtask/domain/`：once、interval、daily、weekly、monthly 调度，时区与 DST、版本/状态门槛和严格结果解析。
- `internal/app/scheduledtask/service/executor.go`：固定 prompt、任务历史、工具和来源作用域，运行前及发布前复查知识库权限。
- `internal/app/scheduledtask/service/proposer.go`：none、clarify、draft、manage 意图，生成任务名称和草稿；HTTP 传入当前用户任务摘要。立即查询、未来事实提问、缺少频率的监测不能因存在相似暂停任务而误判为恢复。
- `internal/app/runtime/task.go` 及 capability、adapter/runtime：任务/版本/计划运行/尝试身份，工具过滤、网页域名和重定向限制、原始结果与 journal、失败时保留 runtime session。
- PostgreSQL 仓储：草稿确认、重复检查、不可变版本、计划运行/尝试、租约续期、重试、幂等发布、未读、低频反馈、连续失败通知及公平扫描。旧版本在途结果保存为 `superseded`。
- `internal/bootstrap/scheduledtask/runtime.go` 与 `cmd/server/main.go`：启动 worker，扫描、领取、并发执行和发布，注册受保护的 `/api/ragent/scheduled-tasks` 路由。
- 会话删除事务同步暂停关联任务、清除会话关联和未读。
- 报告消息保存结构化来源，新建专用会话使用任务名称。旧无名称任务用简短兼容标签，不覆盖 prompt，也不批量改旧会话标题。
- 迁移包括 `20260930110000_create_scheduled_task_tables.sql`、`20260930120000_scheduled_task_feedback.sql`、`20261001100000_scheduled_task_name.sql`。

真实硅基流动调用发现：本轮运行模型在 JSON 模式下会压制工具调用，个别输出甚至声称已获取网页。当前实现先以普通模式执行工具循环，再使用同一上下文和证据生成最终 JSON；没有加入在线资料覆盖门槛。严格结果解析仍会拒绝自由文本与 JSON 混杂的结果。

### 前端已实现及最近修订

主要文件为 `frontend/src/pages/ScheduledTasksPage.tsx`、`frontend/src/services/scheduledTaskService.ts`、`frontend/src/lib/scheduledTaskPreview.ts`，以及 ChatPage、侧栏和 Header。

- 列表显示独立任务名称、执行时间、下次运行和中文状态，不把固定 prompt 直接作为名称。
- 编辑使用弹窗，基础字段为名称、任务内容、频率和时间；时区、汇报规则、网页域名及知识库收进默认折叠的高级设置。工具勾选控件已移除，服务端仍限制已确认的只读工具范围。
- 新草稿可由模型自动命名，手动名称优先保留；所有修改仍需预览与明确确认。选择资料范围所需的配套只读查询能力也进入待确认配置，不绕过确认直接扩大范围。
- 聊天处理管理意图，暂停/恢复须明确确认，修改展示差异草稿。修复首条聊天建会话时丢失任务提议，以及显式汇报会话路由被旧 currentSessionId 导回旧聊天的问题。
- 列表与详情直接显示状态操作：已安排可“暂停”，已暂停可“恢复运行”，已完成可“重新启用”；暂停/恢复不再藏在“···”菜单里。管理按钮点击本身是明确操作，不自动替用户恢复任务。

详情布局按用户提供的参考页面实现：点击任务名称，左侧保留任务列表，右侧先显示**最新已发布的实质汇报**，再显示**历史运行记录**；编辑仍使用独立弹窗。

- `/scheduled-tasks?taskId=...` 支持直达详情。该链接现在打开详情，不再自动弹出编辑框。
- 新增 `GET /scheduled-tasks/:taskId/latest-report`。仓储独立查询已发布的 `report`，不受最近运行列表的 100 条限制；后续 `no_report`、失败或旧版本未发布结果不能把此前报告隐藏或替换。
- 最新报告按 Markdown 展示正文与来源；旧版本报告标注“来自此前配置”。当前时间标签取计划运行时间，不是实际消息发布时间。
- 未曾发布报告时显示“尚无汇报”，不创建空会话。低频反馈和失败通知仍在汇报会话内，不作为最新实质报告。
- 历史目前只列最近 **100 次**，未增加分页；展示无新消息、结果不确定、失败、错过等状态。点击展开该次不可变配置、模型结果、来源、技术错误、重试尝试、原始输出和工具记录。
- 删除会话后仍可从运行记录读取保留的报告；没有原会话时不显示会话入口，不重建或重新发布。查看详情不会清除会话未读，打开汇报会话沿用现有已读流程。
- 切换任务或运行记录时丢弃迟到请求，避免显示前一个任务的结果。

## 已验证结果及证据边界

- 相关 Go 测试通过；此前全仓库编译检查 `go test ./cmd/... ./internal/... -run '^$'` 通过。这不是全仓库全量功能测试。
- 专用 PostgreSQL round5 库的生命周期、失败通知/恢复、低频反馈、并发领取和幂等发布恢复测试通过；此前同库重复执行 `-count=2` 通过。
- 最新详情查询补测通过：连续 101 次无消息运行不遮蔽既有报告；跨用户读取被拒绝；删除汇报会话后报告历史仍可读。
- 真实模型、HTTP、浏览器验证过提醒在浏览器关闭和后端重启后投递、重新打开阅读清未读、跨设备时区、聊天管理确认、管理页修改差异确认及来源展示。
- FIFA 当前状态样本实际 `web_fetch` 获取了官网 HTML 并产生 `report`；事件样本实际调用 4 次搜索、2 次抓取并返回 `no_report`，没有发布消息或创建会话。前者仅验证官网可访问，后者不证明无事件或资料查全，不能据此宣布体育监测试点可靠。
- 真实提议模型 9 种中文表达复测均符合预期；仅为小样本，不代表全面意图召回率。
- 简化编辑界面经过浏览器验证并获用户验证通过：名称、折叠高级设置、无工具勾选、确认前不生效、改名不改变状态和隐藏作用域。
- 最新详情布局浏览器验证通过：报告、来源、历史、直达链接、独立编辑弹窗、无报告空状态，无相关 pageerror。证据为 `tmp/scheduled-task-detail-e2e.mjs`、对应 JSON 与两张截图。
- 显式暂停/恢复/重新启用按钮复用既有已验证 API，最近前端构建通过；新增按钮布局尚未单独进行点击改变状态的浏览器复测。
- 最近 `npm run build` 通过（3615 个模块）。全量 TypeScript 检查仍有其他既存文件约 151 行诊断，最近检查 ScheduledTasksPage、scheduledTaskService 无匹配诊断；不能称全量类型检查通过。

原始脚本、JSON、工具结果和截图保存在 `tmp/scheduled-task-*`，属于临时验证证据，不是长期交付资产。稳定的首轮观察和样本身份见交付审计报告。

## 历史定时任务验收环境与复测方法（2026-10-01）

以下是定时任务阶段的历史快照，不能据此启动当前 Work 预览或停掉同 PID 的其他进程。Work 当前验收库、启动保护、端口和最终证据见后文。

- 后端已重启为 `tmp/scheduled-task-e2e-server-v14.exe`，PID 17084，端口 **9090**；包含最新汇报接口。日志为对应 `.out.log` / `.err.log`。PID 和运行状态仅为当时快照，后续需重新检查。
- 前端 Vite PID 38548，端口 **5173**，页面为 `http://localhost:5173/scheduled-tasks`；状态按钮为前端修改，不需要因此再重启后端。
- 当时后端连接隔离测试库 **`codex_scheduled_20261001_round3`**；数据库集成测试使用 round5，round2/4 也保留。该定时任务验收阶段的测试任务未写入原有 `ragent` 库；之后 Work 验收曾误连接原有库并造成后台写入，见后文事件记录，不能将此前阶段的结论外推到整个实施过程。
- Docker 依赖已启动，外部模型使用硅基流动，配置/密钥来自 `.env`。提议模型为 `Qwen/Qwen3-32B`，本轮运行模型为 `deepseek-ai/DeepSeek-V4-Flash`。加载配置时不要输出密钥。

Go 复测先设置 `$env:GOCACHE='D:\goagent\tmp\codex-gocache'`。相关包测试可执行：

```powershell
go test ./internal/app/scheduledtask/... ./internal/adapter/repository/postgres/scheduledtask ./internal/adapter/http/scheduledtask ./cmd/server -count=1
```

数据库集成用例需要 `SCHEDULED_TASK_TEST_DSN` 指向专用测试库，否则会跳过。前端 `VITE_API_BASE_URL` 为 `/api/ragent`。

系统 npm.cmd 固定 Node 20，与 npm 12 不兼容。验收用 bundled Node 24 执行 `C:\nvm4w\nodejs\node_modules\npm\bin\npm-cli.js run build`，Node 路径为 `C:\Users\1\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe`。构建和浏览器子进程可能需要沙箱提升权限；定时任务阶段未安装或升级依赖，之后 Work 阶段已新增 Tiptap 编辑器依赖并更新 lockfile。类型检查应指向 `frontend/tsconfig.app.json`，仅检查根 tsconfig 不会检查实际应用文件。

浏览器验证使用已安装 Playwright 和 Chromium，通过 `.mjs` 脚本运行。历史 round3/PID 不作为当前启动或停进程依据；后续使用下文受保护的 Work 启动脚本，重新核对实际监听和进程路径，不停掉未知服务。

## 调度默认值

`configs/application.yaml`：扫描 15 秒，每批 20，并发 4，租约 900 秒，一次结果窗口 600 秒，周期结果窗口 1800 秒，最多 3 次尝试，重试间隔 60 秒。日/周/月反馈间隔分别 7/30/60 天，连续失败或不确定通知阈值为 3 次**计划运行**，不是 3 次重试。

间隔不超过一天的监测使用日反馈默认，否则使用周反馈默认。DST 缺失时刻取当天第一个不早于请求的有效本地分钟，重复时刻取第一次；月末不存在的日期跳过。草稿有效期当前固定 24 小时。运行记录保留配置和独立可配置调度抖动尚未落地。

## Work 第一版：实施与交付状态

### 已交付范围（2026-10-02）

已确认的单人与 AI 长期协作专区已接入正式 `/work`、`/work/:topicId` 和受保护 API。创建主题、事项归属、多段对话与明确接续、已确认进展、应用内共同文档、私有资料/链接/附件、归档恢复与删除会话保留边界均已落地。六阶段清单见 [实施计划](superpowers/plans/2026-10-01-work-topics.md)，证据及限制见 [交付报告](superpowers/reports/2026-10-02-work-delivery-audit.md)。

共同文档采用 Tiptap 3.31.4（MIT）、结构化 JSON、稳定块 ID 和不可变版本。人先保存，再交 AI 完成本轮；AI 局部修改保留未触及标题/列表/表格。保存/恢复均采用 CAS，迟到建议保留拟修改正文而不覆盖人写内容。明确创建/修改可直接保存；普通讨论仅产生可预览建议，由人应用文档或确认进展。相同建议跨轮次去重，已确认的相同内容不重复打扰。文档修改不自动确认进展。

后端主要为 `internal/app/work`、`internal/adapter/{http,repository/postgres}/work`、`internal/bootstrap/work`，通过 `cmd/server` 复用 shared runtime/SSE/journal、原会话和 chunk 处理队列。新增 5 项 Work 迁移。私有资料按用户/主题/会话范围约束，默认全局检索、普通聊天与直接 ID 读取排除；Work 不写全局画像/记忆，普通扫描排除 Work。删除会话先撤销本次附件访问再重试清理，保留文档、已确认进展及已加入主题资料，旧 turn/消息不再可读。

界面已实现主题卡片首页、合并导航/聊天/进展文档资料面板、可记忆收起状态、扩大富文本编辑、版本查看与恢复、草稿保留、深色和窄屏返回原聊天。资料上传、处理/失败状态、引用及原文件下载均为实际后端流程。

真实模型与浏览器验证了学习计划多轮协作、技术选型资料→模拟反馈→文档建议→独立确认决定、周报/客户事项隔离及 10 月 2 日接续 10 月 1 日的事项。真实解析/向量/存储验证 TXT、URL 与私有引用，管理员另一身份和匿名访问被拒绝。合成故障注入结合实际数据库/正式 HTTP 验证写成功后回复/助手保存失败的持久化结果、重放、不多建版本和迟到 patch。

完整 Go 测试、专用库 Work/定时任务套件、前端 build 通过。全量前端类型检查仍有 151 行既存错误，Work 无错误；数据库组合套件用 `-p 1`，避免同机现有 ID 生成器跨测试进程碰撞。验收库为 `codex_work_20261001`，Go 数据库用例独立运行迁移；浏览器临时脚本部分有前置对象依赖，不宣称全部独立运行。Work 业务承诺核对为 **aligned**，六阶段清单完成。最后补齐普通聊天知识库选择/实际检索、原管理页上传分块、画像观察仓储普通领取/Work 排除、任务最新汇报/历史详情/关联会话展示。画像回归用真实 DB 和服务测试；汇报 UI 用合成 outcome + 真实发布，不称为真实模型生成了画像或执行该样本任务。Work 专属每种 Word/PDF/图片格式也未逐一实际验收。多人、文件转换、Work 调度关联、永久删除主题仍不在第一版。

原知识库最终回归文件 `tmp/work-legacy-ui-final-result.json` 全链路通过；先前一次分块因预览重启中断的失败保留在旧结果，不能表述为一次无失败通过。任务展示证据为 `tmp/work-report-ui-delivery-result.json`。

原有任务管理回归发现完整配置预览与聊天意图模型规则冲突，已为 `ReviewConfig` 增加内部配置审核模式，仍需完整 draft、严格校验和人工确认；聊天意图门槛不变。实际模型预览/确认/暂停/恢复/删除及原 KB/任务页面已通过。

最后的组合回归发现：真实共享库的 4096 维向量与权限用例的 1024 维查询混合时，默认检索会报维度不匹配。`internal/adapter/vectorstore/pgvector/vector_store.go` 已在距离计算前按查询维度过滤；`source_integration_test.go` 新增同一范围内不同维度向量的验证，匹配命中和私有排除均通过。`tmp/work-mixed-vector-entrypoint-result.json` 证明最终修正版的真实模型/默认共享检索正常。此修正不表示不同模型的同维度向量天然可比较，也不代表自动迁移旧向量。

最终验证证据：

- `tmp/work-backend-delivery-final.log`：最后修正后的 `go test ./cmd/... ./internal/... -count=1` 通过；未设置 DSN 时跳过的数据库用例不计入数据库验收。
- `tmp/work-db-delivery-complete.log`：设置专用 `WORK_TEST_DSN`、`SCHEDULED_TASK_TEST_DSN`，串行运行 Work/装配/定时任务数据库组合套件通过，覆盖最新去重、画像排除及混合维度修正。
- `tmp/work-frontend-delivery.log`：前端 build 通过；`tmp/work-typecheck-delivery.log` 为 151 行既存类型诊断，退出码 2。
- `tmp/work-intent-delivery-result.json`、`tmp/work-delivery-followup-result.json`：真实模型权限分类/澄清、跨自然日接续、任务配置审核与管理回归通过。
- `tmp/work-legacy-ui-final-result.json`、`tmp/work-report-ui-delivery-result.json`：原知识库选择/上传分块/检索、合成结果的真实任务发布与汇报 UI 通过。失败与模拟边界保留在交付报告，不将不同验证层次混为一次真实模型执行。

证据文件在 `tmp`，可能被后续清理；持续接手以交付报告、实际代码及 Go 回归用例为准。配置事件修正后的最终验收仅使用专用库；本次文档同步没有运行服务或修改数据库。

### Work 验收配置事件与后续环境

2026-10-01 曾因使用错误环境变量与 `gotenv.OverLoad` 覆盖配置，误启动服务连接原有 `ragent`：新增 9 项迁移、20 份降级简报（89 条目）及 1 个失败运行，管理员画像更新至 v7。该进程已停止；已核实与未排除影响见 [事件记录](superpowers/reports/2026-10-01-work-preview-config-incident.md)。没有自动回滚。继续开发不授权删除原有数据或覆盖画像。

后续预览使用 `scripts/start-work-preview.ps1`，仅允许 `codex_work_` 专用库；独立目录、实际 `SPRING_DATASOURCE_*` / `SERVER_PORT`、迁移前数据库身份校验 `APP_EXPECTED_DATABASE`、`APP_DISABLE_SCHEDULED_JOBS=true`。`.env` 已改为只补缺省，不能覆盖进程配置。不要复用本文件前面的旧临时服务 round3 启动建议；Work 后续只能用受保护脚本和专用库。当前未提交/部署，不能把未部署误写为原有库没有变化。

### Work 本地预览与复测入口

最近启动确认的预览为前端 `http://127.0.0.1:5175/work`、后端 `http://127.0.0.1:9091/api/ragent`，数据库 `codex_work_20261001`。前端使用 `frontend/work.e2e.config.mjs` 代理到 9091；最后确认前端 HTTP 200、未登录 Work API HTTP 401。临时进程不保证持续运行，后续先检查端口和进程实际路径，不复用历史 PID。此前运行的最终二进制为 `tmp/work-preview-delivered.exe`；继续开发后应重新编译，不能默认旧二进制包含新代码。

```powershell
$env:GOCACHE='D:\goagent\tmp\codex-gocache'
go build -o tmp/work-preview.exe ./cmd/server
./scripts/start-work-preview.ps1

# 两个 DSN 必须事先设置为专用隔离库；不在文档中保存密码。
go test -p 1 ./internal/adapter/repository/postgres/work/... ./internal/bootstrap/work/... ./internal/adapter/repository/postgres/scheduledtask/... -count=1
go test ./cmd/... ./internal/... -count=1
```

同机独立测试进程默认共享 Sonyflake 机器 ID，数据库组合套件必须串行，或使用分别隔离的数据库；不要将跨测试进程主键碰撞误判为 CAS 失效。知识库 chunk 队列当前在内存中，预览重启会中断正在处理的文件；先等处理结束再重启。只读检查或复测前，核对数据库身份，不输出 `.env`、模型密钥、登录 token 或画像正文。

## 未完成工作与建议接续顺序

Work 第一版业务清单已完成；接下来需要分别处理全量 TypeScript 既存基线、各类 Word/PDF/图片 Work 上传的补充实际验证，以及原有数据库事件的影响核对和具体处置授权。原有数据尚未回滚，不能自动删除简报、回滚迁移或覆盖画像。多人、上传文件转共同文档、Work 与定时任务关联、永久删除主题属于后续产品范围，不应作为本版漏项恢复实现。

### 既有定时任务接续顺序

1. 体育事件离线试点：建立已知发布时间的官方公告对照，验证确认前旧事件不触发、确认后新事件触发、条件不满足和来源不足；离线标注漏报、误报、重复发布及稳定版本成功率。现有官网访问样本不足以通过该关口。
2. 补真实知识库选择/检索/撤权，以及运行时暂停、删会话、改版本、模型超时迟到结果、发布事务中断的全链路场景。已有模块和数据库证据不等于真实场景全部通过。
3. 管理体验后续可改进：版本冲突时保留编辑输入并展示差异；历史分页；更清楚的错过一次任务及不可恢复原因。目前版本冲突只返回错误，历史最多 100 条。这些未全部实施，不要混同本轮已完成的详情布局。
4. 明确并实现运行记录保留配置及调度抖动默认规则，再对照计划逐条核对复选框。
5. DailyBrief 阶段 8 已完成：原库 20 条稳定绑定、真实 RunTask 到点生成、同一次 issue/会话发布、历史/工具详情/关联会话浏览器验收和实际旧调度停用均有证据。原库 9090 为 scheduled；重启使用 `scripts/start-dailybrief-server.ps1 -DatabaseName ragent -Port 9090 -Mode scheduled`，不要照抄历史 PID。保留的今日 failed 已明确为 missed，明天按原时刻运行；不删除或补造历史。启动、备份与失败证据见 [迁移完成报告](superpowers/reports/2026-10-06-dailybrief-migration-followup.md)。单样本不证明资料查全，多节点滚动升级和体育试点仍独立开放。

后续宣布交付前按 spec-delivery-audit 核对真实入口、默认配置、结果发布、测试独立性和未满足的关口，不能只凭模块测试或本次界面验证称全部完成。

## 文档维护（2026-10-01）

文档入口见 `docs/README.md`。已清理 27 个过期且未带未提交修改的已跟踪文档，包括旧 agent 接入/引擎计划、旧 ingestion 流水线施工文档、早期评估与综合待办、一次性摘要实验及过期联调计划。保留当前规格、报告、检索样本与仍有代码对应的历史设计；对保留的 wiki、llmgen、并发门控设计标注旧链路失效范围。对话运行时规格的“未实施”状态已修正为分阶段实施。根 AGENT.md 的旧接入文档引用改为当前文档入口，并注明旧架构段落已被现状取代。删除清单与依据记录在索引中；未清理或重置代码工作区。
