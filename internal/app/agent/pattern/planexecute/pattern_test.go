package planexecute

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentexternalevidence "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
)

func TestCompile_RunAnswerPath(t *testing.T) {
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			if query != "golang generics" {
				t.Fatalf("unexpected query: %q", query)
			}
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/generics"},
				Results: []agentsearch.SearchResultItem{{Title: "Go Docs", URL: "https://example.com/generics", Snippet: "type parameters", Domain: "example.com"}},
				Summary: "found one relevant result",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			if !reflect.DeepEqual(urls, []string{"https://example.com/generics"}) {
				t.Fatalf("unexpected fetch urls: %v", urls)
			}
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "fetched readable evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "Go generics let you write reusable functions with type parameters."},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registerHandles(t, searchCapability, fetchCapability)),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-answer", "golang generics", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "type parameters") {
		t.Fatalf("expected grounded final answer, got %+v", result.Snapshot.Answer)
	}
	if result.Snapshot.Plan.Status != agentstate.PlanStatusCompleted {
		t.Fatalf("expected completed plan, got %+v", result.Snapshot.Plan)
	}
	if len(result.Snapshot.Plan.Steps) != 2 {
		t.Fatalf("expected two plan steps, got %+v", result.Snapshot.Plan.Steps)
	}
	if result.Snapshot.Plan.Steps[0].Goal == "" || result.Snapshot.Plan.Steps[0].CompletionPolicy == "" {
		t.Fatalf("expected generalized step semantics on search step, got %+v", result.Snapshot.Plan.Steps[0])
	}
	if result.Snapshot.Plan.Steps[1].Goal == "" || len(result.Snapshot.Plan.Steps[1].Consumes) == 0 || len(result.Snapshot.Plan.Steps[1].Produces) == 0 {
		t.Fatalf("expected generalized step semantics on fetch step, got %+v", result.Snapshot.Plan.Steps[1])
	}
	if result.Snapshot.Plan.LastStepResult.Attempt != 1 || result.Snapshot.Plan.LastStepResult.DurationMs < 0 {
		t.Fatalf("expected last step result execution metadata, got %+v", result.Snapshot.Plan.LastStepResult)
	}
	if len(result.Snapshot.Plan.LastStepResult.Artifacts) == 0 {
		t.Fatalf("expected last step result artifacts, got %+v", result.Snapshot.Plan.LastStepResult)
	}
	if len(result.Snapshot.Evidence.Items) == 0 || !result.Snapshot.Evidence.Sufficient {
		t.Fatalf("expected sufficient evidence, got %+v", result.Snapshot.Evidence)
	}
	if result.Snapshot.Execution.CurrentNode != "finalize" {
		t.Fatalf("expected finalize as terminal node, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_RunHandoffPath(t *testing.T) {
	searchCapability, _ := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/handoff"},
				Results: []agentsearch.SearchResultItem{{Title: "Doc", URL: "https://example.com/handoff", Snippet: "handoff", Domain: "example.com"}},
				Summary: "found one result",
			}, nil
		},
	})
	fetchCapability, _ := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "fetched readable evidence",
				Pages:   []agentfetch.PageResult{{URL: urls[0], Text: "handoff evidence"}},
			}, nil
		},
	})

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registerHandles(t, searchCapability, fetchCapability)),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-handoff", "handoff please", agentstate.OutputModeHandoff))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Answer.Final != "" {
		t.Fatalf("expected handoff mode to skip final answer, got %+v", result.Snapshot.Answer)
	}
	if !contains(result.Snapshot.Context.Notes, "handoff ready: explicit plan completed with grounded evidence") {
		t.Fatalf("expected handoff note, got %+v", result.Snapshot.Context.Notes)
	}
}

func TestCompile_ReplansToUnseenURL(t *testing.T) {
	searchCapability, _ := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{
				Query: query,
				URLs:  []string{"https://example.com/one", "https://example.com/two"},
				Results: []agentsearch.SearchResultItem{
					{Title: "One", URL: "https://example.com/one", Snippet: "first", Domain: "example.com"},
					{Title: "Two", URL: "https://example.com/two", Snippet: "second", Domain: "example.com"},
				},
				Summary: "found two results",
			}, nil
		},
	})
	var fetchCalls [][]string
	fetchCapability, _ := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchCalls = append(fetchCalls, append([]string(nil), urls...))
			if len(fetchCalls) == 1 {
				return agentfetch.Output{
					URLs:          append([]string(nil), urls...),
					Summary:       "first fetch returned no readable content",
					Degraded:      true,
					DegradeReason: "temporary_fetch_failure",
					ErrorMessage:  "temporary fetch failure",
					Pages:         []agentfetch.PageResult{{URL: urls[0], ErrorMessage: "temporary fetch failure"}},
				}, nil
			}
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "second fetch produced readable evidence",
				Pages:   []agentfetch.PageResult{{URL: urls[0], Text: "second fetch produced readable evidence"}},
			}, nil
		},
	})

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registerHandles(t, searchCapability, fetchCapability)),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-replan", "retry fetch", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(fetchCalls) != 2 {
		t.Fatalf("expected two fetch calls across replan, got %v", fetchCalls)
	}
	if !reflect.DeepEqual(fetchCalls[0], []string{"https://example.com/one"}) {
		t.Fatalf("expected first fetch to use first url, got %v", fetchCalls[0])
	}
	if !reflect.DeepEqual(fetchCalls[1], []string{"https://example.com/two"}) {
		t.Fatalf("expected replan to use unseen url, got %v", fetchCalls[1])
	}
	if result.Snapshot.Plan.ReplanCount != 1 {
		t.Fatalf("expected one replan, got %+v", result.Snapshot.Plan)
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "second fetch produced readable evidence") {
		t.Fatalf("expected recovered final answer, got %+v", result.Snapshot.Answer)
	}
}

func TestCompile_SelectorChoosesDocumentInvestigation(t *testing.T) {
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, _ string) (agentsearch.SearchOutput, error) {
			t.Fatal("search capability should not be invoked when selector chooses document investigation")
			return agentsearch.SearchOutput{}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			t.Fatal("fetch capability should not be invoked when selector chooses document investigation")
			return agentfetch.Output{}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}
	documentCapability, err := agentdocumentinvestigation.NewCapability(stubDocumentInvestigator{
		getFn: func(_ context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
			if input.DocumentID != "doc-77" {
				t.Fatalf("unexpected document id: %q", input.DocumentID)
			}
			return knowledgedomain.KnowledgeDocument{
				ID:          "doc-77",
				Name:        "Incident Doc",
				Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
				ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
				PipelineID:  "pipe-77",
			}, nil
		},
		pageLogsFn: func(_ context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
			if input.DocumentID != "doc-77" {
				t.Fatalf("unexpected page input document id: %q", input.DocumentID)
			}
			return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
				Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
					{
						Log: knowledgedomain.KnowledgeDocumentChunkLog{
							Status:       knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
							ErrorMessage: "pipeline failed",
						},
						IngestionTask: &ingestiondomain.Task{
							ID:     "task-77",
							Status: ingestiondomain.TaskStatusFailed,
						},
						IngestionNodes: []ingestiondomain.TaskNode{
							{
								NodeID:       "chunk-index",
								Status:       ingestiondomain.TaskStatusFailed,
								ErrorMessage: "vector store timeout",
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
			OutputMode:               agentstate.OutputModeFinalAnswer,
			CapabilityCatalogBuilder: agentcatalog.NewBuilder(),
			CapabilitySelector: stubSelector{
				selectFn: func(_ context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
					if len(input.Capabilities) < 3 {
						t.Fatalf("expected selector to receive catalog, got %+v", input.Capabilities)
					}
					return selectcapability.SelectionOutput{
						Selections: []selectcapability.CapabilitySelection{
							{
								Family: agentcapability.FamilyDocumentInvestigation,
								Role:   agentcapability.RoleInvestigateDocument,
								Input: map[string]any{
									"document_id": "doc-77",
								},
								Reason:     "Investigate the requested internal document directly",
								Confidence: "high",
							},
						},
					}, nil
				},
			},
			CapabilityResolver: agentresolve.NewRegistryResolver(registry),
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-doc", "why did document doc-77 fail", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Plan.CurrentStepIndex != 0 || len(result.Snapshot.Plan.Steps) != 1 {
		t.Fatalf("expected one selected step, got %+v", result.Snapshot.Plan)
	}
	if result.Snapshot.Plan.Steps[0].CapabilityName != agentcapability.NameDocumentInvestigation {
		t.Fatalf("expected selected document investigation step, got %+v", result.Snapshot.Plan.Steps[0])
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "document ingestion failed at node chunk-index") {
		t.Fatalf("expected final answer grounded in document investigation, got %+v", result.Snapshot.Answer)
	}
}

func TestCompile_SelectorBuildsMixedPlanForDocumentInvestigation(t *testing.T) {
	var searchedQueries []string
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			searchedQueries = append(searchedQueries, query)
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/doc-77-postmortem"},
				Results: []agentsearch.SearchResultItem{{Title: "Postmortem", URL: "https://example.com/doc-77-postmortem", Snippet: "chunk-index timeout", Domain: "example.com"}},
				Summary: "found corroborating result",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "fetched corroborating external evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "External write-up confirms a chunk-index timeout."},
				},
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
				PipelineID:  "pipe-77",
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
							ID:     "task-77",
							Status: ingestiondomain.TaskStatusFailed,
						},
						IngestionNodes: []ingestiondomain.TaskNode{
							{
								NodeID:       "chunk-index",
								Status:       ingestiondomain.TaskStatusFailed,
								ErrorMessage: "vector store timeout",
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
	externalCapability, err := agentexternalevidence.NewCapability(searchCapability, fetchCapability)
	if err != nil {
		t.Fatalf("external evidence capability: %v", err)
	}

	registry := registerHandles(t, searchCapability, fetchCapability, documentCapability, externalCapability)
	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registry),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode:               agentstate.OutputModeFinalAnswer,
			CapabilityCatalogBuilder: agentcatalog.NewBuilder(),
			CapabilitySelector: stubSelector{
				selectFn: func(_ context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
					return selectcapability.SelectionOutput{
						Selections: []selectcapability.CapabilitySelection{
							{
								Family: agentcapability.FamilyDocumentInvestigation,
								Role:   agentcapability.RoleInvestigateDocument,
								Input: map[string]any{
									"document_id": "doc-77",
								},
								Reason:     "Investigate the requested internal document directly",
								Confidence: "high",
							},
						},
					}, nil
				},
			},
			CapabilityResolver: agentresolve.NewRegistryResolver(registry),
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-doc-mixed", "why did document doc-77 fail", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Snapshot.Plan.Steps) != 2 {
		t.Fatalf("expected two-step mixed plan, got %+v", result.Snapshot.Plan.Steps)
	}
	if result.Snapshot.Plan.Steps[0].CapabilityName != agentcapability.NameDocumentInvestigation || result.Snapshot.Plan.Steps[1].CapabilityName != agentcapability.NameExternalEvidenceCollect {
		t.Fatalf("expected mixed document/external evidence plan, got %+v", result.Snapshot.Plan.Steps)
	}
	if result.Snapshot.Plan.Steps[0].Status != agentstate.PlanStepStatusCompleted || result.Snapshot.Plan.Steps[1].Status != agentstate.PlanStepStatusCompleted {
		t.Fatalf("expected both mixed steps to complete, got %+v", result.Snapshot.Plan.Steps)
	}
	if len(searchedQueries) != 1 || !strings.Contains(searchedQueries[0], "why did document doc-77 fail") || !strings.Contains(searchedQueries[0], "document=doc-77") {
		t.Fatalf("expected external evidence query to include request and prior structured context, got %+v", searchedQueries)
	}
	if result.Snapshot.Plan.LastStepResult.CapabilityName != agentcapability.NameExternalEvidenceCollect {
		t.Fatalf("expected last step result from external evidence collect, got %+v", result.Snapshot.Plan.LastStepResult)
	}
}

func TestCompile_RetriesStepBeforeSuccess(t *testing.T) {
	searchCapability, _ := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{Query: query}, nil
		},
	})
	var fetchCalls int
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchCalls++
			if fetchCalls == 1 {
				return agentfetch.Output{
					URLs:          append([]string(nil), urls...),
					Summary:       "first fetch produced no readable evidence",
					Degraded:      true,
					DegradeReason: "temporary_fetch_failure",
					ErrorMessage:  "temporary fetch failure",
					Pages:         []agentfetch.PageResult{{URL: urls[0], ErrorMessage: "temporary fetch failure"}},
				}, nil
			}
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "retry succeeded with readable evidence",
				Pages:   []agentfetch.PageResult{{URL: urls[0], Text: "retry succeeded with readable evidence"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}

	registry := registerHandles(t, searchCapability, fetchCapability)
	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registry),
		Synthesizer: patternPlanSynthesizer{
			result: PlanSynthesisResult{
				Plan: agentstate.PlanState{
					Goal:   "retry fetch",
					PlanID: "plan_retry",
					Status: agentstate.PlanStatusActive,
					Steps: []agentstate.PlanStep{
						{
							StepID:           "step_retry_fetch",
							Title:            "Retry fetch until evidence is readable",
							CapabilityName:   agentcapability.NameWebFetch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleFetch,
							CapabilityInput: map[string]any{
								"urls": []string{"https://example.com/retry"},
							},
							Produces:         []string{artifactKindFetchResults, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      2,
							Status:           agentstate.PlanStepStatusPending,
						},
					},
				},
				Reasoning: "built retry test plan",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-retry-step", "retry fetch", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fetchCalls != 2 {
		t.Fatalf("expected fetch step to retry once, got %d calls", fetchCalls)
	}
	if result.Snapshot.Plan.Steps[0].AttemptCount != 2 {
		t.Fatalf("expected attempt count to reach two, got %+v", result.Snapshot.Plan.Steps[0])
	}
	if result.Snapshot.Plan.Steps[0].Status != agentstate.PlanStepStatusCompleted {
		t.Fatalf("expected retrying step to complete, got %+v", result.Snapshot.Plan.Steps[0])
	}
	if !strings.Contains(result.Snapshot.Answer.Final, "retry succeeded with readable evidence") {
		t.Fatalf("expected final answer from retried success, got %+v", result.Snapshot.Answer)
	}
}

func TestCompile_OptionalStepSkipsAndContinues(t *testing.T) {
	searchCapability, _ := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{Query: query}, nil
		},
	})
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				URLs:          append([]string(nil), urls...),
				Summary:       "optional fetch produced no readable evidence",
				Degraded:      true,
				DegradeReason: "optional_fetch_failure",
				ErrorMessage:  "optional fetch failure",
				Pages:         []agentfetch.PageResult{{URL: urls[0], ErrorMessage: "optional fetch failure"}},
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
				PipelineID:  "pipe-optional",
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
							ID:     "task-optional",
							Status: ingestiondomain.TaskStatusFailed,
						},
						IngestionNodes: []ingestiondomain.TaskNode{
							{
								NodeID:       "indexer",
								Status:       ingestiondomain.TaskStatusFailed,
								ErrorMessage: "vector store timeout",
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
		Synthesizer: patternPlanSynthesizer{
			result: PlanSynthesisResult{
				Plan: agentstate.PlanState{
					Goal:   "skip optional fetch and continue",
					PlanID: "plan_optional",
					Status: agentstate.PlanStatusActive,
					Steps: []agentstate.PlanStep{
						{
							StepID:           "step_optional_fetch",
							Title:            "Optional fetch",
							CapabilityName:   agentcapability.NameWebFetch,
							CapabilityKind:   agentcapability.KindTool,
							CapabilityFamily: agentcapability.FamilyExternalEvidence,
							CapabilityRole:   agentcapability.RoleFetch,
							CapabilityInput: map[string]any{
								"urls": []string{"https://example.com/optional"},
							},
							Produces:         []string{artifactKindFetchResults, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							Optional:         true,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
						{
							StepID:           "step_required_document",
							Title:            "Required document investigation",
							CapabilityName:   agentcapability.NameDocumentInvestigation,
							CapabilityKind:   agentcapability.KindWorkflow,
							CapabilityFamily: agentcapability.FamilyDocumentInvestigation,
							CapabilityRole:   agentcapability.RoleInvestigateDocument,
							CapabilityInput: map[string]any{
								"document_id": "doc-optional",
							},
							Produces:         []string{artifactKindStructuredOutput, artifactKindEvidenceRefs},
							CompletionPolicy: completionPolicyEvidence,
							FailurePolicy:    failurePolicyDegrade,
							MaxAttempts:      1,
							Status:           agentstate.PlanStepStatusPending,
						},
					},
				},
				Reasoning: "built optional-step test plan",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-plan-optional-step", "optional fetch then diagnose", agentstate.OutputModeFinalAnswer))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Plan.Steps[0].Status != agentstate.PlanStepStatusSkipped {
		t.Fatalf("expected optional step to be skipped, got %+v", result.Snapshot.Plan.Steps[0])
	}
	if result.Snapshot.Plan.Steps[1].Status != agentstate.PlanStepStatusCompleted {
		t.Fatalf("expected required follow-up step to complete, got %+v", result.Snapshot.Plan.Steps[1])
	}
	if !strings.Contains(result.Snapshot.Answer.Final, "document ingestion failed at node indexer") {
		t.Fatalf("expected final answer from continued required step, got %+v", result.Snapshot.Answer)
	}
}

type stubSearchInvoker struct {
	search func(ctx context.Context, query string) (agentsearch.SearchOutput, error)
}

func (s stubSearchInvoker) Search(ctx context.Context, query string) (agentsearch.SearchOutput, error) {
	return s.search(ctx, query)
}

type stubFetchInvoker struct {
	fetch func(ctx context.Context, urls []string) (agentfetch.Output, error)
}

func (s stubFetchInvoker) Fetch(ctx context.Context, urls []string) (agentfetch.Output, error) {
	return s.fetch(ctx, urls)
}

type stubSelector struct {
	selectFn func(ctx context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error)
}

func (s stubSelector) Select(ctx context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
	return s.selectFn(ctx, input)
}

type stubDocumentInvestigator struct {
	getFn      func(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error)
	pageLogsFn func(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error)
}

type patternPlanSynthesizer struct {
	result PlanSynthesisResult
	err    error
}

func (s patternPlanSynthesizer) Synthesize(context.Context, PlanSynthesisInput) (PlanSynthesisResult, error) {
	return s.result, s.err
}

func (s stubDocumentInvestigator) Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return s.getFn(ctx, input)
}

func (s stubDocumentInvestigator) PageChunkLogs(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
	return s.pageLogsFn(ctx, input)
}

func registerHandles(t *testing.T, handles ...agentcapability.Handle) *agentcapability.Registry {
	t.Helper()
	registry := agentcapability.NewRegistry()
	for _, handle := range handles {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}
	return registry
}

func defaultAssembly(registry *agentcapability.Registry) agentpattern.AssemblyContext {
	return agentpattern.AssemblyContext{
		Registry: registry,
		Bindings: agentcapability.RoleBindings{
			agentcapability.RoleSearch: agentcapability.NameWebSearch,
			agentcapability.RoleFetch:  agentcapability.NameWebFetch,
		},
	}
}

func newSession(sessionID, question, outputMode string) *agentruntime.RuntimeSession {
	options := agentstate.RuntimeOptions{
		MaxIterations:  4,
		AllowWebSearch: true,
		OutputMode:     outputMode,
	}
	return &agentruntime.RuntimeSession{
		SessionID: sessionID,
		Request: agentruntime.RequestEnvelope{
			Question: question,
			Options:  options,
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question:       question,
				RuntimeOptions: options,
			},
			Execution: agentstate.ExecutionState{
				MaxIterations: options.MaxIterations,
			},
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt: time.Now(),
		},
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
