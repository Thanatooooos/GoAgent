package embedding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/model"
)

func TestSiliconFlowEmbeddingClientBoundsDefaultBatchSize(t *testing.T) {
	const maxExpectedBatchSize = 32

	requestCount := 0
	inputCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestCount++
		var payload struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		if len(payload.Input) > maxExpectedBatchSize {
			t.Fatalf("embedding request contains %d inputs, want at most %d", len(payload.Input), maxExpectedBatchSize)
		}
		inputCount += len(payload.Input)
		data := make([]map[string]any, len(payload.Input))
		for i := range payload.Input {
			data[i] = map[string]any{"embedding": []float32{1}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	client := NewSiliconFlowEmbeddingClient(server.Client())
	texts := make([]string, maxExpectedBatchSize+1)
	for i := range texts {
		texts[i] = "chunk"
	}
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "demo-embedding"},
		Provider: config.ProviderConfig{
			Url:       server.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"embedding": "/embeddings"},
		},
	}

	vectors, err := client.EmbedBatch(texts, target)
	if err != nil {
		t.Fatalf("EmbedBatch returned error: %v", err)
	}
	if len(vectors) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vectors), len(texts))
	}
	if requestCount < 2 {
		t.Fatalf("got %d embedding request, want batching into multiple requests", requestCount)
	}
	if inputCount != len(texts) {
		t.Fatalf("sent %d inputs, want %d", inputCount, len(texts))
	}
}
