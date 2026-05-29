package webfetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentfetch "local/rag-project/internal/app/agent/fetch"
)

func TestToolInvokesFetchService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Fetched readable documentation content.</body></html>`))
	}))
	defer server.Close()

	service := agentfetch.NewService(server.Client())
	tool, err := New(service)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	output, err := tool.Invokable().InvokableRun(context.Background(), `{"urls":["`+server.URL+`"]}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	var decoded agentfetch.Output
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if decoded.SuccessCount != 1 || len(decoded.Pages) != 1 {
		t.Fatalf("unexpected output: %+v", decoded)
	}
}

func TestToolEncodesFetchFailureAsDegradedOutput(t *testing.T) {
	service := agentfetch.NewService(nil)
	tool, err := New(service)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	output, err := tool.Invokable().InvokableRun(context.Background(), `{"urls":["mailto:test@example.com"]}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}

	var decoded agentfetch.Output
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !decoded.Degraded || decoded.FailCount != 1 {
		t.Fatalf("unexpected degraded output: %+v", decoded)
	}
}
