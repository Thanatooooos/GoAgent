package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamCorpusWritesOnlyRequestedPassages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("1\tfirst\n2\tsecond\n3\tthird\n"))
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "corpus.tsv")
	count, err := streamCorpus(context.Background(), server.URL, map[string]struct{}{"1": {}, "3": {}}, output)
	if err != nil {
		t.Fatalf("streamCorpus returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if got, want := string(data), "1\tfirst\n3\tthird\n"; got != want {
		t.Fatalf("corpus = %q, want %q", got, want)
	}
}

func TestWriteSamplesKeepsOnlyPositiveQrels(t *testing.T) {
	output := filepath.Join(t.TempDir(), "samples.json")
	err := writeSamples(output, []query{{ID: "q1", Text: "query"}}, []qrel{
		{QueryID: "q1", PassageID: "negative", Relevance: 0},
		{QueryID: "q1", PassageID: "positive", Relevance: 3},
	})
	if err != nil {
		t.Fatalf("writeSamples returned error: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	if string(data) == "" || !strings.Contains(string(data), `"positive"`) || strings.Contains(string(data), `"negative"`) {
		t.Fatalf("unexpected samples output: %s", data)
	}
}

func TestWriteMarkdownCorpusKeepsPassageIDInFileName(t *testing.T) {
	root := t.TempDir()
	corpusPath := filepath.Join(root, "corpus.tsv")
	if err := os.WriteFile(corpusPath, []byte("1\tfirst\n2\tsecond\n"), 0o644); err != nil {
		t.Fatalf("write corpus: %v", err)
	}
	written, err := writeMarkdownCorpus(corpusPath, filepath.Join(root, "markdown"), 1, 0)
	if err != nil {
		t.Fatalf("writeMarkdownCorpus returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written = %d, want 1", written)
	}
	data, err := os.ReadFile(filepath.Join(root, "markdown", "1.md"))
	if err != nil {
		t.Fatalf("read markdown: %v", err)
	}
	if got, want := string(data), "<!-- t2ranking-pid: 1 -->\n\nfirst\n"; got != want {
		t.Fatalf("markdown = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "markdown", "2.md")); !os.IsNotExist(err) {
		t.Fatalf("expected second passage to be excluded, stat error = %v", err)
	}
}

func TestWriteMarkdownCorpusSkipsOffsetPassages(t *testing.T) {
	root := t.TempDir()
	corpusPath := filepath.Join(root, "corpus.tsv")
	if err := os.WriteFile(corpusPath, []byte("1\tfirst\n2\tsecond\n3\tthird\n"), 0o644); err != nil {
		t.Fatalf("write corpus: %v", err)
	}
	written, err := writeMarkdownCorpus(corpusPath, filepath.Join(root, "markdown"), 1, 1)
	if err != nil {
		t.Fatalf("writeMarkdownCorpus returned error: %v", err)
	}
	if written != 1 {
		t.Fatalf("written = %d, want 1", written)
	}
	if _, err := os.Stat(filepath.Join(root, "markdown", "1.md")); !os.IsNotExist(err) {
		t.Fatalf("expected first passage to be skipped, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "markdown", "2.md")); err != nil {
		t.Fatalf("expected second passage to be written, stat error = %v", err)
	}
}
