package planexecute

import (
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestAssessStepCompletion_SearchResultsPolicySatisfied(t *testing.T) {
	session := newSession("sess-policy-search", "golang generics", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Context.SearchResults = []agentstate.SearchResultRef{
		{ID: "s1", URL: "https://example.com/generics"},
	}

	result := assessStepCompletion(session, agentstate.PlanStep{
		CapabilityName:   agentcapability.NameWebSearch,
		CompletionPolicy: completionPolicySearchResults,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
	})

	if result.disposition != assessmentSatisfied {
		t.Fatalf("expected satisfied disposition, got %+v", result)
	}
	if result.finalize {
		t.Fatalf("expected search results policy to continue, got %+v", result)
	}
	if result.successReason != reasonSearchResultsReady {
		t.Fatalf("expected search-ready reason, got %+v", result)
	}
}

func TestAssessStepCompletion_SearchResultsPolicySatisfiedFromArtifacts(t *testing.T) {
	result := assessStepCompletion(nil, agentstate.PlanStep{
		CapabilityName:   agentcapability.NameWebSearch,
		CompletionPolicy: completionPolicySearchResults,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindSearchResults,
				Kind:         artifactKindSearchResults,
				SourceStepID: "step-search",
				StringValues: []string{"https://example.com/generics"},
				Refs:         []string{"https://example.com/generics"},
			},
		},
	})

	if result.disposition != assessmentSatisfied {
		t.Fatalf("expected satisfied disposition from artifacts, got %+v", result)
	}
	if result.successReason != reasonSearchResultsReady {
		t.Fatalf("expected search-ready reason from artifacts, got %+v", result)
	}
}

func TestAssessStepCompletion_EvidencePolicySatisfiedForFetch(t *testing.T) {
	session := newSession("sess-policy-fetch", "golang generics", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Context.FetchResults = []agentstate.FetchResultRef{
		{
			ID:      "f1",
			URL:     "https://example.com/generics",
			Summary: "Go generics overview",
			Text:    "Go generics let you write reusable functions with type parameters.",
		},
	}

	result := assessStepCompletion(session, agentstate.PlanStep{
		CapabilityName:   agentcapability.NameWebFetch,
		CompletionPolicy: completionPolicyEvidence,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
		URLs:   []string{"https://example.com/generics"},
	})

	if result.disposition != assessmentSatisfied {
		t.Fatalf("expected satisfied disposition, got %+v", result)
	}
	if !result.finalize || !result.sufficient {
		t.Fatalf("expected fetch evidence policy to finalize with sufficient evidence, got %+v", result)
	}
	if len(result.evidenceItems) != 1 {
		t.Fatalf("expected one evidence item, got %+v", result.evidenceItems)
	}
	if result.contextDelta == nil || len(result.contextDelta.SeenURLs) != 1 {
		t.Fatalf("expected seen-urls context update, got %+v", result.contextDelta)
	}
	if result.successReason != reasonFetchEvidenceReady {
		t.Fatalf("expected fetch-evidence-ready reason, got %+v", result)
	}
}

func TestAssessStepCompletion_FetchResultsPolicySatisfiedFromArtifacts(t *testing.T) {
	result := assessStepCompletion(nil, agentstate.PlanStep{
		Produces:         []string{artifactKindFetchResults},
		CompletionPolicy: completionPolicyFetchResults,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
		URLs:   []string{"https://example.com/generics"},
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindFetchResults,
				Kind:         artifactKindFetchResults,
				SourceStepID: "step-fetch",
				StringValues: []string{"https://example.com/generics"},
				Refs:         []string{"https://example.com/generics"},
			},
		},
	})

	if result.disposition != assessmentSatisfied {
		t.Fatalf("expected satisfied fetch-results disposition, got %+v", result)
	}
	if result.finalize {
		t.Fatalf("expected fetch-results policy to continue, got %+v", result)
	}
	if result.successReason != reasonFetchResultsReady {
		t.Fatalf("expected fetch-results-ready reason, got %+v", result)
	}
}

func TestAssessStepCompletion_EvidencePolicySatisfiedFromArtifacts(t *testing.T) {
	session := newSession("sess-policy-fetch-artifacts", "golang generics", agentstate.OutputModeFinalAnswer)

	result := assessStepCompletion(session, agentstate.PlanStep{
		CapabilityName:   agentcapability.NameWebFetch,
		CompletionPolicy: completionPolicyEvidence,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
		URLs:   []string{"https://example.com/generics"},
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindEvidenceRefs,
				Kind:         artifactKindEvidenceRefs,
				SourceStepID: "step-fetch",
				Summary:      "Go generics overview",
				StringValues: []string{"Go generics let you write reusable functions with type parameters."},
				Refs:         []string{"https://example.com/generics"},
			},
		},
	})

	if result.disposition != assessmentSatisfied {
		t.Fatalf("expected satisfied disposition from artifacts, got %+v", result)
	}
	if len(result.evidenceItems) != 1 {
		t.Fatalf("expected one evidence item from artifacts, got %+v", result.evidenceItems)
	}
	if result.evidenceItems[0].Source != "plan_artifact" {
		t.Fatalf("expected artifact-derived evidence source, got %+v", result.evidenceItems[0])
	}
}

func TestCompletionPolicyForStep_PrefersProducesOverCapabilityName(t *testing.T) {
	step := agentstate.PlanStep{
		CapabilityName: "custom_capability",
		Produces:       []string{artifactKindStructuredOutput},
	}

	if got := completionPolicyForStep(step); got != completionPolicyStructuredOutput {
		t.Fatalf("expected structured-output policy from produces, got %q", got)
	}
}

func TestAssessStepCompletion_StructuredOutputPolicySatisfied(t *testing.T) {
	result := assessStepCompletion(nil, agentstate.PlanStep{
		CapabilityName:   agentcapability.NameDocumentInvestigation,
		CompletionPolicy: completionPolicyStructuredOutput,
	}, agentstate.PlanStepResult{
		Status:      agentcapability.StatusSucceeded,
		Observation: "document diagnosis ready",
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         "structured_output",
				Kind:         "structured_output",
				SourceStepID: "step-1",
				Summary:      "document diagnosis ready",
			},
		},
	})

	if result.disposition != assessmentSatisfied {
		t.Fatalf("expected satisfied disposition, got %+v", result)
	}
	if result.finalize {
		t.Fatalf("expected structured-output policy to continue, got %+v", result)
	}
	if result.successReason != reasonPlanCompleted {
		t.Fatalf("expected plan-completed reason, got %+v", result)
	}
}

func TestAssessStepCompletion_StructuredOutputPolicyFailsWithoutStructuredSignal(t *testing.T) {
	result := assessStepCompletion(nil, agentstate.PlanStep{
		Produces:         []string{artifactKindStructuredOutput},
		CompletionPolicy: completionPolicyStructuredOutput,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
		Artifacts: []agentstate.PlanStepArtifact{
			{
				Name:         artifactKindEvidenceRefs,
				Kind:         artifactKindEvidenceRefs,
				SourceStepID: "step-1",
				Summary:      "evidence only",
			},
		},
	})

	if result.disposition != assessmentFailed {
		t.Fatalf("expected structured-output policy to reject non-structured artifacts, got %+v", result)
	}
	if result.failureReason != reasonPlanFailed {
		t.Fatalf("expected plan-failed reason, got %+v", result)
	}
}

func TestAssessStepCompletion_ObservationPolicyFailsWithoutObservation(t *testing.T) {
	result := assessStepCompletion(nil, agentstate.PlanStep{
		CapabilityName:   "custom_capability",
		CompletionPolicy: completionPolicyNonEmptyObservation,
	}, agentstate.PlanStepResult{
		Status: agentcapability.StatusSucceeded,
	})

	if result.disposition != assessmentFailed {
		t.Fatalf("expected failed disposition, got %+v", result)
	}
	if !result.retryable {
		t.Fatalf("expected observation policy failure to be retryable, got %+v", result)
	}
	if result.failureReason != reasonPlanFailed {
		t.Fatalf("expected plan-failed reason, got %+v", result)
	}
}
