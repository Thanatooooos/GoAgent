# Memory Module Code Review: 问题与风险

审查日期：2026-05-26
审查范围：`session_recall_*`, `longtermmemory/recall/*`, `longtermmemory/governance/*`, `port/memory_recall_cache.go`

---

## 一、缓存设计问题（最严重的一类）

### 1.1 缓存 key 嵌入了原始 query 文本——命中率极低

**位置：**
- `session_recall_cache.go:391-404` `buildSessionRecallBaseKey` — 用原始 query 做 key
- `recall/cache_keys.go:17-31` `buildFactRequestCacheKey` — 同样把原始 query 嵌入 key

**问题：** 用户几乎不会在同一会话中两次输入**完全相同**的查询。稍微改一个词、加一个标点，就产生新的 cache key，之前的缓存完全浪费。

对于 session recall，这意味着：
- 用户第一轮问 "那个超时错误是什么原因？"
- 第二轮追问 "刚才说的超时是多少秒？"
- 两个不同 query → 两次独立的 vector search + embedding，conversation cache 白建了

对于 LTM fact ranking，Redis L2 也几乎不会命中——除非 scope version 不变且 query 完全一致。实际收益仅限于：同一请求内重复调用（已被 request-scope L1 覆盖）。

**影响等级：P0 结构性问题。** 整个 conversation-scope 和 Redis fact-ranking 缓存在真实场景下命中率趋近于零。

**建议方向：**
- Session recall：用 embedding vector 的 top-N 维度 hash + fingerprint 做 key
- LTM fact ranking：用 query embedding 的 LSH/local-sensitive hash + scope versions 做 key，允许近似匹配

### 1.2 Fingerprint 粒度太粗——写一条消息就失效全部缓存

**位置：** `session_recall_cache.go:378-389` `buildSessionRecallFingerprintKey`

```go
fingerprint.LatestUpdateTime.UTC().UnixNano(),
fingerprint.LatestChunkID,
fingerprint.LatestMessageID,
```

**问题：** 只要写一条新消息（产生一个新 chunk），fingerprint 就变了——**所有**该 conversation 的旧缓存全部失效，包括与新增 chunk 完全无关的历史 query。

实际上，新消息只新增了几个 chunk，已有 chunk 的 vector 相似度完全没变。正确的做法应该是按 chunk 粒度或 chunk hash set 做增量指纹，只在新 chunk 可能影响召回排序时才失效。

**影响等级：P1。** 放大了 1.1 的问题——不仅命中率低，存下来的缓存还很容易被无效化。

### 1.3 Conversation cache TTL 过短

**位置：** `session_recall_cache.go:137-138`

```go
if options.ConversationTTL <= 0 {
    options.ConversationTTL = 10 * time.Minute
}
```

**问题：** 一次对话可能持续 20-30 分钟。10 分钟 TTL 意味着多数长对话的后半程命中率为 0。而且 fingerprint 不变（用户没发新消息）时，这是一个纯浪费的 recompute。

**影响等级：P1。** 默认 TTL 与对话生命周期不匹配。

---

## 二、排序与召回质量问题

### 2.1 Scope/Type 常量权重淹没真实相关性信号

**位置：** `recall/ranking.go:79` `rankRecallMemories`

```go
score := matchScore + memoryScopePriority(item.ScopeType) + memoryTypePriority(item.MemoryType)
```

其中 `memoryScopePriority`: KB=1000, global=500
`memoryTypePriority`: preference=300, knowledge=200

**问题：** 一个全局 knowledge 条目即使完美匹配 query（keywordScore=50），总分 550；而一个 KB preference 即使完全不相关（keywordScore=0），总分 1300（1000+300）。metadata 权重**完全淹没**了语义相关性。

这意味着：只要用户有一条 "response.language=zh-CN" 的 KB 偏好，它将永远排在所有 global knowledge 事实的前面——即使问的是技术问题。

**影响等级：P1。** 检索排序的实际主导因素不是相关性，而是 scope/type 常量。

### 2.2 无 embedding 时 knowledge 条目不进行相关性过滤

**位置：** `recall/ranking.go:76-78`

```go
if item.MemoryType == domain.MemoryTypeKnowledge && query != "" && !matched && vectorScore <= 0 {
    continue
}
```

**问题：** 条件要求 `!matched && vectorScore <= 0`（且 embedding 服务不可用时 vectorScore 一定是 0），所以：
- 如果 embedding 服务挂了 → 所有 knowledge 条目全部通过，不管是否相关
- 如果根本没配 embedding → 同上

"RocketMQ 已移除" 这类 memory 会在用户问 "怎么配置日志级别" 时被注入上下文。

**影响等级：P2。** 依赖 embedding 可用性来保证基本的检索精度。

### 2.3 Rule memory 加载不做 query 相关性过滤

**位置：** `recall/service.go:125-158` `loadRuleMemories`

**问题：** Rule memory 用 `MemoryTypes: []string{domain.MemoryTypePreference}` 加载全部 active 条目，filter 中没有任何 query/keyword 过滤。所有 preference 条目全部加载，然后只按 scope > importance > last_confirmed_at > update_time 排序。

这意味着 20 条 preferences 全量进入 context（或挤占 budget），不管用户问的是什么。如果用户设了 15 条不同场景的偏好，每条对话都会把这 15 条全部带上。

**现状缓解因素：** rule memory 通常在 10 条以内。但随着使用量增长，这是个隐患。

**影响等级：P2。** 当前影响有限，但会随数据增长恶化。

### 2.4 Rule/Fact 5:5 分预算过于机械

**位置：** `recall/context_renderer.go:22-27`

```go
if len(ruleItems) > 0 && len(factItems) > 0 {
    ruleCharBudget = maxChars / 2
```

**问题：** 1 条 rule + 10 条 fact 时，rule 拿走 50% 预算。10 条 rule + 1 条 fact 时，fact 只拿到 50%。预算分配与条目数、重要性、query 相关性都无关。

**影响等级：P2。**

---

## 三、缓存架构的集成问题

### 3.1 Session recall 和 LTM recall 的 embedding cache key 格式相同但 TTL 可能不同

**位置：**
- `session_recall_cache.go:374-376` `buildSharedEmbeddingCacheKey`
- `recall/cache_keys.go:33-35` `buildEmbeddingRequestCacheKey`

两者都产生 `embed:model:normalizedQuery`。共享同一 Redis `QueryEmbeddingCacheKey`。

**问题：** Session recall 写入时用 `EmbeddingTTL`（默认 30min），LTM recall 用独立配置的 `EmbeddingTTL`。如果两者配置不同，后写入的 TTL 覆盖先写入的。但实际它们请求的是同一个 embedding，TTL 应该是统一的。

**影响等级：P2。** 不会导致错误，但配置语义不清晰。

### 3.2 Scope version 不可用时 request cache key 不含 version，导致二次命中失败

**位置：** `recall/cache_support.go:112` 和 `recall/cache_support.go:126`

```go
// 第一次（scope version unavailable）:
requestKey := buildFactRequestCacheKey(..., port.ScopeVersions{})  // key 不含 version
// 写入 request cache with key-without-version

// 第二次（scope version available）:
requestKey = buildFactRequestCacheKey(..., versions)  // key 含 version
// 读 request cache with key-with-version → MISS
```

**问题：** 同一请求内，如果第一次查 scope version 失败但第二次成功，request-scope cache 的 key 格式不同，导致无法命中。虽然版本查不到的概率很低，但这是 logic gap。

**影响等级：P2。**

---

## 四、治理链路的小缺陷

### 4.1 `bumpRecallCacheVersion` 在 embedding 持久化之前调用

**位置：** `longtermmemory/service.go:80-82`

```go
s.persistMemoryEmbedding(ctx, saved)   // fire-and-forget, 可能失败
s.bumpRecallCacheVersion(ctx, saved)   // 在这之前 bump → cache 失效
```

不对，代码顺序是：先 `persistMemoryEmbedding`（fire-and-forget），后 `bumpRecallCacheVersion`。所以顺序是对的：先尝试写 embedding，再 bump version。

但问题是 `persistMemoryEmbedding` **不保证成功**（fire-and-forget），而 version bump **总是执行**。结果是：version bump → 下次 recall 跳过 Redis → recompute → embedding 还没落库 → 该 memory 对向量搜索不可见。不是致命问题（keyword 匹配仍能命中），但会造成缓存失效 + 向量检索质量下降。

**影响等级：P2。**

### 4.2 Canonical key 注册表硬编码——治理系统的核心配置无法动态更新

**位置：** `governance/schema_registry.go`

**问题：** 新增一个 key 需要改代码 + 重新部署。作为治理系统的核心配置，应该是数据库驱动或至少 YAML 配置驱动。

**影响等级：P2。** 当前 7 个 key 够用，但长期是扩展瓶颈。

### 4.3 `MemoryItemsEquivalent` 的 text 类型等值判断有歧义

**位置：** `governance/conflict_detector.go:114-116`

```go
if comparableMemoryValue(left.ValueType, left.ValueJSON) != "" && ...
```

对于 `value_type=text`，`comparableMemoryValue` 返回 `comparableDisplayValue(ValueJSON)`。但如果 ValueJSON 为空（用户可能只填了 DisplayValue），就会 fall through 到 Content 比较。两条内容相似的 memory 可能被误判为等价。

**影响等级：P3。**

---

## 五、缺失的能力

### 5.1 Session recall 不支持跨 conversation 的 session chunk 共享

当前 session chunk 严格绑定 `conversation_id`。如果用户开了一个新 conversation 但引用之前 conversation 中发过的长内容，系统无法召回。

**状态：** 设计如此（证据型记忆是会话级的），但值得在产品文档中明确标注。

### 5.2 无 recall 结果的质量反馈回路

当前没有任何机制判断 "召回的结果好不好"：
- 没有用户反馈信号（thumbs up/down on memory usage）
- 没有自动检测 "注入的 memory context 是否被 LLM 实际引用"
- 没有被引用次数统计

没有这些信号，后续的 ranking 调优只能靠离线评估样本，无法从生产数据中学习。

### 5.3 无 memory recall 的 hard budget 强制截断

`buildMemoryRecallContext` 按字符数截断，但截断发生在 section 渲染**之后**——item 已经全部处理过了。更好的做法是在评分阶段就按 budget 截断 candidates。

---

## 六、总结与优先级

| 优先级 | 问题 | 影响 |
|--------|------|------|
| **P0** | Cache key 嵌入原始 query → 真实命中率趋近于零 | 缓存体系形同虚设 |
| **P1** | Fingerprint 粒度太粗 → 频繁全量失效 | 放大 P0 问题 |
| **P1** | Conversation TTL 10min 过短 | 长对话后半程缓存全 miss |
| **P1** | Scope/Type 常量淹没相关性评分 | 检索排序不反映语义相关度 |
| **P2** | 无 embedding 时不过滤 knowledge | embedding 挂掉时精度归零 |
| **P2** | Rule memory 不做 query 过滤 | 随数据增长恶化 |
| **P2** | Rule/Fact 5:5 预算分割机械 | 预算分配不合理 |
| **P2** | Canonical key 硬编码 | 长期扩展瓶颈 |
| **P2** | Embedding TTL 双写覆盖 | 配置语义模糊 |
| **P3** | 等值判断边界 case | 罕见 |

---

## 七、关于 "缓存是否合理" 的直接回答

**分层结构合理，但 key 设计导致实际收益极低。**

三层结构（request L1 → conversation/Redis L2 → recompute）在工程上是对的。问题不在分层，在于：

1. **Conversation cache 和 Redis fact-ranking cache 用原始 query 做 key**——这是根本性缺陷。在 chat 场景里，用户不会重复输入完全相同的 query。这两个缓存层的实际命中率在生产中会极低。
2. **Fingerprint 失效太激进**——存下来的少量缓存也容易因新消息写入而作废。
3. **Request-scope L1 是唯一真正有效的缓存层**——因为同一请求内确实可能重复调用。但这层缓存的收益也仅体现为"同一 prepareChat 调用内避免重复 embedding"。

**实用的改进路径：**
- 短期：把 conversation TTL 调到 30min+，并把 fingerprint 从 `nano + chunkID + messageID` 改为 chunk count + hash set（只在新 chunk 可能改变排序时失效）
- 中期：用 query embedding 的 LSH hash 替代原始 query 做 cache key
- 长期：引入语义缓存（embedding 相似度阈值匹配）
