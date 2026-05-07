package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientFetchDocumentContentSuccess(t *testing.T) {
	accessToken := "t-abc123"
	var tokenCallCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCallCount++
			json.NewEncoder(w).Encode(feishuTokenResponse{
				Code:              0,
				TenantAccessToken: accessToken,
				Expire:            7200,
			})
		case "/open-apis/docx/v1/documents/doc-123/raw_content":
			if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(feishuRawContentResponse{
				Code: 0,
				Data: struct {
					Content string `json:"content"`
				}{Content: "# hello world"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		appID:      "app-1",
		appSecret:  "secret-1",
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    server.URL + "/open-apis",
	}

	content, err := client.FetchDocumentContent(context.Background(), "doc-123")
	if err != nil {
		t.Fatalf("FetchDocumentContent() error = %v", err)
	}
	if string(content) != "# hello world" {
		t.Fatalf("unexpected content: %q", string(content))
	}
	if tokenCallCount != 1 {
		t.Fatalf("expected 1 token call, got %d", tokenCallCount)
	}
}

func TestClientFetchDocumentContentTokenCaching(t *testing.T) {
	var tokenCallCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCallCount++
			json.NewEncoder(w).Encode(feishuTokenResponse{
				Code:              0,
				TenantAccessToken: "t-cached",
				Expire:            7200,
			})
		case "/open-apis/docx/v1/documents/doc-1/raw_content":
			json.NewEncoder(w).Encode(feishuRawContentResponse{
				Code: 0,
				Data: struct {
					Content string `json:"content"`
				}{Content: "doc1"},
			})
		case "/open-apis/docx/v1/documents/doc-2/raw_content":
			json.NewEncoder(w).Encode(feishuRawContentResponse{
				Code: 0,
				Data: struct {
					Content string `json:"content"`
				}{Content: "doc2"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		appID:      "app-1",
		appSecret:  "secret-1",
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    server.URL + "/open-apis",
	}

	// 两次请求应共享缓存的 token。
	for _, docID := range []string{"doc-1", "doc-2"} {
		if _, err := client.FetchDocumentContent(context.Background(), docID); err != nil {
			t.Fatalf("FetchDocumentContent(%s) error = %v", docID, err)
		}
	}
	if tokenCallCount != 1 {
		t.Fatalf("expected 1 token call (cached), got %d", tokenCallCount)
	}
}

func TestClientFetchDocumentContentAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(feishuTokenResponse{
			Code: 99991672,
			Msg:  "app secret is invalid",
		})
	}))
	defer server.Close()

	client := &Client{
		appID:      "bad-app",
		appSecret:  "bad-secret",
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    server.URL + "/open-apis",
	}

	_, err := client.FetchDocumentContent(context.Background(), "doc-123")
	if err == nil {
		t.Fatal("expected error for invalid credentials, got nil")
	}
	if !strings.Contains(err.Error(), "app secret is invalid") {
		t.Fatalf("expected auth error message, got: %v", err)
	}
}

func TestClientFetchDocumentContentAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			json.NewEncoder(w).Encode(feishuTokenResponse{
				Code:              0,
				TenantAccessToken: "t-ok",
				Expire:            7200,
			})
		case "/open-apis/docx/v1/documents/doc-404/raw_content":
			json.NewEncoder(w).Encode(feishuRawContentResponse{
				Code: 1740001,
				Msg:  "document not found",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{
		appID:      "app-1",
		appSecret:  "secret-1",
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    server.URL + "/open-apis",
	}

	_, err := client.FetchDocumentContent(context.Background(), "doc-404")
	if err == nil {
		t.Fatal("expected error for not found document, got nil")
	}
	if !strings.Contains(err.Error(), "document not found") {
		t.Fatalf("expected 'document not found' in error, got: %v", err)
	}
}

func TestExtractDocumentID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://xxx.feishu.cn/docx/ABCD1234", "ABCD1234"},
		{"https://xxx.feishu.cn/wiki/XYZ5678", "XYZ5678"},
		{"https://xxx.feishu.cn/docx/ABCD1234?from=share", "ABCD1234"},
		{"ABCD1234", "ABCD1234"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := ExtractDocumentID(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractDocumentID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
