package planexecute

import (
	"context"
	"reflect"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"

	"github.com/cloudwego/eino/compose"
)

func TestCompile_Regression_DocumentSearchFetchArtifactChain(t *testing.T) {
	var searchedQueries []string
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			searchedQueries = append(searchedQueries, query)
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/postmortem"},
				Results: []agentsearch.SearchResultItem{{Title: "Postmortem", URL: "https://example.com/postmortem", Snippet: "vector timeout", Domain: "example.com"}},
				Summary: "found corroborating result",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}

	var fetchedURLs [][]string
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchedURLs = append(fetchedURLs, append([]string(nil), urls...))
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "fetched readable corroboration",
				Pages:   []agentfetch.PageResult{{URL: urls[0], Text: "External postmortem confirms a vector timeout."}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}

	documentCapability, err := agentdocumentinvestigation.NewCapability(stubDocumentInvestigator{
		getFn: func(_ context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
			return knowledgedomain.KnowledgeDocument{
				ID:          input.DocumentID,
				Name:        "Incident Doc",
				Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
				ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
				PipelineID:  "pipe-chain",
			}, nil
		},
		pageLogsFn: func(_ context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
			return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
				Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
					{
						Log: knowledgedomain.KnowledgeDocumentChunkLog{
							Status:       knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
							ErrorMessage: "pipeline failed",
						},
						IngestionTask: &ingestiondomain.Task{
							ID:     "task-chain",
							Status: ingestiondomain.TaskStatusFailed,
						},
						IngestionNodes: []ingestiondomain.TaskNode{
							{
								NodeID:       "indexer",
								Status:       ingestiondomain.TaskStatusFailed,
								ErrorMessage: "vector timeout",
							},
						},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("document capability: %v", err)
	}

	registry := registerHandles(t, searchCapability, fetchCapability, documentCapability)
	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registry),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
		Synthesizer: patternPlanSynthesizer{
			result: PlanSynthesisResult{
				Plan: agentstate.PlanState{
					Goal:   "investigate then corroborate externally",
					PlanID: "plan_chain",
					Status: agentstate.PlanStatusActive,
					Steps: []agentstate.PlanStep{
						{
							StepID:           "step_document",
							Title:            "Investigate document",
							CapabilityName:   agentcapability.NameDocumentInvestigation,
							CapabilityKind:   agentcapability.KindWorkflow,
							CapabilityFamily: agentcapability.FamilyDocumentInvestigation,
							CapabilityRole:   agentcapability.RoleInvestigateDocument,
							CapabilityInput:  map[string]any{"document_id": "doc-chain"},
							Produces:         []string{artifactKindStructuredOutput, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
						{
							StepID:           "step_search",
							Title:            "Search corroborating evidence",
							CapabilityName:   agentcapability.NameWebSearch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleSearch,
							Consumes:         []string{artifactKindStructuredOutput},
							Produces:         []string{artifactKindSearchResults, artifactKindURLs},
							CompletionPolicy: completionPolicySearchResults,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
						{
							StepID:           "step_fetch",
							Title:            "Fetch corroborating evidence",
							CapabilityName:   agentcapability.NameWebFetch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleFetch,
							Consumes:         []string{artifactKindURLs},
							Produces:         []string{artifactKindFetchResults, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
					},
				},
				Reasoning: "built regression artifact chain plan",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-chain", "why did document doc-chain fail", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Snapshot.Plan.Steps) != 3 {
		t.Fatalf("expected three-step regression plan, got %+v", result.Snapshot.Plan.Steps)
	}
	for i, step := range result.Snapshot.Plan.Steps {
		if step.Status != agentstate.PlanStepStatusCompleted {
			t.Fatalf("expected regression step %d to complete, got %+v", i, step)
		}
	}
	if len(searchedQueries) != 1 || !strings.Contains(searchedQueries[0], "why did document doc-chain fail") || !strings.Contains(searchedQueries[0], "document=doc-chain") {
		t.Fatalf("expected artifact-enriched search query, got %+v", searchedQueries)
	}
	if len(fetchedURLs) != 1 || !reflect.DeepEqual(fetchedURLs[0], []string{"https://example.com/postmortem"}) {
		t.Fatalf("expected fetch step to consume search artifact urls, got %+v", fetchedURLs)
	}
	if result.Snapshot.Plan.LastStepResult.CapabilityName != agentcapability.NameWebFetch {
		t.Fatalf("expected last step result to come from fetch corroboration, got %+v", result.Snapshot.Plan.LastStepResult)
	}
	if len(result.Snapshot.Evidence.Items) < 2 {
		t.Fatalf("expected both internal and external evidence to be accumulated, got %+v", result.Snapshot.Evidence.Items)
	}
}

func TestCompile_Regression_MixedRetryOptionalChain(t *testing.T) {
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/one", "https://example.com/two"},
				Results: []agentsearch.SearchResultItem{{Title: "One", URL: "https://example.com/one", Snippet: "first", Domain: "example.com"}},
				Summary: "found retry targets",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}

	var fetchCalls int
	var fetchedURLs [][]string
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchCalls++
			fetchedURLs = append(fetchedURLs, append([]string(nil), urls...))
			switch fetchCalls {
			case 1:
				return agentfetch.Output{
					URLs:          append([]string(nil), urls...),
					Summary:       "optional prefetch failed",
					Degraded:      true,
					DegradeReason: "optional_prefetch_failure",
					ErrorMessage:  "optional prefetch failure",
					Pages:         []agentfetch.PageResult{{URL: urls[0], ErrorMessage: "optional prefetch failure"}},
				}, nil
			case 2:
				return agentfetch.Output{
					URLs:          append([]string(nil), urls...),
					Summary:       "required fetch first attempt failed",
					Degraded:      true,
					DegradeReason: "required_fetch_failure",
					ErrorMessage:  "required fetch failure",
					Pages:         []agentfetch.PageResult{{URL: urls[0], ErrorMessage: "required fetch failure"}},
				}, nil
			default:
				return agentfetch.Output{
					URLs:    append([]string(nil), urls...),
					Summary: "required fetch retry succeeded",
					Pages:   []agentfetch.PageResult{{URL: urls[0], Text: "required fetch retry succeeded"}},
				}, nil
			}
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}

	documentCapability, err := agentdocumentinvestigation.NewCapability(stubDocumentInvestigator{
		getFn: func(_ context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
			return knowledgedomain.KnowledgeDocument{
				ID:          input.DocumentID,
				Name:        "Incident Doc",
				Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
				ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
				PipelineID:  "pipe-regression",
			}, nil
		},
		pageLogsFn: func(_ context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
			return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
				Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
					{
						Log: knowledgedomain.KnowledgeDocumentChunkLog{
							Status:       knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
							ErrorMessage: "pipeline failed",
						},
						IngestionTask: &ingestiondomain.Task{
							ID:     "task-regression",
							Status: ingestiondomain.TaskStatusFailed,
						},
						IngestionNodes: []ingestiondomain.TaskNode{
							{
								NodeID:       "parser",
								Status:       ingestiondomain.TaskStatusFailed,
								ErrorMessage: "decoder panic",
							},
						},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("document capability: %v", err)
	}

	registry := registerHandles(t, searchCapability, fetchCapability, documentCapability)
	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registry),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode: agentstate.OutputModeFinalAnswer,
			Kernel: agentkernel.BuilderConfig{
				CompileOptions: []compose.GraphCompileOption{
					compose.WithMaxRunSteps(32),
				},
			},
		},
		Synthesizer: patternPlanSynthesizer{
			result: PlanSynthesisResult{
				Plan: agentstate.PlanState{
					Goal:   "exercise mixed retry and optional semantics together",
					PlanID: "plan_regression_combo",
					Status: agentstate.PlanStatusActive,
					Steps: []agentstate.PlanStep{
						{
							StepID:           "step_document",
							Title:            "Investigate document",
							CapabilityName:   agentcapability.NameDocumentInvestigation,
							CapabilityKind:   agentcapability.KindWorkflow,
							CapabilityFamily: agentcapability.FamilyDocumentInvestigation,
							CapabilityRole:   agentcapability.RoleInvestigateDocument,
							CapabilityInput:  map[string]any{"document_id": "doc-regression"},
							Produces:         []string{artifactKindStructuredOutput, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
						{
							StepID:           "step_optional_prefetch",
							Title:            "Optional prefetch",
							CapabilityName:   agentcapability.NameWebFetch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleFetch,
							CapabilityInput:  map[string]any{"urls": []string{"https://example.com/optional"}},
							Produces:         []string{artifactKindFetchResults, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							Optional:         true,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
						{
							StepID:           "step_search",
							Title:            "Search follow-up evidence",
							CapabilityName:   agentcapability.NameWebSearch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleSearch,
							Consumes:         []string{artifactKindStructuredOutput},
							Produces:         []string{artifactKindSearchResults, artifactKindURLs},
							CompletionPolicy: completionPolicySearchResults,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
						{
							StepID:           "step_required_fetch",
							Title:            "Required fetch with retry",
							CapabilityName:   agentcapability.NameWebFetch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleFetch,
							Consumes:         []string{artifactKindURLs},
							Produces:         []string{artifactKindFetchResults, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      2,
							Status:           agentstate.PlanStepStatusPending,
						},
					},
				},
				Reasoning: "built regression combo plan",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	session := newSession("sess-plan-regression-combo", "why did document doc-regression fail", agentstate.OutputModeFinalAnswer)
	session.Request.Options.MaxIterations = 8
	session.Snapshot.Request.RuntimeOptions.MaxIterations = 8
	session.Snapshot.Execution.MaxIterations = 8
	result, err := runner.Run(context.Background(), session)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	steps := result.Snapshot.Plan.Steps
	if len(steps) != 4 {
		t.Fatalf("expected four-step regression combo plan, got %+v", steps)
	}
	if steps[0].Status != agentstate.PlanStepStatusCompleted {
		t.Fatalf("expected document step to complete, got %+v", steps[0])
	}
	if steps[1].Status != agentstate.PlanStepStatusSkipped {
		t.Fatalf("expected optional prefetch to skip, got %+v", steps[1])
	}
	if steps[2].Status != agentstate.PlanStepStatusCompleted {
		t.Fatalf("expected search step to complete, got %+v", steps[2])
	}
	if steps[3].Status != agentstate.PlanStepStatusCompleted || steps[3].AttemptCount != 2 {
		t.Fatalf("expected required fetch to complete after retry, got %+v", steps[3])
	}
	if fetchCalls != 3 {
		t.Fatalf("expected three total fetch calls across optional skip and retry, got %d", fetchCalls)
	}
	if !reflect.DeepEqual(fetchedURLs[0], []string{"https://example.com/optional"}) {
		t.Fatalf("expected optional prefetch url first, got %+v", fetchedURLs)
	}
	if len(fetchedURLs) < 3 || !reflect.DeepEqual(fetchedURLs[1], []string{"https://example.com/one"}) || !reflect.DeepEqual(fetchedURLs[2], []string{"https://example.com/one"}) {
		t.Fatalf("expected required fetch to retry same artifact-selected url, got %+v", fetchedURLs)
	}
	if result.Snapshot.Plan.LastStepResult.CapabilityName != agentcapability.NameWebFetch {
		t.Fatalf("expected last step result to come from required fetch retry, got %+v", result.Snapshot.Plan.LastStepResult)
	}
	if len(result.Snapshot.Evidence.Items) < 2 {
		t.Fatalf("expected both document and fetched evidence to be accumulated, got %+v", result.Snapshot.Evidence.Items)
	}
}
