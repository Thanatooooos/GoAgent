package enum

import "strings"

// ModelProvider indicates model provider identity.
type ModelProvider string

const (
	ModelProviderOllama      ModelProvider = "ollama"
	ModelProviderBaiLian     ModelProvider = "bailian"
	ModelProviderSiliconFlow ModelProvider = "siliconflow"
	ModelProviderNoop        ModelProvider = "noop"
)

func (p ModelProvider) ID() string {
	return string(p)
}

func (p ModelProvider) Matches(provider string) bool {
	return strings.EqualFold(strings.TrimSpace(provider), string(p))
}

func ParseModelProvider(provider string) (ModelProvider, bool) {
	provider = strings.TrimSpace(provider)
	for _, item := range AllModelProviders() {
		if item.Matches(provider) {
			return item, true
		}
	}
	return "", false
}

func AllModelProviders() []ModelProvider {
	return []ModelProvider{
		ModelProviderOllama,
		ModelProviderBaiLian,
		ModelProviderSiliconFlow,
		ModelProviderNoop,
	}
}
