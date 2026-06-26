package runtime

import (
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestNormalizeErrorClass_MapsCapabilityClassesToRuntimeClasses(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: agentcapability.ErrorClassValidation, want: ErrorClassValidation},
		{input: agentcapability.ErrorClassPermission, want: ErrorClassPermission},
		{input: agentcapability.ErrorClassDependency, want: ErrorClassDependency},
		{input: agentcapability.ErrorClassExternal, want: ErrorClassExternal},
		{input: "timeout", want: ErrorClassTimeout},
		{input: "mystery", want: ErrorClassUnknown},
		{input: "", want: ErrorClassUnknown},
	}

	for _, tt := range tests {
		if got := NormalizeErrorClass(tt.input); got != tt.want {
			t.Fatalf("NormalizeErrorClass(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestErrorClassForReason_MapsBudgetAndNoProgressReasons(t *testing.T) {
	tests := []struct {
		reason string
		want   string
	}{
		{reason: "approval_rejected", want: ErrorClassApprovalRejected},
		{reason: "iteration_budget_exhausted", want: ErrorClassBudget},
		{reason: "no_progress_across_rounds", want: ErrorClassNoProgress},
		{reason: "other", want: ErrorClassUnknown},
	}

	for _, tt := range tests {
		if got := ErrorClassForReason(tt.reason); got != tt.want {
			t.Fatalf("ErrorClassForReason(%q) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestErrorClassForSession_PrefersSharedRuntimeState(t *testing.T) {
	session := &RuntimeSession{
		Snapshot: agentstate.StateSnapshot{
			Context: agentstate.ContextState{
				FetchErrorClass: agentcapability.ErrorClassPermission,
			},
			Answer: agentstate.AnswerState{
				DegradeReason: "iteration_budget_exhausted",
			},
		},
	}

	if got := ErrorClassForSession(session); got != ErrorClassPermission {
		t.Fatalf("ErrorClassForSession() = %q, want %q", got, ErrorClassPermission)
	}
}
