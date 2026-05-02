package knowledge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	knowledgehttp "local/rag-project/internal/adapter/http/knowledge"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/middleware"
)

type knowledgeDocumentServiceStub struct {
	uploadFn            func(ctx context.Context, input service.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	startChunkFn        func(ctx context.Context, input service.StartChunkKnowledgeDocumentInput) error
	getFn               func(ctx context.Context, input service.GetKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	updateFn            func(ctx context.Context, input service.UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	pageFn              func(ctx context.Context, input service.PageKnowledgeDocumentInput) (service.KnowledgeDocumentPageResult, error)
	searchFn            func(ctx context.Context, input service.SearchKnowledgeDocumentsInput) ([]service.KnowledgeDocumentSearchItem, error)
	enableFn            func(ctx context.Context, input service.EnableKnowledgeDocumentInput) error
	deleteFn            func(ctx context.Context, input service.DeleteKnowledgeDocumentInput) error
	pageChunkLogsFn     func(ctx context.Context, input service.KnowledgeDocumentChunkLogPageInput) (service.KnowledgeDocumentChunkLogPageResult, error)
	pageScheduleExecsFn func(ctx context.Context, input service.PageKnowledgeDocumentScheduleExecInput) (service.KnowledgeDocumentScheduleExecPageResult, error)
}

func (s knowledgeDocumentServiceStub) Upload(ctx context.Context, input service.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s.uploadFn != nil {
		return s.uploadFn(ctx, input)
	}
	return domain.KnowledgeDocument{}, nil
}
func (s knowledgeDocumentServiceStub) StartChunk(ctx context.Context, input service.StartChunkKnowledgeDocumentInput) error {
	if s.startChunkFn != nil {
		return s.startChunkFn(ctx, input)
	}
	return nil
}
func (s knowledgeDocumentServiceStub) Get(ctx context.Context, input service.GetKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s.getFn != nil {
		return s.getFn(ctx, input)
	}
	return domain.KnowledgeDocument{}, nil
}
func (s knowledgeDocumentServiceStub) Update(ctx context.Context, input service.UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, input)
	}
	return domain.KnowledgeDocument{}, nil
}
func (s knowledgeDocumentServiceStub) Page(ctx context.Context, input service.PageKnowledgeDocumentInput) (service.KnowledgeDocumentPageResult, error) {
	if s.pageFn != nil {
		return s.pageFn(ctx, input)
	}
	return service.KnowledgeDocumentPageResult{}, nil
}
func (s knowledgeDocumentServiceStub) Search(ctx context.Context, input service.SearchKnowledgeDocumentsInput) ([]service.KnowledgeDocumentSearchItem, error) {
	if s.searchFn != nil {
		return s.searchFn(ctx, input)
	}
	return nil, nil
}
func (s knowledgeDocumentServiceStub) Enable(ctx context.Context, input service.EnableKnowledgeDocumentInput) error {
	if s.enableFn != nil {
		return s.enableFn(ctx, input)
	}
	return nil
}
func (s knowledgeDocumentServiceStub) Delete(ctx context.Context, input service.DeleteKnowledgeDocumentInput) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, input)
	}
	return nil
}
func (s knowledgeDocumentServiceStub) PageChunkLogs(ctx context.Context, input service.KnowledgeDocumentChunkLogPageInput) (service.KnowledgeDocumentChunkLogPageResult, error) {
	if s.pageChunkLogsFn != nil {
		return s.pageChunkLogsFn(ctx, input)
	}
	return service.KnowledgeDocumentChunkLogPageResult{}, nil
}

func (s knowledgeDocumentServiceStub) PageScheduleExecs(ctx context.Context, input service.PageKnowledgeDocumentScheduleExecInput) (service.KnowledgeDocumentScheduleExecPageResult, error) {
	if s.pageScheduleExecsFn != nil {
		return s.pageScheduleExecsFn(ctx, input)
	}
	return service.KnowledgeDocumentScheduleExecPageResult{}, nil
}

func TestKnowledgeDocumentHandlerUploadMatchesRagentContract(t *testing.T) {
	router := newKnowledgeDocumentRouter(knowledgeDocumentServiceStub{
		uploadFn: func(ctx context.Context, input service.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
			if input.KnowledgeBaseID != "kb-1" || input.SourceType != "file" || input.FileName != "hello.txt" {
				t.Fatalf("unexpected upload input: %+v", input)
			}
			if input.OperatorID != "alice" {
				t.Fatalf("unexpected operator id: %q", input.OperatorID)
			}
			return domain.KnowledgeDocument{
				ID:              "doc-1",
				KnowledgeBaseID: "kb-1",
				Name:            "hello.txt",
				Enabled:         true,
				Status:          "pending",
				SourceType:      "file",
			}, nil
		},
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("sourceType", "file")
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/knowledge-base/kb-1/docs/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			ID      string `json:"id"`
			KbID    string `json:"kbId"`
			DocName string `json:"docName"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.ID != "doc-1" || result.Data.KbID != "kb-1" || result.Data.DocName != "hello.txt" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestKnowledgeDocumentHandlerPageMatchesRagentIPageShape(t *testing.T) {
	router := newKnowledgeDocumentRouter(knowledgeDocumentServiceStub{
		pageFn: func(ctx context.Context, input service.PageKnowledgeDocumentInput) (service.KnowledgeDocumentPageResult, error) {
			if input.KnowledgeBaseID != "kb-1" || input.Page != 2 || input.PageSize != 5 || input.Status != "success" || input.Query != "demo" {
				t.Fatalf("unexpected page input: %+v", input)
			}
			return service.KnowledgeDocumentPageResult{
				Items: []domain.KnowledgeDocument{{
					ID:              "doc-1",
					KnowledgeBaseID: "kb-1",
					Name:            "demo.md",
					Enabled:         true,
					Status:          "success",
					ChunkCount:      3,
				}},
				Total:    6,
				Page:     2,
				PageSize: 5,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/kb-1/docs?current=2&size=5&status=success&keyword=demo", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			Records []struct {
				ID         string `json:"id"`
				ChunkCount int    `json:"chunkCount"`
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
	if result.Code != "0" || result.Data.Total != 6 || result.Data.Size != 5 || result.Data.Current != 2 || result.Data.Pages != 2 {
		t.Fatalf("unexpected page response: %+v", result.Data)
	}
	if len(result.Data.Records) != 1 || result.Data.Records[0].ID != "doc-1" || result.Data.Records[0].ChunkCount != 3 {
		t.Fatalf("unexpected records: %+v", result.Data.Records)
	}
}

func TestKnowledgeDocumentHandlerStartChunk(t *testing.T) {
	router := newKnowledgeDocumentRouter(knowledgeDocumentServiceStub{
		startChunkFn: func(ctx context.Context, input service.StartChunkKnowledgeDocumentInput) error {
			if input.DocumentID != "doc-1" || input.OperatorID != "alice" {
				t.Fatalf("unexpected start chunk input: %+v", input)
			}
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ragent/knowledge-base/docs/doc-1/chunk", nil)
	req.Header.Set("X-User-ID", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestKnowledgeDocumentHandlerScheduleExecsPageShape(t *testing.T) {
	router := newKnowledgeDocumentRouter(knowledgeDocumentServiceStub{
		pageScheduleExecsFn: func(ctx context.Context, input service.PageKnowledgeDocumentScheduleExecInput) (service.KnowledgeDocumentScheduleExecPageResult, error) {
			if input.DocumentID != "doc-1" || input.Page != 2 || input.PageSize != 5 || input.Status != "failed" {
				t.Fatalf("unexpected schedule exec input: %+v", input)
			}
			return service.KnowledgeDocumentScheduleExecPageResult{
				Items: []domain.KnowledgeDocumentScheduleExec{{
					ID:         "exec-1",
					ScheduleID: "schedule-1",
					DocumentID: "doc-1",
					Status:     domain.KnowledgeDocumentScheduleRunStatusFailed,
					Message:    "fetch failed",
				}},
				Total:    6,
				Page:     2,
				PageSize: 5,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/docs/doc-1/schedule-execs?current=2&size=5&status=failed", nil)
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
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"records"`
			Total   int `json:"total"`
			Current int `json:"current"`
			Pages   int `json:"pages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.Total != 6 || result.Data.Current != 2 || result.Data.Pages != 2 {
		t.Fatalf("unexpected schedule exec page response: %+v", result.Data)
	}
	if len(result.Data.Records) != 1 || result.Data.Records[0].ID != "exec-1" || result.Data.Records[0].Status != "failed" {
		t.Fatalf("unexpected schedule exec records: %+v", result.Data.Records)
	}
}

func newKnowledgeDocumentRouter(service knowledgehttp.KnowledgeDocumentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	router.Use(func(c *gin.Context) {
		userID := firstNonEmptyDocument(c.GetHeader("X-User-ID"), "1")
		contextx.Set(c, &contextx.LoginUser{
			UserID:   userID,
			Username: userID,
			Role:     "admin",
		})
		c.Next()
	})
	group := router.Group("/api/ragent")
	knowledgehttp.RegisterKnowledgeDocumentRoutes(group, service)
	return router
}

func firstNonEmptyDocument(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
