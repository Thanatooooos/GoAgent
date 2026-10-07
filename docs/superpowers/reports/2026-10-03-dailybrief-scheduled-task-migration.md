# DailyBrief 通用任务迁移：代码与切换审计

日期：2026-10-03。结论：**partially aligned**。迁移代码、真实 PostgreSQL 事务及正式 worker 的注入 runtime 回归已完成；尚未做真实模型与浏览器联合验收，没有部署或迁移原有订阅，统一任务交付关口保持开放。

2026-10-06 完成：审批阻塞已解除，真实模型到点、HTTP/CLI、浏览器和实库验收通过；原 ragent 的 20 条订阅已显式交接，9090 服务以 scheduled 运行并停用旧链路。历史与偏好未变，唯一今日失败按用户选择保留并记录 missed，明天原时间运行。DailyBrief 范围 aligned，体育试点仍开放。本报告以下保留 10 月 3 日的历史事实；最新部署、修复、前序失败和限制见 [迁移完成报告](2026-10-06-dailybrief-migration-followup.md)。

## 本轮核对与范围

已阅读 `docs/project_progress_context.md`、2026-09-30 通用定时任务规格/计划和 2026-10-01 交付审计，并从 `cmd/server`、DailyBrief bootstrap、通用 worker/executor 和发布事务核对实际链路。开始时 git 工作区有 892 条既存状态记录。本轮只编辑迁移代码、针对性测试和接手文档；未提交、清理、重置，未改旧候选采集器、旧 runtime generator、既存 E2E fixture 或前端源码。构建产物写入 `tmp/dailybrief-frontend-build`。

原 `ragent` 于北京时间 18:16 的显式只读事务检查：20 个订阅全部 enabled，0 个有效旧租约；103 份 ready issue、317 份 failed issue，最新 ready 日期 2026-10-01；通用 task/occurrence 均为 0。迁移截至 Work 的 `20261001160000_work_history_summary.sql`，不包含本轮新迁移。Docker 原未运行，本轮启动现有 Docker Desktop；没有启动连接原库的应用 server。原库及 template1 存在排序规则版本提示，未处置；验收库从 template0 创建。

## 实现与正式装配

- `20261003100000_daily_brief_scheduled_task.sql` 添加版本的 `daily_brief_json` 和每用户唯一的 `t_daily_brief_task_binding`。安装迁移本身不转移任何订阅。数据库触发器拒绝已绑定用户的旧租约及旧 issue 写入，也拒绝旧客户端静默修改已迁移订阅。通用任务生命周期/编辑通过事务末尾同步保存时区、时间、主题、来源和 enabled；会话删除导致的暂停也覆盖。
- `Store.CutoverDailyBrief` 以订阅行锁和发布 advisory lock 原子创建任务、不可变版本和绑定。保留已保存的时区、交付时刻、主题/来源及历史 issue，不创建空会话。重复调用返回原任务；有效旧租约时拒绝切换。今日 ready 从次日开始，今日未发保留该计划时间；如果已过正常结果窗口则拒绝移交，不能静默改成次日。首次订阅若创建晚于当天交付时间，从下次正常计划开始。
- 通用 `Executor.Run` 根据不可移除的 DailyBrief 契约直接调用 `RunTask`，只携带本任务历史、已确认工具/来源范围和静态来源入口提示；不预采集候选。固定 prompt 可编辑，但不能把 DailyBrief 改为非每日、非 always，或绕过结构化输出。校验 headline、topSummary、sections/items、订阅主题、分组、HTTP(S) URL、数量及持久化长度；无效结构作为技术失败交给通用重试。`uncertain` 保留原因，`no_report` 被拒绝。
- 通用发布从 occurrence 已保存结果重新校验 artifact，派生会话 Markdown 和来源，再在**同一事务**投影原 issue/items、消息、未读、notification 和 reported 状态。`published_run_id` 关联 occurrence，用户/本地日期唯一键跨版本去重。旧版本、暂停、删除及已删会话抑制两种视图；既有 ready 日期不会被覆盖或补发到新会话。历史 ready 在任务暂停/删除后仍可读。尚未发布时由只读查询投影 running/failed/uncertain/missed 等页面状态，不把失败产出伪装成 artifact。
- DailyBrief 原订阅 API 与页面保留。订阅保存原子同步通用版本；通用草稿继承服务端 artifact 契约，确认前不改变配置。原时间/主题/来源修改不丢失用户自定义 prompt。删除任务保留墓碑；只有用户之后明确重新启用订阅才建立新任务，后台扫描不会复活墓碑。
- 默认 `APP_DAILY_BRIEF_SCHEDULER=mixed`（未设置亦如此）。旧扫描只扫描未绑定用户，并在领取时复查绑定；通用 worker 负责已绑定用户。`scheduled` 模式在 `cmd/server` 启动时拒绝仍有 enabled 未绑定订阅的数据库；通过门槛后不装配旧 collector/orchestrator/job，之后新确认的订阅由同一保存事务建立通用任务。`APP_DISABLE_SCHEDULED_JOBS=true` 仍关闭所有后台执行。

## 安全切换顺序

1. 先在隔离库完成真实模型、HTTP/浏览器联合验收，再部署包含新迁移与两条路径的新版本，全部实例先使用 `mixed`。确认通用 worker 正常运行且未被 `APP_DISABLE_SCHEDULED_JOBS` 关闭。不要在新 worker 尚未运行时移交绑定。
2. 全部实例升级后再选择灰度用户。最好在用户下次计划之前留出运行余量；正在执行的旧租约必须先排空。不要只改进程环境变量作为发布所有权。
3. 使用现有 datasource 环境变量指向目标库。命令要求显式数据库身份且默认只读；不执行迁移、不启动 worker：

   ```powershell
   go run ./cmd/dailybrief-cutover -database <目标库名> -user <用户ID>
   go run ./cmd/dailybrief-cutover -database <目标库名> -user <用户ID> -apply
   ```

   `-apply` 是显式按用户交接。失败事务不改变所有者；重复交接不创建另一任务。窗口已过的未发日期应先由旧路径完成，或在下一计划前交接，不通过修改日期伪造补发成功。
4. 观察同一 occurrence 的 issue、消息、未读及来源，核对每用户/日期最多一份成功发布。全部 enabled 订阅都有绑定后，逐实例设 `APP_DAILY_BRIEF_SCHEDULER=scheduled`；检查仍有未绑定订阅会拒绝启动，避免直接停旧 loop 后漏掉用户。
5. 回退只能回到**支持绑定的新版本 mixed 模式**，继续由通用 worker 服务已迁移用户。不能降到不支持通用 DailyBrief 的旧二进制、删除绑定或重新开放旧发布者；这些操作会绕过恢复协议。需要暂时停止时暂停通用任务，保留历史和绑定。

旧二进制的租约/issue 写入受数据库门槛约束，但其列表扫描仍可能占满旧批次，因此不把“数据库可阻止双发”解释为允许长期混用任意旧版本。全实例升级是灰度前提。

## 验证证据与边界

专用数据库：`codex_dailybrief_20261003_migration`。所有迁移和测试写入只发生在此库；没有向原 `ragent` 应用本轮迁移、切换订阅或删除数据。

- 新增 Go 单元测试验证直接 RunTask、独立 attempt 身份、无候选来源、artifact 与会话正文/来源一致、非法结构/主题/URL、no_report 拒绝及 uncertain 保留。
- 真实 PostgreSQL + 正式 `NewRuntime/RunOnce` 验证：切换幂等、旧二进制领取/迟到 issue 更新门槛、旧生成中 issue 原位发布；旧发布与切换并发时等待并从次日运行；保存结果后恢复不再调用模型；并发重放只得到同一消息；同一天新配置去重；消息插入故障后 issue/items/空会话全部回滚；旧版本和删除会话不投影 issue；订阅编辑、任务编辑/暂停/删除后显式重新订阅及历史可读。
- `scheduled` bootstrap 验证没有装配旧采集/重试链路，新确认的订阅原子建立绑定、迟于当天时间的新订阅安排未来计划、相同设置不生成额外版本。迁移组合套件在同一隔离库重复运行通过，测试只软删除自身任务、不清空共享数据。
- 切换 CLI 已在隔离库实际执行只读模式，读取保存的偏好和绑定；给出错误的预期库名时拒绝操作。原库最终只读复核仍为 20 enabled、103 ready/317 failed、0 通用任务，本轮迁移 applied 数量为 0。
- 受影响 Go 包常规测试通过；`go test ./cmd/... ./internal/... -run '^$'` 编译检查通过。前端 build 通过，3675 个模块；无前端源码改动，未宣称全量 TypeScript 检查通过。
- 扩大的旧 DailyBrief 两个 E2E 仍失败：`TestDailyBriefPipelineE2E` 的 fixture 只提供两种来源，现有主题扩展另四种来源失败后实际为 degraded，与 SuccessfulRuns 断言不符；`TestDailyBriefRetryE2E` 的现有 generator 在第一次 runtime 错误后调用 fallback 并成功，与“首次应 failed”的断言不符。用 Go overlay 还原本轮 issue 查询/pageState 修改后，两个错误完全复现；相关 fixture 和 generator 没有被本轮修改。不能称旧链路 E2E 全通过。
- 本轮一次新增 bootstrap 测试曾先关闭连接再执行 Cleanup，留下一个自己的测试任务，进而影响既有反馈用例“前两批应扫描到指定任务”的断言。已修正 Cleanup 顺序，并仅在隔离库软删除确切遗留任务 `39925420610613505`（用户 `39925420543504641`）；未清空其他任务。之后受影响组合套件 `-p 1 -count=2` 全部通过。原有两个 legacy E2E 在该通过命令中按既有环境开关跳过，不能计为通过。
- 没有本轮真实模型研究或浏览器联合验证，没有实际启动 server 的常驻到点验收；worker 的 RunTask 为注入 stub，SQL/发布/页面读取是真实。故障为确定性注入，尚未验证 OS 杀进程、真实模型超时、工具故障时覆盖质量、跨实例真实滚动升级。来源 URL 和字段校验不证明模型查全、事实准确或所有来源可用。
- 正常通用窗口、重试和 missed 语义仍生效；这是减少切换导致的漏发/双发，不是对故障期间绝不漏发的承诺。未增加体育试点评估、历史补发或结果保留期策略。

最终通过日志为 `tmp/dailybrief-migration-verified-tests.log` 和 `tmp/dailybrief-migration-final-compile.log`。前序日志在 `tmp/dailybrief-migration-db-tests.log`、`tmp/dailybrief-migration-go-tests.log`、`tmp/dailybrief-legacy-baseline-e2e.log`、`tmp/dailybrief-migration-final-tests.log`，包含扩大回归或清理顺序问题的失败，不能称为全绿。持续接手以本报告和新增独立 Go 用例为准。
