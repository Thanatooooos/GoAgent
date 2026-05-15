package config

import "testing"

func TestLoadConfig_Defaults(t *testing.T) {
	// tests run with package dir as working directory; load from repository configs
	if err := LoadConfig("../../../configs"); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg := Get()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// assert a few known values from configs/application.yaml
	if cfg.AI.Chat.DefaultModel != "qwen3-max" {
		t.Fatalf("unexpected chat.default-model: %s", cfg.AI.Chat.DefaultModel)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("unexpected server.port: %d", cfg.Server.Port)
	}
	if cfg.Spring.Datasource.Username != "postgres" {
		t.Fatalf("expected datasource username from .env, got %q", cfg.Spring.Datasource.Username)
	}
	if cfg.Spring.Datasource.Password != "postgres" {
		t.Fatalf("expected datasource password from .env, got %q", cfg.Spring.Datasource.Password)
	}
	if cfg.Parser.Tika.URL == "" {
		t.Fatal("expected parser.tika.url to be configured")
	}
	if cfg.Rag.Knowledge.Ingestion.MaxConcurrent != 8 {
		t.Fatalf("unexpected rag.knowledge.ingestion.max-concurrent: %d", cfg.Rag.Knowledge.Ingestion.MaxConcurrent)
	}
	if cfg.Rag.Agent.MaxIterations != 3 {
		t.Fatalf("unexpected rag.agent.max-iterations: %d", cfg.Rag.Agent.MaxIterations)
	}
	if !cfg.Rag.Agent.ParallelToolCalls.Enabled {
		t.Fatal("expected rag.agent.parallel-tool-calls.enabled to default to true")
	}
	if cfg.Rag.Agent.ParallelToolCalls.MaxConcurrency != 3 {
		t.Fatalf("unexpected rag.agent.parallel-tool-calls.max-concurrency: %d", cfg.Rag.Agent.ParallelToolCalls.MaxConcurrency)
	}
	if cfg.Rag.Search.WebSearch.SourcePolicy.AllowDomains[0] != "go.dev" {
		t.Fatalf("expected web search source policy allow-domains to load, got %+v", cfg.Rag.Search.WebSearch.SourcePolicy.AllowDomains)
	}
	if cfg.Rag.Search.WebSearch.SourcePolicy.DenyDomains[0] != "quora.com" {
		t.Fatalf("expected web search source policy deny-domains to load, got %+v", cfg.Rag.Search.WebSearch.SourcePolicy.DenyDomains)
	}
}
