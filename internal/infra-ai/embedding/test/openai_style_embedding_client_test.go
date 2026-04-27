package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/embedding"
	"local/rag-project/internal/infra-ai/model"
)

func TestOpenAIStyleEmbeddingClientEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request failed: %v", err)
		}
		if payload["model"] != "demo-embedding" {
			t.Fatalf("unexpected model payload: %+v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	client := embedding.NewOpenAIStyleEmbeddingClient("test-provider", srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "demo-embedding"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"embedding": "/embeddings"},
		},
	}

	vector, err := client.Embed("hello", target)
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("unexpected vector length: %d", len(vector))
	}
}

func TestOpenAIStyleEmbeddingClientEmbedBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var payload struct {
			Input []string `json:"input"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request failed: %v", err)
		}

		data := make([]map[string]any, 0, len(payload.Input))
		for index := range payload.Input {
			data = append(data, map[string]any{
				"embedding": []float32{float32(index + 1)},
			})
		}

		w.Header().Set("Content-Type", "application/json")
		resp, _ := json.Marshal(map[string]any{"data": data})
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	client := embedding.NewOllamaEmbeddingClient(srv.Client(), embedding.WithMaxBatchSize(2))
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "demo-embedding"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			Endpoints: map[string]string{"embedding": "/embeddings"},
		},
	}

	vectors, err := client.EmbedBatch([]string{"a", "b", "c"}, target)
	if err != nil {
		t.Fatalf("EmbedBatch returned error: %v", err)
	}
	if len(vectors) != 3 {
		t.Fatalf("unexpected vector count: %d", len(vectors))
	}
}
