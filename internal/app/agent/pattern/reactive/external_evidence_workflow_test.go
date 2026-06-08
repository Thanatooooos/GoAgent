package reactive

import (
	"context"
	"reflect"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
)

func TestCompile_ExperimentalExternalEvidenceWorkflowAppliesSourceReview(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "workflow source review" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Neutral",
					URL:     "https://neutral.example/doc",
					Snippet: "neutral evidence",
					Domain:  "neutral.example",
				},
				{
					Title:   "Allowed",
					URL:     "https://allow.example/doc",
					Snippet: "allowed evidence",
					Domain:  "allow.example",
					Policy:  "allow",
				},
				{
					Title:   "Denied",
					URL:     "https://deny.example/doc",
					Snippet: "denied evidence",
					Domain:  "deny.example",
					Policy:  "deny",
				},
			}, nil
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
			return agentfetch.Output{
				Summary: "workflow review produced readable evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "workflow review first readable evidence"},
					{URL: urls[1], Text: "workflow review second readable evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
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

	result, err := runner.Run(context.Background(), newSession("sess-workflow-review", "workflow source review", 2))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(fetchCalls, [][]string{{"https://allow.example/doc", "https://neutral.example/doc"}}) {
		t.Fatalf("expected source review to fetch allowed then neutral source, got %v", fetchCalls)
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "workflow review first readable evidence") {
		t.Fatalf("expected workflow path answer to use readable evidence, got %+v", result.Snapshot.Answer)
	}
	if !containsAction(result.Snapshot.Execution.CompletedActions, "external_evidence") {
		t.Fatalf("expected external_evidence workflow node to complete, got %+v", result.Snapshot.Execution)
	}
}
