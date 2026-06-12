package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestAssembleCapabilitiesRegistersWorkflowSample(t *testing.T) {
	searchHandle, err := agentsearch.NewCapability(stubSearchService{})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchHandle, err := agentfetch.NewCapability(stubFetchService{})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}
	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchHandle, fetchHandle} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	assembledRegistry, bindings, err := assembleCapabilities(&agentsearch.Service{}, &agentfetch.Service{}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("assembleCapabilities() error = %v", err)
	}
	if _, ok := assembledRegistry.Spec(agentcapability.NameExternalEvidenceCollect); !ok {
		t.Fatalf("expected workflow sample capability to be registered, got %+v", assembledRegistry.Specs())
	}
	if bindings.Resolve(agentcapability.RoleSearch) != agentcapability.NameWebSearch || bindings.Resolve(agentcapability.RoleFetch) != agentcapability.NameWebFetch {
		t.Fatalf("expected default reactive bindings to remain search/fetch, got %+v", bindings)
	}
	_ = registry
}

func TestAssembleCapabilitiesRegistersDocumentInvestigationWhenProvided(t *testing.T) {
	assembledRegistry, _, err := assembleCapabilities(&agentsearch.Service{}, &agentfetch.Service{}, stubDocumentInvestigator{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("assembleCapabilities() error = %v", err)
	}
	spec, ok := assembledRegistry.Spec(agentcapability.NameDocumentInvestigation)
	if !ok {
		t.Fatalf("expected document investigation capability to be registered, got %+v", assembledRegistry.Specs())
	}
	if spec.Kind != agentcapability.KindWorkflow || spec.Family != agentcapability.FamilyDocumentInvestigation {
		t.Fatalf("unexpected document investigation spec: %+v", spec)
	}
}

func TestAssembleCapabilitiesRegistersThinkByDefault(t *testing.T) {
	assembledRegistry, _, err := assembleCapabilities(&agentsearch.Service{}, &agentfetch.Service{}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("assembleCapabilities() error = %v", err)
	}
	if _, ok := assembledRegistry.Spec(agentcapability.NameThink); !ok {
		t.Fatalf("expected think capability to be registered, got %+v", assembledRegistry.Specs())
	}
}

func TestAssembleCapabilitiesRegistersDiscoveryWhenProvided(t *testing.T) {
	assembledRegistry, _, err := assembleCapabilities(
		&agentsearch.Service{},
		&agentfetch.Service{},
		nil,
		nil,
		&stubKnowledgeDiscoverer{},
		nil,
	)
	if err != nil {
		t.Fatalf("assembleCapabilities() error = %v", err)
	}
	if _, ok := assembledRegistry.Spec(agentcapability.NameKnowledgeDiscovery); !ok {
		t.Fatalf("expected knowledge discovery capability to be registered, got %+v", assembledRegistry.Specs())
	}
}

func TestAssembleCapabilitiesRegistersMemoryRecallWhenProvided(t *testing.T) {
	assembledRegistry, _, err := assembleCapabilities(
		&agentsearch.Service{},
		&agentfetch.Service{},
		nil,
		nil,
		nil,
		&stubMemoryRecaller{},
	)
	if err != nil {
		t.Fatalf("assembleCapabilities() error = %v", err)
	}
	if _, ok := assembledRegistry.Spec(agentcapability.NameMemoryRecall); !ok {
		t.Fatalf("expected memory recall capability to be registered, got %+v", assembledRegistry.Specs())
	}
}

func TestAssembleCapabilitiesSkipsOptionalWorkflowWithoutDependency(t *testing.T) {
	assembledRegistry, _, err := assembleCapabilities(&agentsearch.Service{}, &agentfetch.Service{}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("assembleCapabilities() error = %v", err)
	}
	if _, ok := assembledRegistry.Spec(agentcapability.NameDocumentInvestigation); ok {
		t.Fatalf("expected optional workflow capability to be skipped, got %+v", assembledRegistry.Specs())
	}
	if _, ok := assembledRegistry.Spec(agentcapability.NameExternalEvidenceCollect); !ok {
		t.Fatalf("expected default external evidence workflow to remain registered, got %+v", assembledRegistry.Specs())
	}
}

func TestNewService_PlanExecutePatternRunDetailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>plan execute evidence from local test server</body></html>`))
	}))
	defer server.Close()

	service, err := NewService(ServiceOptions{
		Provider: stubRuntimeProvider{
			search: func(query string) ([]searchprovider.SearchResult, error) {
				if query != "plan execute flow" {
					t.Fatalf("unexpected query: %q", query)
				}
				return []searchprovider.SearchResult{
					{
						Title:   "Plan Execute",
						URL:     server.URL,
						Snippet: "plan execute evidence",
						Domain:  "example.com",
					},
				}, nil
			},
		},
		FetchService: agentfetch.NewService(server.Client()),
		OutputMode:   agentstate.OutputModeFinalAnswer,
		Pattern:      PatternPlanExecute,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "plan execute flow",
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
	if service.pattern != PatternPlanExecute {
		t.Fatalf("expected service to record plan-execute pattern, got %q", service.pattern)
	}
	if service.runtimeName != runtimeNameForPattern(PatternPlanExecute) {
		t.Fatalf("expected plan-execute runtime name, got %q", service.runtimeName)
	}
	if !strings.Contains(result.Response.Summary, "plan execute evidence from local test server") {
		t.Fatalf("expected plan-execute response to use local fetched evidence, got %+v", result.Response)
	}
}

func TestNewService_DefaultPatternUsesPlanExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>default plan execute evidence</body></html>`))
	}))
	defer server.Close()

	service, err := NewService(ServiceOptions{
		Provider: stubRuntimeProvider{
			search: func(query string) ([]searchprovider.SearchResult, error) {
				if query != "default pattern flow" {
					t.Fatalf("unexpected query: %q", query)
				}
				return []searchprovider.SearchResult{
					{
						Title:   "Default Plan Execute",
						URL:     server.URL,
						Snippet: "default plan execute evidence",
						Domain:  "example.com",
					},
				}, nil
			},
		},
		FetchService: agentfetch.NewService(server.Client()),
		OutputMode:   agentstate.OutputModeFinalAnswer,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if service.pattern != PatternPlanExecute {
		t.Fatalf("expected empty pattern to default to plan-execute, got %q", service.pattern)
	}
	if service.runtimeName != runtimeNameForPattern(PatternPlanExecute) {
		t.Fatalf("expected default runtime to be plan-execute, got %q", service.runtimeName)
	}

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "default pattern flow",
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
	if !strings.Contains(result.Response.Summary, "default plan execute evidence") {
		t.Fatalf("expected default plan-execute response to use fetched evidence, got %+v", result.Response)
	}
}

func TestServiceRunDetailed_PlanExecuteApprovalIncludesStepContext(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "plan execute approval flow" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Plan Execute Approval",
					URL:     "https://plan.example/doc",
					Snippet: "requires plan approval",
					Domain:  "plan.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary: "plan execute approved content",
				Pages: []agentfetch.PageResult{
					{URL: "https://plan.example/doc", Text: "plan execute approved evidence"},
				},
			}, nil
		},
	}, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentServiceWithPattern(t, PatternPlanExecute, searchHandle, fetchHandle)
	result, err := service.RunDetailed(context.Background(), Request{
		Question: "plan execute approval flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusAwaitingApproval || result.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", result.Outcome)
	}
	if result.Outcome.Approval.Node != "approval" || result.Outcome.Approval.Trigger != "approval_gate" {
		t.Fatalf("expected plan-execute approval gate outcome, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.CurrentStepID != "step_fetch" || result.Outcome.Approval.CurrentStepTitle != "Fetch the best available source" {
		t.Fatalf("expected plan step context in approval outcome, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.SearchQuery != "plan execute approval flow" {
		t.Fatalf("expected plan-execute approval to expose search query, got %+v", result.Outcome.Approval)
	}
	if len(result.Outcome.Approval.CandidateURLs) != 1 || result.Outcome.Approval.CandidateURLs[0] != "https://plan.example/doc" {
		t.Fatalf("expected plan-execute approval candidate url, got %+v", result.Outcome.Approval)
	}
}

func TestPatternValidation_CommonAnswerPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>shared evidence for both runtime patterns</body></html>`))
	}))
	defer server.Close()

	provider := stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "shared answer validation" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Shared Evidence",
					URL:     server.URL,
					Snippet: "shared evidence",
					Domain:  "example.com",
				},
			}, nil
		},
	}

	for _, pattern := range []string{PatternReactive, PatternPlanExecute} {
		t.Run(pattern, func(t *testing.T) {
			service := newPatternValidationService(t, pattern, provider, server.Client(), agentstate.OutputModeFinalAnswer)
			result, err := service.RunDetailed(context.Background(), Request{
				Question: "shared answer validation",
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
			if !strings.Contains(result.Response.Summary, "shared evidence") {
				t.Fatalf("expected summary to use fetched evidence, got %+v", result.Response)
			}
			if !strings.Contains(result.Response.CombinedText, "shared evidence for both runtime patterns") {
				t.Fatalf("expected combined text to include fetched page body, got %+v", result.Response)
			}
		})
	}
}

func TestPatternValidation_CommonHandoffPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>handoff evidence for both runtime patterns</body></html>`))
	}))
	defer server.Close()

	provider := stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "shared handoff validation" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Shared Handoff Evidence",
					URL:     server.URL,
					Snippet: "handoff evidence",
					Domain:  "example.com",
				},
			}, nil
		},
	}

	for _, pattern := range []string{PatternReactive, PatternPlanExecute} {
		t.Run(pattern, func(t *testing.T) {
			service := newPatternValidationService(t, pattern, provider, server.Client(), agentstate.OutputModeHandoff)
			result, err := service.RunHandoffDetailed(context.Background(), Request{
				Question: "shared handoff validation",
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
				t.Fatalf("expected non-degraded handoff bundle, got %+v", result.Handoff)
			}
			if len(result.Handoff.EvidenceBundle.Pages) == 0 || len(result.Handoff.EvidenceBundle.AcceptedEvidence) == 0 {
				t.Fatalf("expected fetched pages and accepted evidence, got %+v", result.Handoff.EvidenceBundle)
			}
		})
	}
}

func TestPatternValidation_CommonDegradePath(t *testing.T) {
	provider := stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "shared degrade validation" {
				t.Fatalf("unexpected query: %q", query)
			}
			return nil, nil
		},
	}

	for _, pattern := range []string{PatternReactive, PatternPlanExecute} {
		t.Run(pattern, func(t *testing.T) {
			service := newPatternValidationService(t, pattern, provider, nil, agentstate.OutputModeFinalAnswer)
			result, err := service.RunDetailed(context.Background(), Request{
				Question: "shared degrade validation",
				Options: RequestOptions{
					OutputMode: agentstate.OutputModeFinalAnswer,
				},
			})
			if err != nil {
				t.Fatalf("RunDetailed() error = %v", err)
			}
			if result.Outcome.Status != RunStatusDegraded {
				t.Fatalf("expected degraded outcome, got %+v", result.Outcome)
			}
			if !result.Response.Degraded || strings.TrimSpace(result.Response.DegradeReason) == "" {
				t.Fatalf("expected degrade metadata, got %+v", result.Response)
			}
		})
	}
}

func TestPatternValidation_PlanExecuteReplanCapability(t *testing.T) {
	serverOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporary fetch failure", http.StatusBadGateway)
	}))
	defer serverOne.Close()
	serverTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>second source recovered the answer</body></html>`))
	}))
	defer serverTwo.Close()

	service := newPatternValidationService(t, PatternPlanExecute, stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "plan replan validation" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Primary",
					URL:     serverOne.URL,
					Snippet: "first source",
					Domain:  "example.com",
				},
				{
					Title:   "Secondary",
					URL:     serverTwo.URL,
					Snippet: "second source",
					Domain:  "example.com",
				},
			}, nil
		},
	}, serverTwo.Client(), agentstate.OutputModeFinalAnswer)

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "plan replan validation",
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
	if !strings.Contains(result.Response.Summary, "second source recovered the answer") {
		t.Fatalf("expected plan-execute replan path to recover from second source, got %+v", result.Response)
	}
}

func newPatternValidationService(t *testing.T, pattern string, provider searchprovider.SearchProvider, client *http.Client, outputMode string) *Service {
	t.Helper()

	fetchService := agentfetch.NewService(client)
	service, err := NewService(ServiceOptions{
		Provider:     provider,
		FetchService: fetchService,
		OutputMode:   outputMode,
		Pattern:      pattern,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}
