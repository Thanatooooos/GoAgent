package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/config"
)

func TestGetSystemSettingsDoesNotExposeProviderAPIKeys(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, &config.Config{
		AI: config.AIConfig{
			Providers: map[string]config.ProviderConfig{
				"openai": {
					Url:    "https://example.com",
					ApiKey: "super-secret",
					Endpoints: map[string]string{
						"chat": "/v1/chat/completions",
					},
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/rag/settings", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response data missing: %v", body)
	}
	ai, ok := data["ai"].(map[string]any)
	if !ok {
		t.Fatalf("response ai missing: %v", data)
	}
	providers, ok := ai["providers"].(map[string]any)
	if !ok {
		t.Fatalf("response providers missing: %v", ai)
	}
	openai, ok := providers["openai"].(map[string]any)
	if !ok {
		t.Fatalf("response provider missing: %v", providers)
	}
	if _, exists := openai["apiKey"]; exists {
		t.Fatalf("apiKey should not be exposed: %v", openai)
	}
}
