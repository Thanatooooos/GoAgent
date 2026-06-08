package planexecute

import (
	"context"
	"reflect"
	"strings"
	"testing"

	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestCompile_RunWithCheckpointAnswerPath(t *testing.T) {
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			if query != "checkpointed plan execute" {
				t.Fatalf("unexpected query: %q", query)
			}
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/checkpointed"},
				Results: []agentsearch.SearchResultItem{{Title: "Checkpointed", URL: "https://example.com/checkpointed", Snippet: "checkpointed evidence", Domain: "example.com"}},
				Summary: "found one relevant result",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}

	var fetchCalls [][]string
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchCalls = append(fetchCalls, append([]string(nil), urls...))
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "fetched checkpointed evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "checkpointed plan execute evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}

	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registerHandles(t, searchCapability, fetchCapability)),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode: agentstate.OutputModeFinalAnswer,
			Kernel: agentkernel.BuilderConfig{
				CheckpointStore: agentkernel.NewMemoryCheckpointStore(),
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.RunWithCheckpoint(
		context.Background(),
		newSession("sess-plan-checkpoint", "checkpointed plan execute", agentstate.OutputModeFinalAnswer),
		"cp-plan-answer",
	)
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}

	if !reflect.DeepEqual(fetchCalls, [][]string{{"https://example.com/checkpointed"}}) {
		t.Fatalf("expected a single fetch call for checkpointed run, got %v", fetchCalls)
	}
	if result.Snapshot.Plan.Status != agentstate.PlanStatusCompleted {
		t.Fatalf("expected completed plan, got %+v", result.Snapshot.Plan)
	}
	if len(result.Snapshot.Context.SearchResults) == 0 {
		t.Fatalf("expected checkpointed run to retain search results, got %+v", result.Snapshot.Context)
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "checkpointed plan execute evidence") {
		t.Fatalf("expected grounded final answer, got %+v", result.Snapshot.Answer)
	}
}

func TestCompile_RunWithResolverPreservesFetchURLs(t *testing.T) {
	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			if query != "resolver-backed plan execute" {
				t.Fatalf("unexpected query: %q", query)
			}
			return agentsearch.SearchOutput{
				Query:   query,
				URLs:    []string{"https://example.com/resolver"},
				Results: []agentsearch.SearchResultItem{{Title: "Resolver", URL: "https://example.com/resolver", Snippet: "resolver evidence", Domain: "example.com"}},
				Summary: "found one relevant result",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}

	var fetchCalls [][]string
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			fetchCalls = append(fetchCalls, append([]string(nil), urls...))
			return agentfetch.Output{
				URLs:    append([]string(nil), urls...),
				Summary: "fetched resolver-backed evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "resolver-backed plan execute evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}

	registry := registerHandles(t, searchCapability, fetchCapability)
	runner, err := Compile(context.Background(), Config{
		Assembly: defaultAssembly(registry),
		Runtime: agentpattern.RuntimeConfig{
			OutputMode:         agentstate.OutputModeFinalAnswer,
			CapabilityResolver: agentresolve.NewRegistryResolver(registry),
			Kernel: agentkernel.BuilderConfig{
				CheckpointStore: agentkernel.NewMemoryCheckpointStore(),
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	result, err := runner.RunWithCheckpoint(
		context.Background(),
		newSession("sess-plan-resolver", "resolver-backed plan execute", agentstate.OutputModeFinalAnswer),
		"cp-plan-resolver",
	)
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}

	if !reflect.DeepEqual(fetchCalls, [][]string{{"https://example.com/resolver"}}) {
		t.Fatalf("expected resolver-backed run to preserve fetch urls, got %v", fetchCalls)
	}
	if result.Snapshot.Answer.Final == "" || !strings.Contains(result.Snapshot.Answer.Final, "resolver-backed plan execute evidence") {
		t.Fatalf("expected grounded final answer, got %+v", result.Snapshot.Answer)
	}
}
