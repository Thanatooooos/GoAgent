package tool

import (
	"context"
	"strings"
	"testing"
)

func TestWebSearchBehaviorDecodeAndNext(t *testing.T) {
	behavior := WebSearchBehavior()
	result := Result{
		Name: "web_search",
		Data: map[string]any{
			"results": []map[string]any{
				{"title": "A", "url": "https://example.com/a", "policy": "deny"},
				{"title": "B", "url": "https://example.com/b", "policy": "allow"},
			},
		},
	}

	decoded, err := behavior.Decode(result)
	if err != nil {
		t.Fatalf("decode web_search: %v", err)
	}
	view, ok := decoded.(WebSearchResultView)
	if !ok {
		t.Fatalf("expected WebSearchResultView, got %T", decoded)
	}
	if len(view.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(view.Results))
	}

	decision := behavior.Next(result, WorkflowInput{})
	if decision.Done {
		t.Fatal("expected web_search behavior to continue to fetch")
	}
	if len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "web_fetch" {
		t.Fatalf("expected web_fetch hint, got %+v", decision.HintCalls)
	}
}

func TestWebFetchBehaviorObserveRenderAndGuidance(t *testing.T) {
	behavior := WebFetchBehavior()
	result := Result{
		Name:   "web_fetch",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"pages": []map[string]any{
				{
					"url":          "https://example.com/a",
					"text":         "Fetched explanation from the web page.",
					"wasTruncated": true,
				},
			},
		},
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled {
		t.Fatal("expected web_fetch observe hook to handle the result")
	}
	if !observation.Done || observation.Confidence != 0.7 {
		t.Fatalf("unexpected observation: %+v", observation)
	}

	rendered := behavior.RenderContext(result)
	if !strings.Contains(rendered, "Fetched web content") {
		t.Fatalf("expected fetched content render, got %q", rendered)
	}

	guidance := behavior.BuildGuidance(result, GuidanceInput{
		AllResults: []Result{
			{
				Name:    "document_list",
				Status:  CallStatusSuccess,
				Summary: "matched 1 local document",
			},
			{
				Name:   "web_search",
				Status: CallStatusSuccess,
				Data: map[string]any{
					"results": []map[string]any{
						{"title": "Official Guide", "url": "https://example.com/a", "policy": "allow", "sourceType": "official_docs"},
					},
				},
			},
			result,
		},
	})
	if len(guidance) == 0 || !strings.Contains(guidance[0].Text, "搜索结果来源") {
		t.Fatalf("expected web guidance text, got %+v", guidance)
	}
}

func TestExternalEvidenceWorkflowBehaviorObserveRenderAndGuidance(t *testing.T) {
	behavior := ExternalEvidenceWorkflowBehavior()
	result := Result{
		Name:    "external_evidence_workflow",
		Status:  CallStatusSuccess,
		Summary: "external evidence workflow completed",
		Data: map[string]any{
			"selectedUrls":        []string{"https://go.dev/doc/tutorial/generics"},
			"selectedSourceTypes": []string{"official_docs"},
			"sourceCoverage":      "allow_only",
			"readiness":           "ready",
			"readinessConfidence": 0.82,
			"readinessReasoning":  "The fetched source is sufficient to answer with attribution.",
			"answerStrategy":      "Answer directly and cite the official docs first.",
			"citedUrls":           []string{"https://go.dev/doc/tutorial/generics"},
			"sourceReview": map[string]any{
				"selectedSources": []map[string]any{
					{
						"title":      "Generics tutorial",
						"url":        "https://go.dev/doc/tutorial/generics",
						"policy":     "allow",
						"sourceType": "official_docs",
					},
				},
			},
			"qualityAssessment": map[string]any{
				"quality":    "strong",
				"confidence": 0.8,
				"reasoning":  "Readable external evidence was fetched from selected sources.",
			},
			"pages": []map[string]any{
				{
					"url":  "https://go.dev/doc/tutorial/generics",
					"text": "Go generics let you write functions and types that work with multiple types.",
				},
			},
		},
	}

	decoded, err := behavior.Decode(result)
	if err != nil {
		t.Fatalf("decode external evidence: %v", err)
	}
	if _, ok := decoded.(ExternalEvidenceWorkflowView); !ok {
		t.Fatalf("expected ExternalEvidenceWorkflowView, got %T", decoded)
	}

	observation, handled := behavior.Observe(result, ObserveInput{})
	if !handled || !observation.Done {
		t.Fatalf("expected external evidence observe hook to finish, got handled=%v observation=%+v", handled, observation)
	}

	rendered := behavior.RenderContext(result)
	if !strings.Contains(rendered, "Readiness: ready") {
		t.Fatalf("expected readiness in render context, got %q", rendered)
	}

	guidance := behavior.BuildGuidance(result, GuidanceInput{
		AllResults: []Result{
			{
				Name:    "document_list",
				Status:  CallStatusSuccess,
				Summary: "matched 1 local document about Go syntax basics",
			},
			result,
		},
	})
	if len(guidance) == 0 || !strings.Contains(guidance[0].Text, "外部来源质量要求") {
		t.Fatalf("expected external evidence guidance, got %+v", guidance)
	}
}

func TestRuleObserverUsesRegistryBehaviorForExternalEvidenceWorkflow(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "external_evidence_workflow",
			Description: "external evidence workflow",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "question", Type: ParamTypeString, Required: true}},
		},
	}, ToolSpec{
		Capability:          CapabilitySearch,
		EvidenceSources:     []string{EvidenceSourceExternalWeb},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "web",
	}, ExternalEvidenceWorkflowBehavior()).Module())

	observation, err := NewRuleObserver().Observe(context.Background(), ObserveInput{
		RoundResults: []Result{{
			Name:    "external_evidence_workflow",
			Status:  CallStatusSuccess,
			Summary: "workflow complete",
			Data: map[string]any{
				"readinessConfidence": 0.82,
				"readinessReasoning":  "enough evidence",
			},
		}},
		ToolRegistry: registry,
	})
	if err != nil {
		t.Fatalf("observe with registry: %v", err)
	}
	if !observation.Done || observation.State.Phase != "complete" {
		t.Fatalf("expected registry-backed observe result, got %+v", observation)
	}
}

func TestAgentLoopWebModulesUseBehaviorDrivenContinuation(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "web_search",
			Description: "search the web",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "query", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "web_search",
			Status:  CallStatusSuccess,
			Summary: "found 1 web result",
			Data: map[string]any{
				"results": []map[string]any{
					{"title": "Official Guide", "url": "https://example.com/official", "policy": "allow", "sourceType": "official_docs"},
				},
			},
		},
	}, ToolSpec{
		Capability:          CapabilitySearch,
		EvidenceSources:     []string{EvidenceSourceExternalWeb},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "web",
	}, WebSearchBehavior()).Module())
	registry.MustRegisterModule(NewLegacyToolAdapterWithBehavior(staticTool{
		definition: Definition{
			Name:        "web_fetch",
			Description: "fetch web pages",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "urls", Type: ParamTypeArray, Required: true}},
		},
		result: Result{
			Name:    "web_fetch",
			Status:  CallStatusSuccess,
			Summary: "fetched 1 urls: 1 ok, 0 failed",
			Data: map[string]any{
				"pages": []map[string]any{
					{
						"url":  "https://example.com/official",
						"text": "Fetched explanation from official docs.",
					},
				},
			},
		},
	}, ToolSpec{
		Capability:          CapabilitySearch,
		EvidenceSources:     []string{EvidenceSourceExternalWeb},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "web",
	}, WebFetchBehavior()).Module())

	planner := &plannerStub{
		results: []PlanResult{
			{
				Calls: []Call{{Name: "web_search", Arguments: map[string]any{"query": "golang generics"}}},
			},
		},
	}

	loop := NewAgentLoop(NewExecutor(registry))
	loop.SetPlanner(planner)

	result, err := loop.Run(context.Background(), WorkflowInput{
		Question: "golang generics 是什么",
	})
	if err != nil {
		t.Fatalf("run agent loop: %v", err)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected web_search then web_fetch, got %+v", result.Calls)
	}
	if result.Calls[0].Name != "web_search" || result.Calls[1].Name != "web_fetch" {
		t.Fatalf("unexpected call order: %+v", result.Calls)
	}
	if len(result.Rounds) < 1 || result.Rounds[0].State.Phase != "fetching" {
		t.Fatalf("expected first round to continue into fetching via module behavior, got %+v", result.Rounds)
	}
	if !strings.Contains(result.Context, "Fetched web content") {
		t.Fatalf("expected module-backed render context, got %q", result.Context)
	}
	if !strings.Contains(result.AnswerGuidance, "搜索结果来源") {
		t.Fatalf("expected module-backed web guidance, got %q", result.AnswerGuidance)
	}
}
