package planexecute

import (
	"reflect"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestPrepareStepInputs_WebFetchPrefersArtifactURLs(t *testing.T) {
	session := newSession("sess-step-input-fetch", "artifact urls", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Context.SearchResults = []agentstate.SearchResultRef{
		{URL: "https://context.example/one"},
	}
	session.Snapshot.Plan.LastStepResult = agentstate.PlanStepResult{
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindURLs,
				Kind:         artifactKindURLs,
				SourceStepID: "step_search",
				StringValues: []string{"https://artifact.example/one", "https://artifact.example/two"},
			},
		},
	}
	step := agentstate.PlanStep{
		CapabilityName: agentcapability.NameWebFetch,
	}

	prepareStepInputs(session, &step)

	if !reflect.DeepEqual(step.URLs, []string{"https://artifact.example/one"}) {
		t.Fatalf("expected artifact-first fetch urls, got %+v", step.URLs)
	}
	if got := toStringSlice(step.CapabilityInput["urls"]); !reflect.DeepEqual(got, []string{"https://artifact.example/one"}) {
		t.Fatalf("expected capability input urls to follow artifact-first selection, got %+v", step.CapabilityInput)
	}
}

func TestPrepareStepInputs_ExternalEvidenceCollectUsesStructuredArtifactContext(t *testing.T) {
	session := newSession("sess-step-input-external", "why did document doc-77 fail", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Plan.LastStepResult = agentstate.PlanStepResult{
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindStructuredOutput,
				Kind:         artifactKindStructuredOutput,
				SourceStepID: "step_selected_capability",
				StringValues: []string{"document=doc-77 confidence=high conclusion=document ingestion failed at node chunk-index"},
			},
		},
	}
	step := agentstate.PlanStep{
		CapabilityName: agentcapability.NameExternalEvidenceCollect,
		Consumes:       []string{artifactKindStructuredOutput},
	}

	prepareStepInputs(session, &step)

	if step.Query == "" || step.CapabilityInput["query"] == nil {
		t.Fatalf("expected query to be synthesized from request plus structured artifact, got %+v", step)
	}
	if query := step.CapabilityInput["query"].(string); !containsString(query, "why did document doc-77 fail") || !containsString(query, "document=doc-77") {
		t.Fatalf("expected query to include request and structured artifact context, got %q", query)
	}
}

func TestPrepareStepInputs_WebSearchCanConsumeStructuredArtifactContext(t *testing.T) {
	session := newSession("sess-step-input-search", "find corroborating evidence", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Plan.LastStepResult = agentstate.PlanStepResult{
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindStructuredOutput,
				Kind:         artifactKindStructuredOutput,
				SourceStepID: "step_previous",
				StringValues: []string{"latest_node_error=vector store timeout"},
			},
		},
	}
	step := agentstate.PlanStep{
		CapabilityName: agentcapability.NameWebSearch,
		Consumes:       []string{artifactKindStructuredOutput},
	}

	prepareStepInputs(session, &step)

	if query := step.CapabilityInput["query"].(string); !containsString(query, "find corroborating evidence") || !containsString(query, "vector store timeout") {
		t.Fatalf("expected search query to include structured artifact context, got %q", query)
	}
}

func containsString(value, substr string) bool {
	return len(value) > 0 && len(substr) > 0 && strings.Contains(value, substr)
}
