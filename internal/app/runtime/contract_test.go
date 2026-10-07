package runtime

import "testing"

func TestRunRequestEffectivePolicyDisablesUnscopedKnowledgeRetrieval(t *testing.T) {
	t.Parallel()

	request := RunRequest{Policy: Policy{AllowKnowledgeRetrieval: true}}
	if request.EffectivePolicy().AllowKnowledgeRetrieval {
		t.Fatal("empty knowledge-base scope must prohibit retrieval")
	}

	request.KnowledgeBaseIDs = []string{" kb-1 ", "kb-1"}
	if !request.EffectivePolicy().AllowKnowledgeRetrieval {
		t.Fatal("non-empty knowledge-base scope must preserve caller policy")
	}
}

func TestRunRequestValidateRequiresCorrelationIDs(t *testing.T) {
	t.Parallel()

	request := RunRequest{
		ConversationID: "conversation-1",
		UserID:         "user-1",
		UserMessageID:  "message-1",
		Question:       "what changed?",
		TraceID:        "trace-1",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	request.UserMessageID = ""
	if err := request.Validate(); err == nil {
		t.Fatal("missing user message id must be rejected")
	}
}

func TestCanTransitionToolState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		from string
		to   string
		want bool
	}{
		{ToolStatePending, ToolStateExecuting, true},
		{ToolStatePending, ToolStateDenied, true},
		{ToolStatePending, ToolStateFailed, true},
		{ToolStateExecuting, ToolStateCompleted, true},
		{ToolStateExecuting, ToolStateFailed, true},
		{ToolStateCompleted, ToolStateExecuting, false},
		{ToolStateDenied, ToolStateCompleted, false},
		{ToolStateFailed, ToolStateCompleted, false},
	}
	for _, tc := range cases {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			if got := CanTransitionToolState(tc.from, tc.to); got != tc.want {
				t.Fatalf("CanTransitionToolState(%q, %q) = %t, want %t", tc.from, tc.to, got, tc.want)
			}
		})
	}
}
