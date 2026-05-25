# Code Review — 2026-05-24

对 `goagent` 代码库的全面代码审查，覆盖 memory V1 Phase 1.5~4、retrieve、cache、bootstrap、config 等模块。

---

## P0 — 会导致运行时错误或数据丢失

### P0-1. 类型断言的接口合规没有编译期保证

**文件：** [internal/app/rag/service/longtermmemory/service.go:194](internal/app/rag/service/longtermmemory/service.go#L194)

```go
func (s *MemoryService) FactRetriever() ragretrieve.FactMemoryRetriever {
    retriever, _ := s.recall.(ragretrieve.FactMemoryRetriever)
    return retriever
}
```

`retriever, _ := s.recall.(...)` 吞掉了 `ok`。如果 `recallService` 没有实现 `FactMemoryRetriever`（比如 `NewMemoryService` 创建的不带 vector 的纯 keyword recall），静默返回 `nil`。调用方在 [channels.go:151](internal/app/rag/core/retrieve/channels.go#L151) 被 `Enabled()` 跳过——整个 `memory_fact` channel 悄悄不工作，没有任何 warning。

**建议：** 在 `SetEmbeddingSupport` 里显式检查 `recallService` 是否实现了 `FactMemoryRetriever` 接口；或者把 `SearchFacts` 直接定义为 `RecallService` 接口方法。

### P0-2. 基于字符串匹配的 PostgreSQL 约束冲突检测极其脆弱

**文件：** [internal/app/rag/service/longtermmemory/service.go:458-470](internal/app/rag/service/longtermmemory/service.go#L458-L470)

```go
func isSingleValueActiveUniqueViolation(err error) bool {
    if strings.Contains(strings.ToLower(err.Error()), "uk_memory_item_single_active") {
        return true
    }
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        return pgErr.Code == "23505" && strings.EqualFold(strings.TrimSpace(pgErr.ConstraintName), "uk_memory_item_single_active")
    }
    return false
}
```

第二段 `errors.As` 是正确的做法，但第一段 `strings.Contains(err.Error(), ...)` 是危险的兜底：
- 任何包含该 constraint name 的错误消息都会触发（包括被 wrap 之后带错误上下文的非约束冲突错误）
- 如果 GORM 或 pgx 改版后不再把 constraint name 塞进错误消息，这个分支悄悄失效
- 如果有另一个 constraint 名字恰好包含相同前缀，可能错误匹配

**建议：** 删掉 `strings.Contains` 分支，只保留 `errors.As(&pgErr)` 的精确匹配。

### P0-3. `normalizedSaveInput` 的 `valueJSON` 默认值逻辑有隐患

**文件：** [internal/app/rag/service/longtermmemory/normalization.go:58-61](internal/app/rag/service/longtermmemory/normalization.go#L58-L61)

```go
valueJSON := strings.TrimSpace(input.ValueJSON)
if valueJSON == "" {
    valueJSON = content
}
```

用户不传 `valueJSON` 时默认赋值为完整 `content`。但 `content` 不是 JSON 格式，当 `valueType` 被 schema registry 规范化为 `json` 类型时，`canonicalizeJSONObject(content)` 会失败，等价性检测退化。虽然不会 panic，但行为不可预期。

**建议：** 当 `valueType=json` 且 `valueJSON` 为空时，至少 `log.Warnf`。

---

## P1 — 架构与可维护性问题

### P1-1. `longtermmemory` 包过大（约 5000 行），职责混合

**文件范围：** `internal/app/rag/service/longtermmemory/` 下 19 个文件

一个平铺的包承载了：
- 领域类型与常量
- CRUD service
- Recall service + ranking
- Cache 层（L1 request + L2 Redis）
- Gate 校验
- Conflict detector
- Schema registry
- Context renderer
- Retrieve projection
- 治理类型

`cache_support.go` 单个文件就 600+ 行。新成员难以理解模块边界，无法独立演进子系统。

**建议：** 至少拆成两个子包：
- `longterm/` 或 `memory/` — 主 service + gate + conflict + schema
- `memory/recall/` — recall service + cache support + retrieve projection

如果不想改包路径，至少按文件名严格分组，给每组加 `doc.go` 说明职责边界。

### P1-2. 缓存层存在潜在循环依赖风险

**涉及文件：**
- `internal/adapter/cache/redis/rag_memory_cache.go` → 依赖 `longtermmemory` (实现 `RecallCache` 接口)
- `internal/app/rag/service/longtermmemory/cache_support.go` → 依赖 `cache` / `cachemetrics`

当前依赖链：
```
adapter/cache/redis → longtermmemory (实现接口)
longtermmemory → cache / cachemetrics
```

`RecallCache` 接口定义在 `longtermmemory` 中，但 `RequestCache` / `TTLLRUCache` 定义在单独的 `cache` 包里。如果后续 `cache` 包需要引用 `longtermmemory` 的类型，就形成循环依赖。

**建议：** 把 `RecallCache` 接口定义搬到 `port/` 层，让 `longtermmemory` 和 `adapter/cache/redis` 都依赖 `port`。

### P1-3. 手工序列化映射函数重复严重

**文件：** [internal/app/rag/service/longtermmemory/cache_support.go](internal/app/rag/service/longtermmemory/cache_support.go)

四对 serialize/deserialize 函数：
- `memoryItemsToCached` / `cachedMemoryItemsToDomainItems`
- `runtimeFactProjectionsToCached` / `cachedFactProjectionsToRuntime`

加上 `retrieve_projection.go` 中的 `projectFactMemoryChunks`，这些 mapper 加起来超过 120 行纯字段拷贝。每次 `domain.MemoryItem` 加一个字段，至少有 4 处需要同步修改。

**建议：** 使用 Go 1.25 的结构体嵌入减少重复，或者写一个通用转换函数让各 serializer 调用。

### P1-4. `service.go` 中的 nil-receiver 防御策略不一致

**文件：** [internal/app/rag/service/longtermmemory/service.go](internal/app/rag/service/longtermmemory/service.go)

| 方法 | nil receiver 行为 |
|------|------------------|
| `SaveExplicitMemory` | 检查并返回 error |
| `ListMemories` | 检查并返回 error |
| `ExpireMemory` | 检查并返回 error |
| `RecallMemories` | 检查并返回空（**不报错**） |
| `FactRetriever` | 检查并返回 nil（**不报错**） |
| `SetMutationTransaction` | 检查并静默 return |
| `SetEmbeddingSupport` | 检查并静默 return |
| `SetRecallCache` | 检查并静默 return |

半吊子的防御策略比完全不做防御更危险——当调用方传 nil service 时，行为不一致，部分 panic、部分静默返回空、部分报错。

**建议：** 统一策略。要么全部方法都检查 nil receiver，要么在 `bootstrap` 层保证 service 不可能为 nil。二者择一。

---

## P2 — 代码风格与卫生

### P2-1. 冗余的 min/max 辅助函数

**文件：** [internal/app/rag/service/longtermmemory/service.go:393-405](internal/app/rag/service/longtermmemory/service.go#L393-L405)

```go
func minMemoryInt(a int, b int) int { ... }
func maxMemoryInt(a int, b int) int { ... }
```

项目使用 Go 1.25（`go.mod`），`min()` 和 `max()` 已内置。这两个函数可以直接删除。影响范围包括 `service.go`、`recall_service.go`、`context_renderer.go`、`retrieve_projection.go`。

### P2-2. `rag_chat_prepare.go` 中的静默错误吞没

**文件：** [internal/app/rag/service/rag_chat_prepare.go:43-51](internal/app/rag/service/rag_chat_prepare.go#L43-L51)

```go
longTermMemoryStage, err := s.runLongTermMemoryStage(...)
if err != nil {
    longTermMemoryStage = ragChatLongTermMemoryStageResult{}
}

sessionRecallStage, err := s.runSessionRecallStage(...)
if err != nil {
    sessionRecallStage = ragChatSessionRecallStageResult{}
}
```

长期记忆 recall 和 session recall 两个 Phase 4 刚完成的核心功能，失败时静默降级为空结果，没有任何 warning log。如果配置错误导致这两个 stage 总是失败，用户和开发者都完全感知不到。

**建议：** 至少加上 `log.Warnf`。

### P2-3. testdata 目录膨胀严重

- `testdata/t2retrieval_passages_mapping.json` — **50002 行**
- `testdata/docs_markdown_manifest_v2.json` — **26481 行**

这些巨型 JSON 文件在 git clone、fetch、log 操作中增加巨大负担。

**建议：** `.gitattributes` 标记为 `linguist-generated`，或压缩存储（gzip + Go embed），至少确认有对应的生成脚本。

### P2-4. 服务层错误消息信息量不足

**文件：** [internal/app/rag/service/longtermmemory/recall_service.go:142](internal/app/rag/service/longtermmemory/recall_service.go#L142) 等多处

```go
return nil, exception.NewServiceException("failed to list kb rule memory items", err)
```

错误消息非常通用，排障时只能靠堆栈，无法从消息定位是哪个 user 的哪个 scope 出了问题。

**建议：** 在错误消息中包含关键上下文，如 `fmt.Errorf("failed to list kb rule memory items: userID=%s kbIDs=%v: %w", userID, knowledgeBaseIDs, err)`。

### P2-5. `config.go` 硬编码了 Spring 风格的命名

**文件：** [internal/framework/config/config.go](internal/framework/config/config.go)

```go
type SpringConfig struct { ... }
type ServletConfig struct { ... }
type SaTokenConfig struct { ... }
```

这是一个 Go 项目，但配置结构直接沿用了 Java Spring 命名。不影响功能，但对于非 Java 背景的贡献者会困惑。

### P2-6. `.exe` 和 `.pyc` 二进制文件在仓库中

根目录有 `server.exe`、`corpus-loader.exe`、`lexical-rebuild.exe`、`retrieve-eval.exe` 以及 `scripts/__pycache__/*.pyc`。这些应该加到 `.gitignore`。

---

## P3 — 小瑕疵，累积效应

### P3-1. `SessionRecallResult` 中的 unexported 字段作为 API 契约

**文件：** [internal/app/rag/service/session_recall_service.go:50-53](internal/app/rag/service/session_recall_service.go#L50-L53)

```go
type SessionRecallResult struct {
    // ... exported fields ...
    candidateCount         int
    skippedPerMessageLimit int
    truncatedBy            string
}
```

三个 unexported 字段在外部无法读取。如果它们存的是给 caller 用的数据，应改为导出字段或添加 getter。

### P3-2. `go.mod` 使用非标准 module path

```
module local/rag-project
```

`local/` 前缀通常用于 Go workspace 的内部替换。如果项目将来要被其他模块引用，需要改为正式路径。

### P3-3. `CachedMemoryItem` 与 `domain.MemoryItem` 高度重合

**文件：** [internal/app/rag/service/longtermmemory/cache.go:15-33](internal/app/rag/service/longtermmemory/cache.go#L15-L33)

`CachedMemoryItem` 几乎是 `domain.MemoryItem` 的子集，但手动维护了一个独立结构体。domain 结构体增加字段时，cache 层可能丢失数据。

**建议：** 考虑直接在 cache 中序列化 `domain.MemoryItem`（对其加 JSON tag），而不是维护影子类型。

### P3-4. 魔法数字散落各处

```go
const defaultMemoryRecallItems = 6       // types.go
const defaultMemoryRecallMaxChars = 1600 // types.go
const maxRecallSearchTokens = 8          // recall_service.go:490
const candidateLimit = topK * 4          // retrieve_projection.go:35
```

这些常量在定义位置有注释，但在使用位置没有引用回定义的说明。建议集中到 `constants.go` 管理。

---

## 汇总

| 优先级 | 数量 | 核心主题 |
|--------|------|----------|
| P0 | 3 | 运行时正确性（nil 接口、脆弱错误匹配、脏默认值） |
| P1 | 4 | 架构健康（超大包、循环依赖风险、映射重复、nil receiver 不一致） |
| P2 | 6 | 代码卫生（冗余函数、testdata 膨胀、错误吞没、Spring 命名、二进制文件） |
| P3 | 4 | 小瑕疵（unexported 字段、module path、影子类型、魔法数字） |

**建议优先修复顺序：** P0-2 → P0-1 → P1-1 → P1-2，其余随日常开发逐步消化。
