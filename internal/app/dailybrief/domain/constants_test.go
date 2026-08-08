package domain_test

import (
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestSourceCatalogContainsCuratedKeys(t *testing.T) {
	got := domain.SourceKeys()
	if len(got) != 31 {
		t.Fatalf("expected 31 source keys, got %d: %v", len(got), got)
	}
	for _, key := range got {
		if !domain.IsSourceKeySupported(key) {
			t.Fatalf("expected source key %q to be supported", key)
		}
	}
	if domain.IsSourceKeySupported("unknown-source") {
		t.Fatal("expected unknown source key to be rejected")
	}
}

func TestTopicCatalogContainsCuratedLeafKeys(t *testing.T) {
	want := []string{
		"art.architecture",
		"art.contemporary",
		"art.design",
		"art.digital",
		"art.film",
		"art.photography",
		"music.classical",
		"music.electronic",
		"music.industry",
		"music.live",
		"music.rock-pop",
		"music.tech",
		"politics.china",
		"politics.economy-policy",
		"politics.elections",
		"politics.energy",
		"politics.global",
		"politics.tech-policy",
		"tech.ai.models",
		"tech.ai.research",
		"tech.ai.tools",
		"tech.dev",
		"tech.startups",
	}

	assertStringSlicesEqual(t, domain.LeafTopicKeys(), want, "leaf topic catalog")

	for _, key := range want {
		if !domain.IsTopicKeySelectable(key) {
			t.Fatalf("expected topic key %q to be selectable", key)
		}
		if !domain.TopicHasSources(key) {
			t.Fatalf("expected topic key %q to have bound sources", key)
		}
	}

	if domain.IsTopicKeySelectable("tech") {
		t.Fatal("expected non-leaf topic key to be rejected")
	}
	if domain.IsTopicKeySelectable("unknown-topic") {
		t.Fatal("expected unknown topic key to be rejected")
	}
}

func TestEverySourceBindsToSelectableLeafTopic(t *testing.T) {
	for _, spec := range domain.DefaultSourceFeedSpecs() {
		if !domain.IsSourceKeySupported(spec.Key) {
			t.Fatalf("source %q is not in catalog", spec.Key)
		}
		if !domain.IsTopicKeySelectable(spec.Topic) {
			t.Fatalf("source %q binds to non-selectable topic %q", spec.Key, spec.Topic)
		}
	}
}

func TestStatusValidatorsAcceptOnlyKnownValues(t *testing.T) {
	issueStatuses := []string{
		domain.IssueStatusGenerating,
		domain.IssueStatusReady,
		domain.IssueStatusFailed,
	}
	runStatuses := []string{
		domain.GenerationRunStatusRunning,
		domain.GenerationRunStatusSucceeded,
		domain.GenerationRunStatusDegraded,
		domain.GenerationRunStatusFailed,
	}

	assertUniqueNonEmpty(t, issueStatuses, "issue statuses")
	assertUniqueNonEmpty(t, runStatuses, "generation run statuses")

	for _, status := range issueStatuses {
		if !domain.IsValidIssueStatus(status) {
			t.Fatalf("expected issue status %q to be valid", status)
		}
	}
	for _, status := range runStatuses {
		if !domain.IsValidGenerationRunStatus(status) {
			t.Fatalf("expected generation run status %q to be valid", status)
		}
	}

	if domain.IsValidIssueStatus("queued") {
		t.Fatal("expected unknown issue status to be rejected")
	}
	if domain.IsValidGenerationRunStatus("queued") {
		t.Fatal("expected unknown generation run status to be rejected")
	}
}

func assertUniqueNonEmpty(t *testing.T, values []string, name string) {
	t.Helper()

	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			t.Fatalf("expected %s to be non-empty", name)
		}
		if _, exists := seen[value]; exists {
			t.Fatalf("expected %s to contain unique values, duplicate %q", name, value)
		}
		seen[value] = struct{}{}
	}
}

func assertStringSlicesEqual(t *testing.T, got []string, want []string, name string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("unexpected %s length: got %d want %d (%#v vs %#v)", name, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected %s at index %d: got %q want %q (%#v vs %#v)", name, i, got[i], want[i], got, want)
		}
	}
}
