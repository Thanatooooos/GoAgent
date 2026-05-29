package provider

import (
	"fmt"
	"strings"
)

type FallbackSearchProvider struct {
	Name      string
	Primary   SearchProvider
	Secondary SearchProvider
}

func NewFallbackSearchProvider(name string, primary SearchProvider, secondary SearchProvider) *FallbackSearchProvider {
	return &FallbackSearchProvider{
		Name:      strings.TrimSpace(name),
		Primary:   primary,
		Secondary: secondary,
	}
}

func (p *FallbackSearchProvider) ProviderName() string {
	if strings.TrimSpace(p.Name) != "" {
		return strings.TrimSpace(p.Name)
	}
	if p.Primary != nil {
		return ProviderName(p.Primary)
	}
	if p.Secondary != nil {
		return ProviderName(p.Secondary)
	}
	return "unknown"
}

func (p *FallbackSearchProvider) Search(query string) ([]SearchResult, error) {
	if p.Primary == nil && p.Secondary == nil {
		return nil, fmt.Errorf("no search providers configured")
	}
	if p.Primary != nil {
		results, err := p.Primary.Search(query)
		if err == nil {
			return results, nil
		}
		if p.Secondary == nil {
			return nil, err
		}
		fallbackResults, fallbackErr := p.Secondary.Search(query)
		if fallbackErr != nil {
			return nil, fmt.Errorf("primary provider failed: %v; fallback provider failed: %w", err, fallbackErr)
		}
		return annotateFallbackResults(fallbackResults, ProviderName(p.Secondary)), nil
	}

	results, err := p.Secondary.Search(query)
	if err != nil {
		return nil, err
	}
	return annotateFallbackResults(results, ProviderName(p.Secondary)), nil
}

func annotateFallbackResults(results []SearchResult, actualProvider string) []SearchResult {
	if len(results) == 0 {
		return results
	}
	tagged := make([]SearchResult, 0, len(results))
	actualProvider = strings.TrimSpace(actualProvider)
	for _, result := range results {
		result.FallbackUsed = true
		result.ActualProvider = actualProvider
		if strings.TrimSpace(result.Provider) == "" {
			result.Provider = actualProvider
		}
		tagged = append(tagged, result)
	}
	return tagged
}
