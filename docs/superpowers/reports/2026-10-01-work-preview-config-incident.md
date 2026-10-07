# Work 验收服务配置误启动记录

记录日期：2026-10-02。状态：误启动进程已停止，配置防护已修正；受影响数据未自动回滚。

## 经过

2026-10-01 重启本次验收服务 `tmp/work-e2e-server-v6.exe`（PID 34152）时，工作目录设为仓库根目录。配置初始化使用 `gotenv.OverLoad`，覆盖了进程环境；同时启动命令使用了不被项目识别的 `DATABASE_URL`、`PORT`。新进程因此连接原有 `ragent` 库，而不是专用 `codex_work_20261001`，并启动原有后台任务。它从约 21:25 开始运行，在 21:42 首次数据库审计前已停止。没有自动回滚或删除原有数据库记录。

这是本次实施中的操作错误，不能将本轮描述为“原有数据库未改动”。后续验收恢复到独立目录和专用数据库。

## 只读核对结果

- `t_migration_version` 显示 9 项迁移在 2026-10-01 13:25:51–13:25:52 UTC 应用：4 项定时任务迁移（20260930110000、20260930120000、20261001090000、20261001100000）及 5 项 Work 迁移（20261001120000 至 20261001160000）。
- 原有库 `t_work_topic` 为 0；`t_scheduled_task` 为 0。本次 Work 验收内容均保存在隔离库。
- 进程日志中的 21 个简报 generation run ID 对应 20 条 `degraded`、1 条 `failed`。20 份简报均为本次新建，合计 89 条简报条目；`create_time` 为本地时间 21:25:52 左右。
- 该启动窗口的 runtime task session 有 16 条完成的 `daily_brief`、5 条失败的 `daily_brief`、5 条完成的 `daily_brief_fallback`。这是执行会话统计，不是 26 份简报。
- `t_user_memory_profile` 中管理员 user_id=1 的画像在 21:28:22 更新至 version 7。本轮没有恢复此前正文。
- 以上是已核实影响，不表示全部后台维护表都已排除影响。画像更新前内容和订阅锁等后台元数据仍需进一步核对，不能按时间窗口批量删除。

核对仅查询迁移、身份、时间、状态及计数，未在报告中保存模型密钥或画像正文。原始本地证据为 `tmp/work-e2e-server-v6.{out,err}.log`、`tmp/work-preview-config-incident.json` 和只读查询 `tmp/work-preview-incident-read.sql`。

## 已落实的防护

1. `internal/framework/config/env.go` 改为 `gotenv.Load`，显式进程配置优先于 `.env`。子进程测试验证真实包初始化保留显式设置，仍加载文件中的缺省值。
2. `cmd/server/main.go` 支持 `APP_EXPECTED_DATABASE`，在执行迁移和启动后台任务之前查询 `current_database()`；不匹配立即退出。已用真实隔离库和故意错误的期望名称验证退出码 1。
3. `APP_DISABLE_SCHEDULED_JOBS=true` 跳过每日简报与通用定时任务后台调度，保留管理 API。缺省行为不变。
4. `scripts/start-work-preview.ps1` 仅接受 `codex_work_` 前缀的专用库，使用独立临时目录、项目实际识别的 `SPRING_DATASOURCE_*` / `SERVER_PORT`，并同时设置上述数据库校验和调度禁用选项。
5. 修正后的预览连接统计仅出现专用库连接；之后的 Work 资料、模型与界面测试在隔离库执行。

## 数据处置边界

本报告提供影响清单与证据，尚未执行回滚。后续若处理这些简报、迁移或画像，需要先读取当前状态与可恢复的历史/备份，形成具体方案，再获得用户对原有数据库写入或删除的明确授权。不能以“继续开发 Work”推定允许删除原有库记录。
