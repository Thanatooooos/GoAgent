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
	upsertedPages    int
	called           bool
	linkifyCalled    bool
	linkifyExtra     []knowledgedomain.WikiLink
	rebuiltCalled    bool
	cleanedCalled    bool
	deadLinks        int
}

func (s *stubWikiServicePort) UpsertPagesFromDocument(ctx context.Context, kbID string, pages []knowledgedomain.WikiPage, links []knowledgedomain.WikiLink) error {
	s.called = true
	s.upsertedPages = len(pages)
	return nil
}

func (s *stubWikiServicePort) LinkifyAndPersist(ctx context.Context, kbID string, pages []knowledgedomain.WikiPage, extraLinks []knowledgedomain.WikiLink) (int, error) {
	s.linkifyCalled = true
	s.linkifyExtra = extraLinks
	return len(pages), nil
}

func (s *stubWikiServicePort) RebuildLinkCounts(ctx context.Context, kbID string) error {
	s.rebuiltCalled = true
	return nil
}

func (s *stubWikiServicePort) CleanDeadLinks(ctx context.Context, kbID string) (int, error) {
	s.cleanedCalled = true
	return s.deadLinks, nil
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
	gen := &stubWikiGenerator{
		pages: []knowledgedomain.WikiPage{{Slug: "entity/go", Title: "Go"}},
		links: []knowledgedomain.WikiLink{
			{FromPageID: "entity/go", ToPageID: "entity/fmt"},
			{FromPageID: "entity/fmt", ToPageID: "entity/go"},
		},
	}
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
	if !svc.linkifyCalled || !svc.rebuiltCalled || !svc.cleanedCalled {
		t.Fatalf("linkify=%v rebuild=%v clean=%v", svc.linkifyCalled, svc.rebuiltCalled, svc.cleanedCalled)
	}
	if len(svc.linkifyExtra) != 2 {
		t.Fatalf("linkify extra links = %#v, want 2 (generator links passed)", svc.linkifyExtra)
	}
	if output["pageCount"] != 1 || output["linkCount"] != 1 || output["generatorLinks"] != 2 || output["deadLinksCleaned"] != 0 {
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
