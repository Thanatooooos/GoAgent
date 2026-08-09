package wiki

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
)

type stubWikiPageRepo struct {
	upserted []domain.WikiPage
	bySlug   map[string]domain.WikiPage
	counts   map[string]domain.WikiLinkCounts
	deleted  bool
	offset   int
	limit    int
	list     []domain.WikiPage
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
	return s.list, len(s.list), nil
}
func (s *stubWikiPageRepo) ListBySlugs(_ context.Context, kbID string, slugs []string) ([]domain.WikiPage, error) {
	requested := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		requested[slug] = true
	}
	var pages []domain.WikiPage
	for slug, page := range s.bySlug {
		if requested[slug] {
			pages = append(pages, page)
		}
	}
	return pages, nil
}
func (s *stubWikiPageRepo) UpdateLinkCounts(_ context.Context, kbID string, counts map[string]domain.WikiLinkCounts) error {
	s.counts = counts
	return nil
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
	created       [][]domain.WikiLink
	replaced      []replacedLinks
	inCounts      map[string]int
	outCounts     map[string]int
	deletedValid  []string
	deletedCount  int
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
func (s *stubWikiLinkRepo) DeleteMissingTargets(_ context.Context, kbID string, validPageIDs []string) (int, error) {
	s.deletedValid = validPageIDs
	return s.deletedCount, nil
}
func (s *stubWikiLinkRepo) CountLinksByPage(_ context.Context, kbID string) (map[string]int, map[string]int, error) {
	return s.inCounts, s.outCounts, nil
}

func TestUpsertPagesFromDocumentPersistsPagesAndLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	// fixtures 不带 KnowledgeBaseID：service 必须负责 stamp，否则会落到 kb_id=''。
	pages := []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go", PageType: domain.WikiPageTypeEntity, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
		{ID: "p2", Slug: "concept/并发", Title: "并发", PageType: domain.WikiPageTypeConcept, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
	}
	links := []domain.WikiLink{
		{ID: "l1", FromPageID: "entity/go", ToPageID: "concept/并发", TargetType: domain.WikiLinkTargetTypeWiki, Anchor: "并发"},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, links); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(pageRepo.upserted) != 2 {
		t.Fatalf("upserted pages = %d, want 2", len(pageRepo.upserted))
	}
	for _, page := range pageRepo.upserted {
		if page.KnowledgeBaseID != "kb1" {
			t.Fatalf("upserted page %q has kbID=%q, want kb1", page.Slug, page.KnowledgeBaseID)
		}
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	entry := linkRepo.replaced[0]
	if entry.from != "p1" {
		t.Fatalf("replaced from = %q, want p1 (real page id, not slug)", entry.from)
	}
	if len(entry.links) != 1 {
		t.Fatalf("links for p1 = %#v, want 1", entry.links)
	}
	link := entry.links[0]
	if link.FromPageID != "p1" || link.ToPageID != "p2" {
		t.Fatalf("link from/to = %q/%q, want p1/p2 (real page ids)", link.FromPageID, link.ToPageID)
	}
	if link.KnowledgeBaseID != "kb1" {
		t.Fatalf("link for p1 has kbID=%q, want kb1", link.KnowledgeBaseID)
	}
	if len(linkRepo.created) != 0 {
		t.Fatalf("created = %#v, want empty", linkRepo.created)
	}
}

func TestUpsertPagesFromDocumentWithoutLinksSkipsLinkReplacement(t *testing.T) {
	pageRepo := &stubWikiPageRepo{}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", PageType: domain.WikiPageTypeEntity, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, nil); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(pageRepo.upserted) != 1 {
		t.Fatalf("upserted pages = %d, want 1", len(pageRepo.upserted))
	}
	if len(linkRepo.replaced) != 0 {
		t.Fatalf("replaced = %#v, want none", linkRepo.replaced)
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
	if len(linkRepo.replaced) != 0 {
		t.Fatalf("empty-from link should be dropped, got %#v", linkRepo.replaced)
	}
}

func TestUpsertPagesFromDocumentResolvesExternalSlugViaRepo(t *testing.T) {
	pageRepo := &stubWikiPageRepo{bySlug: map[string]domain.WikiPage{
		"entity/kubernetes": {ID: "p9", Slug: "entity/kubernetes", Title: "Kubernetes"},
	}}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", KnowledgeBaseID: "kb1", Slug: "entity/go", Title: "Go", PageType: domain.WikiPageTypeEntity, Status: domain.WikiPageStatusPublished, Content: "content", CreatedBy: "u"},
	}
	links := []domain.WikiLink{
		{ID: "l1", KnowledgeBaseID: "kb1", FromPageID: "entity/go", ToPageID: "entity/kubernetes", TargetType: domain.WikiLinkTargetTypeWiki},
		{ID: "l2", KnowledgeBaseID: "kb1", FromPageID: "entity/go", ToPageID: "concept/nonexistent", TargetType: domain.WikiLinkTargetTypeWiki},
	}

	if err := svc.UpsertPagesFromDocument(context.Background(), "kb1", pages, links); err != nil {
		t.Fatalf("UpsertPagesFromDocument: %v", err)
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	entry := linkRepo.replaced[0]
	if entry.from != "p1" {
		t.Fatalf("replaced from = %q, want p1", entry.from)
	}
	if len(entry.links) != 1 {
		t.Fatalf("links = %#v, want 1 (unresolvable link dropped)", entry.links)
	}
	link := entry.links[0]
	if link.FromPageID != "p1" || link.ToPageID != "p9" {
		t.Fatalf("link from/to = %q/%q, want p1/p9", link.FromPageID, link.ToPageID)
	}
	if link.ID != "l1" {
		t.Fatalf("link id = %q, want l1", link.ID)
	}
	if link.KnowledgeBaseID != "kb1" {
		t.Fatalf("link has kbID=%q, want kb1", link.KnowledgeBaseID)
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

func TestLinkifyAndPersistWritesLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{list: []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go"},
		{ID: "t1", Slug: "concept/并发", Title: "并发"},
	}}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go", Content: "参考 并发。"},
	}
	n, err := svc.LinkifyAndPersist(context.Background(), "kb1", pages, nil)
	if err != nil {
		t.Fatalf("LinkifyAndPersist: %v", err)
	}
	if n != 1 {
		t.Fatalf("link count = %d, want 1", n)
	}
	if len(pageRepo.upserted) != 1 {
		t.Fatalf("upserted = %d, want 1", len(pageRepo.upserted))
	}
	if content := pageRepo.upserted[0].Content; !strings.Contains(content, "[[concept/并发|并发]]") {
		t.Fatalf("content missing link: %s", content)
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	entry := linkRepo.replaced[0]
	if entry.from != "p1" {
		t.Fatalf("replaced from = %q, want p1 (real page id)", entry.from)
	}
	if len(entry.links) != 1 {
		t.Fatalf("links for p1 = %#v, want 1", entry.links)
	}
}

func TestLinkifyAndPersistClearsStaleLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{list: []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go"},
		{ID: "t1", Slug: "concept/并发", Title: "并发"},
	}}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go", Content: "无关联内容。"},
	}
	n, err := svc.LinkifyAndPersist(context.Background(), "kb1", pages, nil)
	if err != nil {
		t.Fatalf("LinkifyAndPersist: %v", err)
	}
	if n != 0 {
		t.Fatalf("link count = %d, want 0", n)
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	entry := linkRepo.replaced[0]
	if entry.from != "p1" {
		t.Fatalf("replaced from = %q, want p1", entry.from)
	}
	if len(entry.links) != 0 {
		t.Fatalf("expected empty group to clear stale links, got %#v", entry.links)
	}
}

func TestLinkifyAndPersistMergesExtraLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{list: []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go"},
		{ID: "t1", Slug: "concept/并发", Title: "并发"},
		{ID: "t2", Slug: "entity/fmt", Title: "fmt"},
	}}
	linkRepo := &stubWikiLinkRepo{}
	svc := NewWikiPageService(pageRepo, linkRepo)

	pages := []domain.WikiPage{
		{ID: "p1", Slug: "entity/go", Title: "Go", Content: "参考 并发。"},
	}
	extraLinks := []domain.WikiLink{
		{FromPageID: "entity/go", ToPageID: "entity/fmt"},
		{FromPageID: "entity/go", ToPageID: "concept/并发"}, // 与派生重复，派生优先
	}
	n, err := svc.LinkifyAndPersist(context.Background(), "kb1", pages, extraLinks)
	if err != nil {
		t.Fatalf("LinkifyAndPersist: %v", err)
	}
	if n != 2 {
		t.Fatalf("link count = %d, want 2 (1 derived + 1 extra, dedup)", n)
	}
	if len(linkRepo.replaced) != 1 {
		t.Fatalf("replaced = %#v, want 1 entry", linkRepo.replaced)
	}
	entry := linkRepo.replaced[0]
	if entry.from != "p1" {
		t.Fatalf("replaced from = %q, want p1", entry.from)
	}
	if len(entry.links) != 2 {
		t.Fatalf("links for p1 = %#v, want 2", entry.links)
	}
	byTo := map[string]domain.WikiLink{}
	for _, link := range entry.links {
		byTo[link.ToPageID] = link
	}
	if _, ok := byTo["t1"]; !ok {
		t.Fatalf("missing derived link to t1: %#v", entry.links)
	}
	if _, ok := byTo["t2"]; !ok {
		t.Fatalf("missing extra link to t2: %#v", entry.links)
	}
	if byTo["t1"].Anchor != "并发" {
		t.Fatalf("derived link should win dedup and keep anchor, got %+v", byTo["t1"])
	}
}

func TestRebuildLinkCounts(t *testing.T) {
	pageRepo := &stubWikiPageRepo{list: []domain.WikiPage{
		{ID: "pageA", Slug: "a"},
		{ID: "pageB", Slug: "b"},
		{ID: "pageC", Slug: "c"},
	}}
	linkRepo := &stubWikiLinkRepo{
		inCounts:  map[string]int{"pageA": 2},
		outCounts: map[string]int{"pageB": 1},
	}
	svc := NewWikiPageService(pageRepo, linkRepo)

	if err := svc.RebuildLinkCounts(context.Background(), "kb1"); err != nil {
		t.Fatalf("RebuildLinkCounts: %v", err)
	}
	if pageRepo.counts == nil {
		t.Fatalf("UpdateLinkCounts not called")
	}
	if c := pageRepo.counts["pageA"]; c.In != 2 || c.Out != 0 {
		t.Fatalf("counts[pageA] = %+v, want {2 0}", c)
	}
	if c := pageRepo.counts["pageB"]; c.In != 0 || c.Out != 1 {
		t.Fatalf("counts[pageB] = %+v, want {0 1}", c)
	}
	if c := pageRepo.counts["pageC"]; c.In != 0 || c.Out != 0 {
		t.Fatalf("counts[pageC] = %+v, want {0 0} (zero-reset)", c)
	}
}

func TestCleanDeadLinks(t *testing.T) {
	pageRepo := &stubWikiPageRepo{list: []domain.WikiPage{
		{ID: "a", Slug: "a"},
		{ID: "b", Slug: "b"},
	}}
	linkRepo := &stubWikiLinkRepo{deletedCount: 2}
	svc := NewWikiPageService(pageRepo, linkRepo)

	n, err := svc.CleanDeadLinks(context.Background(), "kb1")
	if err != nil {
		t.Fatalf("CleanDeadLinks: %v", err)
	}
	if n != 2 {
		t.Fatalf("cleaned = %d, want 2", n)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(linkRepo.deletedValid, want) {
		t.Fatalf("validIDs = %#v, want %#v", linkRepo.deletedValid, want)
	}
}
