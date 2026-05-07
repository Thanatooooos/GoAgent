# Development Notes 2026-04-29

更新时间：2026-04-29

这份文档记录今天确认下来的开发规范，以及实现阶段暴露出的代码层问题。

说明：

- 这里只记录代码与结构层面的规范和坑
- 不记录环境启动、容器、端口、服务编排类问题

## 今日确认的开发规范

### 1. adapter / repository 继续按业务域拆分

后续新增持久化适配代码时，继续遵循：

```text
internal/adapter/repository/postgres/
  knowledge/
  user/
  ...
```

不要再把新业务 repo、model、mapper 平铺回 `postgres/` 根目录。

### 2. 新业务模块优先补齐完整骨架

如果是新的业务域，优先一次性补齐：

- `internal/app/<domain>/domain`
- `internal/app/<domain>/port`
- `internal/app/<domain>/service`
- `internal/adapter/repository/postgres/<domain>`
- `internal/adapter/http/<domain>`
- `internal/bootstrap/<domain>`

不要只先写 handler 或只先写前端接口占位。

### 3. 统一复用现有 HTTP 契约

当前后端统一响应结构已经稳定：

```json
{
  "code": "0",
  "message": "",
  "requestId": "...",
  "data": {}
}
```

分页结构保持：

```json
{
  "records": [],
  "total": 0,
  "size": 10,
  "current": 1,
  "pages": 0
}
```

后续新接口应继续沿用，不要单独发明新的分页或响应包装格式。

### 4. 用户上下文统一走 `Authorization`

当前 `UserContextMiddleware` 已支持从 `Authorization` 请求头提取 token。

后续新接口如果依赖登录态：

- 优先沿用统一中间件
- 优先使用 `contextx.Get(c)` 取当前登录用户
- 不要在 handler 里重新手写一套 token 提取逻辑

### 5. 管理权限继续通过中间件表达

当前已经具备：

- `RequireLogin()`
- `RequireRole("admin")`

后续后台管理能力优先通过路由分组挂中间件，而不是在 handler 内部散落权限判断。

## 今日踩到的代码层坑

### 1. Gin 动态路由参数名必须在同一前缀下保持一致

今天确认了一类很容易忽略的问题：

同一条路径前缀下，如果混用：

- `:doc-id`
- `:docId`

Gin 会认为它们冲突，并在启动时直接 panic。

本次实际修正方式是统一为同一种命名风格：

- `:docId`
- `:chunkId`

后续规范：

- 同一资源前缀下的动态路径段命名必须统一
- 推荐统一使用驼峰风格参数名，如 `:docId`、`:userId`、`:chunkId`

### 2. 新增数据库能力时，不能只写 migration 而忽略旧数据卷场景

今天也确认了一个很典型的维护问题：

- 新增表结构后
- 旧数据库实例不一定会自动获得这些表

这不是启动问题，而是“增量开发时要考虑已有数据状态”的问题。

后续规范：

- 新增表、种子数据、关键索引后，要明确“全新初始化”和“已有库补齐”两种路径
- 文档里要能说明当前功能依赖哪些新增表或种子数据

### 3. 启动期依赖过重时，需要保留模块级降级开关

今天为避免非核心依赖阻塞主链路联调，确认了一条实用规范：

- 对于像 MQ 这种非所有接口都强依赖的能力
- runtime 层可以保留显式降级开关

这样可以先打通同步主链路，再恢复完整异步链路。

后续规范：

- 降级开关应放在 runtime / bootstrap 层
- 业务 handler / service 不要散落判断
- 开关的使用范围要清楚，只用于开发联调或局部兜底

## 对后续开发的建议

### 1. 新增业务接口前先检查路由树一致性

尤其是：

- 资源路径前缀
- 动态参数命名
- 是否和已有路径产生冲突

### 2. 新增表结构后，同步准备补库方案

不要默认所有开发环境都会重新初始化数据库。

### 3. 优先保证“主链路可联调”

当某个外围依赖不稳定时，优先保留可控降级方式，先让核心接口和前后端联调继续推进。

