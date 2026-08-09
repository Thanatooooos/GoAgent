package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("AI_CHAT_DEFAULT_MODEL", "qwen3-32b")
	t.Setenv("AI_CHAT_DEEP_THINKING_MODEL", "qwen3-32b")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SPRING_DATASOURCE_DRIVER_CLASS_NAME", "org.postgresql.Driver")
	t.Setenv("SPRING_DATASOURCE_URL", "jdbc:postgresql://127.0.0.1:5432/ragent?client_encoding=UTF8")
	t.Setenv("PARSER_TIKA_URL", "http://localhost:9998/tika")
	t.Setenv("RAG_KNOWLEDGE_INGESTION_MAX_CONCURRENT", "8")
	t.Setenv("RAG_AGENT_MAX_ITERATIONS", "3")
	t.Setenv("RAG_AGENT_PARALLEL_TOOL_CALLS_ENABLED", "true")
	t.Setenv("RAG_AGENT_PARALLEL_TOOL_CALLS_MAX_CONCURRENCY", "3")
	t.Setenv("RAG_AGENT_CHAT_MODE", "always")
	t.Setenv("RAG_AGENT_RUNTIME_PERSISTENCE_ENABLED", "false")
	t.Setenv("RAG_AGENT_RUNTIME_PERSISTENCE_DIR", ".agent-runtime")
	t.Setenv("RAG_RETRIEVE_PARALLEL_SUBQUESTIONS_ENABLED", "true")
	t.Setenv("RAG_RETRIEVE_PARALLEL_SUBQUESTIONS_MAX_CONCURRENCY", "2")
	t.Setenv("RAG_SEARCH_WEB_SEARCH_PROVIDER", "tavily-mcp")
	t.Setenv("RAG_SEARCH_WEB_SEARCH_FALLBACK_PROVIDER", "tavily")
	t.Setenv("RAG_SEARCH_WEB_SEARCH_MCP_SERVER", "tavily")

	dir := writeConfigFixture(t, `server:
  port: 9090
spring:
  datasource:
    driver-class-name: org.postgresql.Driver
    url: jdbc:postgresql://127.0.0.1:5432/ragent?client_encoding=UTF8
rag:
  knowledge:
    ingestion:
      max-concurrent: 8
  agent:
    max-iterations: 3
    chat:
      mode: always
    parallel-tool-calls:
      enabled: true
      max-concurrency: 3
    runtime-persistence:
      enabled: false
      dir: .agent-runtime
  retrieve:
    parallel-subquestions:
      enabled: true
      max-concurrency: 2
  search:
    web-search:
      provider: tavily-mcp
      fallback-provider: tavily
      mcp:
        server: tavily
      source-policy:
        allow-domains:
          - go.dev
        deny-domains:
          - quora.com
  mcp:
    servers:
      tavily:
        enabled: true
ai:
  concurrency:
    max-per-model: 4
  chat:
    default-model: qwen3-32b
    deep-thinking-model: qwen3-32b
parser:
  tika:
    url: http://localhost:9998/tika
`)

	if err := LoadConfig(dir); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg := Get()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.AI.Chat.DefaultModel != "qwen3-32b" {
		t.Fatalf("unexpected chat.default-model: %s", cfg.AI.Chat.DefaultModel)
	}
	if cfg.AI.Chat.DeepThinkingModel != "qwen3-32b" {
		t.Fatalf("unexpected chat.deep-thinking-model: %s", cfg.AI.Chat.DeepThinkingModel)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("unexpected server.port: %d", cfg.Server.Port)
	}
	if cfg.Spring.Datasource.DriverClassName != "org.postgresql.Driver" {
		t.Fatalf("unexpected datasource driver-class-name: %q", cfg.Spring.Datasource.DriverClassName)
	}
	if cfg.Spring.Datasource.Url != "jdbc:postgresql://127.0.0.1:5432/ragent?client_encoding=UTF8" {
		t.Fatalf("unexpected datasource url: %q", cfg.Spring.Datasource.Url)
	}
	if cfg.Parser.Tika.URL != "http://localhost:9998/tika" {
		t.Fatalf("unexpected parser.tika.url: %q", cfg.Parser.Tika.URL)
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
	if cfg.AI.Concurrency.MaxPerModel != 4 {
		t.Fatalf("unexpected ai.concurrency.max-per-model: %d", cfg.AI.Concurrency.MaxPerModel)
	}
	if cfg.Rag.Agent.Chat.Mode != "always" {
		t.Fatalf("unexpected rag.agent.chat.mode: %q", cfg.Rag.Agent.Chat.Mode)
	}
	if cfg.Rag.Agent.RuntimePersistence.Enabled {
		t.Fatal("expected rag.agent.runtime-persistence.enabled to default to false")
	}
	if cfg.Rag.Agent.RuntimePersistence.Dir != ".agent-runtime" {
		t.Fatalf("unexpected rag.agent.runtime-persistence.dir: %q", cfg.Rag.Agent.RuntimePersistence.Dir)
	}
	if !cfg.Rag.Retrieve.ParallelSubquestions.Enabled {
		t.Fatal("expected rag.retrieve.parallel-subquestions.enabled to default to true")
	}
	if cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency != 2 {
		t.Fatalf("unexpected rag.retrieve.parallel-subquestions.max-concurrency: %d", cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency)
	}
	if cfg.Rag.Search.WebSearch.SourcePolicy.AllowDomains[0] != "go.dev" {
		t.Fatalf("expected web search source policy allow-domains to load, got %+v", cfg.Rag.Search.WebSearch.SourcePolicy.AllowDomains)
	}
	if cfg.Rag.Search.WebSearch.SourcePolicy.DenyDomains[0] != "quora.com" {
		t.Fatalf("expected web search source policy deny-domains to load, got %+v", cfg.Rag.Search.WebSearch.SourcePolicy.DenyDomains)
	}
	if cfg.Rag.Search.WebSearch.Provider != "tavily-mcp" {
		t.Fatalf("unexpected web search provider: %q", cfg.Rag.Search.WebSearch.Provider)
	}
	if cfg.Rag.Search.WebSearch.FallbackProvider != "tavily" {
		t.Fatalf("unexpected web search fallback provider: %q", cfg.Rag.Search.WebSearch.FallbackProvider)
	}
	if cfg.Rag.Search.WebSearch.MCP.Server != "tavily" {
		t.Fatalf("unexpected web search MCP server: %q", cfg.Rag.Search.WebSearch.MCP.Server)
	}
	if _, ok := cfg.Rag.MCP.Servers["tavily"]; !ok {
		t.Fatalf("expected rag.mcp.servers.tavily to load, got %+v", cfg.Rag.MCP.Servers)
	}
}

func TestLoadConfig_DailyBriefDefaults(t *testing.T) {
	dir := writeConfigFixture(t, `daily-brief:
  schedule:
    scan-delay-ms: 10000
    run-timeout-ms: 30000
    lock-seconds: 900
    batch-size: 20
  retry:
    max-attempts: 2
    backoff-minutes: 30
  generation:
    max-candidates: 20
    max-items: 5
    max-items-per-topic: 0
    prompt-version: v1
    model: qwen3-32b
`)

	if err := LoadConfig(dir); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	cfg := Get()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.DailyBrief.Schedule.ScanDelayMs != 10000 {
		t.Fatalf("unexpected daily-brief.schedule.scan-delay-ms: %d", cfg.DailyBrief.Schedule.ScanDelayMs)
	}
	if cfg.DailyBrief.Schedule.RunTimeoutMs != 30000 {
		t.Fatalf("unexpected daily-brief.schedule.run-timeout-ms: %d", cfg.DailyBrief.Schedule.RunTimeoutMs)
	}
	if cfg.DailyBrief.Schedule.LockSeconds != 900 {
		t.Fatalf("unexpected daily-brief.schedule.lock-seconds: %d", cfg.DailyBrief.Schedule.LockSeconds)
	}
	if cfg.DailyBrief.Schedule.BatchSize != 20 {
		t.Fatalf("unexpected daily-brief.schedule.batch-size: %d", cfg.DailyBrief.Schedule.BatchSize)
	}
	if cfg.DailyBrief.Retry.MaxAttempts != 2 {
		t.Fatalf("unexpected daily-brief.retry.max-attempts: %d", cfg.DailyBrief.Retry.MaxAttempts)
	}
	if cfg.DailyBrief.Retry.BackoffMinutes != 30 {
		t.Fatalf("unexpected daily-brief.retry.backoff-minutes: %d", cfg.DailyBrief.Retry.BackoffMinutes)
	}
	if cfg.DailyBrief.Generation.MaxCandidates != 20 {
		t.Fatalf("unexpected daily-brief.generation.max-candidates: %d", cfg.DailyBrief.Generation.MaxCandidates)
	}
	if cfg.DailyBrief.Generation.MaxItems != 5 {
		t.Fatalf("unexpected daily-brief.generation.max-items: %d", cfg.DailyBrief.Generation.MaxItems)
	}
	if cfg.DailyBrief.Generation.MaxItemsPerTopic != 0 {
		t.Fatalf("unexpected daily-brief.generation.max-items-per-topic: %d", cfg.DailyBrief.Generation.MaxItemsPerTopic)
	}
	if cfg.DailyBrief.Generation.PromptVersion != "v1" {
		t.Fatalf("unexpected daily-brief.generation.prompt-version: %q", cfg.DailyBrief.Generation.PromptVersion)
	}
	if cfg.DailyBrief.Generation.Model != "qwen3-32b" {
		t.Fatalf("unexpected daily-brief.generation.model: %q", cfg.DailyBrief.Generation.Model)
	}
}

func writeConfigFixture(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "application.yaml")
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return dir
}
