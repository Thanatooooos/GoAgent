package wikiretrieval

import (
	"context"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

type stubPageSearcher struct {
	search map[string][]domain.WikiPage
	byIDs  map[string]domain.WikiPage
}

func (s *stubPageSearcher) Search(_ context.Context, kbID, query string, limit int) ([]domain.WikiPage, error) {
	return s.search[kbID+"|"+query], nil
}

func (s *stubPageSearcher) ListByIDs(_ context.Context, kbID string, ids []string) ([]domain.WikiPage, error) {
	var pages []domain.WikiPage
	for _, id := range ids {
		if p, ok := s.byIDs[id]; ok {
			pages = append(pages, p)
		}
	}
	return pages, nil
}

type stubLinkLister struct {
	links []domain.WikiLink
}

func (s *stubLinkLister) ListByKB(_ context.Context, kbID string) ([]domain.WikiLink, error) {
	return s.links, nil
}

func TestSearchWikiReturnsMatchedAndNeighbors(t *testing.T) {
	searcher := &stubPageSearcher{
		search: map[string][]domain.WikiPage{
			"kb1|并发": {{ID: "p1", KnowledgeBaseID: "kb1", Slug: "concept/并发", Title: "并发", Content: "并发模型"}},
		},
		byIDs: map[string]domain.WikiPage{
			"p2": {ID: "p2", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", Content: "Go 语言"},
		},
	}
	lister := &stubLinkLister{links: []domain.WikiLink{
		{KnowledgeBaseID: "kb1", FromPageID: "p1", ToPageID: "p2", TargetType: domain.WikiLinkTargetTypeWiki},
	}}
	r := NewRetriever(searcher, lister)
	chunks, err := r.SearchWiki(context.Background(), ragretrieve.WikiSearchRequest{
		Query:            "并发",
		KnowledgeBaseIDs: []string{"kb1"},
		TopK:             5,
	})
	if err != nil {
		t.Fatalf("SearchWiki: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2 (matched + neighbor)", len(chunks))
	}
	if chunks[0].ID != "p1" || chunks[0].Metadata["record_type"] != "wiki_page" {
		t.Fatalf("matched chunk = %+v", chunks[0])
	}
	if chunks[1].ID != "p2" || chunks[1].Metadata["wiki_neighbor"] != true {
		t.Fatalf("neighbor chunk = %+v", chunks[1])
	}
}

func TestSearchWikiNoMatches(t *testing.T) {
	r := NewRetriever(&stubPageSearcher{}, &stubLinkLister{})
	chunks, err := r.SearchWiki(context.Background(), ragretrieve.WikiSearchRequest{Query: "x", KnowledgeBaseIDs: []string{"kb1"}, TopK: 5})
	if err != nil {
		t.Fatalf("SearchWiki: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestSearchWikiNilRetriever(t *testing.T) {
	var r *Retriever
	chunks, err := r.SearchWiki(context.Background(), ragretrieve.WikiSearchRequest{Query: "x", KnowledgeBaseIDs: []string{"kb1"}, TopK: 5})
	if err != nil {
		t.Fatalf("SearchWiki nil: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("chunks = %#v", chunks)
	}
}
