package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
)

func TestNewRuntime(t *testing.T) {
	loadAssemblyTestConfig(t)

	runtime := infraai.NewRuntime()
	if runtime == nil {
		t.Fatal("expected runtime to be non-nil")
	}
	if runtime.Chat == nil {
		t.Fatal("expected chat service to be initialized")
	}
	if runtime.Embedding == nil {
		t.Fatal("expected embedding service to be initialized")
	}
	if runtime.Rerank == nil {
		t.Fatal("expected rerank service to be initialized")
	}
	if runtime.HealthStore == nil || runtime.Selector == nil || runtime.Executor == nil {
		t.Fatal("expected shared infra components to be initialized")
	}
	if len(runtime.ChatClients) != 3 {
		t.Fatalf("expected 3 chat clients, got %d", len(runtime.ChatClients))
	}
	targets := runtime.Selector.SelectChatCandidates(false)
	if len(targets) == 0 {
		t.Fatal("expected at least one chat target")
	}
	if targets[0].Id != "qwen-plus" {
		t.Fatalf("expected first chat target qwen-plus, got %s", targets[0].Id)
	}
	if targets[0].Candidate.Provider != "bailian" {
		t.Fatalf("expected first chat target provider bailian, got %s", targets[0].Candidate.Provider)
	}
	if targets[0].Candidate.Model != "qwen-plus-latest" {
		t.Fatalf("expected first chat target model qwen-plus-latest, got %s", targets[0].Candidate.Model)
	}
	deepThinkingTargets := runtime.Selector.SelectChatCandidates(true)
	if len(deepThinkingTargets) == 0 {
		t.Fatal("expected at least one deep thinking chat target")
	}
	if deepThinkingTargets[0].Id != "qwen-plus" {
		t.Fatalf("expected first deep thinking chat target qwen-plus, got %s", deepThinkingTargets[0].Id)
	}
	if len(runtime.EmbeddingClients) != 2 {
		t.Fatalf("expected 2 embedding clients, got %d", len(runtime.EmbeddingClients))
	}
	if len(runtime.RerankClients) != 3 {
		t.Fatalf("expected 3 rerank clients, got %d", len(runtime.RerankClients))
	}
	if runtime.HTTPClient == nil || runtime.StreamHTTPClient == nil {
		t.Fatal("expected http clients to be initialized")
	}
	if runtime.StreamHTTPClient.Timeout != 2*time.Second {
		t.Fatalf("expected stream timeout from config, got %s", runtime.StreamHTTPClient.Timeout)
	}
}

func loadAssemblyTestConfig(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	content := []byte(`
rag:
  default:
    sse-timeout-ms: 2000
ai:
  providers:
    ollama:
      url: http://localhost:11434
      endpoints:
        chat: /v1/chat/completions
        embedding: /v1/embeddings
    bailian:
      url: https://dashscope.aliyuncs.com
      api-key: test-key
      endpoints:
        chat: /compatible-mode/v1/chat/completions
        rerank: /api/v1/services/rerank/text-rerank/text-rerank
    siliconflow:
      url: https://api.siliconflow.cn
      api-key: test-key
      endpoints:
        chat: /v1/chat/completions
        embedding: /v1/embeddings
        rerank: /v1/rerank
  selection:
    failure-threshold: 1
    open-duration-ms: 10
  chat:
    default-model: qwen-plus
    deep-thinking-model: qwen-plus
    candidates:
      - id: deepseek-r1
        provider: siliconflow
        model: deepseek-ai/DeepSeek-R1
        supports-thinking: true
      - id: qwen-plus
        provider: bailian
        model: qwen-plus-latest
        supports-thinking: true
      - id: glm
        provider: siliconflow
        model: glm-4.7
        supports-thinking: true
      - id: qwen-local
        provider: ollama
        model: qwen3:8b
  embedding:
    default-model: emb
    candidates:
      - id: emb
        provider: siliconflow
        model: emb-model
        dimension: 1024
      - id: emb-local
        provider: ollama
        model: emb-local
        dimension: 1024
  rerank:
    default-model: rr
    candidates:
      - id: rr
        provider: siliconflow
        model: Qwen/Qwen3-Reranker-8B
      - id: rr-bailian
        provider: bailian
        model: qwen-rerank
      - id: rr-noop
        provider: noop
        model: noop
`)

	path := filepath.Join(dir, "application.yaml")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := config.LoadConfig(dir); err != nil {
		t.Fatalf("load config failed: %v", err)
	}
}
