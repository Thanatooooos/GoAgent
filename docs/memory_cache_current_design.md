# Memory Cache Current Design

日期：2026-05-26

---

## 1. 文档目的

这份文档只描述 **当前代码已经实现的 memory cache 设计**，不讨论重构设想。

覆盖范围：

- `session recall` 缓存
- `long-term memory recall` 缓存
- 共享 `MemoryRecallCache` Redis 适配层
- cache key、value、读取时机、写入时机、TTL 和失效方式

相关主文件：

- `internal/app/rag/service/session_recall_service.go`
- `internal/app/rag/service/session_recall_cache.go`
- `internal/app/rag/service/longtermmemory/recall/cache_support.go`
- `internal/app/rag/service/longtermmemory/recall/cache_keys.go`
- `internal/adapter/cache/redis/rag_memory_cache.go`
- `internal/app/rag/port/memory_recall_cache.go`
- `internal/bootstrap/rag/runtime.go`

---

## 2. 总体设计

当前 memory cache 不是单层缓存，而是按能力拆成了几条链路：

1. `session recall result cache`
2. `query embedding cache`
3. `long-term memory rule recall cache`
4. `long-term memory fact ranking cache`
5. `scope version cache / invalidation counter`

其中有三种作用域：

- `request-scope L1`
  - 只在单次请求上下文内有效
  - 主要避免同一请求内重复 recall / 重复 embedding
- `conversation-scope L1`
  - 只用于 `session recall`
  - 是进程内 TTL + LRU cache
- `Redis L2`
  - 用于共享 rule memory、fact ranking、query embedding、scope versions

并不是所有链路都有三层：

- `session recall result`
  - `request L1 -> conversation L1 -> recompute`
- `session recall query embedding`
  - `request L1 -> Redis L2 -> recompute`
- `LTM rule recall`
  - `request L1 -> Redis L2 -> DB`
- `LTM fact ranking`
  - `request L1 -> Redis L2 -> recompute`
- `LTM query embedding`
  - `request L1 -> Redis L2 -> recompute`

---

## 3. 运行时装配

缓存能力在 `internal/bootstrap/rag/runtime.go` 接入：

- 显式长期记忆服务 `explicitMemoryService` 会挂上 `RecallCacheOptions`
- `sessionRecallService` 会挂上 `SessionRecallCacheOptions`
- 两者共用同一个 `MemoryRecallCache` Redis 适配器

当前默认 TTL：

- `session recall conversation TTL`
  - 10 分钟
- `session recall empty result TTL`
  - 30 秒
- `query embedding TTL`
  - 30 分钟
- `LTM rule TTL`
  - 10 分钟
- `LTM fact TTL`
  - 3 分钟
- `LTM empty fact TTL`
  - 30 秒

这些默认值分别来自：

- `session_recall_cache.go`
- `longtermmemory/types/options.go`

---

## 4. 共享 Redis 接口

共享接口定义在 `internal/app/rag/port/memory_recall_cache.go`。

当前 Redis 层支持这些对象：

- `RuleMemoryCacheKey -> RuleMemoryCacheValue`
- `FactRankingCacheKey -> FactRankingCacheValue`
- `QueryEmbeddingCacheKey -> []float32`
- `GlobalVersion / KBVersion`

也就是说，当前 Redis L2 **不缓存 session recall result 本身**，只缓存：

- LTM rule memories
- LTM fact rankings
- query embeddings
- scope version counters

Redis 适配器在 `internal/adapter/cache/redis/rag_memory_cache.go`。

当前主要 Redis key 家族：

- `goagent:rag:embed:v1:<model>:<queryHash>`
- `goagent:rag:memory:rules:v1:<user>:<kbHash>:<globalVersion>:<kbVersionHash>`
- `goagent:rag:memory:facts:v1:<user>:<queryHash>:<kbHash>:<globalVersion>:<kbVersionHash>:<candidateLimit>:<model>:<rankVersion>`
- `goagent:rag:memory:ver:global:<user>`
- `goagent:rag:memory:ver:kb:<user>:<kbID>`

---

## 5. Session Recall Cache

### 5.1 缓存了什么

`session recall` 当前缓存两类东西：

1. recall 最终结果
2. query embedding

其中：

- recall 最终结果不会进 Redis
- query embedding 可以进 Redis

### 5.2 recall result 的 value 是什么

缓存值是 `SessionRecallResult`，里面包含：

- `Used`
- `Hits`
- `Context`
- `TopScore`
- `RecallFingerprint`
- `EmbeddingCacheLayer`
- `RecomputeReason`

这表示缓存的是一整次 session recall 的最终产物，而不是中间候选集。

### 5.3 recall result 的 key 是什么

当前 key 分两层：

- `baseKey`
- `fullKey`

`baseKey` 由这些字段组成：

- `conversationID`
- `userID`
- `lower(query)`
- `excludeMessageID`
- `MaxExcerpts`
- `MaxChunksPerMessage`
- `MaxPromptTokens`
- `ExcerptTargetTokens`
- `ExcerptOverlapTokens`

`fingerprintKey` 由这些字段组成：

- `RecallableCount`
- `LatestUpdateTime`
- `LatestChunkID`
- `LatestMessageID`

`fullKey = baseKey + "|" + fingerprintKey`

设计意图是：

- `baseKey` 表示“这次怎么查”
- `fingerprintKey` 表示“可 recall chunk 集合的当前版本”
- `fullKey` 表示“在这个 chunk 集合版本上，这次 query 的 recall 结果”

### 5.4 recall result 存在哪

当前有两层：

1. `request-scope cache`
2. `conversation-scope cache`

`request-scope cache`

- key：`fullKey`
- value：`SessionRecallResult`
- 生命周期：随 request context 结束而结束

`conversation-scope cache`

- 底层：进程内 `TTLLRUCache`
- 真正存值：`store[fullKey] = SessionRecallResult`
- 辅助映射：
  - `baseKeys[baseKey] = fullKey`
  - `fullKeys[fullKey] = baseKey`

辅助映射的作用是做 fingerprint 失效控制。

### 5.5 recall result 什么时候读取

`Recall(...)` 主流程中，顺序是：

1. 先读 fingerprint
2. 如果没有 recallable chunks，直接返回空结果
3. 查 request L1
4. 查 conversation L1
5. 都没命中，再真正执行 embedding + vector search + excerpt selection

### 5.6 recall result 什么时候写入

以下场景会写缓存：

- `fingerprint.Exists == false`
  - 写入空结果
- 向量检索没有候选
  - 写入空结果
- 有候选但最终没有选出 hit
  - 写入空结果
- 成功生成 `SessionRecallResult`
  - 写入完整结果

写入时同时写：

- request L1
- conversation L1

### 5.7 recall result 什么时候过期

`request L1`

- 没有独立 TTL
- 请求结束即失效

`conversation L1`

- 正常命中结果：`ConversationTTL`
- 空结果：`EmptyResultTTL`

当前默认：

- 正常结果：10 分钟
- 空结果：30 秒

### 5.8 recall result 什么时候删除或失效

有四种方式：

1. request 生命周期结束
2. conversation cache TTL 到期
3. LRU 淘汰
4. fingerprint 变化导致旧 `fullKey` 失效

第 4 种是当前最重要的失效机制：

- 同一个 `baseKey`
- 如果现在对应的 `fullKey` 已经变了
- 说明底层 recallable chunk 集合变了
- 旧缓存会被删掉

### 5.9 session recall fingerprint 是怎么来的

当前 fingerprint 来自数据库聚合查询，只统计：

- 当前 `conversation_id`
- 当前 `user_id`
- `role = 'user'`
- `is_summarized = true`

也就是说，只有“被长消息处理过并切了 chunk 的用户消息”才会进入 recallable 集合。

如果用户只是发了普通短消息，不产生 recallable chunk，fingerprint 通常不会变。

---

## 6. Session Recall Query Embedding Cache

### 6.1 缓存了什么

缓存值是 query 的 embedding 向量：

- key：`QueryEmbeddingCacheKey`
- value：`[]float32`

### 6.2 key 是什么

接口层 key：

- `Query`
- `EmbeddingModel`

Redis 层最终 key 形式：

- `goagent:rag:embed:v1:<model>:<queryHash>`

### 6.3 读取顺序

当前顺序是：

1. request L1
2. Redis L2
3. 调 embedding 服务实时计算

### 6.4 写入时机

只有在实际计算出 embedding 之后才会回填：

- 写 request L1
- 如果启用了 Redis，再写 Redis L2

### 6.5 TTL

默认 TTL 为 30 分钟。

---

## 7. Long-Term Memory Recall Cache

长期记忆 recall 当前拆成两条缓存链：

1. `rule memory cache`
2. `fact ranking cache`

两者都支持：

- request L1
- Redis L2

没有 conversation-scope L1。

### 7.1 Rule Memory Cache

#### 缓存了什么

缓存的是“当前 scope 下的 active preference memory 列表”。

- key：`RuleMemoryCacheKey`
- value：`RuleMemoryCacheValue{ Items []CachedMemoryItem }`

#### key 是什么

由这些字段组成：

- `UserID`
- `KnowledgeBaseIDs`
- `ScopeVersions`

注意：

- 不含 query

这是因为 rule memory 当前被视为“scope 相关、query 弱相关”的稳定集合。

#### 读取顺序

1. 读取 scope versions
2. 查 request L1
3. 查 Redis L2
4. 都没命中就查 DB

#### 写入时机

当 Redis miss 后，DB 加载出的原始 rule memories 会写入 Redis。

随后会基于 query 临时投影成 `memoryRecallProjection`，再写入 request L1。

也就是说：

- Redis 缓存的是原始 memory items
- request L1 缓存的是当前请求内可直接使用的 projections

#### TTL

默认 TTL：10 分钟

#### 失效方式

依赖 `ScopeVersions`：

- global memory 变更 -> `IncrGlobalVersion`
- KB memory 变更 -> `IncrKBVersion`

只要 version 变了，Redis key 就变，旧值自然失效。

### 7.2 Fact Ranking Cache

#### 缓存了什么

缓存的是“已经排好序的 fact recall projections”。

- key：`FactRankingCacheKey`
- value：`FactRankingCacheValue`

value 里主要是：

- `CandidateCount`
- `Items []CachedFactProjection`

也就是说，当前 Redis L2 缓存的是 **query-specific 排名结果**，不是原始候选集。

#### key 是什么

由这些字段组成：

- `UserID`
- `Query`
- `KnowledgeBaseIDs`
- `CandidateLimit`
- `EmbeddingModel`
- `RankVersion`
- `ScopeVersions`

所以它同时受三类因素影响：

- query 文本
- 检索参数
- memory scope 版本

#### 读取顺序

1. 先查 request L1
2. 如果 Redis 可用，先读 scope versions
3. 用带 version 的 key 查 Redis L2
4. miss 后再现场计算 ranking

#### 写入时机

实际计算完成后会：

- 写 Redis L2
- 写 request L1

如果结果为空，会使用 `EmptyFactTTL`。

#### TTL

默认：

- 正常结果：3 分钟
- 空结果：30 秒

#### 失效方式

依赖 `ScopeVersions`：

- memory item 保存/更新/过期后
- 对应 scope version 会自增
- 下次 fact ranking key 改变
- 旧缓存自然失效

### 7.3 LTM Query Embedding Cache

长期记忆 fact recall 也会缓存 query embedding，链路和 `session recall` 类似：

1. request L1
2. Redis L2
3. 现场计算

当前 Redis 也是复用 `QueryEmbeddingCacheKey`。

---

## 8. Scope Version 与缓存失效

长期记忆缓存的核心失效机制不是 TTL，而是 `scope version`。

当前有两类版本号：

- `global version`
- `kb version`

Redis 中对应两个 key 家族：

- `rag:memory:ver:global:<user>`
- `rag:memory:ver:kb:<user>:<kbID>`

当 memory item 写入或状态变化时：

- 如果是 `global` memory
  - `IncrGlobalVersion`
- 如果是 `kb` memory
  - `IncrKBVersion`

因此：

- rule memory cache
- fact ranking cache

都会随着 version 变化自动换 key，而不是主动删除旧 key。

旧 key 是否继续存在，由 Redis TTL 决定。

---

## 9. 当前设计的实际工作方式

如果从运行时视角总结，当前 cache 行为可以概括为：

### 9.1 session recall

- 先看当前会话里有没有 recallable chunk
- 如果有，再尝试复用同 query、同 chunk 版本下的 recall result
- 如果结果缓存没命中，再复用 query embedding
- 最后才真正做向量检索和 excerpt selection

### 9.2 long-term memory rule recall

- 先用 scope versions 判断当前 scope 的 rule memory 集合版本
- Redis 缓存原始 rule memory 列表
- 每次请求内再基于 query 做轻量 projection 和排序

### 9.3 long-term memory fact recall

- 先复用 query embedding
- 再尝试复用“同 query、同 scope version、同 rank 参数”的 fact ranking 结果
- miss 时重新跑候选加载、keyword/vector 融合和 rerank

---

## 10. 当前设计的几个关键特点

1. `session recall result` 没有进 Redis，只做了本地 request/conversation 两层缓存。
2. `query embedding` 是两条 recall 链路共享的 Redis 能力。
3. `LTM rule cache` 缓存的是原始 memory items，不是最终 context。
4. `LTM fact cache` 缓存的是 query-specific 排名结果。
5. `LTM` 的强失效机制依赖 `scope versions`，不是主动删 key。
6. `session recall` 的强失效机制依赖 `fingerprint`，不是主动删 key。
7. 空结果会单独使用更短 TTL，避免“空命中”长期占位。

---

## 11. 一句话总结

当前 memory cache 的真实设计是：

- `session recall` 偏向“本地短生命周期缓存 + fingerprint 保正确性”
- `long-term memory recall` 偏向“request L1 + Redis L2 + scope version 做跨请求复用”
- `query embedding` 是两条链路共享的基础缓存能力

它并不是一个统一的单层缓存系统，而是围绕不同 recall 路径分别做了针对性缓存。
