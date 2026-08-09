package wiki

import (
	"context"
	"errors"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
)

type stubWikiPageRepo struct {
	upserted []domain.WikiPage
	bySlug   map[string]domain.WikiPage
	deleted  bool
}

func (s *stubWikiPageRepo) Upsert(_ context.Context, page domain.WikiPage) (domain.WikiPage, error) {
	s.upserted = append(s.upserted, page)
	if s.bySlug == nil {
		s.bySlug = map[string]domain.WikiPage{}
	}
	s.bySlug[page.Slug] = page
	return page, nil
}
func (s *stubWikiPageRepo) GetBySlug(_ context.Context, kbID, slug string) (domain.WikiPage, error) {
	page, ok := s.bySlug[slug]
	if !ok {
		return domain.WikiPage{}, errors.New("not found")
	}
	return page, nil
}
func (s *stubWikiPageRepo) ListByKB(_ context.Context, kbID string, offset, limit int) ([]domain.WikiPage, int, error) {
	return nil, 0, nil
}
func (s *stubWikiPageRepo) DeleteByKB(_ context.Context, kbID string) error {
	s.deleted = true
	return nil
}

type stubWikiLinkRepo struct {
	batches [][]domain.WikiLink
}

func (s *stubWikiLinkRepo) CreateBatch(_ context.Context, links []domain.WikiLink) error {
	s.batches = append(s.batches, links)
	return nil
}
func (s *stubWikiLinkRepo) ReplaceByKBAndFrom(_ context.Context, kbID, fromPageID string, links []domain.WikiLink) error {
	s.batches = append(s.batches, links)
	return nil
}
func (s *stubWikiLinkRepo) ListByKB(_ context.Context, kbID string) ([]domain.WikiLink, error) {
	return nil, nil
}

func TestUpsertPagesFromDocumentPersistsPagesAndLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", PageType: domain.WikiPageTypeEntity, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
		{ID: "p2", KnowledgeBaseID: "kb1", Slug: "concept/并发", Title: "并发", PageType: domain.WikiPageTypeConcept, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
	}
	links := []domain.WikiLink{
		{ID: "l1", KnowledgeBaseID: "kb1", FromPageID: "p1", ToPageID: "p2", TargetType: domain.WikiLinkTargetTypeWiki, Anchor: "并发"},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, links); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(pageRepo.upserted) != 2 {
		t.Fatalf("upserted pages = %d, want 2", len(pageRepo.upserted))
	}
	if len(linkRepo.batches) != 1 || len(linkRepo.batches[0]) != 1 {
		t.Fatalf("links batches = %#v", linkRepo.batches)
	}
}

func TestGetBySlugReturnsPage(t *testing.T) {
	pageRepo := &stubWikiPageRepo{bySlug: map[string]domain.WikiPage{"entity/go": {ID: "p1", Slug: "entity/go"}}}
	svc := NewWikiPageService(pageRepo, &stubWikiLinkRepo{})
	page, err := svc.GetBySlug(context.Background(), "kb1", "entity/go")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if page.Slug != "entity/go" {
		t.Fatalf("page = %+v", page)
	}
}
