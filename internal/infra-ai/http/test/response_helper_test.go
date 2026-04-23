package test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	aihttp "local/rag-project/internal/infra-ai/http"
)

// errReadCloser 模拟 Read 出错的 Body
type errReadCloser struct{}

func (e *errReadCloser) Read(p []byte) (int, error) { return 0, errors.New("read error") }
func (e *errReadCloser) Close() error               { return nil }

func TestReadBody_Nil(t *testing.T) {
	h := aihttp.NewResponseHelper()
	got, err := h.ReadBody(nil)
	if err != nil {
		t.Fatalf("ReadBody(nil) returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("ReadBody(nil) = %q, want empty", got)
	}
}

func TestReadBody_Success(t *testing.T) {
	h := aihttp.NewResponseHelper()
	body := io.NopCloser(strings.NewReader("hello"))
	got, err := h.ReadBody(body)
	if err != nil {
		t.Fatalf("ReadBody failed: %v", err)
	}
	if got != "hello" {
		t.Fatalf("ReadBody = %q, want %q", got, "hello")
	}
}

func TestParseJSON_NilBody(t *testing.T) {
	h := aihttp.NewResponseHelper()
	var v map[string]interface{}
	err := h.ParseJSON(nil, "Label", &v)
	if err == nil {
		t.Fatal("ParseJSON(nil) should return error")
	}
	if !aihttp.IsModelClientException(err) {
		t.Fatalf("expected ModelClientException, got %T", err)
	}
	e, _ := aihttp.AsModelClientException(err)
	if e.ErrorType != aihttp.ErrorTypeInvalidResponse {
		t.Fatalf("expected ErrorTypeInvalidResponse, got %v", e.ErrorType)
	}
}

func TestParseJSON_ReadError(t *testing.T) {
	h := aihttp.NewResponseHelper()
	var v map[string]interface{}
	err := h.ParseJSON(&errReadCloser{}, "Label", &v)
	if err == nil {
		t.Fatal("ParseJSON with read error should return error")
	}
	if !aihttp.IsModelClientException(err) {
		t.Fatalf("expected ModelClientException, got %T", err)
	}
	e, _ := aihttp.AsModelClientException(err)
	if e.ErrorType != aihttp.ErrorTypeNetworkError {
		t.Fatalf("expected ErrorTypeNetworkError, got %v", e.ErrorType)
	}
}

func TestParseJSON_InvalidJSON(t *testing.T) {
	h := aihttp.NewResponseHelper()
	var v map[string]interface{}
	body := io.NopCloser(strings.NewReader("not-json"))
	err := h.ParseJSON(body, "Label", &v)
	if err == nil {
		t.Fatal("ParseJSON with invalid json should return error")
	}
	if !aihttp.IsModelClientException(err) {
		t.Fatalf("expected ModelClientException, got %T", err)
	}
	e, _ := aihttp.AsModelClientException(err)
	if e.ErrorType != aihttp.ErrorTypeInvalidResponse {
		t.Fatalf("expected ErrorTypeInvalidResponse, got %v", e.ErrorType)
	}
}

func TestParseJSON_SuccessAndParseJSONMap(t *testing.T) {
	h := aihttp.NewResponseHelper()
	var v struct {
		A int `json:"a"`
	}
	body := io.NopCloser(strings.NewReader(`{"a":123}`))
	if err := h.ParseJSON(body, "Label", &v); err != nil {
		t.Fatalf("ParseJSON should succeed, got %v", err)
	}
	if v.A != 123 {
		t.Fatalf("parsed value wrong: %v", v.A)
	}

	mBody := io.NopCloser(strings.NewReader(`{"x":"y"}`))
	m, err := h.ParseJSONMap(mBody, "Label")
	if err != nil {
		t.Fatalf("ParseJSONMap failed: %v", err)
	}
	if m["x"] != "y" {
		t.Fatalf("ParseJSONMap wrong value: %v", m)
	}
}

func TestCheckResponse_Non2xx(t *testing.T) {
	h := aihttp.NewResponseHelper()
	resp := &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("errbody"))}
	err := h.CheckResponse(resp, "Label")
	if err == nil {
		t.Fatal("CheckResponse should return error for 500")
	}
	if !aihttp.IsModelClientException(err) {
		t.Fatalf("expected ModelClientException, got %T", err)
	}
	e, _ := aihttp.AsModelClientException(err)
	if e.StatusCode != 500 {
		t.Fatalf("expected status 500, got %d", e.StatusCode)
	}
	if e.ErrorType != aihttp.ErrorTypeServerError {
		t.Fatalf("expected ErrorTypeServerError, got %v", e.ErrorType)
	}
}

func TestRequireHelpers(t *testing.T) {
	h := aihttp.NewResponseHelper()
	if err := h.RequireProvider(nil, "Label"); err == nil {
		t.Fatal("RequireProvider(nil) should error")
	}
	if err := h.RequireAPIKey("", "Label"); err == nil {
		t.Fatal("RequireAPIKey(empty) should error")
	}
	if err := h.RequireModel("", "Label"); err == nil {
		t.Fatal("RequireModel(empty) should error")
	}
}
