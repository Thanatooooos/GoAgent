package wiki_write

import (
	"context"
	"errors"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	"local/rag-project/internal/app/knowledge/domain"
)

type stubReader struct {
	title   string
	content string
	err     error
}

func (s *stubReader) ReadContent(ctx context.Context, documentID string) (string, string, error) {
	return s.title, s.content, s.err
}

type stubWriter struct {
	upsertedPages int
	linkified     int
}

func (s *stubWriter) UpsertPagesFromDocument(ctx context.Context, kbID string, pages []domain.WikiPage, links []domain.WikiLink) error {
	s.upsertedPages = len(pages)
	return nil
}

func (s *stubWriter) LinkifyAndPersist(ctx context.Context, kbID string, pages []domain.WikiPage, extraLinks []domain.WikiLink) (int, error) {
	s.linkified = len(pages)
	return s.linkified, nil
}

type stubCompleter struct {
	response string
	err      error
}

func (s *stubCompleter) Chat(prompt string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.response, nil
}

func validWikiJSON() string {
	return `{"pages":[{"slug":"entity/go","title":"Go","type":"entity","summary":"Go 语言","content":"# Go\nGo 是编程语言。"}],"links":[]}`
}

func TestWikiWriteInvokeGeneratesAndPersists(t *testing.T) {
	reader := &stubReader{title: "Go 指南", content: "正文"}
	writer := &stubWriter{}
	completer := &stubCompleter{response: validWikiJSON()}
	handle, err := NewCapability(reader, writer, completer)
	if err != nil {
		t.Fatalf("NewCapability: %v", err)
	}
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: map[string]any{"knowledge_base_id": "kb1", "document_id": "doc1"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if writer.upsertedPages != 1 {
		t.Fatalf("upserted pages = %d", writer.upsertedPages)
	}
	output := result.Output.(CapabilityOutput)
	if output.PageCount != 1 {
		t.Fatalf("output = %+v", output)
	}
}

func TestWikiWriteInvokeDegradesOnEmptyContent(t *testing.T) {
	handle, _ := NewCapability(&stubReader{content: ""}, &stubWriter{}, &stubCompleter{response: validWikiJSON()})
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: map[string]any{"knowledge_base_id": "kb1", "document_id": "doc1"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestWikiWriteInvokeDegradesOnCompleterError(t *testing.T) {
	handle, _ := NewCapability(&stubReader{content: "c"}, &stubWriter{}, &stubCompleter{err: errors.New("upstream")})
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: map[string]any{"knowledge_base_id": "kb1", "document_id": "doc1"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	_ = result
}
