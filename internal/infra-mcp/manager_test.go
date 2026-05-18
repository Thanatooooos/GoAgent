package inframcp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchInput struct {
	Query string `json:"query"`
}

type searchOutput struct {
	Results []searchItem `json:"results"`
}

type searchItem struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type slowInput struct {
	SleepMs int `json:"sleepMs"`
}

func TestManagerListToolsAndCallTool(t *testing.T) {
	manager := NewManager(map[string]ServerConfig{
		"test": helperServerConfig(),
	})
	t.Cleanup(func() { _ = manager.Close() })

	tools, err := manager.ListTools(context.Background(), "test")
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if !hasTool(tools, "tavily-search") {
		t.Fatalf("expected tavily-search to be listed, got %+v", tools)
	}

	result, err := manager.CallTool(context.Background(), "test", "tavily-search", map[string]any{
		"query": "golang generics",
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful tool result, got error content %+v", result.Content)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured content map, got %T", result.StructuredContent)
	}
	results, ok := structured["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one structured result, got %#v", structured["results"])
	}
}

func TestManagerCallToolTimeout(t *testing.T) {
	manager := NewManager(map[string]ServerConfig{
		"test": func() ServerConfig {
			cfg := helperServerConfig()
			cfg.CallTimeoutMs = 50
			return cfg
		}(),
	})
	t.Cleanup(func() { _ = manager.Close() })

	_, err := manager.CallTool(context.Background(), "test", "slow-tool", map[string]any{"sleepMs": 200})
	if !errors.Is(err, ErrCallTimeout) {
		t.Fatalf("expected ErrCallTimeout, got %v", err)
	}
}

func TestManagerCallToolToolNotFound(t *testing.T) {
	manager := NewManager(map[string]ServerConfig{
		"test": helperServerConfig(),
	})
	t.Cleanup(func() { _ = manager.Close() })

	_, err := manager.CallTool(context.Background(), "test", "missing-tool", nil)
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("expected ErrToolNotFound, got %v", err)
	}
}

func TestManagerReportsConfigurationErrors(t *testing.T) {
	tests := []struct {
		name       string
		servers    map[string]ServerConfig
		serverName string
		wantErr    error
	}{
		{
			name:       "server missing",
			servers:    map[string]ServerConfig{},
			serverName: "missing",
			wantErr:    ErrServerNotConfigured,
		},
		{
			name: "server disabled",
			servers: map[string]ServerConfig{
				"disabled": {Enabled: false, Transport: "stdio", Command: "cmd"},
			},
			serverName: "disabled",
			wantErr:    ErrServerDisabled,
		},
		{
			name: "command missing",
			servers: map[string]ServerConfig{
				"invalid": {Enabled: true, Transport: "stdio"},
			},
			serverName: "invalid",
			wantErr:    ErrServerCommandMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(tt.servers)
			t.Cleanup(func() { _ = manager.Close() })
			_, err := manager.ListTools(context.Background(), tt.serverName)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestManagerCloseIsIdempotent(t *testing.T) {
	manager := NewManager(map[string]ServerConfig{
		"test": helperServerConfig(),
	})

	if _, err := manager.ListTools(context.Background(), "test"); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := manager.ListTools(context.Background(), "test"); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("expected ErrManagerClosed after close, got %v", err)
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GOAGENT_MCP_HELPER") != "1" {
		return
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "goagent-mcp-helper", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "tavily-search",
		Description: "search the web",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		return nil, searchOutput{
			Results: []searchItem{{
				Title:   "Go Generics",
				URL:     "https://go.dev/doc/tutorial/generics",
				Content: "An introduction to generics in Go.",
				Score:   0.98,
			}},
		}, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "slow-tool",
		Description: "sleep before returning",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input slowInput) (*mcp.CallToolResult, map[string]any, error) {
		if input.SleepMs > 0 {
			time.Sleep(time.Duration(input.SleepMs) * time.Millisecond)
		}
		return nil, map[string]any{"ok": true}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !strings.Contains(err.Error(), "file already closed") {
		t.Fatalf("helper server failed: %v", err)
	}
	os.Exit(0)
}

func helperServerConfig() ServerConfig {
	return ServerConfig{
		Enabled:          true,
		Transport:        "stdio",
		Command:          os.Args[0],
		Args:             []string{"-test.run=TestMCPHelperProcess"},
		Env:              map[string]string{"GOAGENT_MCP_HELPER": "1"},
		StartupTimeoutMs: 2000,
		CallTimeoutMs:    1000,
	}
}
