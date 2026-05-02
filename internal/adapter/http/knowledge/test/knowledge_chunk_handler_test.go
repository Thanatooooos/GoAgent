package knowledge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

type knowledgeChunkServiceStub struct {
	pageFn               func(ctx context.Context, input service.PageKnowledgeChunkInput) (service.KnowledgeChunkPageResult, error)
	createFn             func(ctx context.Context, input service.CreateKnowledgeChunkInput) (domain.KnowledgeChunk, error)
	updateFn             func(ctx context.Context, input service.UpdateKnowledgeChunkInput) error
	deleteFn             func(ctx context.Context, input service.DeleteKnowledgeChunkInput) error
	enableFn             func(ctx context.Context, input service.EnableKnowledgeChunkInput) error
	batchToggleEnabledFn func(ctx context.Context, input service.BatchToggleKnowledgeChunksInput) error
}

func (s knowledgeChunkServiceStub) Page(ctx context.Context, input service.PageKnowledgeChunkInput) (service.KnowledgeChunkPageResult, error) {
	if s.pageFn != nil {
		return s.pageFn(ctx, input)
	}
	return service.KnowledgeChunkPageResult{}, nil
}

func (s knowledgeChunkServiceStub) Create(ctx context.Context, input service.CreateKnowledgeChunkInput) (domain.KnowledgeChunk, error) {
	if s.createFn != nil {
		return s.createFn(ctx, input)
	}
	return domain.KnowledgeChunk{}, nil
}

func (s knowledgeChunkServiceStub) Update(ctx context.Context, input service.UpdateKnowledgeChunkInput) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, input)
	}
	return nil
}

func (s knowledgeChunkServiceStub) Delete(ctx context.Context, input service.DeleteKnowledgeChunkInput) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, input)
	}
	return nil
}

func (s knowledgeChunkServiceStub) Enable(ctx context.Context, input service.EnableKnowledgeChunkInput) error {
	if s.enableFn != nil {
		return s.enableFn(ctx, input)
	}
	return nil
}

func (s knowledgeChunkServiceStub) BatchToggleEnabled(ctx context.Context, input service.BatchToggleKnowledgeChunksInput) error {
	if s.batchToggleEnabledFn != nil {
		return s.batchToggleEnabledFn(ctx, input)
	}
	return nil
}

func TestKnowledgeChunkHandlerPageMatchesRagentIPageShape(t *testing.T) {
	router := newKnowledgeChunkRouter(knowledgeChunkServiceStub{
		pageFn: func(ctx context.Context, input service.PageKnowledgeChunkInput) (service.KnowledgeChunkPageResult, error) {
			if input.DocumentID != "doc-1" || input.Page != 2 || input.PageSize != 5 {
				t.Fatalf("unexpected page input: %+v", input)
			}
			if input.Enabled == nil || !*input.Enabled {
				t.Fatalf("unexpected enabled filter: %+v", input.Enabled)
			}
			return service.KnowledgeChunkPageResult{
				Items: []domain.KnowledgeChunk{{
					ID:              "chunk-1",
					KnowledgeBaseID: "kb-1",
					DocumentID:      "doc-1",
					ChunkIndex:      0,
					Content:         "demo",
					Enabled:         true,
				}},
				Total:    6,
				Page:     2,
				PageSize: 5,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/docs/doc-1/chunks?current=2&size=5&enabled=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Records []struct {
				ID      string `json:"id"`
				Content string `json:"content"`
			} `json:"records"`
			Total int `json:"total"`
			Pages int `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.Total != 6 || result.Data.Pages != 2 {
		t.Fatalf("unexpected page response: %+v", result.Data)
	}
	if len(result.Data.Records) != 1 || result.Data.Records[0].ID != "chunk-1" || result.Data.Records[0].Content != "demo" {
		t.Fatalf("unexpected records: %+v", result.Data.Records)
	}
}

func TestKnowledgeChunkHandlerCreate(t *testing.T) {
	router := newKnowledgeChunkRouter(knowledgeChunkServiceStub{
		createFn: func(ctx context.Context, input service.CreateKnowledgeChunkInput) (domain.KnowledgeChunk, error) {
			if input.DocumentID != "doc-1" || input.Content != "hello" || input.OperatorID != "alice" {
				t.Fatalf("unexpected create input: %+v", input)
			}
			return domain.KnowledgeChunk{
				ID:              "chunk-1",
				KnowledgeBaseID: "kb-1",
				DocumentID:      "doc-1",
				ChunkIndex:      1,
				Content:         "hello",
				Enabled:         true,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/knowledge-base/docs/doc-1/chunks", strings.NewReader(`{"content":"hello","index":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeChunkHandlerBatchEnable(t *testing.T) {
	router := newKnowledgeChunkRouter(knowledgeChunkServiceStub{
		batchToggleEnabledFn: func(ctx context.Context, input service.BatchToggleKnowledgeChunksInput) error {
			if input.DocumentID != "doc-1" || !input.Enabled || input.OperatorID != "alice" {
				t.Fatalf("unexpected batch input: %+v", input)
			}
			if len(input.ChunkIDs) != 2 || input.ChunkIDs[0] != "c1" || input.ChunkIDs[1] != "c2" {
				t.Fatalf("unexpected chunk ids: %+v", input.ChunkIDs)
			}
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/ragent/knowledge-base/docs/doc-1/chunks/batch-enable?value=true", strings.NewReader(`{"chunkIds":["c1","c2"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func newKnowledgeChunkRouter(service knowledgehttp.KnowledgeChunkService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	router.Use(func(c *gin.Context) {
		userID := firstNonEmptyChunk(c.GetHeader("X-User-ID"), "1")
		contextx.Set(c, &contextx.LoginUser{
			UserID:   userID,
			Username: userID,
			Role:     "admin",
		})
		c.Next()
	})
	group := router.Group("/api/ragent")
	knowledgehttp.RegisterKnowledgeChunkRoutes(group, service)
	return router
}

func firstNonEmptyChunk(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
