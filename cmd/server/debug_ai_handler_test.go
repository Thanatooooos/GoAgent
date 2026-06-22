package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/contextx"
	infraai "local/rag-project/internal/infra-ai"
)

func newDebugTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// 注入 admin 用户，使 debug 路由认证通过
	router.Use(func(c *gin.Context) {
		contextx.Set(c, &contextx.LoginUser{UserID: "test-admin", Username: "admin", Role: "admin"})
		c.Next()
	})
	return router
}

func TestRegisterDebugAIRoutesRuntime(t *testing.T) {
	loadServerTestConfig(t)
	runtime := infraai.NewRuntime()

	router := newDebugTestRouter()
	registerDebugAIRoutes(router, runtime)

	req := httptest.NewRequest(http.MethodGet, "/debug/ai/runtime", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestRegisterDebugAIRoutesChatBadRequest(t *testing.T) {
	loadServerTestConfig(t)
	runtime := infraai.NewRuntime()

	router := newDebugTestRouter()
	registerDebugAIRoutes(router, runtime)

	req := httptest.NewRequest(http.MethodPost, "/debug/ai/chat", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func loadServerTestConfig(t *testing.T) {
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
