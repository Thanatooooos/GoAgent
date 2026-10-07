package rag_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	raghttp "local/rag-project/internal/adapter/http/rag"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

type memoryRepoStub struct {
	createFn              func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	updateFn              func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	getByID               func(context.Context, string) (domain.MemoryItem, error)
	listFn                func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error)
	countFn               func(context.Context, port.MemoryItemListFilter) (int64, error)
	listActiveByKeyFn     func(context.Context, string, string, string, string) ([]domain.MemoryItem, error)
	listActiveConflictsFn func(context.Context, []string) ([]port.ActiveMemoryConflict, error)
	touchFn               func(context.Context, string, []string, time.Time) error
	expireByIDsFn         func(context.Context, []string, string, time.Time) (int64, error)
	deleteBeforeFn        func(context.Context, []string, time.Time, int) (int64, error)
}

func (s memoryRepoStub) Create(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	if s.createFn != nil {
		return s.createFn(ctx, item)
	}
	return item, nil
}

func (s memoryRepoStub) Update(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, item)
	}
	return item, nil
}

func (s memoryRepoStub) GetByID(ctx context.Context, id string) (domain.MemoryItem, error) {
	if s.getByID != nil {
		return s.getByID(ctx, id)
	}
	return domain.MemoryItem{}, nil
}

func (s memoryRepoStub) List(ctx context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

func (s memoryRepoStub) Count(ctx context.Context, filter port.MemoryItemListFilter) (int64, error) {
	if s.countFn != nil {
		return s.countFn(ctx, filter)
	}
	return 0, nil
}

func (s memoryRepoStub) ListActiveByCanonicalKey(ctx context.Context, userID string, scopeType string, scopeID string, canonicalKey string) ([]domain.MemoryItem, error) {
	if s.listActiveByKeyFn != nil {
		return s.listActiveByKeyFn(ctx, userID, scopeType, scopeID, canonicalKey)
	}
	if s.listFn == nil {
		return nil, nil
	}
	filter := port.MemoryItemListFilter{
		UserID:        userID,
		ScopeTypes:    []string{scopeType},
		CanonicalKeys: []string{canonicalKey},
		Statuses:      []string{domain.MemoryStatusActive},
	}
	if strings.TrimSpace(scopeType) == domain.MemoryScopeKB {
		filter.ScopeIDs = []string{scopeID}
	}
	return s.listFn(ctx, filter)
}

func (s memoryRepoStub) ListActiveSingleValueConflicts(ctx context.Context, canonicalKeys []string) ([]port.ActiveMemoryConflict, error) {
	if s.listActiveConflictsFn != nil {
		return s.listActiveConflictsFn(ctx, canonicalKeys)
	}
	return nil, nil
}

func (s memoryRepoStub) TouchLastUsed(ctx context.Context, userID string, ids []string, at time.Time) error {
	if s.touchFn != nil {
		return s.touchFn(ctx, userID, ids, at)
	}
	return nil
}

func (s memoryRepoStub) ExpireByIDs(ctx context.Context, ids []string, updatedBy string, at time.Time) (int64, error) {
	if s.expireByIDsFn != nil {
		return s.expireByIDsFn(ctx, ids, updatedBy, at)
	}
	return 0, nil
}

func (s memoryRepoStub) DeleteByStatusesUpdatedBefore(ctx context.Context, statuses []string, updatedBefore time.Time, limit int) (int64, error) {
	if s.deleteBeforeFn != nil {
		return s.deleteBeforeFn(ctx, statuses, updatedBefore, limit)
	}
	return 0, nil
}

func TestMemoryHandlerRememberAcceptsLegacyRequestShape(t *testing.T) {
	service := longtermmemory.NewMemoryService(memoryRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			item.ID = "mem-1"
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, longtermmemory.MemoryServiceOptions{})
	router := newMemoryRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/remember", bytes.NewBufferString(`{"content":"项目使用自定义 chunker"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			ID         string `json:"id"`
			MemoryType string `json:"memoryType"`
			Category   string `json:"category"`
			Namespace  string `json:"namespace"`
			ValueType  string `json:"valueType"`
			ValueJSON  string `json:"valueJson"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.ID != "mem-1" {
		t.Fatalf("unexpected response: %+v", result)
	}
	if result.Data.MemoryType != domain.MemoryTypeKnowledge || result.Data.Category != domain.MemoryCategoryGeneral {
		t.Fatalf("unexpected normalized defaults: %+v", result.Data)
	}
	if result.Data.Namespace != "global:global" || result.Data.ValueType != domain.MemoryValueTypeText || result.Data.ValueJSON != "项目使用自定义 chunker" {
		t.Fatalf("unexpected governance fields: %+v", result.Data)
	}
}

func TestMemoryHandlerRememberAcceptsGovernanceFields(t *testing.T) {
	var created domain.MemoryItem
	service := longtermmemory.NewMemoryService(memoryRepoStub{
		createFn: func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
			created = item
			item.ID = "mem-2"
			return item, nil
		},
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn:   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) { return nil, nil },
	}, longtermmemory.MemoryServiceOptions{})
	router := newMemoryRouter(service)

	body := `{"scopeType":"kb","scopeId":"kb-1","memoryType":"knowledge","category":"project","canonicalKey":"project.integrations","valueType":"text","valueJson":"github","displayValue":"GitHub","importance":70,"content":"项目集成 GitHub"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/remember", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if created.ScopeType != domain.MemoryScopeKB || created.ScopeID != "kb-1" {
		t.Fatalf("unexpected scope: %+v", created)
	}
	if created.CanonicalKey != "project.integrations" || created.Category != domain.MemoryCategoryProject {
		t.Fatalf("unexpected key/category: %+v", created)
	}
	if created.ValueJSON != "github" || created.DisplayValue != "GitHub" || created.Importance != 70 {
		t.Fatalf("unexpected governance payload: %+v", created)
	}
}

func TestMemoryHandlerListMemoriesSupportsCategoryAndCanonicalKeyFilters(t *testing.T) {
	service := longtermmemory.NewMemoryService(memoryRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.Categories) != 1 || filter.Categories[0] != domain.MemoryCategoryProject {
				t.Fatalf("unexpected category filter: %+v", filter)
			}
			if len(filter.CanonicalKeys) != 1 || filter.CanonicalKeys[0] != "project.integrations" {
				t.Fatalf("unexpected canonical key filter: %+v", filter)
			}
			if len(filter.Statuses) != 1 || filter.Statuses[0] != domain.MemoryStatusSuperseded {
				t.Fatalf("unexpected status filter: %+v", filter)
			}
			return []domain.MemoryItem{{ID: "mem-1", Category: domain.MemoryCategoryProject, CanonicalKey: "project.integrations", Status: domain.MemoryStatusSuperseded}}, nil
		},
	}, longtermmemory.MemoryServiceOptions{})
	router := newMemoryRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/memories?category=project&canonicalKey=project.integrations&status=superseded", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data []struct {
			ID           string `json:"id"`
			Category     string `json:"category"`
			CanonicalKey string `json:"canonicalKey"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || len(result.Data) != 1 || result.Data[0].ID != "mem-1" {
		t.Fatalf("unexpected list response: %+v", result)
	}
}

func newMemoryRouter(memoryService *longtermmemory.MemoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	router.Use(func(c *gin.Context) {
		contextx.Set(c, &contextx.LoginUser{
			UserID:   "user-1",
			Username: "tester",
			Role:     "admin",
		})
		c.Next()
	})

	handler := raghttp.NewHandler(nil, nil, memoryService, nil, nil, nil)
	group := router.Group("/api/ragent")
	group.GET("/rag/v3/memories", handler.ListMemories)
	group.POST("/rag/v3/remember", handler.Remember)
	return router
}
