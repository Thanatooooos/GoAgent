package recall

import (
	"fmt"
	"sort"
	"strings"

	"local/rag-project/internal/app/rag/port"
)

func buildRuleRequestCacheKey(userID string, knowledgeBaseIDs []string, versions port.ScopeVersions) string {
	ids := trimMemoryValues(knowledgeBaseIDs)
	sort.Strings(ids)
	return fmt.Sprintf("ltm:rules:%s:%s:%d:%s", strings.TrimSpace(userID), strings.Join(ids, ","), versions.GlobalVersion, hashScopeVersions(versions.KBVersions))
}

func buildFactRequestCacheKey(userID string, query string, knowledgeBaseIDs []string, candidateLimit int, embeddingModel string, rankVersion string, versions port.ScopeVersions) string {
	ids := trimMemoryValues(knowledgeBaseIDs)
	sort.Strings(ids)
	return fmt.Sprintf(
		"ltm:facts:%s:%s:%s:%d:%s:%s:%d:%s",
		strings.TrimSpace(userID),
		normalizeQueryCacheText(query),
		strings.Join(ids, ","),
		candidateLimit,
		strings.TrimSpace(embeddingModel),
		strings.TrimSpace(rankVersion),
		versions.GlobalVersion,
		hashScopeVersions(versions.KBVersions),
	)
}

func buildEmbeddingRequestCacheKey(query string, modelID string) string {
	return fmt.Sprintf("embed:%s:%s", strings.TrimSpace(modelID), normalizeQueryCacheText(query))
}

func normalizeQueryCacheText(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func hashScopeVersions(values map[string]int64) string {
	if len(values) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, strings.TrimSpace(key))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		parts = append(parts, fmt.Sprintf("%s=%d", strings.TrimSpace(key), value))
	}
	return strings.Join(parts, ",")
}
