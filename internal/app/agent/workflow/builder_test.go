package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	"local/rag-project/internal/app/agent/webfetch"
	"local/rag-project/internal/app/agent/websearch"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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

func TestWorkflowRunsSearchFetchObserveAndBreaksLoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Go generics add type parameters to functions and types.</body></html>`))
	}))
	defer server.Close()

	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return []searchprovider.SearchResult{{
				Title:   "Go Docs",
				URL:     server.URL,
				Snippet: "Generics in Go.",
			}}, nil
		},
	}, nil)
	searchTool, err := websearch.New(searchService)
	if err != nil {
		t.Fatalf("websearch.New() error = %v", err)
	}
	fetchTool, err := webfetch.New(agentfetch.NewService(server.Client()))
	if err != nil {
		t.Fatalf("webfetch.New() error = %v", err)
	}

	workflowAgent, err := New(context.Background(), Config{
		SearchTool:    searchTool.Invokable(),
		FetchTool:     fetchTool.Invokable(),
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: workflowAgent})
	iter := runner.Run(
		context.Background(),
		[]adk.Message{schema.UserMessage("Go generics tutorial")},
		adk.WithSessionValues(SessionValues(Request{Question: "Go generics tutorial"})),
	)

	eventCount := 0
	var final *FinalState
	var breakLoopSeen bool
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("unexpected event error: %v", event.Err)
		}
		eventCount++
		if event.Action != nil && event.Action.BreakLoop != nil {
			breakLoopSeen = event.Action.BreakLoop.Done
		}
		if event.Output != nil && event.Output.CustomizedOutput != nil {
			if state, ok := event.Output.CustomizedOutput.(FinalState); ok {
				final = &state
			}
		}
	}

	if eventCount != 4 {
		t.Fatalf("expected 4 events (plan/search/fetch/observe), got %d", eventCount)
	}
	if !breakLoopSeen {
		t.Fatal("expected observe step to break loop")
	}
	if final == nil || final.SearchOutput.ResultCount != 1 {
		t.Fatalf("unexpected final state: %+v", final)
	}
	if final.FetchOutput == nil || final.FetchOutput.SuccessCount != 1 {
		t.Fatalf("expected fetched page content, got %+v", final)
	}
}

func TestWorkflowMarksProviderFailureAsDegraded(t *testing.T) {
	searchService := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return nil, context.DeadlineExceeded
		},
	}, nil)
	searchTool, err := websearch.New(searchService)
	if err != nil {
		t.Fatalf("websearch.New() error = %v", err)
	}
	fetchTool, err := webfetch.New(agentfetch.NewService(nil))
	if err != nil {
		t.Fatalf("webfetch.New() error = %v", err)
	}

	workflowAgent, err := New(context.Background(), Config{
		SearchTool:    searchTool.Invokable(),
		FetchTool:     fetchTool.Invokable(),
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	runner := adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: workflowAgent})
	iter := runner.Run(
		context.Background(),
		[]adk.Message{schema.UserMessage("Go generics tutorial")},
		adk.WithSessionValues(SessionValues(Request{Question: "Go generics tutorial"})),
	)

	var final *FinalState
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatalf("unexpected event error: %v", event.Err)
		}
		if event.Output != nil && event.Output.CustomizedOutput != nil {
			if state, ok := event.Output.CustomizedOutput.(FinalState); ok {
				final = &state
			}
		}
	}

	if final == nil || !final.Degraded {
		t.Fatalf("expected degraded final state, got %+v", final)
	}
}
