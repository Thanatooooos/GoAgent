# Memory Cache Refactor Plan

日期：2026-05-26

---

## 一、改什么，为什么改

### 当前的核心问题就一个

**Fact ranking 缓存用了原始 query 文本做 key。** 用户不会在同一会话里敲两遍同样的句子，所以 Redis L2 和 conversation L1 的实际命中率趋近于零。三层缓存里，只有 request L1（同一请求内）真正在干活。

### 改的目标

```
改动前：
  Fact 候选 → rank → 缓存排名结果（key 含 query）  ← 命中率 ~0%

改动后：
  Fact 候选列表 → 缓存候选本身（key 不含 query）  ← 命中率取决于 scope version
  Rank → 本地 CPU（不缓存）                        ← 毫秒级，不需要缓存
```

---

## 二、具体改动

### 改动 1：新增 `FactCandidateCache` 替代 `FactRankingCache`（Redis）

**为什么：** 候选列表（用户的 active knowledge memory 本身）与 query 无关，只跟 user + scope + version 有关。一个用户问 "怎么配超时" 和 "timeout 在哪" 用的是同一批候选，只是排名不同。

**具体做法：**

在 `port/memory_recall_cache.go` 新增：

```go
// FactCandidateCacheKey 候选列表缓存 key —— 不含 query
type FactCandidateCacheKey struct {
    UserID           string
    KnowledgeBaseIDs []string
    Limit            int
    ScopeVersions    ScopeVersions
}

// FactCandidateCacheValue 候选列表的完整数据
type FactCandidateCacheValue struct {
    Items         []CachedMemoryItem       `json:"items"`
    Embeddings    map[string][]float32     `json:"embeddings"`    // memoryID → vector
    CandidateCount int                     `json:"candidateCount"`
}
```

在 `MemoryRecallCache` 接口新增两个方法：

```go
GetFactCandidates(ctx, key FactCandidateCacheKey) (FactCandidateCacheValue, bool, error)
SetFactCandidates(ctx, key FactCandidateCacheKey, value FactCandidateCacheValue, ttl time.Duration) error
```

旧方法 `GetFactRankings` / `SetFactRankings` 保留方法签名但标记为 deprecated（编译兼容），适配器层直接返回 `false, nil`。

### 改动 2：`recall/cache_support.go` — 重写 `loadFactRankingProjections`

**当前**（`cache_support.go:104-180`）：一个长函数同时做缓存读写和 ranking，Redis key 里嵌 query。

**改为**：拆成两步。

```go
// 第一步：加载候选列表（带 Redis 缓存）
func (r *recallService) loadFactCandidates(
    ctx context.Context,
    userID string,
    query string,             // 仅用于本地 keyword filter
    knowledgeBaseIDs []string,
    candidateLimit int,
) ([]domain.MemoryItem, map[string]float32, port.ScopeVersions, string, error) {

    // 1. request L1（key 不带 query，只有 user + scope + version）
    // 2. Redis L2 → GetFactCandidates
    //    命中 → 把候选列表 + vectors 从 Redis 拿出来
    //    未命中 → DB 查（不带 SearchText 的 List，加载全部 active knowledge）
    //           → pgvector 批量搜
    //           → 写入 Redis
    // 3. 本地做 keyword filter + vector score 合并
}
```

`loadFactRankingProjections` 变为薄调用：

```go
func (r *recallService) loadFactRankingProjections(
    ctx context.Context,
    userID, query string,
    knowledgeBaseIDs []string,
    candidateLimit int,
) ([]memoryRecallProjection, int, port.ScopeVersions, string, string, string, error) {

    // 1. 先查 request L1（保留，同一请求内同理 query 可能走两次：RecallMemories + SearchFacts）
    requestKey := buildFactRequestCacheKey(userID, query, knowledgeBaseIDs, candidateLimit, ...)
    if cached, hit := r.readFactRequestCache(ctx, requestKey); hit {
        return ...
    }

    // 2. 加载候选（走新的 Redis 候选缓存，或 DB）
    candidates, vectorScores, versions, embeddingLayer, err := r.loadFactCandidates(...)

    // 3. 本地 rank（纯 CPU，不缓存）
    ranked := rankRecallMemories(query, candidates, vectorScores)
    if len(vectorScores) > 0 {
        ranked = rerankRecallMemoriesWithVectorScores(ranked, vectorScores)
    }

    // 4. 写 request L1
    r.writeFactRequestCache(ctx, requestKey, ranked, len(candidates))

    return ranked, len(candidates), versions, "computed", embeddingLayer, "candidate_cache_hit", nil
}
```

删掉的代码：
- `loadFactRankingProjections` 中所有 `GetFactRankings` / `SetFactRankings` 调用
- `FactRankingCacheKey` 类型的直接使用（类型保留，方法标记 deprecated）

### 改动 3：`session_recall_cache.go` — cache key 去 query

**为什么：** 当前 `buildSessionRecallBaseKey` 把原始 query 嵌在 key 里。Session recall 的候选结果（同一个 conversation 的 chunk 集合）跟 query 的关系很小——query 影响的是 excerpt selection（选哪个窗口），而那是纯 CPU 的 token match，不需要缓存。

**具体做法：**

```go
// 改动前（session_recall_cache.go:391-404）
func buildSessionRecallBaseKey(conversationID, userID, query, excludeMessageID string, options SessionRecallOptions) string {
    return fmt.Sprintf(
        "%s|%s|%s|%s|%d|%d|%d|%d|%d",
        conversationID, userID,
        query,  // ← 去掉这个
        excludeMessageID,
        options.MaxExcerpts, ...
    )
}

// 改动后
func buildSessionRecallBaseKey(conversationID, userID, fingerprintKey string, options SessionRecallOptions) string {
    return fmt.Sprintf(
        "%s|%s|%s|%d|%d|%d|%d|%d",
        conversationID, userID,
        fingerprintKey,  // ← 用 fingerprint 替代 query
        options.MaxExcerpts, ...
    )
}
```

调用方 `Recall` 函数相应调整。fingerprint 只包含 chunk 集合的结构信息（count + chunk IDs hash），不包含 latest chunk 的时间戳，避免新增一条消息就全失效。

### 改动 4：fingerprint 从 "latest" 改为 "set hash"

**当前：** `buildSessionRecallFingerprintKey` 包含 `LatestUpdateTime.UnixNano()`, `LatestChunkID`, `LatestMessageID`。写一条新消息 → 新 chunk → fingerprint 完全变化 → 所有缓存失效。

**改为：**

```go
func buildSessionRecallFingerprintKey(fingerprint domain.SessionRecallFingerprint) string {
    if !fingerprint.Exists {
        return "none"
    }
    return fmt.Sprintf(
        "%d|%s",  // ← 只保留 count + 所有 chunk ID 的 hash
        fingerprint.RecallableCount,
        fingerprint.ChunkIDsHash,  // ← repo 层需新增此字段
    )
}
```

`SessionRecallFingerprint` 需要新增 `ChunkIDsHash string`。Repo 层 `GetRecallFingerprint` 返回时计算 `MD5(sortedChunkIDs)`。

这样：写一条新消息 → count +1, hash 变化 → 缓存失效。但仅发消息不产生新 recallable chunk（`IsSummarized=false`）时 fingerprint 不变。

### 改动 5：Conversation cache TTL 默认值提高

```go
// session_recall_cache.go:137
if options.ConversationTTL <= 0 {
    options.ConversationTTL = 30 * time.Minute  // 原来 10 分钟
}
```

### 改动 6：清理配置项

删除 config 中的：
- `rag.memory.cache.fact-ttl-seconds` — fact ranking 不再进 Redis
- `rag.memory.cache.empty-fact-ttl-seconds` — 同上

新增：
- `rag.memory.cache.fact-candidate-ttl-seconds` — 候选列表 Redis TTL（默认 10 分钟）

---

## 三、改动涉及的文件清单

```
# 核心改动（必须）
port/memory_recall_cache.go          → 新增 FactCandidateCache* 类型和方法
recall/cache_support.go              → 重写 loadFactRankingProjections，新增 loadFactCandidates
recall/cache_keys.go                 → 新增 buildFactCandidateCacheKey
recall/service.go                    → loadFactMemoryCandidatesWithLimit 调整为全量加载 + 本地 filter
session_recall_cache.go              → 重写 cache key 构建逻辑，去 query
session_recall_service.go            → fingerprint hash 传入

# Redis 适配器（必须）
adapter/cache/redis/rag_memory_cache.go → 实现 GetFactCandidates / SetFactCandidates

# Domain 模型（必须）
domain/memory_item.go                → SessionRecallFingerprint 新增 ChunkIDsHash

# Postgres 适配器（必须）
adapter/repository/postgres/rag/memory_item_repo.go → GetRecallFingerprint 返回 ChunkIDsHash

# 配置（必须）
framework/config/                    → 增删 cache 配置项
bootstrap/rag/runtime.go             → 更新配置传递

# 测试（必须更新）
recall/cache_keys_test.go            → 新 key 格式
recall/ranking_test.go               → 不受影响（ranking 逻辑不变）
recall/projection_test.go            → 不受影响
adapter/cache/redis/*_test.go        → 新增 candidate cache 测试
adapter/http/rag/*_test.go           → 少量引用更新
```

---

## 四、不影响的部分

以下模块**不需要改动**：

- **Query embedding 缓存**：三层结构正确，key 是 query 文本但对 embedding 来说这是对的（同样 query = 同样 embedding）
- **Rule memory 缓存**：Redis + request L1 正确，scope version 失效机制正确
- **Request-scope L1**：整体机制正确，继续保留
- **Ranking 逻辑**：`rankRecallMemories` / `rerankRecallMemoriesWithVectorScores` / `scoreMemoryText` / `buildRecallSearchTokens` 全部不变
- **Governance 链路**：`gate.go` / `conflict_detector.go` / `save_service.go` 不变
- **`TouchLastUsed` / `persistMemoryEmbedding`**：不变
- **HTTP API 层**：`RagChatService` / handler 层不变

---

## 五、改动后的缓存效果

```
改动后的一次 recall：

query embedding:
  request L1 ──→ Redis ──→ 计算     ✅ 命中率高（same query = same embedding）

rule memories:
  request L1 ──→ Redis ──→ DB       ✅ scope version 失效，命中率高

fact candidates:
  request L1 ──→ Redis ──→ DB       ✅ key 不含 query，同 scope 复用
  (Redis 存完整 active knowledge 列表 + vectors)

fact ranking:
  本地 CPU 算，不缓存                  ✅ 毫秒级，无意义缓存

session recall:
  request L1 ──→ conversation L1    ✅ key = convID + fingerprint hash
  ──→ vector search                 ✅ 不随 query 变化而失效
```

---

## 六、实施顺序

1. **先改 session recall cache key**（`session_recall_cache.go` + fingerprint） — 独立改动，风险最低
2. **新增 `FactCandidateCache` 类型和接口**（`port` + Redis 适配器）— 纯增量
3. **重写 `loadFactRankingProjections`** — 切换到候选缓存
4. **清理旧 FactRanking 缓存代码和配置** — 删除 dead code
5. **跑全量测试** — 确认回归

每步都可以独立提交，不会破坏中间状态。

---

## 七、风险与缓解

| 风险 | 缓解 |
|------|------|
| 候选列表不下载 keyword filter，大量 memory 时 Redis value 过大 | 设 `MaxCandidatesPerScope` 上限（默认 96），超过时在 DB 层截断 |
| Fingerprint hash 计算增加 DB 查询负担 | 只在 `GetRecallFingerprint` 中查询 recallable chunk IDs，可复用已有的 `ExistsRecallable` 查询 |
| 旧 Redis fact ranking key 仍占用内存 | 旧 key 有 TTL，自然过期；不主动清理 |
