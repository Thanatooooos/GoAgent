package tokenbudget

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestEstimateMessagesAddsEnvelopeOverhead(t *testing.T) {
	got := EstimateMessages([]convention.ChatMessage{
		convention.UserMessage("a"),
		convention.AssistantMessage("b"),
	}, FixedEstimator(10), 4)
	if got != 28 {
		t.Fatalf("EstimateMessages() = %d, want 28", got)
	}
}

func TestApplySafetyFactorRoundsUp(t *testing.T) {
	if got := ApplySafetyFactor(101, 1.15); got != 117 {
		t.Fatalf("ApplySafetyFactor() = %d, want 117", got)
	}
}

func TestDefaultEstimatorReturnsPositiveMixedLanguageEstimate(t *testing.T) {
	got := NewDefaultEstimator().EstimateTokens(`中文 hello JSON: {"ok":true}`)
	if got <= 0 {
		t.Fatalf("EstimateTokens() = %d, want > 0", got)
	}
}
