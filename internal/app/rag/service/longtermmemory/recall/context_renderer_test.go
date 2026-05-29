package recall

import (
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
)

func TestBuildMemoryRecallContextSplitsRuleAndFactSections(t *testing.T) {
	t.Parallel()

	rule := memoryRecallProjection{
		item:    domain.MemoryItem{ID: "rule-1", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypePreference},
		summary: "Always answer in Chinese",
		detail:  "User prefers zh-CN",
	}
	fact := memoryRecallProjection{
		item:    domain.MemoryItem{ID: "fact-1", ScopeType: domain.MemoryScopeKB, ScopeID: "kb-1", MemoryType: domain.MemoryTypeKnowledge},
		summary: "Main bus removed",
		detail:  "RocketMQ has been removed from the project",
	}

	selectedRules, selectedFacts, contextText, truncated := buildMemoryRecallContext([]memoryRecallProjection{rule}, []memoryRecallProjection{fact}, 4, 400)

	if truncated {
		t.Fatal("expected non-truncated context")
	}
	if len(selectedRules) != 1 || len(selectedFacts) != 1 {
		t.Fatalf("selectedRules=%d selectedFacts=%d", len(selectedRules), len(selectedFacts))
	}
	if !strings.Contains(contextText, "Rule Memories:") || !strings.Contains(contextText, "Fact Memories:") {
		t.Fatalf("expected both sections in context: %q", contextText)
	}
	if !strings.Contains(contextText, "Global Memories:") || !strings.Contains(contextText, "KB-Scoped Memories:") {
		t.Fatalf("expected scope sections in context: %q", contextText)
	}
}

func TestBuildMemorySectionContextStopsWhenBudgetExceeded(t *testing.T) {
	t.Parallel()

	items := []memoryRecallProjection{
		{item: domain.MemoryItem{ID: "1", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypePreference}, summary: "short"},
		{item: domain.MemoryItem{ID: "2", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypePreference}, summary: strings.Repeat("very long ", 20)},
	}

	selected, section, truncated := buildMemorySectionContext("Rule Memories:", items, 5, 120)
	if !truncated {
		t.Fatal("expected section truncation when char budget is exceeded")
	}
	if len(selected) != 1 {
		t.Fatalf("expected only first item selected, got %d", len(selected))
	}
	if !strings.Contains(section, "short") {
		t.Fatalf("expected first item summary in section: %q", section)
	}
}

func TestProjectedCountsReflectScopesSourcesAndContributionKinds(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	items := []memoryRecallProjection{
		{
			item:           domain.MemoryItem{ID: "kb", ScopeType: domain.MemoryScopeKB, MemoryType: domain.MemoryTypeKnowledge, UpdateTime: now},
			keywordMatched: true,
			vectorMatched:  true,
		},
		{
			item:           domain.MemoryItem{ID: "global", ScopeType: domain.MemoryScopeGlobal, MemoryType: domain.MemoryTypePreference, UpdateTime: now},
			keywordMatched: true,
		},
	}

	scopeCounts := projectedScopeCounts(items)
	if scopeCounts[domain.MemoryScopeKB] != 1 || scopeCounts[domain.MemoryScopeGlobal] != 1 {
		t.Fatalf("unexpected scopeCounts: %+v", scopeCounts)
	}

	sourceCounts := projectedSourceCounts(items)
	if sourceCounts[memoryHitSourceKeyword] != 2 || sourceCounts[memoryHitSourceVector] != 1 {
		t.Fatalf("unexpected sourceCounts: %+v", sourceCounts)
	}

	contributionCounts := projectedContributionCounts(items)
	if contributionCounts[memoryContributionHybrid] != 1 || contributionCounts[memoryContributionKeywordOnly] != 1 {
		t.Fatalf("unexpected contributionCounts: %+v", contributionCounts)
	}
}
