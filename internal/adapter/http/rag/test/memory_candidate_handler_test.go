package rag_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	raghttp "local/rag-project/internal/adapter/http/rag"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/middleware"
)

type preferenceCandidateServiceStub struct {
	listFn    func(context.Context, longtermmemory.ListPreferenceCandidatesInput) (longtermmemory.PreferenceCandidatePageResult, error)
	confirmFn func(context.Context, longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error)
	rejectFn  func(context.Context, longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error)
}

func (s preferenceCandidateServiceStub) ListPendingPreferenceCandidates(ctx context.Context, input longtermmemory.ListPreferenceCandidatesInput) (longtermmemory.PreferenceCandidatePageResult, error) {
	if s.listFn != nil {
		return s.listFn(ctx, input)
	}
	return longtermmemory.PreferenceCandidatePageResult{}, nil
}

func (s preferenceCandidateServiceStub) ConfirmPreferenceCandidate(ctx context.Context, input longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error) {
	if s.confirmFn != nil {
		return s.confirmFn(ctx, input)
	}
	return longtermmemory.PreferenceCandidate{}, nil
}

func (s preferenceCandidateServiceStub) RejectPreferenceCandidate(ctx context.Context, input longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error) {
	if s.rejectFn != nil {
		return s.rejectFn(ctx, input)
	}
	return longtermmemory.PreferenceCandidate{}, nil
}

func TestMemoryCandidateHandlerListPendingMatchesIPageShape(t *testing.T) {
	router := newMemoryCandidateRouter(preferenceCandidateServiceStub{
		listFn: func(_ context.Context, input longtermmemory.ListPreferenceCandidatesInput) (longtermmemory.PreferenceCandidatePageResult, error) {
			if input.UserID != "user-1" || input.Page != 2 || input.PageSize != 5 {
				t.Fatalf("unexpected list input: %+v", input)
			}
			return longtermmemory.PreferenceCandidatePageResult{
				Items: []longtermmemory.PreferenceCandidate{{
					ID:               "cand-1",
					ScopeType:        domain.MemoryScopeGlobal,
					MemoryType:       domain.MemoryTypePreference,
					CanonicalKey:     "workflow.troubleshooting.first_step",
					Summary:          "遇到问题先看日志",
					Content:          "先看错误日志",
					SourceMessageID:  "msg-1",
					ExtractionMethod: domain.MemoryExtractionMethodLLM,
					Confidence:       0.93,
					Status:           domain.MemoryStatusPending,
				}},
				Total:    1,
				Page:     2,
				PageSize: 5,
			}, nil
		},
	}, true)

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/preferences/candidates/pending?current=2&size=5", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Records []struct {
				ID               string  `json:"id"`
				ScopeType        string  `json:"scopeType"`
				MemoryType       string  `json:"memoryType"`
				CanonicalKey     string  `json:"canonicalKey"`
				Summary          string  `json:"summary"`
				Content          string  `json:"content"`
				SourceMessageID  string  `json:"sourceMessageId"`
				ExtractionMethod string  `json:"extractionMethod"`
				Confidence       float64 `json:"confidence"`
				Status           string  `json:"status"`
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
	if result.Code != "0" || result.Data.Total != 1 || result.Data.Size != 5 || result.Data.Current != 2 || result.Data.Pages != 1 {
		t.Fatalf("unexpected page response: %+v", result.Data)
	}
	if len(result.Data.Records) != 1 || result.Data.Records[0].ID != "cand-1" || result.Data.Records[0].Status != domain.MemoryStatusPending {
		t.Fatalf("unexpected candidate records: %+v", result.Data.Records)
	}
}

func TestMemoryCandidateHandlerConfirmReturnsCandidate(t *testing.T) {
	router := newMemoryCandidateRouter(preferenceCandidateServiceStub{
		confirmFn: func(_ context.Context, input longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error) {
			if input.UserID != "user-1" || input.CandidateID != "cand-1" {
				t.Fatalf("unexpected confirm input: %+v", input)
			}
			return longtermmemory.PreferenceCandidate{
				ID:               "cand-1",
				ScopeType:        domain.MemoryScopeGlobal,
				MemoryType:       domain.MemoryTypePreference,
				CanonicalKey:     "response.language",
				Summary:          "以后都用中文回答",
				Content:          "以后都用中文回答",
				SourceMessageID:  "msg-2",
				ExtractionMethod: domain.MemoryExtractionMethodLLM,
				Confidence:       0.99,
				Status:           domain.MemoryStatusActive,
			}, nil
		},
	}, true)

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/preferences/candidates/cand-1/confirm", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			ID           string `json:"id"`
			CanonicalKey string `json:"canonicalKey"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.ID != "cand-1" || result.Data.CanonicalKey != "response.language" || result.Data.Status != domain.MemoryStatusActive {
		t.Fatalf("unexpected confirm response: %+v", result)
	}
}

func TestMemoryCandidateHandlerRejectReturnsCandidate(t *testing.T) {
	router := newMemoryCandidateRouter(preferenceCandidateServiceStub{
		rejectFn: func(_ context.Context, input longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error) {
			if input.UserID != "user-1" || input.CandidateID != "cand-2" {
				t.Fatalf("unexpected reject input: %+v", input)
			}
			return longtermmemory.PreferenceCandidate{
				ID:               "cand-2",
				ScopeType:        domain.MemoryScopeGlobal,
				MemoryType:       domain.MemoryTypePreference,
				CanonicalKey:     "behavior.avoid",
				Summary:          "避免寒暄式开头",
				Content:          "不要寒暄",
				SourceMessageID:  "msg-3",
				ExtractionMethod: domain.MemoryExtractionMethodLLM,
				Confidence:       0.88,
				Status:           domain.MemoryStatusRejected,
			}, nil
		},
	}, true)

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/preferences/candidates/cand-2/reject", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.ID != "cand-2" || result.Data.Status != domain.MemoryStatusRejected {
		t.Fatalf("unexpected reject response: %+v", result)
	}
}

func TestMemoryCandidateHandlerListRequiresLoginUser(t *testing.T) {
	router := newMemoryCandidateRouter(preferenceCandidateServiceStub{}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/preferences/candidates/pending", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "A000001" || result.Message != "unauthorized" {
		t.Fatalf("unexpected unauthorized response: %+v", result)
	}
}

func TestMemoryCandidateHandlerConfirmSurfacesStableClientError(t *testing.T) {
	router := newMemoryCandidateRouter(preferenceCandidateServiceStub{
		confirmFn: func(context.Context, longtermmemory.DecidePreferenceCandidateInput) (longtermmemory.PreferenceCandidate, error) {
			return longtermmemory.PreferenceCandidate{}, exception.NewClientException("preference candidate not found", nil)
		},
	}, true)

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/preferences/candidates/cand-missing/confirm", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "A000001" || result.Message != "preference candidate not found" {
		t.Fatalf("unexpected error response: %+v", result)
	}
}

func newMemoryCandidateRouter(service longtermmemory.PreferenceCandidateService, withLogin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	if withLogin {
		router.Use(func(c *gin.Context) {
			contextx.Set(c, &contextx.LoginUser{
				UserID:   "user-1",
				Username: "tester",
				Role:     "admin",
			})
			c.Next()
		})
	}

	handler := raghttp.NewHandler(nil, nil, nil, nil, nil, service)
	group := router.Group("/api/ragent")
	group.GET("/rag/v3/preferences/candidates/pending", handler.ListPendingPreferenceCandidates)
	group.POST("/rag/v3/preferences/candidates/:candidateId/confirm", handler.ConfirmPreferenceCandidate)
	group.POST("/rag/v3/preferences/candidates/:candidateId/reject", handler.RejectPreferenceCandidate)
	return router
}
