package redis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"local/rag-project/internal/app/rag/service/longtermmemory"
)

const defaultKeyPrefix = "goagent"

type RagMemoryCache struct {
	client    *goredis.Client
	keyPrefix string
}

func NewRagMemoryCache(client *goredis.Client) *RagMemoryCache {
	return NewRagMemoryCacheWithPrefix(client, defaultKeyPrefix)
}

func NewRagMemoryCacheWithPrefix(client *goredis.Client, prefix string) *RagMemoryCache {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = defaultKeyPrefix
	}
	return &RagMemoryCache{
		client:    client,
		keyPrefix: prefix,
	}
}

func (c *RagMemoryCache) GetRuleMemories(ctx context.Context, key longtermmemory.RuleMemoryCacheKey) (longtermmemory.RuleMemoryCacheValue, bool, error) {
	return readJSONValue[longtermmemory.RuleMemoryCacheValue](ctx, c.client, c.ruleKey(key))
}

func (c *RagMemoryCache) SetRuleMemories(ctx context.Context, key longtermmemory.RuleMemoryCacheKey, value longtermmemory.RuleMemoryCacheValue, ttl time.Duration) error {
	return writeJSONValue(ctx, c.client, c.ruleKey(key), value, ttl)
}

func (c *RagMemoryCache) GetFactRankings(ctx context.Context, key longtermmemory.FactRankingCacheKey) (longtermmemory.FactRankingCacheValue, bool, error) {
	return readJSONValue[longtermmemory.FactRankingCacheValue](ctx, c.client, c.factKey(key))
}

func (c *RagMemoryCache) SetFactRankings(ctx context.Context, key longtermmemory.FactRankingCacheKey, value longtermmemory.FactRankingCacheValue, ttl time.Duration) error {
	return writeJSONValue(ctx, c.client, c.factKey(key), value, ttl)
}

func (c *RagMemoryCache) GetQueryEmbedding(ctx context.Context, key longtermmemory.QueryEmbeddingCacheKey) ([]float32, bool, error) {
	return readJSONValue[[]float32](ctx, c.client, c.embeddingKey(key))
}

func (c *RagMemoryCache) SetQueryEmbedding(ctx context.Context, key longtermmemory.QueryEmbeddingCacheKey, value []float32, ttl time.Duration) error {
	return writeJSONValue(ctx, c.client, c.embeddingKey(key), value, ttl)
}

func (c *RagMemoryCache) IncrGlobalVersion(ctx context.Context, userID string) error {
	return c.client.Incr(ctx, c.globalVersionKey(userID)).Err()
}

func (c *RagMemoryCache) IncrKBVersion(ctx context.Context, userID string, kbID string) error {
	return c.client.Incr(ctx, c.kbVersionKey(userID, kbID)).Err()
}

func (c *RagMemoryCache) GetScopeVersions(ctx context.Context, userID string, kbIDs []string) (longtermmemory.ScopeVersions, error) {
	result := longtermmemory.ScopeVersions{
		KBVersions: map[string]int64{},
	}
	keys := []string{c.globalVersionKey(userID)}
	orderedKBIDs := normalizeSortedValues(kbIDs)
	for _, kbID := range orderedKBIDs {
		keys = append(keys, c.kbVersionKey(userID, kbID))
	}
	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return result, err
	}
	result.GlobalVersion = parseVersionValue(values[0])
	for idx, kbID := range orderedKBIDs {
		result.KBVersions[kbID] = parseVersionValue(values[idx+1])
	}
	return result, nil
}

func (c *RagMemoryCache) globalVersionKey(userID string) string {
	return c.joinKey("rag", "memory", "ver", "global", strings.TrimSpace(userID))
}

func (c *RagMemoryCache) kbVersionKey(userID string, kbID string) string {
	return c.joinKey("rag", "memory", "ver", "kb", strings.TrimSpace(userID), strings.TrimSpace(kbID))
}

func (c *RagMemoryCache) embeddingKey(key longtermmemory.QueryEmbeddingCacheKey) string {
	model := strings.TrimSpace(key.EmbeddingModel)
	if model == "" {
		model = "default"
	}
	return c.joinKey("rag", "embed", "v1", model, hashText(normalizeQuery(key.Query)))
}

func (c *RagMemoryCache) ruleKey(key longtermmemory.RuleMemoryCacheKey) string {
	return c.joinKey(
		"rag",
		"memory",
		"rules",
		"v1",
		strings.TrimSpace(key.UserID),
		hashText(strings.Join(normalizeSortedValues(key.KnowledgeBaseIDs), ",")),
		strconv.FormatInt(key.ScopeVersions.GlobalVersion, 10),
		hashVersions(key.ScopeVersions.KBVersions),
	)
}

func (c *RagMemoryCache) factKey(key longtermmemory.FactRankingCacheKey) string {
	rankVersion := strings.TrimSpace(key.RankVersion)
	if rankVersion == "" {
		rankVersion = "v1"
	}
	model := strings.TrimSpace(key.EmbeddingModel)
	if model == "" {
		model = "default"
	}
	return c.joinKey(
		"rag",
		"memory",
		"facts",
		"v1",
		strings.TrimSpace(key.UserID),
		hashText(normalizeQuery(key.Query)),
		hashText(strings.Join(normalizeSortedValues(key.KnowledgeBaseIDs), ",")),
		strconv.FormatInt(key.ScopeVersions.GlobalVersion, 10),
		hashVersions(key.ScopeVersions.KBVersions),
		strconv.Itoa(key.CandidateLimit),
		model,
		rankVersion,
	)
}

func (c *RagMemoryCache) joinKey(parts ...string) string {
	filtered := make([]string, 0, len(parts)+1)
	filtered = append(filtered, c.keyPrefix)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, ":")
}

func readJSONValue[T any](ctx context.Context, client *goredis.Client, key string) (T, bool, error) {
	var zero T
	if client == nil || strings.TrimSpace(key) == "" {
		return zero, false, nil
	}
	raw, err := client.Get(ctx, key).Result()
	if err == goredis.Nil {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return zero, false, fmt.Errorf("unmarshal redis cache value: %w", err)
	}
	return value, true, nil
}

func writeJSONValue(ctx context.Context, client *goredis.Client, key string, value any, ttl time.Duration) error {
	if client == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal redis cache value: %w", err)
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	return client.Set(ctx, key, payload, ttl).Err()
}

func normalizeSortedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func hashVersions(values map[string]int64) string {
	if len(values) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.FormatInt(values[key], 10))
	}
	return hashText(strings.Join(parts, ","))
}

func hashText(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func normalizeQuery(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func parseVersionValue(value any) int64 {
	switch current := value.(type) {
	case nil:
		return 0
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(current), 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	case int64:
		return current
	case int:
		return int64(current)
	default:
		parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(current)), 10, 64)
		if err != nil {
			return 0
		}
		return parsed
	}
}
