package knowledge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

type knowledgeBaseServiceStub struct {
	createFn func(ctx context.Context, input service.CreateKnowledgeBaseInput) (domain.KnowledgeBase, error)
	updateFn func(ctx context.Context, input service.UpdateKnowledgeBaseInput) (domain.KnowledgeBase, error)
	deleteFn func(ctx context.Context, input service.DeleteKnowledgeBaseInput) error
	getFn    func(ctx context.Context, input service.GetKnowledgeBaseInput) (domain.KnowledgeBase, error)
	pageFn   func(ctx context.Context, input service.PageKnowledgeBaseInput) (service.KnowledgeBasePageResult, error)
}

func (s knowledgeBaseServiceStub) Create(ctx context.Context, input service.CreateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	if s.createFn != nil {
		return s.createFn(ctx, input)
	}
	return domain.KnowledgeBase{}, nil
}

func (s knowledgeBaseServiceStub) Update(ctx context.Context, input service.UpdateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, input)
	}
	return domain.KnowledgeBase{}, nil
}

func (s knowledgeBaseServiceStub) Delete(ctx context.Context, input service.DeleteKnowledgeBaseInput) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, input)
	}
	return nil
}

func (s knowledgeBaseServiceStub) Get(ctx context.Context, input service.GetKnowledgeBaseInput) (domain.KnowledgeBase, error) {
	if s.getFn != nil {
		return s.getFn(ctx, input)
	}
	return domain.KnowledgeBase{}, nil
}

func (s knowledgeBaseServiceStub) Page(ctx context.Context, input service.PageKnowledgeBaseInput) (service.KnowledgeBasePageResult, error) {
	if s.pageFn != nil {
		return s.pageFn(ctx, input)
	}
	return service.KnowledgeBasePageResult{}, nil
}

func TestKnowledgeBaseHandlerCreateMatchesRagentContract(t *testing.T) {
	router := newKnowledgeBaseRouter(knowledgeBaseServiceStub{
		createFn: func(ctx context.Context, input service.CreateKnowledgeBaseInput) (domain.KnowledgeBase, error) {
			if input.Name != "Docs" || input.EmbeddingModel != "emb" || input.CollectionName != "docs" {
				t.Fatalf("unexpected create input: %+v", input)
			}
			if input.OperatorID != "alice" {
				t.Fatalf("unexpected operator id: %q", input.OperatorID)
			}
			return domain.KnowledgeBase{ID: "kb-1"}, nil
		},
	})

	body := bytes.NewBufferString(`{"name":"Docs","embeddingModel":"emb","collectionName":"docs"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/ragent/knowledge-base", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data != "kb-1" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestKnowledgeBaseHandlerPageMatchesRagentIPageShape(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	router := newKnowledgeBaseRouter(knowledgeBaseServiceStub{
		pageFn: func(ctx context.Context, input service.PageKnowledgeBaseInput) (service.KnowledgeBasePageResult, error) {
			if input.Page != 2 || input.PageSize != 5 || input.Query != "Docs" {
				t.Fatalf("unexpected page input: %+v", input)
			}
			return service.KnowledgeBasePageResult{
				Items: []domain.KnowledgeBase{{
					ID:             "kb-1",
					Name:           "Docs",
					EmbeddingModel: "emb",
					CollectionName: "docs",
					CreatedBy:      "alice",
					CreatedAt:      now,
					UpdatedAt:      now,
				}},
				DocumentCounts: map[string]int{"kb-1": 3},
				Total:          11,
				Page:           2,
				PageSize:       5,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base?current=2&size=5&name=Docs", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Records []struct {
				ID            string `json:"id"`
				DocumentCount int    `json:"documentCount"`
			} `json:"records"`
			Total   int `json:"total"`
			Size    int `json:"size"`
			Current int `json:"current"`
			Pages   int `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.Total != 11 || result.Data.Size != 5 || result.Data.Current != 2 || result.Data.Pages != 3 {
		t.Fatalf("unexpected page response: %+v", result.Data)
	}
	if len(result.Data.Records) != 1 || result.Data.Records[0].ID != "kb-1" || result.Data.Records[0].DocumentCount != 3 {
		t.Fatalf("unexpected records: %+v", result.Data.Records)
	}
}

func TestKnowledgeBaseHandlerChunkStrategies(t *testing.T) {
	router := newKnowledgeBaseRouter(knowledgeBaseServiceStub{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/chunk-strategies", nil)

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data []struct {
			Value         string         `json:"value"`
			DefaultConfig map[string]int `json:"defaultConfig"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || len(result.Data) != 2 || result.Data[0].Value != "fixed_size" || result.Data[1].Value != "structure_aware" {
		t.Fatalf("unexpected strategies: %+v", result.Data)
	}
	if result.Data[0].DefaultConfig["chunkSize"] != 512 {
		t.Fatalf("unexpected fixed_size config: %+v", result.Data[0].DefaultConfig)
	}
}

func newKnowledgeBaseRouter(service knowledgehttp.KnowledgeBaseService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	router.Use(testAdminUserMiddleware())
	group := router.Group("/api/ragent")
	knowledgehttp.RegisterKnowledgeBaseRoutes(group, service)
	return router
}

func testAdminUserMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := firstNonEmptyBase(c.GetHeader("X-User-ID"), "1")
		contextx.Set(c, &contextx.LoginUser{
			UserID:   userID,
			Username: userID,
			Role:     "admin",
		})
		c.Next()
	}
}

func firstNonEmptyBase(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
