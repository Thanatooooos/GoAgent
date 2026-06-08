package state

import "testing"

func TestClonePlanState_DeepCopiesGeneralizedStepFields(t *testing.T) {
	original := PlanState{
		Steps: []PlanStep{
			{
				StepID:           "step-1",
				Goal:             "collect evidence",
				CapabilityInput:  map[string]any{"query": "golang", "filters": []any{"docs"}},
				Consumes:         []string{"search_results"},
				Produces:         []string{"fetch_results", "evidence_refs"},
				URLs:             []string{"https://example.com/1"},
				DependsOn:        []string{"step-0"},
				ExpectedEvidence: []string{"readable fetched evidence"},
			},
		},
		LastStepResult: PlanStepResult{
			StepID: "step-1",
			Artifacts: []PlanStepArtifact{
				{
					Name:         "fetch_results",
					Kind:         "fetch_results",
					SourceStepID: "step-1",
					StringValues: []string{"https://example.com/1"},
					Refs:         []string{"https://example.com/1"},
					Metadata: map[string]string{
						"page_count": "1",
					},
				},
			},
			URLs: []string{"https://example.com/1"},
		},
	}

	cloned := ClonePlanState(original)

	cloned.Steps[0].CapabilityInput["query"] = "rust"
	cloned.Steps[0].Consumes[0] = "structured_output"
	cloned.Steps[0].Produces[0] = "diagnosis_summary"
	cloned.Steps[0].URLs[0] = "https://example.com/2"
	cloned.Steps[0].DependsOn[0] = "step-x"
	cloned.Steps[0].ExpectedEvidence[0] = "changed"

	typedFilters, ok := cloned.Steps[0].CapabilityInput["filters"].([]any)
	if !ok || len(typedFilters) != 1 {
		t.Fatalf("expected cloned nested filters slice, got %#v", cloned.Steps[0].CapabilityInput["filters"])
	}
	typedFilters[0] = "blogs"
	cloned.LastStepResult.Artifacts[0].StringValues[0] = "https://example.com/2"
	cloned.LastStepResult.Artifacts[0].Refs[0] = "https://example.com/ref-2"
	cloned.LastStepResult.Artifacts[0].Metadata["page_count"] = "2"
	cloned.LastStepResult.URLs[0] = "https://example.com/2"

	if original.Steps[0].CapabilityInput["query"] != "golang" {
		t.Fatalf("expected original capability input to remain unchanged, got %#v", original.Steps[0].CapabilityInput["query"])
	}
	originalFilters, ok := original.Steps[0].CapabilityInput["filters"].([]any)
	if !ok || len(originalFilters) != 1 || originalFilters[0] != "docs" {
		t.Fatalf("expected original nested filters slice to remain unchanged, got %#v", original.Steps[0].CapabilityInput["filters"])
	}
	if original.Steps[0].Consumes[0] != "search_results" {
		t.Fatalf("expected original consumes to remain unchanged, got %#v", original.Steps[0].Consumes)
	}
	if original.Steps[0].Produces[0] != "fetch_results" {
		t.Fatalf("expected original produces to remain unchanged, got %#v", original.Steps[0].Produces)
	}
	if original.Steps[0].URLs[0] != "https://example.com/1" {
		t.Fatalf("expected original urls to remain unchanged, got %#v", original.Steps[0].URLs)
	}
	if original.Steps[0].DependsOn[0] != "step-0" {
		t.Fatalf("expected original dependencies to remain unchanged, got %#v", original.Steps[0].DependsOn)
	}
	if original.Steps[0].ExpectedEvidence[0] != "readable fetched evidence" {
		t.Fatalf("expected original expected evidence to remain unchanged, got %#v", original.Steps[0].ExpectedEvidence)
	}
	if original.LastStepResult.Artifacts[0].StringValues[0] != "https://example.com/1" {
		t.Fatalf("expected original artifact string values to remain unchanged, got %#v", original.LastStepResult.Artifacts[0].StringValues)
	}
	if original.LastStepResult.Artifacts[0].Refs[0] != "https://example.com/1" {
		t.Fatalf("expected original artifact refs to remain unchanged, got %#v", original.LastStepResult.Artifacts[0].Refs)
	}
	if original.LastStepResult.Artifacts[0].Metadata["page_count"] != "1" {
		t.Fatalf("expected original artifact metadata to remain unchanged, got %#v", original.LastStepResult.Artifacts[0].Metadata)
	}
	if original.LastStepResult.URLs[0] != "https://example.com/1" {
		t.Fatalf("expected original last-step urls to remain unchanged, got %#v", original.LastStepResult.URLs)
	}
}
