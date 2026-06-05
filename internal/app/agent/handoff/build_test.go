package handoff

import (
	"strings"
	"testing"
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestBuildReturnsPromptReadyHandoff(t *testing.T) {
	now := time.Now()
	session := &agentruntime.RuntimeSession{
		SessionID: "sess-handoff-build",
		Request: agentruntime.RequestEnvelope{
			Question: "Go generics tutorial",
			Options: agentstate.RuntimeOptions{
				MaxIterations:  2,
				AllowWebSearch: true,
				OutputMode:     agentstate.OutputModeHandoff,
			},
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "Go generics tutorial",
				RuntimeOptions: agentstate.RuntimeOptions{
					MaxIterations:  2,
					AllowWebSearch: true,
					OutputMode:     agentstate.OutputModeHandoff,
				},
			},
			Context: agentstate.ContextState{
				SearchQuery:          "Go generics tutorial",
				SearchProviderActual: "stub",
				SearchResults: []agentstate.SearchResultRef{
					{Title: "Go Docs", URL: "https://go.dev/doc", Snippet: "Generics in Go.", Domain: "go.dev"},
				},
				FetchResults: []agentstate.FetchResultRef{
					{URL: "https://go.dev/doc", Text: "Generics allow reusable code with type parameters."},
				},
			},
			Evidence: agentstate.EvidenceState{
				Items: []agentstate.EvidenceItem{
					{ID: "fetch_1", Source: "fetch", Content: "Generics allow reusable code with type parameters.", Level: "high", SourceRef: "https://go.dev/doc"},
				},
				Sufficient:        true,
				SufficiencyReason: "fetched_readable_evidence",
			},
			Execution: agentstate.ExecutionState{
				CurrentNode:          "handoff",
				Iteration:            1,
				MaxIterations:        2,
				LastBranchTarget:     "handoff",
				LastBranchReason:     "fetched_readable_evidence",
				LastNewURLCount:      1,
				LastNewEvidenceCount: 1,
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{Node: "search", EventType: agentstate.EventTypeCapabilityStart},
			{Node: "fetch", EventType: agentstate.EventTypeCapabilityResult},
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	builder := NewBuilder([]CapabilityProfile{
		{
			Node:               "search",
			Name:               "web_search",
			WorkflowCapability: "search",
			Kind:               "tool",
			RiskLevel:          "low",
		},
		{
			Node:               "fetch",
			Name:               "web_fetch",
			WorkflowCapability: "search",
			Kind:               "tool",
			RiskLevel:          "medium",
			RequiresApproval:   true,
			SupportsParallel:   true,
		},
	})
	result := builder.Build(session)

	if !result.Used || result.Degraded {
		t.Fatalf("expected non-degraded used handoff, got %+v", result)
	}
	if result.DecisionSummary.FinalAction != ActionHandoffToRAG {
		t.Fatalf("expected handoff final action, got %+v", result.DecisionSummary)
	}
	if len(result.EvidenceBundle.SearchResults) != 1 || len(result.EvidenceBundle.Pages) != 1 || len(result.EvidenceBundle.AcceptedEvidence) != 1 {
		t.Fatalf("expected structured evidence bundle, got %+v", result.EvidenceBundle)
	}
	if !strings.Contains(result.ToolContext, "Search results:") || !strings.Contains(result.ToolContext, "Fetched web content:") {
		t.Fatalf("expected prompt-ready tool context, got %q", result.ToolContext)
	}
	if !strings.Contains(result.AnswerGuidance, "Lead with the answer") {
		t.Fatalf("expected grounded answer guidance, got %q", result.AnswerGuidance)
	}
	if !strings.Contains(result.WorkflowPolicy, "output_mode: handoff") {
		t.Fatalf("expected workflow policy to include output mode, got %q", result.WorkflowPolicy)
	}
	if !strings.Contains(result.WorkflowPolicy, "risk_level: medium") {
		t.Fatalf("expected workflow policy to derive highest risk level from used capabilities, got %q", result.WorkflowPolicy)
	}
	if !strings.Contains(result.WorkflowPolicy, "approval_requirement: required") {
		t.Fatalf("expected workflow policy to derive approval requirement from used capabilities, got %q", result.WorkflowPolicy)
	}
}

func TestBuilderWorkflowPolicySummaryDefaultsWithoutProfiles(t *testing.T) {
	builder := NewBuilder(nil)
	session := &agentruntime.RuntimeSession{
		Request: agentruntime.RequestEnvelope{
			Options: agentstate.RuntimeOptions{
				AllowWebSearch: true,
				OutputMode:     agentstate.OutputModeHandoff,
			},
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				RuntimeOptions: agentstate.RuntimeOptions{
					AllowWebSearch: true,
					OutputMode:     agentstate.OutputModeHandoff,
				},
			},
		},
	}

	summary := builder.BuildWorkflowPolicySummary(session)
	if summary.Capability != "general" || summary.ExecutionMode != "read_only" || summary.RiskLevel != "low" {
		t.Fatalf("unexpected default summary: %+v", summary)
	}
}
