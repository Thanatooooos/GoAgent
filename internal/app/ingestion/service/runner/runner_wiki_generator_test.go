package runner

import (
	"context"
	"testing"

	"local/rag-project/internal/app/ingestion/domain"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	wikiservice "local/rag-project/internal/app/knowledge/service/wiki"
)

type stubWikiServicePort struct {
	upsertedPages int
	called        bool
}

func (s *stubWikiServicePort) UpsertPagesFromDocument(ctx context.Context, kbID string, pages []knowledgedomain.WikiPage, links []knowledgedomain.WikiLink) error {
	s.called = true
	s.upsertedPages = len(pages)
	return nil
}

type stubWikiGenerator struct {
	pages []knowledgedomain.WikiPage
	links []knowledgedomain.WikiLink
}

func (s *stubWikiGenerator) GenerateFromDocument(ctx context.Context, title, content string, options wikiservice.WikiGenerationOptions) (wikiservice.WikiGenerationResult, error) {
	return wikiservice.WikiGenerationResult{Pages: s.pages, Links: s.links}, nil
}

func TestWikiGeneratorNodeRunnerWritesPages(t *testing.T) {
	svc := &stubWikiServicePort{}
	gen := &stubWikiGenerator{pages: []knowledgedomain.WikiPage{{Slug: "entity/go", Title: "Go"}}}
	runner := NewWikiGeneratorNodeRunner(svc, gen)
	_, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Title: "Go 指南", Content: "正文"},
		Task:   domain.Task{Metadata: map[string]any{"knowledgeBaseId": "kb1"}},
	}, domain.PipelineNode{Settings: map[string]any{"maxPages": 3}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !svc.called || svc.upsertedPages != 1 {
		t.Fatalf("service called=%v pages=%d", svc.called, svc.upsertedPages)
	}
	if output["pageCount"] != 1 {
		t.Fatalf("output = %+v", output)
	}
}

func TestWikiGeneratorNodeRunnerDegradesWithoutKB(t *testing.T) {
	svc := &stubWikiServicePort{}
	gen := &stubWikiGenerator{pages: []knowledgedomain.WikiPage{{Slug: "x"}}}
	runner := NewWikiGeneratorNodeRunner(svc, gen)
	_, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Title: "t", Content: "c"},
		Task:   domain.Task{Metadata: map[string]any{}},
	}, domain.PipelineNode{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if svc.called {
		t.Fatal("service should not be called without kb id")
	}
	if output["degraded"] != true {
		t.Fatalf("output = %+v", output)
	}
}
