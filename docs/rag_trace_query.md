# RAG Trace 查询能力说明

本文档说明本次新增的 `trace` 查询能力，包括功能范围、接口契约、后端实现链路，以及它和现有 `chat trace` 落库链路的关系。

## 1. 功能目标

当前 `chat` 主链路已经会在后端落两类 trace 数据：

- `t_rag_trace_run`
- `t_rag_trace_node`

但此前只有“写入”能力，没有“查询”能力，所以前端的 trace 页面虽然已经存在：

- `frontend/src/pages/admin/traces/RagTracePage.tsx`
- `frontend/src/pages/admin/traces/RagTraceDetailPage.tsx`

却无法真正访问后端数据。

本次新增的目标是补齐最小查询闭环，让后台可以：

1. 分页查看 trace run 列表
2. 查看某一条 trace 的 run + nodes 详情
3. 单独查询某条 trace 的节点列表

## 2. 提供的接口

本次新增 3 个只读接口，均位于登录保护下，并额外要求 `admin` 角色。

### 2.1 `GET /rag/traces/runs`

用途：

- 分页查询 trace run 列表

查询参数：

- `current`
- `size`
- `traceId`
- `conversationId`
- `taskId`
- `status`

返回结构：

```json
{
  "records": [
    {
      "traceId": "1746...",
      "traceName": "rag_chat",
      "entryMethod": "rag.v3.chat",
      "conversationId": "1746...",
      "taskId": "1746...",
      "userId": "1",
      "status": "success",
      "errorMessage": "",
      "durationMs": 1280,
      "startTime": "2026-05-01T18:01:02+08:00",
      "endTime": "2026-05-01T18:01:03+08:00"
    }
  ],
  "total": 1,
  "size": 10,
  "current": 1,
  "pages": 1
}
```

### 2.2 `GET /rag/traces/runs/:traceId`

用途：

- 查询单条 trace 的完整详情

返回结构：

```json
{
  "run": {
    "traceId": "1746...",
    "traceName": "rag_chat",
    "entryMethod": "rag.v3.chat",
    "conversationId": "1746...",
    "taskId": "1746...",
    "userId": "1",
    "status": "success"
  },
  "nodes": [
    {
      "traceId": "1746...",
      "nodeId": "retrieve",
      "depth": 1,
      "nodeType": "retrieve",
      "nodeName": "vector_retrieve",
      "status": "success",
      "durationMs": 0
    }
  ]
}
```

### 2.3 `GET /rag/traces/runs/:traceId/nodes`

用途：

- 单独查询某条 trace 的节点列表

这个接口和详情接口返回的 `nodes` 数据一致，只是方便前端按需拆分调用。

## 3. 权限与范围

### 3.1 权限控制

trace 路由在 `internal/adapter/http/rag/handlers.go` 中注册时，被单独挂到了一个 `admin` 子路由组：

- 先继承 RAG 模块已有的 `RequireLogin()`
- 再追加 `RequireRole("admin")`

也就是说：

- 普通聊天接口仍然是登录即可访问
- trace 查询接口改成只有管理员可访问

这样更符合 trace 数据的后台观测属性。

### 3.2 查询范围

当前实现按系统级数据查询，不额外按当前登录用户过滤。

原因是：

- 现有前端 trace 页面是后台管理页
- 后续排障通常需要跨用户查看运行记录

如果后面要扩成“用户只看自己的 trace”，可以在 `TraceService.PageRuns()` 里补 `userId` 过滤条件。

## 4. 后端实现结构

本次新增实现分成四层：

1. repository：已有
2. service：新增 `TraceService`
3. HTTP handler：新增 `TraceHandler`
4. runtime / route：把 trace service 接进启动和路由注册

### 4.1 repository 层

现有 repository 已经具备最小查询能力：

- `internal/adapter/repository/postgres/rag/rag_trace_run_repo.go`
- `internal/adapter/repository/postgres/rag/rag_trace_node_repo.go`

其中：

- `RagTraceRunRepository`
  - `GetByTraceID()`
  - `Count()`
  - `List()`
- `RagTraceNodeRepository`
  - `ListByTraceID()`

也就是说，这次主要不是补 repository，而是把它们向上编排成可用的 service 和 HTTP 接口。

### 4.2 service 层

新增文件：

- `internal/app/rag/service/trace_service.go`

新增核心类型：

- `TraceService`
- `PageTraceRunsInput`
- `TraceRunPageResult`
- `TraceDetail`

#### `TraceService.PageRuns()`

职责：

1. 校验 repository 依赖
2. 规范化分页参数
3. 组装 `port.RagTraceRunListFilter`
4. 调用 `traceRunRepo.Count()`
5. 调用 `traceRunRepo.List()`
6. 返回统一分页结果

其中分页会做两个限制：

- 默认页码：`1`
- 默认页大小：`10`
- 最大页大小：`100`

#### `TraceService.GetDetail()`

职责：

1. 校验 `traceId`
2. 调用 `traceRunRepo.GetByTraceID()`
3. 如果 run 不存在，返回 `not found`
4. 调用 `traceNodeRepo.ListByTraceID()`
5. 组装 `TraceDetail`

#### `TraceService.ListNodes()`

职责：

- 复用 `GetDetail()` 的存在性校验
- 返回其中的 `nodes`

这样可以避免出现“节点列表查出来了，但 run 本身不存在”的不一致行为。

### 4.3 HTTP handler 层

新增文件：

- `internal/adapter/http/rag/trace_handlers.go`

新增类型：

- `TraceHandler`
- `ragTraceRunVO`
- `ragTraceNodeVO`
- `ragTraceDetailVO`

#### `RegisterTraceRoutes()`

注册了 3 个接口：

- `GET /rag/traces/runs`
- `GET /rag/traces/runs/:traceId`
- `GET /rag/traces/runs/:traceId/nodes`

#### `ListRuns()`

职责：

1. 解析 query 参数
2. 调用 `TraceService.PageRuns()`
3. 把 `domain.RagTraceRun` 转成前端所需 VO
4. 计算 `pages`
5. 输出统一分页结构

#### `GetDetail()`

职责：

1. 读取路径参数 `traceId`
2. 调用 `TraceService.GetDetail()`
3. 输出 `{ run, nodes }`

#### `ListNodes()`

职责：

1. 读取路径参数 `traceId`
2. 调用 `TraceService.ListNodes()`
3. 返回节点数组

### 4.4 runtime 与路由接入

为让 `TraceService` 成为正式运行时能力，本次还修改了：

- `internal/bootstrap/rag/runtime.go`
- `cmd/server/main.go`
- `internal/adapter/http/rag/handlers.go`

#### `runtime.go`

新增：

- `Runtime.Trace *ragservice.TraceService`

并在构造 runtime 时调用：

- `traceService := ragservice.NewTraceService(traceRunRepo, traceNodeRepo)`

#### `handlers.go`

`RegisterRoutes(...)` 新增了一个参数：

- `traceService *ragservice.TraceService`

同时把 trace 路由挂到管理员子组：

- `admin := r.Group("/")`
- `admin.Use(middleware.RequireRole("admin"))`
- `RegisterTraceRoutes(admin, traceService)`

#### `main.go`

在 RAG 路由注册时，把 `runtime.Trace` 一并传入：

```go
raghttp.RegisterRoutes(protected, runtime.Conversation, runtime.Message, runtime.Feedback, runtime.Chat, runtime.Trace)
```

## 5. 和 chat trace 写入链路的关系

这次新增的是“查”的能力，原来的“写”链路没有改动。

现有 chat 写 trace 的位置仍然在：

- `internal/app/rag/service/rag_chat_service.go`

主要写入点有：

- `startTraceRun(...)`
- `finishTraceRun(...)`
- `recordTraceNode(...)`
- `recordChatTraceNode(...)`

也就是说，当前 trace 数据生命周期分成两段：

1. `RagChatService` 在聊天过程中持续写入 run/node
2. `TraceService` 在后台页面查询时把这些 run/node 查出来

这两部分现在已经闭环。

## 6. 当前前端对接情况

前端原本就已经有对应调用：

- `frontend/src/services/ragTraceService.ts`

它的 3 个请求：

- `getRagTraceRuns()`
- `getRagTraceDetail()`
- `getRagTraceNodes()`

和本次后端新增接口一一对应，因此本次落地后，前端不需要改接口地址即可直接联调。

## 7. 测试覆盖

本次新增了 service 层测试：

- `internal/app/rag/service/trace_service_test.go`

覆盖的核心场景包括：

1. `PageRuns()` 会正确应用默认分页和最大页大小
2. `GetDetail()` 能返回 run + nodes
3. run 不存在时会返回错误
4. repository 报错时会被正确包装

## 8. 当前边界与后续建议

### 8.1 当前边界

当前 trace 查询能力是“最小闭环”，所以有几个明确边界：

- 只支持按 `traceId / conversationId / taskId / status` 过滤
- 不支持更复杂的时间范围筛选
- 不解析 `ExtraData` 成结构化字段
- 不补 `username/userName` 联表信息

### 8.2 后续建议

后面如果继续增强，我建议优先做这几件事：

1. 增加时间范围筛选
2. 给 run 列表补 `username`
3. 把 node 的 `ExtraData` 解成结构化 JSON 返回
4. 补 trace run / node 的 HTTP handler 测试
5. 给 trace 页面增加 conversationId/taskId 筛选项

## 9. 结论

本次 trace 工作的价值是把原来“只能写、不能看”的观测数据真正变成可用的后台能力。

落地后，项目在 RAG 方向的能力状态会从：

- `chat 可运行，但排障困难`

推进到：

- `chat 可运行，且已有最小可观测闭环`

这对后续继续做：

- 检索质量分析
- Prompt 排查
- 取消异常定位
- pipeline / ingestion 观测扩展

都会有直接帮助。
