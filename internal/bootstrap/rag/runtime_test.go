package rag

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/config"
	inframcp "local/rag-project/internal/infra-mcp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuildMCPManagerInjectsTavilyAPIKey(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.MCP.Servers = map[string]config.RagMCPServerConfig{
		"tavily": {
			Enabled:          true,
			Transport:        "stdio",
			Command:          os.Args[0],
			Args:             []string{"-test.run=TestBuildMCPManagerHelperProcess"},
			Env:              map[string]string{"GOAGENT_RAG_MCP_HELPER": "1"},
			StartupTimeoutMs: 2000,
			CallTimeoutMs:    1000,
		},
	}
	cfg.Rag.Search.WebSearch.ApiKey = "injected-key"

	manager := buildMCPManager(cfg)
	if manager == nil {
		t.Fatal("expected non-nil MCP manager")
	}
	t.Cleanup(func() { _ = manager.Close() })

	result, err := manager.CallTool(context.Background(), "tavily", "env-info", nil)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured content map, got %T", result.StructuredContent)
	}
	if got, _ := structured["apiKey"].(string); got != "injected-key" {
		t.Fatalf("expected injected api key, got %#v", structured["apiKey"])
	}
}

func TestRuntimeCloseClosesMCPManager(t *testing.T) {
	runtime := &Runtime{
		mcpManager: inframcp.NewManager(nil),
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := runtime.mcpManager.ListTools(context.Background(), "missing"); !errors.Is(err, inframcp.ErrManagerClosed) {
		t.Fatalf("expected ErrManagerClosed, got %v", err)
	}
}

func TestReadMemoryCacheMetricsEnabledRespectsConfig(t *testing.T) {
	cfg := &config.Config{}
	if readMemoryCacheMetricsEnabled(cfg) {
		t.Fatal("expected metrics disabled when memory cache is disabled")
	}

	cfg.Rag.Memory.Cache.Enabled = true
	if readMemoryCacheMetricsEnabled(cfg) {
		t.Fatal("expected metrics disabled when metrics flag is false")
	}

	cfg.Rag.Memory.Cache.MetricsEnabled = true
	if !readMemoryCacheMetricsEnabled(cfg) {
		t.Fatal("expected metrics enabled when metrics flag is true")
	}
}

func TestRuntimeMemoryMaintenanceLoopDisabledDoesNotStart(t *testing.T) {
	cfg := &config.Config{}
	var calls atomic.Int32
	runtime := &Runtime{
		memoryMaintenanceRunner: func(context.Context, longtermmemory.MaintenanceInput) (longtermmemory.MaintenanceResult, error) {
			calls.Add(1)
			return longtermmemory.MaintenanceResult{}, nil
		},
	}

	runtime.startMemoryMaintenanceLoop(cfg)
	time.Sleep(30 * time.Millisecond)
	runtime.stopMemoryMaintenanceLoop()

	if calls.Load() != 0 {
		t.Fatalf("expected no maintenance calls when disabled, got %d", calls.Load())
	}
}

func TestRuntimeMemoryMaintenanceLoopRunsImmediatelyAndStopsOnClose(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.Maintenance.Enabled = true
	cfg.Rag.Memory.Maintenance.ScanDelayMs = 1000
	cfg.Rag.Memory.Maintenance.RunTimeoutMs = 100

	calls := make(chan struct{}, 2)
	var total atomic.Int32
	runtime := &Runtime{
		memoryMaintenanceRunner: func(context.Context, longtermmemory.MaintenanceInput) (longtermmemory.MaintenanceResult, error) {
			total.Add(1)
			select {
			case calls <- struct{}{}:
			default:
			}
			return longtermmemory.MaintenanceResult{}, nil
		},
	}

	runtime.startMemoryMaintenanceLoop(cfg)
	select {
	case <-calls:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected immediate maintenance run")
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if total.Load() != 1 {
		t.Fatalf("expected maintenance loop to stop after Close, got %d calls", total.Load())
	}
}

func TestRuntimeMemoryMaintenanceLoopContinuesAfterTimeout(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.Maintenance.Enabled = true
	cfg.Rag.Memory.Maintenance.ScanDelayMs = 20
	cfg.Rag.Memory.Maintenance.RunTimeoutMs = 10

	var calls atomic.Int32
	done := make(chan struct{}, 4)
	runtime := &Runtime{
		memoryMaintenanceRunner: func(ctx context.Context, _ longtermmemory.MaintenanceInput) (longtermmemory.MaintenanceResult, error) {
			current := calls.Add(1)
			if current == 1 {
				<-ctx.Done()
				done <- struct{}{}
				return longtermmemory.MaintenanceResult{}, ctx.Err()
			}
			done <- struct{}{}
			return longtermmemory.MaintenanceResult{}, nil
		},
	}

	runtime.startMemoryMaintenanceLoop(cfg)
	defer runtime.stopMemoryMaintenanceLoop()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected first maintenance iteration to time out")
	}
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected maintenance loop to continue after timeout")
	}

	if calls.Load() < 2 {
		t.Fatalf("expected at least two maintenance iterations, got %d", calls.Load())
	}
}

func TestBuildMCPManagerHelperProcess(t *testing.T) {
	if os.Getenv("GOAGENT_RAG_MCP_HELPER") != "1" {
		return
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "goagent-rag-helper", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "env-info",
		Description: "return selected environment variables",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, map[string]any, error) {
		return nil, map[string]any{
			"apiKey": os.Getenv("TAVILY_API_KEY"),
		}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !strings.Contains(err.Error(), "file already closed") {
		t.Fatalf("helper server failed: %v", err)
	}
	os.Exit(0)
}
