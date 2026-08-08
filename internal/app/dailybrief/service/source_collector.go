package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type SourceCollectResult struct {
	Candidates []domain.Candidate
	Failures   map[string]error
}

type SourceCollector struct {
	registry *SourceRegistry
	client   port.HTTPClient
}

func NewSourceCollector(registry *SourceRegistry, client port.HTTPClient) *SourceCollector {
	return &SourceCollector{
		registry: registry,
		client:   client,
	}
}

func (c *SourceCollector) Collect(ctx context.Context, sourceKeys []string) SourceCollectResult {
	result := SourceCollectResult{
		Candidates: []domain.Candidate{},
		Failures:   make(map[string]error),
	}
	if c == nil || c.registry == nil {
		result.Failures["registry"] = fmt.Errorf("source registry is not configured")
		return result
	}
	if c.client == nil {
		result.Failures["http-client"] = fmt.Errorf("http client is not configured")
		return result
	}

	normalizedKeys := normalizeSourceKeys(sourceKeys)
	for _, sourceKey := range normalizedKeys {
		provider, err := c.registry.MustGet(sourceKey)
		if err != nil {
			result.Failures[sourceKey] = err
			continue
		}
		candidates, fetchErr := provider.Fetch(ctx, c.client)
		if fetchErr != nil {
			result.Failures[sourceKey] = fetchErr
			continue
		}
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate.Source) == "" {
				candidate.Source = sourceKey
			}
			result.Candidates = append(result.Candidates, candidate)
		}
	}
	return result
}

func normalizeSourceKeys(sourceKeys []string) []string {
	seen := make(map[string]struct{}, len(sourceKeys))
	keys := make([]string, 0, len(sourceKeys))
	for _, sourceKey := range sourceKeys {
		trimmed := strings.TrimSpace(sourceKey)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		keys = append(keys, trimmed)
	}
	sort.Strings(keys)
	return keys
}
