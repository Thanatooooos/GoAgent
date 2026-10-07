# DailyBrief 迁移完成与运行验收

日期：2026-10-06。DailyBrief 迁移范围结论：**aligned**。原 ragent 的 20 条订阅已显式交接，原库服务已切换为 scheduled 模式；真实模型、正式 HTTP/CLI、浏览器与真实 PostgreSQL 验收已完成。通用定时任务整体仍为 partially aligned，体育试点及其他未验收范围独立保留。

## 原库交接与部署结果

用户授权继续迁移并更新文档；对于唯一今日过期失败订阅，用户明确选择“保留失败记录，明确记为今天错过后迁移，明天按原时间运行”。此前自动审批 503 的阻塞在本轮批准执行机制下解除，实际环境检查和交接命令已执行。

交接前重新核验：20 条订阅全部 enabled、0 有效旧租约、0 旧 running、0 绑定；历史为 140 ready / 320 failed issue、700 个条目。19 位用户今日 ready，另 1 位今日 failed。原库在本轮开始时已安装截至 20261004100000_chat_execution_leases.sql 的当前迁移，本轮启动没有新增这些历史迁移。

先启动原库 mixed 过渡服务并明确暂停旧 DailyBrief loop，通用 worker 保持运行；健康检查和租约排空检查通过后，实际 cutover CLI 逐条移交。19 条使用普通交接；唯一失败订阅使用 -acknowledge-missed-date 2026-10-06。随后仅停止本轮拥有的 mixed 进程，启动原库 scheduled 服务。

最终数据库核对：

| 项目 | 结果 |
| --- | --- |
| 稳定绑定 | 20 条；enabled 未绑定为 0 |
| 通用 DailyBrief 任务 | 20 active，均为版本 1 |
| 次日计划 | 全部为保存时区的 2026-10-07，时刻与原订阅一致 |
| 契约映射 | 时区、时间、主题与来源差异为 0 |
| 原库历史 | 460 issue（140 ready / 320 failed）、700 条目，内容摘要不变 |
| 迁移即新建会话 | 0 |
| 过期失败 | 唯一 2026-10-06 occurrence 为 missed，旧 issue 仍 failed |
| 其他通用任务 | 原有 2 条仍 active，未修改其配置 |

历史与偏好校验摘要（有序整行 JSON / 偏好串的 MD5，仅作前后变化检测）：

- issues：4ac801da2fc60dfc76c3556e4afc6c7a。
- items：f21153037b4a8458384ee1324ab1629d。
- preferences：456aa9a911dee6a23f5f30457ba44907。

交接前后这三项完全一致。失败用户 26002327718654209 的任务为 40366777456193793，下一次仍是 UTC 08:00；今日失败未删除、未补发、未伪造成成功，也未创建空会话。

原库部署快照：北京时间 19:28:49，PID 30316，端口 9090，二进制 D:/goagent/tmp/dailybrief-migrated-server.exe，数据库 ragent，模式 scheduled。/ping 返回 pong；无旧 generator 装配日志。PID 只作当时证据，操作前需重新确认进程路径与端口。脚本开启通用 worker，关闭迁移部署不需要的画像观察和记忆维护。

## 修复与交付实现

1. 移除 SubscriptionModel.Enabled 的 GORM default:1，避免显式 enabled=false 被替换为 true。复现测试修复前失败、修复后通过；真实库与 HTTP 验证关闭订阅后原任务 paused、清空 next due 且不新增配置版本。
2. 明确 DailyBrief 的输出信号只能 report / uncertain，拒绝 normal、success、no_report；保留严格结构、主题、URL 和字段校验。
3. 复用旧生成器的确定性条目上限规则：先校验全部条目，再按总数/每主题配额截断；归一化 artifact、来源和会话正文一致保存，重复归一化不改变结果，无效的超额条目也会拒绝。补充总条数、每主题配额及最多六轮工具研究后收尾的提示；资料不足仍需明确覆盖缺口，不加入“查全”判定。
4. 新增显式过期日期交接。普通交接默认仍拒绝过期未发；日期必须匹配实际过期的当地日期，同事务记录 missed 并从下一次正常时间开始，保留旧失败记录。错误日期、幂等及次日时刻的真实库回归通过。
5. 修正 issue generated_at / published_at 的读取时间映射。旧表为 TIMESTAMP，写入使用服务端当地墙钟；按与旧 generation run 相同的约定恢复时区，避免浏览器把生成时间再加 8 小时。不改历史存储值；这个旧表约定依赖部署节点时区一致。
6. 新增受保护启动脚本 scripts/start-dailybrief-server.ps1，限定目标库、独立配置目录及迁移前实际数据库身份校验，不停止未知端口进程。.env 仅补缺省。正式 cmd/server 的 scheduled 模式仍拒绝存在 enabled 未绑定订阅，不把默认模式修改当作迁移。

checked-in 默认仍为 mixed，以保护其他尚未交接的环境。本机原库部署通过启动脚本明确使用 scheduled；以后按下文重启，不依赖默认 mixed。

## 真实验收与失败边界

正式入口为 cmd/server/dailybrief_migration_live_test.go 的 TestDailyBriefMigrationLiveServer，仅显式 DAILYBRIEF_LIVE_ACCEPTANCE=1 时运行。要求本机专用 codex_dailybrief_ 空库，使用正常配置的真实外部模型与网页工具；不会清空共享数据，也不注入 TaskRuntime。

保留所有尝试的事实：

- 第一轮真实工具成功，但模型返回 signal=normal，被严格结果解析拒绝；据此补明确信号提示。
- live_v2 模型返回 4 条、契约上限为 2，发布前校验失败；据此复用确定性配额归一化。该轮未通过。
- live_v3 首次尝试耗尽 runtime 轮次，有限重试后的第二次尝试成功；整个正式验收通过。
- 最终 live_v4 使用补充配额和收尾提示的版本，一次尝试成功：3 次工具调用（2 fetch、1 search），runtime 共 3 轮模型循环（含最终草稿），随后同一证据上下文格式化 JSON。2026-10-06 11:21 UTC 实际到点运行，11:22:20 UTC 发布。用例 PASS，约 202 秒。它不是对长期成功率的统计承诺。

最终样本：数据库 codex_dailybrief_20261006_live_v4，任务 40365941464297729，occurrence 40366092744454401，attempt 40366092761231617。

该正式验收证明：

- mixed 经正式认证/订阅 API 建订阅；未绑定时实际 scheduled 可执行文件拒绝启动。
- 实际 CLI 连续两次交接，仅一份绑定，计划时间不变、不提前创建会话。
- scheduled 正式 worker 真实到点 RunTask，保留成功工具 journal。
- 同一 occurrence 同时投影 issue 与助手消息，来源/摘要一致，只有一条发布和一次未读。
- 保留合成历史样本；重启不重复发布，读取清未读，HTTP 关闭订阅暂停任务，旧 generation run 为 0。

浏览器以最终构建读取 live_v4：简报标题、分组、来源、覆盖缺口、修正后的生成时间、前一天历史、通用任务最新汇报、一次运行详情/工具记录及关联会话均已检查。页面明确展示 HN RSS 不可用、GitHub feed 缓存陈旧等覆盖缺口；这只证明缺口能展示，不验证模型说法或新闻事实准确。历史 fixture 明确为合成数据，不计作真实模型结果。

普通 user 的 Chat 周边页面仍出现既存 knowledge-base 403、sample-questions / legacy approval 路由 404 的提示；关联会话消息请求为 200，简报报告实际显示。没有为本轮迁移重写这些无关路由，不能称整个 Chat 页面无错误。

## 回归、证据与备份

- 本轮全仓功能测试 go test -p 1 ./cmd/... ./internal/... -count=1 通过；当时未设置的数据库/真实模型用例按开关跳过，不能用该日志替代实库证据。
- 最终受影响包与隔离数据库组合回归通过，含订阅关闭、显式 missed、发布、恢复及最新配额和时间映射。
- 受影响包 go vet 通过，实际 server / cutover 二进制构建通过。
- 前端 npm run build 通过。未宣称全量 TypeScript 检查通过；既存全量类型基线未在本轮修复。
- 首次审计中的两个旧 DailyBrief E2E fixture/fallback 断言失败保留，本轮未修改那些无关链路。

关键临时证据：

- tmp/dailybrief-retry-live-v4.log、tmp/dailybrief-live-7137408/published-report.json 及正式服务日志。
- tmp/dailybrief-retry-live-v3.log（含重试后 PASS），以及 live / live_v2 的失败日志。
- tmp/dailybrief-delivery-final-tests.log、tmp/dailybrief-final-db-quota.log、tmp/dailybrief-missed-handoff-test.log。
- tmp/dailybrief-final-go.log、tmp/dailybrief-final-frontend.log。
- tmp/dailybrief-original-before-cutover.txt、tmp/dailybrief-original-after-cutover.txt、tmp/dailybrief-original-cutover.log。
- tmp/dailybrief-final-page.jpg、dailybrief-final-history.jpg、dailybrief-final-task.jpg、dailybrief-final-run.jpg、dailybrief-final-conversation.jpg。
- 原库运行日志 tmp/dailybrief-runtime-ragent/server-scheduled-20261006-192849.out.log / .err.log。

备份在交接前已完成：tmp/dailybrief-before-cutover-20261006.dump，230638928 字节，SHA256 为 6DD0D4D67EAA452591C665BDC017B5DCAA582E8C047A6E5369EB0413F1AB2A9A。容器另保留 /tmp/dailybrief-before-cutover-20261006.dump。备份未经恢复演练；没有自动恢复或删除。原库/template1 的 collation 版本提示仍存在，未擅自修复，验收库使用 template0 创建。

## 重启与后续边界

在仓库目录使用：

~~~powershell
$env:GOCACHE='D:\goagent\tmp\codex-gocache'
go build -o tmp/dailybrief-migrated-server.exe ./cmd/server
./scripts/start-dailybrief-server.ps1 -DatabaseName ragent -Port 9090 -Mode scheduled
~~~

端口已占用时脚本会拒绝启动；先核对当前服务身份并按正常运维关闭，不能照抄历史 PID 停进程。不要用 dailybrief-regen 删除旧失败或修改历史。回退仅允许支持绑定的新版本 mixed + 通用 worker，不能删除绑定、降到旧二进制或重新允许旧发布者。

隔离浏览器预览仍在 5175 → 9091，连接 live_v4；原库服务在 9090，不能把预览当原库。验收样本任务已暂停，避免明日再次花费模型调用。普通本机后台进程未安装为开机自启动服务；机器/服务关闭后应使用上述命令重启。

明天原订阅的实际投递尚未发生，本轮证明已正确安排；真实单样本不证明事实准确、资料查全或长期成功率。多节点滚动升级未全面验收，体育试点离线复盘仍开放。本轮未提交、合并、清理或重置工作区。临时证据不是长期交付资产，后续以本报告和可重复 Go 回归为准。
