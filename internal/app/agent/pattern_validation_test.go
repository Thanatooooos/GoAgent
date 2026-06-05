package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
)

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
