package knowledge_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

type wikiServiceStub struct {
	pages []domain.WikiPage
	total int
	err   error
	kbID  string
	graph domain.WikiGraph
}

func (s *wikiServiceStub) GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error) {
	if s.err != nil {
		return domain.WikiPage{}, s.err
	}
	s.kbID = kbID
	for _, p := range s.pages {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.WikiPage{}, nil
}

func (s *wikiServiceStub) ListByKB(ctx context.Context, kbID string, page, pageSize int) ([]domain.WikiPage, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	s.kbID = kbID
	return s.pages, s.total, nil
}

func (s *wikiServiceStub) GetGraph(ctx context.Context, kbID string) (domain.WikiGraph, error) {
	if s.err != nil {
		return domain.WikiGraph{}, s.err
	}
	s.kbID = kbID
	return s.graph, nil
}

func newWikiRouter(svc knowledgehttp.WikiPageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	router.Use(func(c *gin.Context) {
		contextx.Set(c, &contextx.LoginUser{UserID: "1", Username: "u", Role: "admin"})
		c.Next()
	})
	group := router.Group("/api/ragent")
	knowledgehttp.RegisterWikiPageRoutes(group, svc)
	return router
}

func TestAllKnowledgeRoutesRegisterWithoutConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/ragent")
	knowledgehttp.RegisterKnowledgeBaseRoutes(group, nil)
	knowledgehttp.RegisterKnowledgeDocumentRoutes(group, nil)
	knowledgehttp.RegisterKnowledgeChunkRoutes(group, nil)
	knowledgehttp.RegisterWikiPageRoutes(group, nil)
}

func TestWikiPageHandlerGetBySlug(t *testing.T) {
	svc := &wikiServiceStub{pages: []domain.WikiPage{{ID: "p1", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", Content: "content", InLinks: 2, OutLinks: 3}}}
	router := newWikiRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/kb1/wiki/pages/entity/go", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Slug     string `json:"slug"`
			InLinks  int    `json:"inLinks"`
			OutLinks int    `json:"outLinks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Slug != "entity/go" {
		t.Fatalf("data = %+v", result.Data)
	}
	if result.Data.InLinks != 2 || result.Data.OutLinks != 3 {
		t.Fatalf("link counts = %+v", result.Data)
	}
	if svc.kbID != "kb1" {
		t.Fatalf("kbID = %q, want kb1", svc.kbID)
	}
}

func TestWikiPageHandlerList(t *testing.T) {
	svc := &wikiServiceStub{pages: []domain.WikiPage{{ID: "p1", Slug: "entity/go"}}, total: 1}
	router := newWikiRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/kb1/wiki/pages?current=1&size=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Total != 1 {
		t.Fatalf("data = %+v", result.Data)
	}
	if svc.kbID != "kb1" {
		t.Fatalf("kbID = %q, want kb1", svc.kbID)
	}
}

func TestWikiPageHandlerGraph(t *testing.T) {
	svc := &wikiServiceStub{graph: domain.WikiGraph{Nodes: []domain.WikiGraphNode{
		{ID: "p1", Slug: "entity/go", Title: "Go"},
		{ID: "p2", Slug: "concept/并发", Title: "并发"},
	}}}
	router := newWikiRouter(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/kb1/wiki/graph", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Nodes []domain.WikiGraphNode `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Data.Nodes) != 2 {
		t.Fatalf("nodes = %+v, want 2", result.Data.Nodes)
	}
	if result.Data.Nodes[0].Slug != "entity/go" {
		t.Fatalf("first node = %+v", result.Data.Nodes[0])
	}
	if svc.kbID != "kb1" {
		t.Fatalf("kbID = %q, want kb1", svc.kbID)
	}
}
