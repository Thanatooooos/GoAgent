package tokenbudget

import (
	"strings"
	"testing"
)

func TestTruncateTextRespectsBudget(t *testing.T) {
	got, truncated := TruncateText("abcdefghij", 6, RuneEstimator{})
	if !truncated {
		t.Fatal("TruncateText() truncated = false, want true")
	}
	if tokens := (RuneEstimator{}).EstimateTokens(got); tokens > 6 {
		t.Fatalf("TruncateText() tokens = %d, want <= 6; text=%q", tokens, got)
	}
}

func TestTruncateTextPreservesTextWithinBudget(t *testing.T) {
	got, truncated := TruncateText("abc", 3, RuneEstimator{})
	if truncated {
		t.Fatal("TruncateText() truncated = true, want false")
	}
	if got != "abc" {
		t.Fatalf("TruncateText() = %q, want %q", got, "abc")
	}
}

func TestJoinSectionsWithinBudgetRetainsRequiredSections(t *testing.T) {
	sections := []Section{
		{Name: "conclusion", Text: "root cause confirmed", Priority: 100, Required: true},
		{Name: "source", Text: "https://example.com", Priority: 90, Required: true},
		{Name: "detail", Text: strings.Repeat("verbose ", 100), Priority: 10},
	}

	text, stats := JoinSectionsWithinBudget(sections, 45, RuneEstimator{}, 12000)
	if !strings.Contains(text, "root cause confirmed") || !strings.Contains(text, "https://example.com") {
		t.Fatalf("required sections missing: %q", text)
	}
	if stats.DroppedSections == 0 {
		t.Fatalf("expected lower-priority detail to be dropped: %+v", stats)
	}
}
