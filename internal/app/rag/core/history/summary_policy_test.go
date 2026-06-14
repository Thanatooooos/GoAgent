package history

import "testing"

func TestSelectSummaryBudgetTierPromotesDenseTechnicalContent(t *testing.T) {
	tier := SelectSummaryBudget(SummaryBudgetInput{
		MessageCount: 4,
		TotalChars:   260,
		Messages: []string{
			"vector store unavailable",
			"document id doc_fail_01",
			"summary-max-chars=200",
		},
	}, SummaryBudgetOptions{
		SmallMaxChars:         400,
		MediumMaxChars:        600,
		LargeMaxChars:         800,
		MediumMessageCountMin: 6,
		LargeMessageCountMin:  10,
	})

	if tier.MaxChars != 600 {
		t.Fatalf("expected dense technical content to promote to medium tier, got %+v", tier)
	}
	if tier.Name != "medium" {
		t.Fatalf("expected medium tier name, got %+v", tier)
	}
}
