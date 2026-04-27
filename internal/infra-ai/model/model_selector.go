package model

import (
	"fmt"
	"sort"
	"strings"

	"local/rag-project/internal/framework/config"
	fwlog "local/rag-project/internal/framework/log"
	aienum "local/rag-project/internal/infra-ai/enum"
)

type ModelTarget struct {
	Id        string
	Candidate config.ModelCandidate
	Provider  config.ProviderConfig
}

func (m *ModelTarget) GetId() string {
	return m.Id
}

type ModelSelector struct {
	healthStore *ModelHealthStore
}

func newModelSelector(healthStore *ModelHealthStore) *ModelSelector {
	if healthStore == nil {
		healthStore = NewModelHealthStore()
	}
	return &ModelSelector{healthStore: healthStore}
}

func NewModelSelector(healthStore *ModelHealthStore) *ModelSelector {
	return newModelSelector(healthStore)
}

func (ms *ModelSelector) resolveFirstChoiceModel(group config.ModelGroup, deepThinking bool) string {
	if deepThinking && group.DeepThinkingModel != "" {
		return group.DeepThinkingModel
	}
	return group.DefaultModel
}

func (ms *ModelSelector) SelectEmbeddingCandidates() []ModelTarget {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}
	return ms.selectCandidates(cfg.AI.Embedding, cfg.AI.Embedding.DefaultModel, false)
}

func (ms *ModelSelector) SelectRerankCandidates() []ModelTarget {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}
	return ms.selectCandidates(cfg.AI.Rerank, cfg.AI.Rerank.DefaultModel, false)
}

func (ms *ModelSelector) SelectChatCandidates(deepThinking bool) []ModelTarget {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}
	group := cfg.AI.Chat
	firstChoiceModelID := ms.resolveFirstChoiceModel(group, deepThinking)
	return ms.selectCandidates(group, firstChoiceModelID, deepThinking)
}

func (ms *ModelSelector) selectCandidates(group config.ModelGroup, firstChoiceModelID string, deepThinking bool) []ModelTarget {
	if len(group.Candidates) == 0 {
		return nil
	}
	orderedCandidates := ms.filterAndSortCandidates(group.Candidates, firstChoiceModelID, deepThinking)
	return ms.buildAvailableTargets(orderedCandidates)
}

func (ms *ModelSelector) filterAndSortCandidates(candidates []config.ModelCandidate, firstChoiceModelID string, deepThinking bool) []config.ModelCandidate {
	if len(candidates) == 0 {
		return nil
	}

	enabled := make([]config.ModelCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Enabled != nil && !*candidate.Enabled {
			continue
		}
		if deepThinking && (candidate.SupportsThinking == nil || !*candidate.SupportsThinking) {
			continue
		}
		enabled = append(enabled, candidate)
	}

	sort.SliceStable(enabled, func(i, j int) bool {
		a := enabled[i]
		b := enabled[j]

		aID := resolveId(a)
		bID := resolveId(b)

		aIsFirst := aID == firstChoiceModelID
		bIsFirst := bID == firstChoiceModelID
		if aIsFirst != bIsFirst {
			return aIsFirst
		}

		aPriority := normalizedPriority(a.Priority)
		bPriority := normalizedPriority(b.Priority)
		if aPriority != bPriority {
			return aPriority < bPriority
		}

		return strings.Compare(aID, bID) < 0
	})

	return enabled
}

func normalizedPriority(priority int) int {
	if priority == 0 {
		return 100
	}
	return priority
}

func resolveId(candidate config.ModelCandidate) string {
	if strings.TrimSpace(candidate.Id) != "" {
		return candidate.Id
	}
	provider := strings.TrimSpace(candidate.Provider)
	if provider == "" {
		provider = "unknown"
	}
	model := strings.TrimSpace(candidate.Model)
	if model == "" {
		model = "unknown"
	}
	return fmt.Sprintf("%s::%s", provider, model)
}

func (ms *ModelSelector) buildAvailableTargets(candidates []config.ModelCandidate) []ModelTarget {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}

	targets := make([]ModelTarget, 0, len(candidates))
	for _, candidate := range candidates {
		target := ms.buildModelTarget(candidate, cfg.AI.Providers)
		if target != nil {
			targets = append(targets, *target)
		}
	}
	return targets
}

func (ms *ModelSelector) buildModelTarget(candidate config.ModelCandidate, providers map[string]config.ProviderConfig) *ModelTarget {
	modelID := resolveId(candidate)
	if ms.healthStore != nil && ms.healthStore.isUnavailable(modelID) {
		return nil
	}

	provider, ok := providers[candidate.Provider]
	if !ok && !aienum.ModelProviderNoop.Matches(candidate.Provider) {
		fwlog.Warnf("provider config missing: provider=%s, modelId=%s", candidate.Provider, modelID)
		return nil
	}

	return &ModelTarget{
		Id:        modelID,
		Candidate: candidate,
		Provider:  provider,
	}
}
