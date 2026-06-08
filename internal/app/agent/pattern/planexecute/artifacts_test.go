package planexecute

import (
	"reflect"
	"testing"

	agentstate "local/rag-project/internal/app/agent/state"
)

func TestSelectedFetchURLs_PrefersArtifactURLsOverContext(t *testing.T) {
	session := newSession("sess-artifacts-select", "artifact urls", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Plan.LastStepResult = agentstate.PlanStepResult{
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindURLs,
				Kind:         artifactKindURLs,
				SourceStepID: "step-search",
				StringValues: []string{"https://artifact.example/one", "https://artifact.example/two"},
				Refs:         []string{"https://artifact.example/one", "https://artifact.example/two"},
			},
		},
	}
	session.Snapshot.Context.SearchResults = []agentstate.SearchResultRef{
		{ID: "ctx-1", URL: "https://context.example/one"},
	}

	selected := selectedFetchURLs(session)
	if !reflect.DeepEqual(selected, []string{"https://artifact.example/one"}) {
		t.Fatalf("expected artifact urls to be preferred, got %+v", selected)
	}
}
