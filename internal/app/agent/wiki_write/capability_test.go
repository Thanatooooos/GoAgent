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
	if result.Delta.Evidence == nil || len(result.Delta.Evidence.AddItems) != 1 {
		t.Fatalf("expected wiki write evidence, got %+v", result.Delta.Evidence)
	}
	if result.Delta.Evidence.AddItems[0].Content != "wiki generated 1 pages / 1 links" {
		t.Fatalf("unexpected wiki write evidence: %+v", result.Delta.Evidence.AddItems[0])
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
	if result.Status != agentcapability.StatusDegraded {
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
	if result.Status != agentcapability.StatusDegraded {
		t.Fatalf("status = %s", result.Status)
	}
}

func TestWikiWriteInvokeRejectsEmptyDocumentID(t *testing.T) {
	handle, _ := NewCapability(&stubReader{title: "Go 指南", content: "正文"}, &stubWriter{}, &stubCompleter{response: validWikiJSON()})
	_, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: map[string]any{"knowledge_base_id": "kb1"},
	})
	if err == nil {
		t.Fatalf("expected precondition error for missing document_id")
	}
}
