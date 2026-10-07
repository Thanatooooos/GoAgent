package parser_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	parser "local/rag-project/internal/app/core/parser"
)

func TestHTTPOCRClientStatuses(t *testing.T) {
	for _, tc := range []struct {
		response string
		status   string
		text     string
	}{
		{`{"status":"success","text":"Hello"}`, "success", "Hello"},
		{`{"status":"no_text","text":""}`, "no_text", ""},
	} {
		t.Run(tc.status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/recognize" {
					t.Errorf("unexpected OCR request: %s %s", r.Method, r.URL.Path)
				}
				_, _ = fmt.Fprint(w, tc.response)
			}))
			defer server.Close()
			result, err := parser.NewHTTPOCRClient(server.URL, time.Second).Recognize(context.Background(), []byte("image"))
			if err != nil || result.Status != tc.status || result.Text != tc.text {
				t.Fatalf("OCR result=%#v err=%v", result, err)
			}
		})
	}
}

func TestHTTPOCRClientSanitizesProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, "private image contents")
	}))
	defer server.Close()
	_, err := parser.NewHTTPOCRClient(server.URL, time.Second).Recognize(context.Background(), []byte("image"))
	if err == nil || strings.Contains(err.Error(), "private image contents") {
		t.Fatalf("OCR error leaked provider body: %v", err)
	}
	if code, permanent := parser.ClassifyOCRError(err); code != "ocr_rate_limit" || permanent {
		t.Fatalf("unexpected OCR classification: %s permanent=%v", code, permanent)
	}
}
