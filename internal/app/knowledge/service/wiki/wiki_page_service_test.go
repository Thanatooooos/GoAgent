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
	offset   int
	limit    int
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
	s.offset = offset
	s.limit = limit
	return nil, 0, nil
}
func (s *stubWikiPageRepo) DeleteByKB(_ context.Context, kbID string) error {
	s.deleted = true
	return nil
}

type replacedLinks struct {
	from  string
	links []domain.WikiLink
}

type stubWikiLinkRepo struct {
	created  [][]domain.WikiLink
	replaced []replacedLinks
}

func (s *stubWikiLinkRepo) CreateBatch(_ context.Context, links []domain.WikiLink) error {
	s.created = append(s.created, links)
	return nil
}
func (s *stubWikiLinkRepo) ReplaceByKBAndFrom(_ context.Context, kbID, fromPageID string, links []domain.WikiLink) error {
	s.replaced = append(s.replaced, replacedLinks{from: fromPageID, links: links})
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
		{ID: "l1", KnowledgeBaseID: "kb1", FromPageID: "entity/go", ToPageID: "concept/并发", TargetType: domain.WikiLinkTargetTypeWiki, Anchor: "并发"},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, links); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(pageRepo.upserted) != 2 {
		t.Fatalf("upserted pages = %d, want 2", len(pageRepo.upserted))
	}
	if len(linkRepo.replaced) != 2 {
		t.Fatalf("replaced = %#v, want 2 entries", linkRepo.replaced)
	}
	var goLinks []domain.WikiLink
	for _, entry := range linkRepo.replaced {
		if entry.from == "entity/go" {
			goLinks = entry.links
		}
	}
	if len(goLinks) != 1 {
		t.Fatalf("links for entity/go = %#v, want 1", goLinks)
	}
	if len(linkRepo.created) != 0 {
		t.Fatalf("created = %#v, want empty", linkRepo.created)
	}
}

func TestUpsertPagesFromDocumentClearsStaleLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", PageType: domain.WikiPageTypeEntity, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, nil); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	if len(linkRepo.replaced[0].links) != 0 {
		t.Fatalf("stale links should be cleared, got %#v", linkRepo.replaced[0].links)
	}
}

func TestUpsertPagesFromDocumentIgnoresEmptyFrom(t *testing.T) {
	pageRepo := &stubWikiPageRepo{}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", PageType: domain.WikiPageTypeEntity, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
	}
	links := []domain.WikiLink{
		{ID: "l1", KnowledgeBaseID: "kb1", FromPageID: "", ToPageID: "concept/并发", TargetType: domain.WikiLinkTargetTypeWiki},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, links); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	if len(linkRepo.replaced[0].links) != 0 {
		t.Fatalf("empty-from link should be skipped, got %#v", linkRepo.replaced[0].links)
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

func TestListByKBNormalizesPagination(t *testing.T) {
	pageRepo := &stubWikiPageRepo{}
	svc := NewWikiPageService(pageRepo, &stubWikiLinkRepo{})

	if _, _, err := svc.ListByKB(context.Background(), "kb1", 0, 0); err != nil {
		t.Fatalf("ListByKB(page=0,size=0): %v", err)
	}
	if pageRepo.offset != 0 || pageRepo.limit != 10 {
		t.Fatalf("page=0 size=0: offset=%d limit=%d, want 0/10", pageRepo.offset, pageRepo.limit)
	}
	if _, _, err := svc.ListByKB(context.Background(), "kb1", 3, 5); err != nil {
		t.Fatalf("ListByKB(page=3,size=5): %v", err)
	}
	if pageRepo.offset != 10 || pageRepo.limit != 5 {
		t.Fatalf("page=3 size=5: offset=%d limit=%d, want 10/5", pageRepo.offset, pageRepo.limit)
	}
}
