# Work 第一版交付核对

更新：2026-10-02。核对依据为已确认产品、共同文档、交互、视觉与六阶段实施计划。

**结论：aligned（Work 第一版业务承诺）。** 六阶段清单已完成，正式 `/work`、受保护 HTTP API 和 `cmd/server` 已接入，并通过真实模型、数据库、解析/向量服务和浏览器验证。最后补齐普通聊天知识库选择/实际检索、管理页上传分块、画像观察的 Work 排除与普通领取、任务汇报与关联会话 UI 回归。此结论不表示整个项目已交付：既有定时任务试点/迁移待办、全量前端类型错误和原有数据事件的后续处置仍分别记录。

本轮存在一次预览配置误启动，原有数据库发生迁移及后台数据写入；进程已停止，数据未自动回滚。影响与防护见 [事件记录](2026-10-01-work-preview-config-incident.md)。这与 Work 功能验收分开记录，不隐去、不按时间窗口自动删除。

## 1. 承诺与正式入口

范围为单人与 AI 的长期协作主题：学习、技术选型、日常工作复用同一界面；主题内事项、多段对话与明确接续；已确认进展；独立文档及按轮次共同编辑；私有资料、对话附件与共享库关联；版本、冲突、归档恢复和删除会话边界。多人协作、Work 定时任务关联、永久删除主题、上传文件转换成共同文档不在第一版。

| 层次 | 实际落点与结果 |
| --- | --- |
| module | `internal/app/work/domain` 定义结构化正文、稳定块 ID、版本/建议/轮次/资料；仓储实现事务、CAS、幂等和范围校验 |
| integration | `cmd/server/main.go` 注册 `internal/bootstrap/work`；复用 shared runtime、模型、SSE、journal、会话与分块处理队列 |
| delivery | 登录后的 `/work` 和 `/work/:topicId` 可创建/接续、编辑、交给 AI、确认/忽略建议、查看引用与版本；真实结果持久化后刷新可读 |
| default path | 正式主入口自动应用 5 项新增迁移；空主题可从两字段创建开始。集成套件自行运行迁移，必须指定隔离 DSN；预览脚本强制数据库身份校验 |

主题页采用合并导航、聊天、可收起“进展/文档/资料”面板；文档展开扩大正文并保留聊天。Tiptap 3.31.4（MIT）提供标题 1–3、列表和简单表格，结构化 JSON 为唯一正文。人保存后交给 AI，AI 一轮完成后原子保存新版本，再交回人；恢复历史版本也新建版本。

普通讨论只允许建议。明确创建/局部修改/整篇重写才允许对应写入；聚焦文档和来源内容不授予权限。AI 建议文档可预览拟修改正文，由人应用后才产生保存版本；进展独立确认，文档修改不自动完成事项。

## 2. 数据与权限

- 5 项迁移：`20261001120000_create_work_tables`、`20261001130000_work_turns`、`20261001140000_work_proposals`、`20261001150000_work_sources`、`20261001160000_work_history_summary`。文档建议复用建议表 JSONB 的类型分支，没有为其追加另一套表。
- 主题、事项、会话、文档、版本、建议、轮次和来源均校验当前用户及主题；归档和在途写入在提交前复查。
- 当前事项及未分事项上下文按范围选择；历史读取/搜索有界，接续不会用旧摘要覆盖当前确认状态。Work 不挂全局画像/记忆写入，普通跨会话画像扫描排除 Work。
- Work 私有知识库从默认全局列表与检索排除。向量、关键词、metadata、parent、预测问题与直接 ID 读取遵循资料范围；缺少等价范围约束的 wiki/fact 通道不启用。
- 原文件通过登录保护的 Work 来源路由读取。真实匿名对象存储 URL 返回 403；普通用户可读取自己的资料，另一身份即使为管理员仍不能访问其 Work 私有来源。
- 删除会话保留独立文档、已确认进展与已提升资料；本次附件先撤销访问，再执行可重试清理。被删除会话的消息、turn/snapshot/stream 读取拒绝访问。
- 同一 request/tool 操作可重放；输入改变拒绝复用。建议按内容和基准版本去重，模型换生成 ID 或摘要不产生新的相同建议；已确认的相同进展与无变化文档不再建议。复用旧建议不再产生本轮新通知卡片。

## 3. 本地服务和验证命令

专用 PostgreSQL 库为 `codex_work_20261001`，身份由集成测试读取 `current_database()` 校验，Work 套件拒绝非 `codex_work_` 库。真实模型为本地配置的硅基流动 `Qwen/Qwen3-32B`；文件路径使用现有 RustFS、解析、chunk 队列和 embedding 服务。报告不保存密钥、登录 token 或画像正文。

```powershell
$env:GOCACHE='D:\goagent\tmp\codex-gocache'
# WORK_TEST_DSN 与 SCHEDULED_TASK_TEST_DSN 应由本地环境设置，均指向专用隔离库。
go test -p 1 ./internal/adapter/repository/postgres/work/... ./internal/bootstrap/work/... ./internal/adapter/repository/postgres/scheduledtask/... -count=1
go test ./cmd/... ./internal/... -count=1
go build -o tmp/work-preview.exe ./cmd/server
./scripts/start-work-preview.ps1
```

本机同一数据库中的不同测试包不能并行运行：现有 Sonyflake 默认机器 ID 在同机独立进程中相同，曾导致会话/消息主键碰撞。串行包执行通过；同一套件内针对事务并发的测试仍实际并发执行。未改造整个项目的多实例 ID 配置，不把这次失败归为业务 CAS 失败。

预览脚本仅接受专用库，使用独立工作目录、实际有效的 `SPRING_DATASOURCE_*` / `SERVER_PORT`、`APP_EXPECTED_DATABASE` 和 `APP_DISABLE_SCHEDULED_JOBS=true`。禁用后台调度不影响任务管理 API；生产缺省仍启用原调度。配置 `.env` 只补缺省，不覆盖显式进程变量。

前端使用已安装 bundled Node 24 + npm-cli.js 执行 `run build`，构建通过。`tsc --noEmit -p tsconfig.app.json` 退出 2，输出 151 行既存错误，Work 文件无报错；不能表述为全量类型检查通过。构建仍有原有大 bundle 提示，未为本轮升级依赖。

## 4. 证据清单

以下为本地验收文件及持久化对象，均位于专用库。浏览器脚本使用已安装 Playwright/Chromium，真实模型不由 mock 代替；故障注入用例单独标明。

| 验证 | 结果与证据 |
| --- | --- |
| 手动 API / 持久化 | `http_integration_test.go`：真实 Gin 路由、独立仓储装配后读取、CAS 409、重复请求及跨用户/主题拒绝 |
| 同一共同文档 | `tmp/work-acceptance-result.json`：人 v2 → AI 局部 v3，标题/列表/表格未修改结构逐项一致；刷新保留结果卡片。主题 `39646373649379585`，文档 `39646373682934017` |
| 私有资料全链路 | 同上：真实 TXT 上传、解析、摘要/问题/embedding、ready、模型引用及登录原文件读取；全局直接 document ID 404。来源 `39646405089882369` |
| 编辑与生命周期 | `tmp/work-boundaries-result.json`：冲突保留草稿、恢复新版本、归档恢复、停止迟到写入、断连/重放、重复 POST、删除附件/保留文档与进展/提升资料 |
| URL | `https://example.com` 实际解析/向量完成。初次断言错误假设正文含页面标题而失败；`tmp/work-url-verify.mjs` 按真实正文重新验证，通过。不能把初次整段脚本称为一次无失败通过 |
| 学习 | `tmp/work-scenarios-result.json`：实际自动意图创建计划、人未保存反馈→新对话保存交接→同一文档 AI 继续、编辑建议确认后刷新。主题 `39652096827519233`，文档 `39652117765484801` |
| 日常工作 | 同上：周报/客户反馈切换不混用，忽略不更新进展，第二期为新文档而第一期保留。主题 `39652140968374529` |
| 跨自然日 | `tmp/work-delivery-followup-result.json`：2026-10-02 在昨天的周报事项中新开明确接续对话，真实模型读出 `WEEKLY_ONLY_28717`，未包含客户代号、未写文档/建议/进展 |
| 技术选型 | `tmp/work-final-entrypoints-result.json` 前 5 项：模型读取私有消息队列资料，基于验收者提供的模拟实测反馈提出文档建议和待确认决定；人预览/应用同一文档新版本，再单独确认 RabbitMQ 决定与落地下一步。turn `39657646411804929` |
| 写入授权/澄清 | `tmp/work-intent-delivery-result.json`：讨论保持 discuss，明确局部修改为 edit_document，无聚焦对象为 discuss；真实对话先澄清且无文档写入 |
| 故障后可信结果 | `runtime_failure_integration_test.go`：真实 PostgreSQL + 正式 HTTP/shared runtime/journal；合成模型及助手保存故障注入后，已提交文档仍可读，重启装配/重复 POST 不再调用模型或多建版本；迟到 patch 保留只读拟修改正文 |
| 事务与清理 | `lifecycle_integration_test.go`：真实 DB，注入事务回调/存储清理失败；删除回滚、先撤销访问、提升竞争及清理重试；删除依据后未来进展仍可保存 |
| 建议幂等/去重 | `document_proposal_integration_test.go`、`proposal_dedup_integration_test.go`：待确认不写正文，重复应用不另建文档/版本，晚到建议 CAS 保留；应用后文本工具按接受版本重放；相同建议跨轮次不再创建通知 |
| 视觉/窄屏 | `tmp/work-visual-final-result.json`：主题卡片活动时间、富文本版本视图、深色持久化、正文对比 12.7、按钮对比至少 4.5、键盘焦点、移动导航/文档返回普通聊天。截图 `tmp/work-acceptance-{desktop,mobile}.png`、`tmp/work-dark-final.png` |
| 原有聊天 | `tmp/work-final-entrypoints-result.json`：Work 会话不进入普通列表，强制普通执行拒绝；普通模型/SSE、消息保存、列表及删除通过 |
| 原有任务/KB 入口 | `tmp/work-delivery-followup-result.json`：管理页和知识库页无 pageerror；模型配置预览→确认→暂停→恢复→删除通过。首次/重试预览失败证据保留于旧结果，见下节 |
| 原 KB/选择/检索 | `tmp/work-legacy-ui-final-result.json`：普通聊天实际选择共享库；原管理页上传 TXT 并启动分块，真实 parser/embedding 完成，普通模型检索返回 `PUBLIC_KB_20261002`。库 `39752059708305665`、文档 `39752076267417857`。初次空聊天选择器定位/按钮名称错误已修正；另一次因预览重启中断分块而失败，记录在 `tmp/work-legacy-ui-delivery-result.json`，最终是新样本完整通过 |
| 混合向量维度 | 新共享库的实际 4096 维向量使组合测试的 1024 维默认检索报错。`pgvector/vector_store.go` 现按查询维度过滤后计算距离；`source_integration_test.go` 在同一范围写入不同维度向量，验证匹配命中与私有排除。`tmp/work-mixed-vector-entrypoint-result.json` 证明最终预览的实际默认共享检索正常 |
| 画像观察基座 | `lifecycle_integration_test.go` 新增实际 DB 用例：普通真实会话被观察仓储领取，Work 会话即使登记 pending 也被排除；原 profile service/observer/worker 测试通过。未用这些证据宣称真实模型已形成某个具体用户画像 |
| 任务汇报 UI | `tmp/work-report-ui-delivery-result.json`：真实 Store 领取/完成/原子发布合成 outcome 后，原管理页最新汇报、历史运行详情、关联汇报会话均显示 `REPORT_UI_20261002`；无 pageerror。合成样本不是外部事实或新增真实模型任务执行证据 |
| 最终套件 | `tmp/work-backend-delivery-final.log` 完整 Go 测试通过；最终混合维度修正后的 `tmp/work-db-delivery-complete.log` Work + 定时任务实际数据库套件通过；`tmp/work-frontend-delivery.log` 前端构建通过 |

学习、日常工作和选型的反馈是开发验收者提供的模拟业务事实，不是实际客户实践或对 RabbitMQ 自身性能的测量。这些证明产品持久化和协作流程，不能作为外部技术选型结论的真实性证据。截图/脚本位于临时目录，报告与 Go 回归用例为持续接手依据；浏览器脚本部分依赖前一段验收创建的对象，不宣称它们是独立、可随意顺序运行的通用测试套件。

## 5. 原入口回归修正与遗留项

真实任务管理预览曾两次返回 `configuration review did not return a complete draft`：管理页已经提交完整配置，但 `ReviewConfig` 复用要求当前聊天文字同时包含未来动作与时间的模型规则。现增加仅由 `ReviewConfig` 内部启用的明确配置审核模式，仍要求完整 `draft` + `taskId=preview`，严格校验、知识库权限及人工确认不变。聊天 `Propose` 保持原意图门槛。focused 测试验证模式隔离和不完整预览拒绝，真实模型和任务 API 回归已通过。

仍未完成或不应外推的范围：

1. 原功能回归已按上表完成。画像观察无独立 UI，使用正式仓储领取与既有服务/worker 测试；任务汇报 UI 使用合成 outcome + 真实发布事务，不能称这份汇报由真实模型执行产生。
2. 全量应用 TypeScript 基线错误未清理；此处没有全量类型检查通过结论。
3. Work 特定图片、Word/PDF 每一种文件格式未分别做完整实际上传验收；复用现有解析链路，已有解析模块证据不能替代每种 Work 上传场景的测试。共同编辑只支持应用内正文，文件转换未实现且不在范围内。
4. 同机多测试进程共享一个库需 `-p 1`；通用多实例 ID 配置仍属既有基础设施限制。
5. 原有数据库事件的数据处置尚未授权执行。不得自动删除简报、回滚迁移或覆盖画像。

预览运行最终修正版，后端 9091，前端 5175；前端使用本地验收代理配置。用户可从 `http://127.0.0.1:5175/work` 查看实际业务。服务重启会中断现有内存分块队列，开发验收须等处理完成再重启；本轮被中断的隔离样本没有被伪装为处理成功。

本轮没有提交、合并或部署。对工作区已有改动未清理/重置。上述事件导致原有库实际变化，不能把“未部署”写成“原有数据完全未动”。
