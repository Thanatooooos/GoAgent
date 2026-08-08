package service

import (
	"fmt"
	"sort"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type SourceRegistry struct {
	providers map[string]port.SourceProvider
}

func NewSourceRegistry(providers ...port.SourceProvider) *SourceRegistry {
	registry := &SourceRegistry{
		providers: make(map[string]port.SourceProvider, len(providers)),
	}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		registry.providers[provider.SourceKey()] = provider
	}
	return registry
}

func NewDefaultSourceRegistry() (*SourceRegistry, error) {
	specs := domain.DefaultSourceFeedSpecs()
	providers := make([]port.SourceProvider, 0, len(specs))
	for _, spec := range specs {
		provider, err := NewFeedSource(spec)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return NewSourceRegistry(providers...), nil
}

func (r *SourceRegistry) Get(sourceKey string) (port.SourceProvider, bool) {
	provider, ok := r.providers[sourceKey]
	return provider, ok
}

func (r *SourceRegistry) MustGet(sourceKey string) (port.SourceProvider, error) {
	provider, ok := r.Get(sourceKey)
	if !ok {
		return nil, fmt.Errorf("source provider not registered for key %q", sourceKey)
	}
	return provider, nil
}

func (r *SourceRegistry) Keys() []string {
	keys := make([]string, 0, len(r.providers))
	for key := range r.providers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
