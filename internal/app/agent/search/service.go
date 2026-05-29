package search

import (
	"context"
	"fmt"
	"strings"

	searchprovider "local/rag-project/internal/app/agent/search/provider"
)

type Service struct {
	provider     searchprovider.SearchProvider
	sourcePolicy *searchprovider.SourcePolicyEngine
}

func NewService(provider searchprovider.SearchProvider, sourcePolicy *searchprovider.SourcePolicyEngine) *Service {
	if sourcePolicy == nil {
		sourcePolicy = searchprovider.NewSourcePolicyEngine(searchprovider.SourcePolicyConfig{})
	}
	return &Service{
		provider:     provider,
		sourcePolicy: sourcePolicy,
	}
}

func (s *Service) Search(_ context.Context, query string) (SearchOutput, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchOutput{
			Query:         query,
			Degraded:      true,
			DegradeReason: "query is required",
			ErrorMessage:  "query is required",
			Summary:       "web search failed: query is required",
			Results:       []SearchResultItem{},
		}, fmt.Errorf("query is required")
	}
	if s == nil || s.provider == nil {
		return SearchOutput{
			Query:         query,
			Degraded:      true,
			DegradeReason: "no search provider configured",
			ErrorMessage:  "no search provider configured",
			Summary:       "web search failed: no search provider configured",
			Results:       []SearchResultItem{},
		}, fmt.Errorf("no search provider configured")
	}

	results, err := s.provider.Search(query)
	if err != nil {
		output := SearchOutput{
			Query:                query,
			Provider:             searchprovider.ProviderName(s.provider),
			Results:              []SearchResultItem{},
			Degraded:             true,
			DegradeReason:        err.Error(),
			ErrorMessage:         err.Error(),
			Summary:              fmt.Sprintf("web search failed: %v", err),
			ProviderFallbackUsed: false,
		}
		return output, err
	}

	providerName := searchprovider.ProviderName(s.provider)
	items := make([]SearchResultItem, 0, len(results))
	fetchableURLs := make([]string, 0, len(results))
	actualProvider := ""
	fallbackUsed := false
	allowedCount := 0
	neutralCount := 0
	deniedCount := 0
	for _, raw := range results {
		result := s.enrichResult(raw, providerName)
		if strings.TrimSpace(result.ActualProvider) != "" && actualProvider == "" {
			actualProvider = strings.TrimSpace(result.ActualProvider)
		}
		fallbackUsed = fallbackUsed || result.FallbackUsed
		switch result.Policy {
		case searchprovider.SourcePolicyDeny:
			deniedCount++
		case searchprovider.SourcePolicyAllow:
			allowedCount++
		default:
			neutralCount++
		}
		if result.Policy != searchprovider.SourcePolicyDeny && strings.TrimSpace(result.URL) != "" {
			fetchableURLs = append(fetchableURLs, strings.TrimSpace(result.URL))
		}
		items = append(items, SearchResultItem{
			Title:         result.Title,
			URL:           result.URL,
			Snippet:       result.Snippet,
			Domain:        result.Domain,
			SourceType:    result.SourceType,
			Policy:        result.Policy,
			RiskFlags:     append([]string(nil), result.RiskFlags...),
			Reasons:       append([]string(nil), result.Reasons...),
			ProviderScore: cloneFloat64(result.ProviderScore),
		})
	}

	summary := fmt.Sprintf("found %d web results for %q (allow=%d, neutral=%d, deny=%d)", len(results), query, allowedCount, neutralCount, deniedCount)
	if len(results) == 0 {
		summary = fmt.Sprintf("no results found for query %q", query)
	}

	return SearchOutput{
		Query:                query,
		Provider:             providerName,
		ProviderActual:       actualProvider,
		ProviderFallbackUsed: fallbackUsed,
		ResultCount:          len(results),
		AllowedCount:         allowedCount,
		NeutralCount:         neutralCount,
		DeniedCount:          deniedCount,
		URLs:                 fetchableURLs,
		Results:              items,
		Summary:              summary,
	}, nil
}

func (s *Service) enrichResult(result searchprovider.SearchResult, providerName string) searchprovider.SearchResult {
	result.Title = strings.TrimSpace(result.Title)
	result.URL = strings.TrimSpace(result.URL)
	result.Snippet = strings.TrimSpace(result.Snippet)
	if strings.TrimSpace(result.Provider) == "" {
		result.Provider = providerName
	}

	assessment := s.sourcePolicy.Evaluate(result.URL)
	if strings.TrimSpace(result.Domain) == "" {
		result.Domain = assessment.Domain
	}
	if strings.TrimSpace(result.SourceType) == "" {
		result.SourceType = assessment.SourceType
	}
	if strings.TrimSpace(result.Policy) == "" {
		result.Policy = assessment.Policy
	}
	result.RiskFlags = uniqueTrimmedValues(append(result.RiskFlags, assessment.RiskFlags...))
	result.Reasons = uniqueTrimmedValues(append(result.Reasons, assessment.Reasons...))
	return result
}

func uniqueTrimmedValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
