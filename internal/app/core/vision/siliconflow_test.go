package vision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDescribeSendsImageAndParsesCaption(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected request path or authorization")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != Model || len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 ||
			body.Messages[0].Content[1].Type != "image_url" ||
			!strings.HasPrefix(body.Messages[0].Content[1].ImageURL.URL, "data:image/png;base64,") {
			t.Errorf("unexpected multimodal request: %#v", body)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"一张流程图，箭头连接两个方框。"}}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "secret", time.Second, 256)
	result, err := client.Describe(context.Background(), []byte("PNG"), "image/png")
	if err != nil || result.Status != "described" || result.Text == "" || result.InputTokens != 10 || result.OutputTokens != 20 {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
}

func TestDescribeNoContentAndSanitizedHTTPError(t *testing.T) {
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"NO_CONTENT"}}]}`))
		} else {
			_, _ = w.Write([]byte(`{"secret":"do not reveal"}`))
		}
	}))
	defer server.Close()
	client := NewClient(server.URL, "secret", time.Second, 256)
	result, err := client.Describe(context.Background(), []byte("PNG"), "image/png")
	if err != nil || result.Status != "no_content" {
		t.Fatalf("unexpected no-content result: %#v, %v", result, err)
	}
	status = http.StatusTooManyRequests
	_, err = client.Describe(context.Background(), []byte("PNG"), "image/png")
	if err == nil || !strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "do not reveal") {
		t.Fatalf("unexpected provider error: %v", err)
	}
	if code, permanent := Classify(err); code != "vision_rate_limit" || permanent {
		t.Fatalf("unexpected rate limit classification: %s permanent=%v", code, permanent)
	}
	status = http.StatusUnauthorized
	_, err = client.Describe(context.Background(), []byte("PNG"), "image/png")
	if code, permanent := Classify(err); code != "vision_auth" || !permanent {
		t.Fatalf("unexpected auth classification: %s permanent=%v", code, permanent)
	}
}
