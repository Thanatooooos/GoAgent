# GoAgent 源码审计报告

生成日期：2026-05-03

---

本文档基于对 `goagent` 全量源码的静态分析，梳理已存在的代码问题与潜在风险，按严重程度分为 **CRITICAL / HIGH / MEDIUM / LOW** 四级。

---

## CRITICAL — 必须立即修复

### 1. `.env` 文件包含真实 API 密钥

**位置**：`.env` L4-L7

```
AI_PROVIDERS_BAILIAN_API_KEY=sk-a30df46fc0d94b40b340c8c54f86b4c8
AI_PROVIDERS_SILICONFLOW_API_KEY=sk-hnmxugynmnjvwmrpmhkaegmrzjnjhqbetgveeqjpniuiehmt
```

虽然 `.env` 已在 `.gitignore` 中，文件仍存在于开发者本地磁盘。`config.go:355` 通过 `v.AutomaticEnv()` 将环境变量直映射到 `Config` 结构体，密钥在进程内存中以明文存在。

**建议**：
- 立即轮换上述已泄露的 API Key
- 生产部署改用密钥管理服务（HashiCorp Vault / 云 KMS）注入环境变量
- 本地开发使用 `.env.example` 模板，真实密钥通过 1Password CLI 等方式注入

**参考**：[internal/framework/config/config.go:355](internal/framework/config/config.go#L355)

---

### 2. Debug AI 路由无认证保护

**位置**：[cmd/server/debug_ai_handler.go:19-30](cmd/server/debug_ai_handler.go#L19)

```
GET  /debug/ai/runtime   — 无认证
POST /debug/ai/chat      — 无认证，可直接消费付费 AI API
```

外部攻击者可直接调用 AI chat 接口，造成 API 计费损失和信息泄露。

**建议**：
- 加上 `RequireLogin()` + `RequireRole("admin")` 中间件
- 或增加配置开关 `app.debug-mode`，在生产环境默认禁用

---

### 3. 任意文件读取（路径穿越）

**位置**：[internal/app/ingestion/service/fetcher_node_runner.go:284-292](internal/app/ingestion/service/fetcher_node_runner.go#L284-L292)

```go
func normalizeLocalFilePath(location string) string {
    // ...
    return filepath.Clean(location)  // "../../../etc/passwd" 不变
}
```

`filepath.Clean` 无法防御 `../` 逃逸。返回值随后直接传给 `os.ReadFile()`（L159）。攻击者通过 `sourceLocation` 参数可读取服务器任意文件。

**建议**：
- 增加安全基路径（如 `data/documents/`），将 `filepath.Clean` 后的路径用 `filepath.Abs` 展开
- 与允许的基础目录做前缀匹配，拒绝不匹配的路径
- 实际文件读取前校验最终路径是否在允许范围内

---

### 4. SSRF 风险（两处）

**位置**：
- [internal/app/ingestion/service/fetcher_node_runner.go:211](internal/app/ingestion/service/fetcher_node_runner.go#L211)
- [internal/app/knowledge/schedule/remote_file_fetcher.go:237](internal/app/knowledge/schedule/remote_file_fetcher.go#L237)

对用户提供的 URL 发起 HTTP 请求时，仅校验 Scheme 为 `http/https`，未阻止对内网地址的访问：
- `http://localhost:5432` 可探测内部 PostgreSQL
- `http://169.254.169.254/latest/meta-data/` 可读取云实例元数据

且 `remote_file_fetcher.go:28` 使用了裸 `&http.Client{}`（无超时、无重定向限制）。

**建议**：
- 创建 HTTP transport 时增加 `Control` 回调，检查目标 IP 是否属于私有/回环/链路本地地址段
- 为 HTTP client 设置合理超时（已部分实现，30s，但不完整）
- 限制重定向次数，禁用跨协议重定向

---

## HIGH — 应尽快修复

### 5. 无优雅关闭机制

**位置**：[cmd/server/main.go:133-138](cmd/server/main.go#L133)

```go
r.Run(addr)  // 阻塞，无 signal.Notify，无 http.Server.Shutdown
```

进程被 kill 后无法等待进行中的请求和处理完成。
- `ExecutorService.Close()` 已实现等待在途 workflow，但 `main.go` 从不调用
- `ScheduleLockManager` 的分布式锁租约无法在退出前主动释放

**建议**：
- 增加 `signal.NotifyContext` 监听 `SIGINT`/`SIGTERM`
- 收到信号后依次：停止接收新请求 → 等待现有请求完 → 调用各 Runtime `Close()` → 关闭 HTTP Server

---

### 6. AutoMigrate 用于生产环境（两处）

**位置**：
- [internal/bootstrap/rag/runtime.go:144-156](internal/bootstrap/rag/runtime.go#L144-L156)
- [internal/bootstrap/ingestion/runtime.go:127-135](internal/bootstrap/ingestion/runtime.go#L127-L135)

RAG 和 ingestion 使用 GORM `AutoMigrate`，而 knowledge 有独立的 SQL migration 文件 (`migrations/`)。三种建表方式并存，AutoMigrate 在线上可能：
- 静默删除被移除字段对应的列
- 无法版本回滚
- 索引命名和默认值不可精确控制

**建议**：
- 统一使用 goose SQL migration（已有 `migrations/` 目录）
- 将 RAG 和 ingestion 的建表逻辑迁移为 SQL 文件
- 启动时通过 migration runner 执行，移除所有 `AutoMigrate` 调用

---

### 7. 多个后台 goroutine 缺少 recover()

唯一正确使用 `recover()` 的 goroutine：[executor_service.go:180-194](internal/app/ingestion/service/executor_service.go#L180-L194)，应作为参考模板。

| 文件 | 行号 | goroutine 用途 | panic 后果 |
|------|------|---------------|-----------|
| [bootstrap/knowledge/runtime.go](internal/bootstrap/knowledge/runtime.go#L251) | 251 | 定时 schedule 循环 | 文档调度静默停止 |
| [app/knowledge/schedule/knowledge_document_schedule_job.go](internal/app/knowledge/schedule/knowledge_document_schedule_job.go#L196) | 196 | 任务分发 | 可能崩溃应用 |
| [app/knowledge/schedule/schedule_lock_manager.go](internal/app/knowledge/schedule/schedule_lock_manager.go#L101) | 101 | 分布式锁心跳 | 锁租约静默丢失 |
| [infra-ai/chat/openai_style_chat_client.go](internal/infra-ai/chat/openai_style_chat_client.go#L223) | 223 | SSE 流读取 | 崩溃应用 |

**建议**：为每个 goroutine 增加 `defer recover()` 包装，panic 时记录堆栈日志，并确保 WaitGroup 计数正确递减。

---

### 8. context.WithTimeout 的 cancel 函数被丢弃（两处）

**位置**：
- [internal/app/knowledge/schedule/knowledge_document_schedule_job.go:237](internal/app/knowledge/schedule/knowledge_document_schedule_job.go#L237)
- [internal/app/knowledge/service/knowledge_document_service.go:1135](internal/app/knowledge/service/knowledge_document_service.go#L1135)

```go
taskCtx, _ := context.WithTimeout(base, timeout)
```

cancel 函数被赋给 `_`，context 直到超时到期才会被 GC，即使任务提前完成也无法释放。

**建议**：保存 cancel 函数，在任务完成时 `defer cancel()`。

---

### 9. 5 个无分页上限的 List 查询

| 文件 | 方法 | 行号 | 风险 |
|------|------|------|------|
| `rag/conversation_repo.go` | `ListByUserID` | 98-112 | 用户历史会话无限增长导致 OOM |
| `rag/rag_trace_node_repo.go` | `ListByTraceID` | 69-84 | Trace 节点数多时 OOM |
| `ingestion/task_node_repo.go` | `ListByTaskID` | 79-90 | Task 节点数多时 OOM |
| `knowledge/knowledge_document_schedule_repo.go` | `ListDue` | 97-119 | limit=0 时全量扫描 |
| `rag/conversation_message_repo.go` | `List` | 45-92 | limit=0 时全量返回 |

**建议**：为上述方法增加默认上限（如 1000），或强制调用方传入 limit 参数。

---

## MEDIUM — 应纳入近期计划

### 10. 约 189 处错误被静默丢弃 (`_ =`)

三大重灾区：

**a) SSE 发送错误全部丢弃**

**文件**：[internal/app/rag/service/rag_chat_service.go](internal/app/rag/service/rag_chat_service.go#L192)

约 20 处 `_ = sink.Send*()`，SSE 连接断开时调用方完全无感知。

**b) 数据库和存储操作错误丢弃**

**文件**：[internal/app/knowledge/service/knowledge_document_service.go:333](internal/app/knowledge/service/knowledge_document_service.go#L333)

`documentRepo.Delete()`、`storage.Delete()`、`markDocumentFailed()` 等关键操作错误全部丢弃。

**c) HTTP handler 错误返回丢弃**

所有 handler 文件中 `_ = c.Error(err)` 从不检查返回值，HTTP 响应写入失败无信号。

**建议**：至少对数据变更操作（写/删）的丢弃错误增加日志告警。

---

### 11. 无 CORS 中间件

**位置**：[cmd/server/main.go:118-121](cmd/server/main.go#L118)

Gin 引擎未配置任何 CORS 头。`UserContextMiddleware` 跳过 OPTIONS 但不返回 CORS 头。前端开发依赖 Vite proxy，但生产部署无反向代理时直接不可用。

**建议**：增加 `github.com/gin-contrib/cors` 中间件，配置允许的来源列表。

---

### 12. Ingestion 和 User 领域缺少事务支持

| 领域 | 现状 | 孤儿记录风险 |
|------|------|-------------|
| Knowledge | 4 个事务封装（DocumentProcess/Chunk/Delete/Schedule） | 无 |
| RAG | 1 个事务封装（ConversationDelete） | 无 |
| **Ingestion** | 无事务 | Task 创建后 TaskNode 创建失败 → 孤儿 Task |
| **User** | 无事务 | User 创建后 Session 创建失败 → 数据不一致 |

**建议**：参考 `document_process_transaction.go` 模式，为 ingestion 和 user 补充事务封装。

---

### 13. X-Login-Id 请求头可绕过 Token 认证

**位置**：[internal/middleware/user_context_middleware.go:26-28](internal/middleware/user_context_middleware.go#L26)

```go
if id := c.GetHeader("X-Login-Id"); id != "" {
    return id, nil  // 直接信任请求头中的 ID
}
```

如果 `LoadLoginUser` 实现未做额外校验，发送 `X-Login-Id: admin` 即可冒充管理员。

**建议**：在非调试环境下去除 `X-Login-Id` 提取逻辑，或仅在 `app.demo-mode=true` 时启用。

---

### 14. 弱密码策略

**位置**：[internal/app/user/service/user_service.go:25](internal/app/user/service/user_service.go#L25)

```go
const minPasswordLength = 6
```

仅校验最小长度，无大写字母、小写字母、数字、特殊字符要求。

---

### 15. 5 个废弃/僵尸依赖

| 依赖 | 版本 | 问题 |
|------|------|------|
| `github.com/golang/mock` | v1.3.1 | 2019 年停止维护，官方推荐 `go.uber.org/mock` |
| `github.com/pkg/errors` | v0.8.1 | Go 1.13+ 标准库已覆盖 `%w`，无必要保留 |
| `github.com/sirupsen/logrus` | v1.4.0 | 极旧，且项目已在使用 zap，应移除 |
| `github.com/patrickmn/go-cache` | v2.1.0+incompatible | 无维护，建议换 `github.com/dgraph-io/ristretto` |
| `golang.org/x/lint` | legacy | 已废弃，应换 `golangci-lint` 或 `staticcheck` |

---

### 16. 大量代码重复

| 重复元素 | 出现次数 | 建议共享位置 |
|----------|---------|-------------|
| `timePointer(t time.Time)` | 7 | `framework/convention/time_helpers.go` |
| `writeSuccess[T any](c, data)` | 4 | `framework/convention/response.go` |
| `parsePositiveInt(s, default)` | 3 | `framework/convention/query_helpers.go` |
| `pageResult[T any]` 结构体 | 3 | `framework/convention/paging.go` |
| `requireLoginUser(c)` | 2 | `framework/convention/auth_helpers.go` |
| `parseBool(s)` | 2 | `framework/convention/query_helpers.go` |
| context-path 路由逻辑 | 4 | `cmd/server/main.go` 内提取 `resolveContextPath()` |

---

### 17. 无 CI/CD 流程

项目无 `.github`、`.gitlab-ci.yml` 或任何 CI 配置。
- 3 个集成测试需手动设置环境变量才能运行
- 无自动 lint / build / test 检查
- 无自动部署流程

---

### 18. 三种建表方式并存，一致性差

| 模块 | 建表方式 |
|------|---------|
| Knowledge | SQL migration 文件（`migrations/`），但无代码执行 |
| RAG | GORM AutoMigrate |
| Ingestion | GORM AutoMigrate |
| Docker 初始化 | `docker/postgres/init/001_knowledge_schema.sql`（部分表） |

**建议**：统一为 goose SQL migration，启动时由代码自动执行。

---

## LOW — 可逐步改进

### 19. `interface{}` 未统一替换为 `any`（18 处）

Go 1.25 项目仍使用 `interface{}`，主要在：
- [framework/web/sse_emitter_sender.go](internal/framework/web/sse_emitter_sender.go#L21)
- [infra-ai/http/response_helper.go](internal/infra-ai/http/response_helper.go#L33)
- [framework/log/log.go](internal/framework/log/log.go#L22)

---

### 20. 缺少安全响应头

无 `X-Content-Type-Options`、`X-Frame-Options`、`Strict-Transport-Security`、`Content-Security-Policy`、`Referrer-Policy`。

**建议**：增加 security header 中间件。

---

### 21. PostgreSQL SSL 禁用

**位置**：[internal/adapter/repository/postgres/conn.go:97](internal/adapter/repository/postgres/conn.go#L97)

```go
query.Set("sslmode", "disable")
```

生产环境数据库连接未加密。建议通过配置项控制，生产环境要求 `require` 或 `verify-full`。

---

### 22. 缺少全局请求体大小限制

Gin 全局未设置 `MaxBytesReader`。虽然后端服务层有文件大小校验，但恶意大请求体可在到达业务逻辑前耗尽内存。

**建议**：增加全局中间件限制请求体大小。

---

### 23. 分页默认值不一致

| 页面 | 默认 page size |
|------|---------------|
| Users | 20 |
| Knowledge bases | 20 |
| Knowledge documents | 10 |
| Knowledge chunks | 10 |
| RAG traces | 10 |

**建议**：统一默认值为 20，或提取为常量。

---

### 24. 日志初始化错误被丢弃

**位置**：[cmd/server/main.go:29](cmd/server/main.go#L29)

```go
_ = fwlog.Init()
```

日志系统初始化失败时静默继续，后续所有日志调用可能 panic 或静默失败。

**建议**：初始化失败应 `log.Fatal` 终止启动。

---

### 25. 模块路径非标准

**位置**：[go.mod:1](go.mod#L1)

```
module local/rag-project
```

`local/` 前缀使模块无法被外部 `go get`，且 `go mod tidy` 行为与标准路径有差异。建议改为标准域名路径（如 `github.com/your-org/rag-project`）。

---

## 汇总

| 优先级 | 数量 | 最紧迫项 |
|--------|------|---------|
| CRITICAL | 4 | 密钥泄露、无认证AI路由、路径穿越、SSRF |
| HIGH | 5 | 无优雅关闭、AutoMigrate、缺recover、context泄露、无分页查询 |
| MEDIUM | 9 | 错误丢弃、无CORS、缺事务、认证绕过、弱密码、废弃依赖、代码重复、无CI、migration混乱 |
| LOW | 7 | interface{}、安全头、SSL、请求限制、分页不一致、日志初始化、模块路径 |

## 正面发现

- 生产代码中 **零** `panic()` 调用
- `ExecutorService` 的 goroutine 管理是模范模式（recover + semaphore + WaitGroup + asyncCtx）
- 所有 SQL 查询使用参数化占位符，**无 SQL 注入风险**
- 所有 `sync.WaitGroup` 使用正确 `Add`-before-spawn 模式
- `sync.Once` 在流处理回调中防重复调用设计正确
- 项目遵循清晰的 hexagonal / ports-adapters 分层架构
