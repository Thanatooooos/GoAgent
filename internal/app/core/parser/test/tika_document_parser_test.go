package parser_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	parser "local/rag-project/internal/app/core/parser"
)

func TestTikaDocumentParserParse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT request, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/pdf" {
			t.Fatalf("unexpected content type: %s", got)
		}
		if got := r.Header.Get("X-Tika-Filename"); got != "report.pdf" {
			t.Fatalf("unexpected tika filename: %s", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "pdf-bytes" {
			t.Fatalf("unexpected request body: %q", string(body))
		}
		_, _ = w.Write([]byte("line1\r\nline2"))
	}))
	defer server.Close()

	docParser := parser.NewTikaDocumentParser(server.Client(), server.URL)
	result, err := docParser.Parse([]byte("pdf-bytes"), "application/pdf", map[string]any{
		"file_name": "report.pdf",
	})
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if result.Text != "line1\nline2" {
		t.Fatalf("unexpected text: %q", result.Text)
	}
}

func TestTikaDocumentParserExtractText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Tika-Filename"); got != "notes.txt" {
			t.Fatalf("unexpected tika filename: %s", got)
		}
		_, _ = w.Write([]byte("hello\r\nworld"))
	}))
	defer server.Close()

	docParser := parser.NewTikaDocumentParser(server.Client(), server.URL)
	text, err := docParser.ExtractText(strings.NewReader("content"), "notes.txt")
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}
	if text != "hello\nworld" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestTikaDocumentParserReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	docParser := parser.NewTikaDocumentParser(server.Client(), server.URL)
	_, err := docParser.Parse([]byte("content"), "application/pdf", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
}
