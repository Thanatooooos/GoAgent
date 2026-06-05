package reactive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentplanner "local/rag-project/internal/app/agent/planner"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

type stubProvider struct {
	name   string
	search func(query string) ([]searchprovider.SearchResult, error)
}

func (p stubProvider) Search(query string) ([]searchprovider.SearchResult, error) {
	return p.search(query)
}

func (p stubProvider) ProviderName() string {
	return p.name
}

func TestCompile_RunAnswerPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Go generics let you write reusable functions with type parameters.</body></html>`))
	}))
	defer server.Close()

	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "golang generics" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Go Docs",
					URL:     server.URL,
					Snippet: "Go generics introduction.",
					Domain:  "go.dev",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(server.Client())
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-answer", "golang generics", 2))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Answer.Final == "" {
		t.Fatal("expected final answer to be populated")
	}
	if !strings.Contains(result.Snapshot.Answer.Final, "type parameters") {
		t.Fatalf("expected final answer to use fetched evidence, got %q", result.Snapshot.Answer.Final)
	}
	if result.Snapshot.Answer.DegradeReason != "" {
		t.Fatalf("expected answer path, got degrade reason %q", result.Snapshot.Answer.DegradeReason)
	}
	if !result.Snapshot.Evidence.Sufficient {
		t.Fatal("expected evidence to be sufficient")
	}
	if result.Snapshot.Evidence.NewItemsThisRound != 1 {
		t.Fatalf("expected new evidence count for answer path, got %+v", result.Snapshot.Evidence)
	}
	if result.Snapshot.Execution.CurrentNode != "answer" {
		t.Fatalf("expected answer as final execution node, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.Iteration != 1 {
		t.Fatalf("expected one reactive iteration, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchAnswer || result.Snapshot.Execution.LastBranchReason != "fetched_readable_evidence" {
		t.Fatalf("expected answer branch tracking, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastProgressKind != progressEvidenceGained {
		t.Fatalf("expected evidence-gained progress kind, got %+v", result.Snapshot.Execution)
	}
	if !containsAction(result.Snapshot.Execution.ScheduledActions, "search") || !containsAction(result.Snapshot.Execution.ScheduledActions, "answer") {
		t.Fatalf("expected scheduled action history to include search and answer, got %+v", result.Snapshot.Execution)
	}
	if !containsAction(result.Snapshot.Execution.CompletedActions, "prepare") || !containsAction(result.Snapshot.Execution.CompletedActions, "observe") {
		t.Fatalf("expected completed action history to include prepare and observe, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.Interrupted {
		t.Fatalf("expected answer path to finish non-interrupted, got %+v", result.Snapshot.Execution)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeCapabilityStart) {
		t.Fatalf("expected capability_start event, got %+v", result.Journal)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeCapabilityResult) {
		t.Fatalf("expected capability_result event, got %+v", result.Journal)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeBranchSelected) {
		t.Fatalf("expected branch_selected event, got %+v", result.Journal)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeAnswerFinalized) {
		t.Fatalf("expected answer_finalized event, got %+v", result.Journal)
	}
}

func TestCompile_ResolvesBindingsByRoleWhenUnique(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "unique role binding" {
				t.Fatalf("unexpected query: %q", query)
			}
			return nil, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(nil)
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: agentpattern.AssemblyContext{
			Registry: registry,
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if _, err := runner.Run(context.Background(), newSession("sess-role-default", "unique role binding", 1)); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestCompile_RequiresExplicitBindingWhenRoleHasMultipleCandidates(t *testing.T) {
	searchCapability, err := agentsearch.NewCapability(&stubSearchInvokerPattern{})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	altSearchCapability, err := newNamedSearchCapability("web_search_alt")
	if err != nil {
		t.Fatalf("newNamedSearchCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(nil)
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() primary search error = %v", err)
	}
	if err := registry.Register(altSearchCapability); err != nil {
		t.Fatalf("Register() alternate search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	if _, err := Compile(context.Background(), Config{
		Assembly: agentpattern.AssemblyContext{
			Registry: registry,
		},
	}); err == nil {
		t.Fatal("expected explicit binding error when multiple search candidates are registered")
	}
}

func TestCompile_RunHandoffPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Go generics let you write reusable functions with type parameters.</body></html>`))
	}))
	defer server.Close()

	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "golang generics" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Go Docs",
					URL:     server.URL,
					Snippet: "Go generics introduction.",
					Domain:  "go.dev",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(server.Client())
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSessionWithOutputMode("sess-handoff", "golang generics", 2, agentstate.OutputModeHandoff))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Execution.CurrentNode != "handoff" {
		t.Fatalf("expected handoff as final execution node, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchHandoff || result.Snapshot.Execution.LastBranchReason != "fetched_readable_evidence" {
		t.Fatalf("expected handoff branch tracking, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Answer.Final != "" {
		t.Fatalf("expected handoff path to skip final answer generation, got %+v", result.Snapshot.Answer)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeHandoffFinalized) {
		t.Fatalf("expected handoff_finalized event, got %+v", result.Journal)
	}
}

func TestCompile_RunDegradePath(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "no results please" {
				t.Fatalf("unexpected query: %q", query)
			}
			return nil, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(nil)
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-degrade", "no results please", 2))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Answer.DegradeReason == "" {
		t.Fatal("expected degrade reason to be populated")
	}
	if result.Snapshot.Answer.Final == "" {
		t.Fatal("expected degraded final answer to be populated")
	}
	if result.Snapshot.Evidence.Sufficient {
		t.Fatal("expected insufficient evidence on degrade path")
	}
	if result.Snapshot.Answer.DegradeReason != "no_new_fetchable_urls" {
		t.Fatalf("expected no-new-fetchable-urls degrade reason, got %q", result.Snapshot.Answer.DegradeReason)
	}
	if result.Snapshot.Execution.CurrentNode != "degrade" {
		t.Fatalf("expected degrade as final execution node, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.Iteration != 1 {
		t.Fatalf("expected one reactive iteration on degrade path, got %+v", result.Snapshot.Execution)
	}
	if !containsAction(result.Snapshot.Execution.CompletedActions, "degrade") {
		t.Fatalf("expected degrade to be tracked as completed, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchDegrade || result.Snapshot.Execution.LastBranchReason != "no_new_fetchable_urls" {
		t.Fatalf("expected degrade branch tracking, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastProgressKind != progressNone {
		t.Fatalf("expected no-progress kind on direct degrade path, got %+v", result.Snapshot.Execution)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeCapabilitySkipped) {
		t.Fatalf("expected capability_skipped event, got %+v", result.Journal)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeDegraded) {
		t.Fatalf("expected degraded event, got %+v", result.Journal)
	}
}

func TestCompile_RunContinueAfterRetryableSearchFailure(t *testing.T) {
	attempt := 0
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			attempt++
			if query != "retry search once" {
				t.Fatalf("unexpected query: %q", query)
			}
			if attempt == 1 {
				return nil, context.DeadlineExceeded
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Recovered",
					URL:     "https://example.com/recovered",
					Snippet: "search recovered",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary: "fetch produced readable evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "Search recovered and fetch produced readable evidence."},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchCapability, fetchCapability} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-search-retry", "retry search once", 3))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Execution.ContinueCount != 1 {
		t.Fatalf("expected one continue after retryable search failure, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "readable evidence") {
		t.Fatalf("expected recovered answer after search retry, got %+v", result.Snapshot.Answer)
	}
	if countBranchTarget(result.Journal, branchContinue) == 0 {
		t.Fatalf("expected continue branch after retryable search failure, got %+v", result.Journal)
	}
}

func TestCompile_RunContinueThenAnswerPath(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "retry fetch once" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Example",
					URL:     "https://example.com/retry",
					Snippet: "retryable content",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	retryOutput := retryFetchOutput()
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			return retryOutput()
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-continue-answer", "retry fetch once", 2))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Execution.Iteration != 2 {
		t.Fatalf("expected two reactive iterations, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.ContinueCount != 1 {
		t.Fatalf("expected one continue transition, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.CurrentNode != "answer" {
		t.Fatalf("expected answer after continue loop, got %+v", result.Snapshot.Execution)
	}
	if countBranchTarget(result.Journal, branchContinue) == 0 {
		t.Fatalf("expected continue branch to be selected at least once, got %+v", result.Journal)
	}
	if !strings.Contains(result.Snapshot.Answer.Final, "second attempt produced readable evidence") {
		t.Fatalf("expected second fetch attempt evidence in final answer, got %q", result.Snapshot.Answer.Final)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchAnswer || result.Snapshot.Execution.LastBranchReason != "fetched_readable_evidence" {
		t.Fatalf("expected final answer branch tracking after continue, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastProgressKind != progressEvidenceGained {
		t.Fatalf("expected evidence-gained progress kind after continue, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_UnknownIdempotencyBlocksFetchRetry(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "unknown idempotency" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Example",
					URL:     "https://example.com/idempotency",
					Snippet: "retryable content",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary:       "fetch failed with retryable external error",
				Degraded:      true,
				DegradeReason: "temporary_fetch_failure",
				ErrorMessage:  "temporary fetch failure",
				Pages: []agentfetch.PageResult{
					{URL: "https://example.com/idempotency", ErrorMessage: "temporary fetch failure"},
				},
			}, nil
		},
	}, agentcapability.WithIdempotency(agentcapability.IdempotencyUnknown))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchCapability, fetchCapability} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-unknown-idempotency", "unknown idempotency", 3))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Answer.DegradeReason != "retry_blocked_unknown_idempotency" {
		t.Fatalf("expected retry-blocked degrade reason, got %+v", result.Snapshot.Answer)
	}
	if result.Snapshot.Execution.ContinueCount != 0 {
		t.Fatalf("expected no automatic continue when idempotency is unknown, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_ResumeBlocksFetchRetryWhenCapabilityDoesNotSupportResume(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "resume unsafe retry" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Example",
					URL:     "https://example.com/resume",
					Snippet: "resume retry content",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary:       "fetch failed after resume",
				Degraded:      true,
				DegradeReason: "temporary_fetch_failure",
				ErrorMessage:  "temporary fetch failure",
				Pages: []agentfetch.PageResult{
					{URL: "https://example.com/resume", ErrorMessage: "temporary fetch failure"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchCapability, fetchCapability} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	session := newSession("sess-resume-unsafe", "resume unsafe retry", 3)
	session.Metadata.ResumeCount = 1

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), session)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Answer.DegradeReason != "resume_retry_not_supported" {
		t.Fatalf("expected resume retry to be blocked, got %+v", result.Snapshot.Answer)
	}
	if result.Snapshot.Execution.ContinueCount != 0 {
		t.Fatalf("expected no continue after resumed unsafe retry, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_RunContinueThenDegradeAtIterationLimit(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "retry until limit" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Example",
					URL:     "https://example.com/unreadable",
					Snippet: "unreadable content",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary:       "fetch returned no readable content",
				Degraded:      true,
				DegradeReason: "temporary_fetch_failure",
				ErrorMessage:  "temporary fetch failure",
				Pages: []agentfetch.PageResult{
					{
						URL:          "https://example.com/unreadable",
						ErrorMessage: "temporary fetch failure",
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-continue-degrade", "retry until limit", 2))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Execution.Iteration != 2 {
		t.Fatalf("expected two reactive iterations before degrade, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.ContinueCount != 1 {
		t.Fatalf("expected one continue before degrade, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.CurrentNode != "degrade" {
		t.Fatalf("expected degrade after exhausting iteration budget, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Answer.DegradeReason != "iteration_budget_exhausted" {
		t.Fatalf("expected iteration budget degrade reason, got %q", result.Snapshot.Answer.DegradeReason)
	}
	if countBranchTarget(result.Journal, branchContinue) == 0 {
		t.Fatalf("expected continue branch before degrade, got %+v", result.Journal)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchDegrade || result.Snapshot.Execution.LastBranchReason != "iteration_budget_exhausted" {
		t.Fatalf("expected iteration-budget degrade branch tracking, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastProgressKind != progressRetryableFetchFailed {
		t.Fatalf("expected retryable-failure progress kind before budget degrade, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_RunRetryableFailureThenNoProgressDegrade(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "retry until no progress" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Example",
					URL:     "https://example.com/no-progress",
					Snippet: "retry same failed source",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, _ []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary:       "fetch returned temporary failure",
				Degraded:      true,
				DegradeReason: "temporary_fetch_failure",
				ErrorMessage:  "temporary fetch failure",
				Pages: []agentfetch.PageResult{
					{
						URL:          "https://example.com/no-progress",
						ErrorMessage: "temporary fetch failure",
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-no-progress", "retry until no progress", 4))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Execution.Iteration != 3 {
		t.Fatalf("expected three observe rounds before no-progress degrade, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.ContinueCount != 2 {
		t.Fatalf("expected two continue transitions before no-progress degrade, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Answer.DegradeReason != "no_progress_across_rounds" {
		t.Fatalf("expected no-progress degrade reason, got %q", result.Snapshot.Answer.DegradeReason)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchDegrade || result.Snapshot.Execution.LastBranchReason != "no_progress_across_rounds" {
		t.Fatalf("expected no-progress branch tracking, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastProgressKind != progressRetryableFetchFailed {
		t.Fatalf("expected retryable-failure progress kind on no-progress degrade, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.ConsecutiveNoProgressRounds != 2 {
		t.Fatalf("expected two consecutive no-progress rounds, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_PlannerOverridesNextQueryAndFetchGuidance(t *testing.T) {
	var queries []string
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			queries = append(queries, query)
			switch query {
			case "refine me":
				return []searchprovider.SearchResult{
					{Title: "A", URL: "https://example.com/a", Snippet: "first", Domain: "example.com"},
					{Title: "B", URL: "https://example.com/b", Snippet: "second", Domain: "example.com"},
				}, nil
			case "refined query":
				return []searchprovider.SearchResult{
					{Title: "A", URL: "https://example.com/a", Snippet: "first", Domain: "example.com"},
					{Title: "B", URL: "https://example.com/b", Snippet: "second", Domain: "example.com"},
				}, nil
			default:
				t.Fatalf("unexpected query: %q", query)
				return nil, nil
			}
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}

	var fetchCalls [][]string
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchCalls = append(fetchCalls, append([]string(nil), urls...))
			if len(fetchCalls) == 1 {
				return agentfetch.Output{
					Summary:       "first fetch returned no readable content",
					Degraded:      true,
					DegradeReason: "temporary_fetch_failure",
					ErrorMessage:  "temporary fetch failure",
					Pages: []agentfetch.PageResult{
						{URL: "https://example.com/a", ErrorMessage: "temporary fetch failure"},
						{URL: "https://example.com/b", ErrorMessage: "temporary fetch failure"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "second fetch produced readable evidence",
				Pages: []agentfetch.PageResult{
					{URL: "https://example.com/b", Text: "planner directed fetch to the better page"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
		Runtime: agentpattern.RuntimeConfig{
			Planner: stubPlanner{
				plan: func(_ context.Context, input agentplanner.PlanInput) (agentplanner.PlanResult, error) {
					if input.BaselineDecision != branchContinue {
						return agentplanner.PlanResult{}, nil
					}
					return agentplanner.PlanResult{
						Decision:      branchContinue,
						Reason:        "need_more_sources",
						Confidence:    0.77,
						NextQuery:     "refined query",
						PreferredURLs: []string{"https://example.com/b"},
						AvoidURLs:     []string{"https://example.com/a"},
					}, nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-planner-guidance", "refine me", 3))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !reflect.DeepEqual(queries, []string{"refine me", "refined query"}) {
		t.Fatalf("expected planner to refine next search query, got %v", queries)
	}
	if len(fetchCalls) != 2 {
		t.Fatalf("expected two fetch calls, got %v", fetchCalls)
	}
	if !reflect.DeepEqual(fetchCalls[1], []string{"https://example.com/b"}) {
		t.Fatalf("expected planner-guided fetch urls, got %v", fetchCalls[1])
	}
	if result.Snapshot.Context.SearchQuery != "refined query" {
		t.Fatalf("expected final search query to reflect planner guidance, got %+v", result.Snapshot.Context)
	}
	if len(result.Snapshot.Context.SearchResults) != 2 {
		t.Fatalf("expected refined-query continue to reset prior search results, got %+v", result.Snapshot.Context.SearchResults)
	}
	if len(result.Snapshot.Context.FetchResults) != 1 {
		t.Fatalf("expected refined-query continue to reset prior fetch results, got %+v", result.Snapshot.Context.FetchResults)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchAnswer {
		t.Fatalf("expected planner-guided loop to end in answer, got %+v", result.Snapshot.Execution)
	}
}

func TestCompile_ApprovalGatedFetchInterruptsBeforeNode(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "approval please" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Example",
					URL:     "https://example.com/doc",
					Snippet: "needs approval before fetch",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(nil, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}
	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
		Runtime: agentpattern.RuntimeConfig{
			Kernel: agentkernel.BuilderConfig{
				CheckpointStore: agentkernel.NewMemoryCheckpointStore(),
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.RunWithCheckpoint(context.Background(), newSession("sess-approval", "approval please", 2), "cp-fetch-approval")
	if err == nil {
		t.Fatal("expected interrupt error before approval-gated fetch")
	}
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Fatalf("expected interrupt info, got %v", err)
	}
	if result == nil {
		t.Fatal("expected interrupted runtime session")
	}
	if result.Checkpoint == nil || result.Checkpoint.Node != "fetch" {
		t.Fatalf("expected checkpoint on fetch node, got %+v", result.Checkpoint)
	}
	if result.Snapshot.Execution.CurrentNode != "fetch" || !result.Snapshot.Execution.Interrupted {
		t.Fatalf("expected execution interrupt state on fetch, got %+v", result.Snapshot.Execution)
	}
	if !hasEventType(result.Journal, agentstate.EventTypeInterrupt) {
		t.Fatalf("expected interrupt event, got %+v", result.Journal)
	}
	if len(result.Snapshot.Context.SearchResults) == 0 {
		t.Fatalf("expected search to finish before fetch interrupt, got %+v", result.Snapshot.Context)
	}
	if len(result.Snapshot.Context.FetchResults) != 0 {
		t.Fatalf("expected fetch to be blocked before execution, got %+v", result.Snapshot.Context.FetchResults)
	}
}

func TestCompile_RunPermissionFailureTransitionsToApprovalNode(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "runtime approval please" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Restricted",
					URL:     "https://restricted.example/doc",
					Snippet: "requires approval before fetch",
					Domain:  "restricted.example",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchService{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary:       "fetch requires approval",
				Degraded:      true,
				DegradeReason: "provider requires approval",
				ErrorMessage:  "permission denied by upstream provider",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], ErrorMessage: "403 forbidden"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchCapability, fetchCapability} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssemblyContext(registry),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	session := newSession("sess-runtime-approval", "runtime approval please", 2)
	session.Request.Options.RequireApproval = true
	session.Snapshot.Request.RuntimeOptions.RequireApproval = true

	result, err := runner.Run(context.Background(), session)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Execution.CurrentNode != "approval" {
		t.Fatalf("expected approval node to terminate the run, got %+v", result.Snapshot.Execution)
	}
	if !result.Snapshot.Execution.Interrupted {
		t.Fatalf("expected runtime approval path to mark execution interrupted, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.InterruptReason != "fetch_approval_required" {
		t.Fatalf("expected approval interrupt reason, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Execution.LastBranchTarget != branchApproval || result.Snapshot.Execution.LastBranchReason != "fetch_approval_required" {
		t.Fatalf("expected approval branch tracking, got %+v", result.Snapshot.Execution)
	}
	if result.Snapshot.Answer.DegradeReason != "" {
		t.Fatalf("expected runtime approval path to avoid degrade answer, got %+v", result.Snapshot.Answer)
	}
	if !hasNodeEvent(result.Journal, "approval", agentstate.EventTypeInterrupt) {
		t.Fatalf("expected approval interrupt event, got %+v", result.Journal)
	}
}

func TestCompile_RunExperimentalExternalEvidenceWorkflowPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Workflow evidence path produced readable content.</body></html>`))
	}))
	defer server.Close()

	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "workflow path" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Workflow Doc",
					URL:     server.URL,
					Snippet: "Workflow evidence path",
					Domain:  "example.com",
				},
			}, nil
		},
	}, nil)
	searchCapability, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := newTestFetchCapability(server.Client())
	if err != nil {
		t.Fatalf("newTestFetchCapability() error = %v", err)
	}
	workflowCapability, err := agentexternal.NewCapability(searchCapability, fetchCapability)
	if err != nil {
		t.Fatalf("external_evidence.NewCapability() error = %v", err)
	}

	registry := agentcapability.NewRegistry()
	for _, handle := range []agentcapability.Handle{searchCapability, fetchCapability, workflowCapability} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: agentpattern.AssemblyContext{
			Registry: registry,
			Bindings: agentcapability.RoleBindings{
				agentcapability.RoleCollectExternalEvidence: agentcapability.NameExternalEvidenceCollect,
			},
		},
		Runtime: agentpattern.RuntimeConfig{
			PreferExternalEvidenceWorkflow: true,
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.Run(context.Background(), newSession("sess-workflow", "workflow path", 2))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "Workflow evidence path") {
		t.Fatalf("expected workflow path answer to use readable evidence, got %+v", result.Snapshot.Answer)
	}
	if !hasNodeEvent(result.Journal, "external_evidence", agentstate.EventTypeCapabilityResult) {
		t.Fatalf("expected external_evidence capability result event, got %+v", result.Journal)
	}
	if hasNodeEvent(result.Journal, "search", agentstate.EventTypeCapabilityStart) || hasNodeEvent(result.Journal, "fetch", agentstate.EventTypeCapabilityStart) {
		t.Fatalf("expected experimental workflow path to avoid direct search/fetch runtime nodes, got %+v", result.Journal)
	}
}

func TestHandoffBindingsProjectsReactiveNodeMap(t *testing.T) {
	bindings := HandoffBindings(agentcapability.RoleBindings{
		agentcapability.RoleSearch: "search_capability",
		agentcapability.RoleFetch:  "fetch_capability",
	})

	if len(bindings) != 2 {
		t.Fatalf("expected two projected bindings, got %+v", bindings)
	}
	if bindings[0].Node != "search" || bindings[0].Capability != "search_capability" {
		t.Fatalf("expected search projection, got %+v", bindings[0])
	}
	if bindings[1].Node != "fetch" || bindings[1].Capability != "fetch_capability" {
		t.Fatalf("expected fetch projection, got %+v", bindings[1])
	}
}

func newSession(sessionID, question string, maxIterations int) *agentruntime.RuntimeSession {
	return newSessionWithOutputMode(sessionID, question, maxIterations, agentstate.OutputModeFinalAnswer)
}

func newSessionWithOutputMode(sessionID, question string, maxIterations int, outputMode string) *agentruntime.RuntimeSession {
	if maxIterations <= 0 {
		maxIterations = defaultMaxIterations
	}
	if strings.TrimSpace(outputMode) == "" {
		outputMode = agentstate.OutputModeFinalAnswer
	}
	options := agentstate.RuntimeOptions{
		MaxIterations:  maxIterations,
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
				MaxIterations: maxIterations,
			},
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt: time.Now(),
		},
	}
}

func defaultAssemblyContext(registry *agentcapability.Registry) agentpattern.AssemblyContext {
	return agentpattern.AssemblyContext{
		Registry: registry,
		Bindings: agentcapability.RoleBindings{
			agentcapability.RoleSearch: agentcapability.NameWebSearch,
			agentcapability.RoleFetch:  agentcapability.NameWebFetch,
		},
	}
}

func newTestFetchCapability(client *http.Client, options ...agentcapability.Option) (agentcapability.Handle, error) {
	return agentfetch.NewCapability(agentfetchWrapper{client: client}, options...)
}

type agentfetchWrapper struct {
	client *http.Client
}

func (w agentfetchWrapper) Fetch(ctx context.Context, urls []string) (output agentfetch.Output, err error) {
	service := agentfetch.NewService(w.client)
	return service.Fetch(ctx, urls)
}

func hasEventType(events []agentstate.RuntimeEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func hasNodeEvent(events []agentstate.RuntimeEvent, node string, eventType string) bool {
	for _, event := range events {
		if event.Node == node && event.EventType == eventType {
			return true
		}
	}
	return false
}

func containsAction(actions []string, action string) bool {
	for _, item := range actions {
		if item == action {
			return true
		}
	}
	return false
}

type stubFetchService struct {
	fetch func(ctx context.Context, urls []string) (agentfetch.Output, error)
}

func (s stubFetchService) Fetch(ctx context.Context, urls []string) (agentfetch.Output, error) {
	return s.fetch(ctx, urls)
}

type stubPlanner struct {
	plan func(ctx context.Context, input agentplanner.PlanInput) (agentplanner.PlanResult, error)
}

func (p stubPlanner) Plan(ctx context.Context, input agentplanner.PlanInput) (agentplanner.PlanResult, error) {
	return p.plan(ctx, input)
}

type stubSearchInvokerPattern struct{}

func (s *stubSearchInvokerPattern) Search(_ context.Context, _ string) (agentsearch.SearchOutput, error) {
	return agentsearch.SearchOutput{}, nil
}

func newNamedSearchCapability(name string) (agentcapability.Handle, error) {
	base, err := agentsearch.NewCapability(&stubSearchInvokerPattern{})
	if err != nil {
		return nil, err
	}
	return namedSearchCapability{name: name, inner: base}, nil
}

type namedSearchCapability struct {
	name  string
	inner agentcapability.Handle
}

func (c namedSearchCapability) Spec() agentcapability.Spec {
	spec := c.inner.Spec()
	spec.Name = c.name
	return spec
}

func (c namedSearchCapability) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	return c.inner.Invoke(ctx, req)
}

func retryFetchOutput() func() (agentfetch.Output, error) {
	attempt := 0
	return func() (agentfetch.Output, error) {
		attempt++
		if attempt == 1 {
			return agentfetch.Output{
				Summary:       "first fetch attempt returned no readable content",
				Degraded:      true,
				DegradeReason: "temporary_fetch_failure",
				ErrorMessage:  "temporary fetch failure",
				Pages: []agentfetch.PageResult{
					{
						URL:          "https://example.com/retry",
						ErrorMessage: "temporary fetch failure",
					},
				},
			}, nil
		}
		return agentfetch.Output{
			Summary: "second fetch attempt produced readable evidence",
			Pages: []agentfetch.PageResult{
				{
					URL:  "https://example.com/retry",
					Text: "second attempt produced readable evidence",
				},
			},
		}, nil
	}
}

func countBranchTarget(events []agentstate.RuntimeEvent, target string) int {
	count := 0
	for _, event := range events {
		if event.EventType == agentstate.EventTypeBranchSelected && event.PayloadText == target {
			count++
		}
	}
	return count
}
