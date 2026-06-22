package longtermmemory

import (
	"reflect"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestPhase1PreferenceCanonicalKeysMatchAllowlist(t *testing.T) {
	keys := Phase1PreferenceCanonicalKeys()
	want := []string{
		"behavior.avoid",
		"response.language",
		"workflow.troubleshooting.first_step",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("Phase1PreferenceCanonicalKeys() = %+v, want %+v", keys, want)
	}
}

func TestNormalizePendingPreferenceCandidateDefaultsPhase1Contract(t *testing.T) {
	candidate, err := NormalizePendingPreferenceCandidate(PreferenceCandidate{
		CanonicalKey:    " Workflow.Troubleshooting.First_Step ",
		Summary:         "遇到问题先看日志",
		Content:         "先看错误日志",
		SourceMessageID: "msg-1",
		ExtractionMethod: domain.MemoryExtractionMethodLLM,
		Confidence:      0.91,
	})
	if err != nil {
		t.Fatalf("NormalizePendingPreferenceCandidate returned error: %v", err)
	}
	if candidate.ScopeType != domain.MemoryScopeGlobal {
		t.Fatalf("expected global scope, got %+v", candidate)
	}
	if candidate.MemoryType != domain.MemoryTypePreference {
		t.Fatalf("expected preference type, got %+v", candidate)
	}
	if candidate.Status != domain.MemoryStatusPending {
		t.Fatalf("expected pending status, got %+v", candidate)
	}
	if candidate.CanonicalKey != "workflow.troubleshooting.first_step" {
		t.Fatalf("expected normalized canonical key, got %+v", candidate)
	}
}

func TestNormalizePendingPreferenceCandidateRejectsLegacyWorkflowFirstStep(t *testing.T) {
	_, err := NormalizePendingPreferenceCandidate(PreferenceCandidate{
		ScopeType:     domain.MemoryScopeGlobal,
		MemoryType:    domain.MemoryTypePreference,
		CanonicalKey:  "workflow.first_step",
		Summary:       "遇到问题先做排查",
		Content:       "先分析一下",
		Status:        domain.MemoryStatusPending,
		Confidence:    0.9,
	})
	if err == nil {
		t.Fatal("expected legacy workflow.first_step to be rejected")
	}
}

func TestPreferenceCandidateStructOmitsRequiresConfirmationField(t *testing.T) {
	typ := reflect.TypeOf(PreferenceCandidate{})
	if _, ok := typ.FieldByName("RequiresConfirmation"); ok {
		t.Fatal("expected PreferenceCandidate to omit RequiresConfirmation field")
	}
}
