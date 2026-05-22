package runtime

import (
	"testing"

	. "local/rag-project/internal/app/rag/tool/core"
)

func TestNormalizeObservationStateClearsHintsWhenDone(t *testing.T) {
	state := normalizeObservationState(ObserveResult{
		Done: true,
		State: AgentState{
			Phase: "complete",
			NextHintCalls: []HintCall{{
				Name:      "document_query",
				Arguments: map[string]any{"documentId": "doc-1"},
			}},
		},
	}, AgentState{})
	if len(state.NextHintCalls) != 0 {
		t.Fatalf("expected next hint calls to be cleared, got %+v", state.NextHintCalls)
	}
	if state.NextHint != "" {
		t.Fatalf("expected next hint string to be cleared, got %q", state.NextHint)
	}
}

func TestNormalizeObservationStateFallsBackToPreviousState(t *testing.T) {
	state := normalizeObservationState(ObserveResult{}, AgentState{
		Phase:      "triage",
		Hypothesis: "previous hypothesis",
	})
	if state.Phase != "triage" {
		t.Fatalf("expected previous phase to be preserved, got %q", state.Phase)
	}
	if state.Hypothesis != "previous hypothesis" {
		t.Fatalf("expected previous hypothesis to be preserved, got %q", state.Hypothesis)
	}
}
