package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
)

func TestPlanExecuteService_MixedAnswerPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>external corroboration confirms an indexer vector timeout</body></html>`))
	}))
	defer server.Close()

	var searchQueries []string
	service := newPlanExecuteMixedService(t, planExecuteMixedServiceOptions{
		outputMode: agentstate.OutputModeFinalAnswer,
		client:     server.Client(),
		provider: stubRuntimeProvider{
			search: func(query string) ([]searchprovider.SearchResult, error) {
				searchQueries = append(searchQueries, query)
				return []searchprovider.SearchResult{
					{
						Title:   "Corroborating Postmortem",
						URL:     server.URL,
						Snippet: "confirms vector timeout",
						Domain:  "example.com",
					},
				}, nil
			},
		},
	})

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "why did document doc-mixed fail",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", result.Outcome)
	}
	if result.Response.Degraded {
		t.Fatalf("expected non-degraded response, got %+v", result.Response)
	}
	if len(result.Response.Pages) != 1 {
		t.Fatalf("expected one fetched page from mixed path, got %+v", result.Response.Pages)
	}
	if !strings.Contains(result.Response.CombinedText, "indexer vector timeout") {
		t.Fatalf("expected combined text to include fetched corroboration, got %+v", result.Response)
	}
	if len(searchQueries) != 1 {
		t.Fatalf("expected exactly one provider search query, got %+v", searchQueries)
	}
	if !strings.Contains(searchQueries[0], "document=doc-mixed") || !strings.Contains(searchQueries[0], "why did document doc-mixed fail") {
		t.Fatalf("expected mixed plan query to include request and document artifact context, got %+v", searchQueries)
	}
}

func TestPlanExecuteService_MixedHandoffPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>external corroboration confirms an indexer vector timeout</body></html>`))
	}))
	defer server.Close()

	var searchQueries []string
	service := newPlanExecuteMixedService(t, planExecuteMixedServiceOptions{
		outputMode: agentstate.OutputModeHandoff,
		client:     server.Client(),
		provider: stubRuntimeProvider{
			search: func(query string) ([]searchprovider.SearchResult, error) {
				searchQueries = append(searchQueries, query)
				return []searchprovider.SearchResult{
					{
						Title:   "Corroborating Postmortem",
						URL:     server.URL,
						Snippet: "confirms vector timeout",
						Domain:  "example.com",
					},
				}, nil
			},
		},
	})

	result, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "why did document doc-mixed fail",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", result.Outcome)
	}
	if !result.Handoff.Used || result.Handoff.Degraded {
		t.Fatalf("expected non-degraded handoff, got %+v", result.Handoff)
	}
	if len(result.Handoff.EvidenceBundle.Pages) != 1 {
		t.Fatalf("expected handoff to include one fetched page, got %+v", result.Handoff.EvidenceBundle)
	}
	if !containsAcceptedEvidenceSource(result.Handoff.EvidenceBundle.AcceptedEvidence, "document_investigation") {
		t.Fatalf("expected handoff to include document investigation evidence, got %+v", result.Handoff.EvidenceBundle.AcceptedEvidence)
	}
	if !containsAcceptedEvidenceSource(result.Handoff.EvidenceBundle.AcceptedEvidence, "fetch") {
		t.Fatalf("expected handoff to include external fetch evidence, got %+v", result.Handoff.EvidenceBundle.AcceptedEvidence)
	}
	if len(searchQueries) != 1 || !strings.Contains(searchQueries[0], "document=doc-mixed") {
		t.Fatalf("expected handoff mixed query to include document artifact context, got %+v", searchQueries)
	}
}

func TestPlanExecuteService_DocumentApprovalResumeFlow(t *testing.T) {
	store := newRecordingSessionStore()
	service := newPlanExecuteDocumentApprovalService(t, agentstate.OutputModeFinalAnswer, store)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "why did document doc-approval fail",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}
	if initial.Outcome.Approval.CurrentStepID != "step_selected_capability" {
		t.Fatalf("expected selected capability step approval, got %+v", initial.Outcome.Approval)
	}
	if initial.Outcome.Approval.CapabilityName != agentcapability.NameDocumentInvestigation {
		t.Fatalf("expected document investigation approval, got %+v", initial.Outcome.Approval)
	}
	if initial.Outcome.Approval.CurrentStepTitle != "Investigate failed document before external corroboration" {
		t.Fatalf("expected stable document step title, got %+v", initial.Outcome.Approval)
	}

	resumed, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "looks safe",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}
	if resumed.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed outcome after approval, got %+v", resumed.Outcome)
	}
	if resumed.Response.Degraded {
		t.Fatalf("expected approved run to remain non-degraded, got %+v", resumed.Response)
	}
	if !strings.Contains(resumed.Response.Summary, "document ingestion failed at node indexer") {
		t.Fatalf("expected resumed summary to surface document diagnosis evidence, got %+v", resumed.Response)
	}
	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestPlanExecuteService_DocumentApprovalDuplicateResumeReturnsNotFound(t *testing.T) {
	testCases := []struct {
		name           string
		decision       string
		expectedStatus string
	}{
		{
			name:           "approved",
			decision:       ApprovalDecisionApproved,
			expectedStatus: RunStatusCompleted,
		},
		{
			name:           "rejected",
			decision:       ApprovalDecisionRejected,
			expectedStatus: RunStatusDegraded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := newRecordingSessionStore()
			service := newPlanExecuteDocumentApprovalService(t, agentstate.OutputModeFinalAnswer, store)

			initial, err := service.RunDetailed(context.Background(), Request{
				Question: "why did document doc-approval fail",
				Options: RequestOptions{
					OutputMode: agentstate.OutputModeFinalAnswer,
				},
			})
			if err != nil {
				t.Fatalf("RunDetailed() error = %v", err)
			}
			if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
				t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
			}

			firstResume, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
				CheckpointID: initial.Outcome.CheckpointID,
				Decision:     tc.decision,
			})
			if err != nil {
				t.Fatalf("ResumeAfterApproval(first) error = %v", err)
			}
			if firstResume.Outcome.Status != tc.expectedStatus {
				t.Fatalf("expected first resume status %q, got %+v", tc.expectedStatus, firstResume.Outcome)
			}

			_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
				CheckpointID: initial.Outcome.CheckpointID,
				Decision:     tc.decision,
			})
			if err == nil {
				t.Fatal("expected duplicate resume to fail")
			}
			if ServiceErrorCode(err) != ErrorCodeApprovalSessionNotFound {
				t.Fatalf("expected approval session not found, got %q (%v)", ServiceErrorCode(err), err)
			}
			assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
		})
	}
}

type planExecuteMixedServiceOptions struct {
	outputMode string
	client     *http.Client
	provider   searchprovider.SearchProvider
}

func newPlanExecuteMixedService(t *testing.T, opts planExecuteMixedServiceOptions) *Service {
	t.Helper()

	service, err := NewService(ServiceOptions{
		Provider:             opts.provider,
		FetchService:         agentfetch.NewService(opts.client),
		OutputMode:           opts.outputMode,
		Pattern:              PatternPlanExecute,
		DocumentInvestigator: newPlanExecuteServiceDocumentInvestigator(),
		CapabilitySelector: planExecuteServiceSelector{
			selectFn: func(_ context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
				if len(input.Capabilities) == 0 {
					t.Fatal("expected capability catalog to be populated")
				}
				return selectcapability.SelectionOutput{
					Selections: []selectcapability.CapabilitySelection{
						{
							Name:       agentcapability.NameDocumentInvestigation,
							Family:     agentcapability.FamilyDocumentInvestigation,
							Role:       agentcapability.RoleInvestigateDocument,
							Input:      map[string]any{"document_id": "doc-mixed"},
							Reason:     "Investigate failed document before external corroboration",
							Confidence: "high",
						},
					},
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func newPlanExecuteDocumentApprovalService(t *testing.T, outputMode string, sessionStore agentruntime.SessionStore) *Service {
	t.Helper()

	searchHandle, err := agentsearch.NewCapability(contractSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{
				Query:        query,
				Provider:     "contract-provider",
				ResultCount:  1,
				AllowedCount: 1,
				URLs:         []string{"https://approval.example/doc"},
				Results: []agentsearch.SearchResultItem{
					{
						Title:   "Unused Search Result",
						URL:     "https://approval.example/doc",
						Snippet: "unused during document-only approval flow",
						Domain:  "approval.example",
					},
				},
				Summary: "unused search evidence",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}

	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "unused fetch evidence",
				Pages:   []agentfetch.PageResult{{URL: "https://approval.example/doc", Text: "unused fetch evidence"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	documentHandle, err := agentdocumentinvestigation.NewCapability(
		newPlanExecuteServiceDocumentInvestigator(),
		agentcapability.WithRequiresApproval(true),
	)
	if err != nil {
		t.Fatalf("document_investigation.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchHandle, fetchHandle, documentHandle} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("registry.Register(%q) error = %v", handle.Spec().Name, err)
		}
	}

	bindings := agentcapability.RoleBindings{
		agentcapability.RoleSearch: agentcapability.NameWebSearch,
		agentcapability.RoleFetch:  agentcapability.NameWebFetch,
	}
	checkpointStore := agentkernel.NewMemoryCheckpointStore()
	runner, err := compileRunner(context.Background(), PatternPlanExecute, registry, bindings, agentpattern.RuntimeConfig{
		OutputMode:               outputMode,
		CapabilityCatalogBuilder: agentcatalog.NewBuilder(),
		CapabilitySelector: planExecuteServiceSelector{
			selectFn: func(_ context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
				if len(input.Capabilities) == 0 {
					t.Fatal("expected capability catalog to be populated")
				}
				return selectcapability.SelectionOutput{
					Selections: []selectcapability.CapabilitySelection{
						{
							Name:       agentcapability.NameDocumentInvestigation,
							Family:     agentcapability.FamilyDocumentInvestigation,
							Role:       agentcapability.RoleInvestigateDocument,
							Input:      map[string]any{"document_id": "doc-approval"},
							Reason:     "Investigate failed document before external corroboration",
							Confidence: "high",
						},
					},
				}, nil
			},
		},
		CapabilityResolver:   agentresolve.NewRegistryResolver(registry),
		ApprovalSessionStore: sessionStore,
		Kernel: agentkernel.BuilderConfig{
			GraphName:       "agent_service_plan_execute_document_approval_test",
			Reducer:         agentstate.DefaultReducer{},
			CheckpointStore: checkpointStore,
		},
	})
	if err != nil {
		t.Fatalf("compileRunner() error = %v", err)
	}

	return &Service{
		kernelRunner:  runner,
		runtimeEngine: agentruntime.NewEngine(runner),
		handoff:       buildHandoffBuilder(registry, bindings, PatternPlanExecute),
		registry:      registry,
		bindings:      bindings,
		sessionStore:  sessionStore,
		reducer:       agentstate.DefaultReducer{},
		maxIterations: 2,
		outputMode:    outputMode,
		pattern:       PatternPlanExecute,
		runtimeName:   runtimeNameForPattern(PatternPlanExecute),
	}
}

type planExecuteServiceSelector struct {
	selectFn func(context.Context, selectcapability.SelectionInput) (selectcapability.SelectionOutput, error)
}

func (s planExecuteServiceSelector) Select(ctx context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
	return s.selectFn(ctx, input)
}

type planExecuteServiceDocumentInvestigator struct{}

func newPlanExecuteServiceDocumentInvestigator() planExecuteServiceDocumentInvestigator {
	return planExecuteServiceDocumentInvestigator{}
}

func (planExecuteServiceDocumentInvestigator) Get(_ context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return knowledgedomain.KnowledgeDocument{
		ID:          input.DocumentID,
		Name:        "Incident Doc",
		Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
		ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
		PipelineID:  "pipe-service",
	}, nil
}

func (planExecuteServiceDocumentInvestigator) PageChunkLogs(_ context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
	return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
		Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
			{
				Log: knowledgedomain.KnowledgeDocumentChunkLog{
					Status:       knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
					ErrorMessage: "pipeline failed",
				},
				IngestionTask: &ingestiondomain.Task{
					ID:     "task-service",
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
}

func containsAcceptedEvidenceSource(items []AcceptedEvidenceItem, source string) bool {
	for _, item := range items {
		if strings.TrimSpace(item.Source) == strings.TrimSpace(source) {
			return true
		}
	}
	return false
}
