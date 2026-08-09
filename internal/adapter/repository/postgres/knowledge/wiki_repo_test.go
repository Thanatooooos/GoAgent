package knowledge

import (
	"reflect"
	"testing"
	"time"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/domain"
)

func TestWikiPageModelRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now()
	page := domain.WikiPage{
		ID:                "p-1",
		KnowledgeBaseID:   "kb-1",
		Slug:              "entity/go",
		Title:             "Go",
		PageType:          domain.WikiPageTypeEntity,
		Status:            domain.WikiPageStatusPublished,
		Content:           "# Go\nGo 语言",
		Summary:           "Go 编程语言",
		SourceDocumentIDs: []string{"d-1", "d-2"},
		SourceChunkIDs:    []string{"c-1", "c-2"},
		CreatedBy:         "u-1",
		UpdatedBy:         "u-2",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	model := toWikiPageModel(page)
	if string(model.SourceDocumentIDs) == "" || string(model.SourceChunkIDs) == "" {
		t.Fatalf("expected non-empty jsonb bytes, got %q / %q", model.SourceDocumentIDs, model.SourceChunkIDs)
	}

	roundTrip := toWikiPageDomain(model)
	if roundTrip.ID != page.ID || roundTrip.Slug != page.Slug || roundTrip.PageType != page.PageType || roundTrip.Status != page.Status {
		t.Fatalf("unexpected wiki page round trip result: %+v", roundTrip)
	}
	if !reflect.DeepEqual(roundTrip.SourceDocumentIDs, page.SourceDocumentIDs) {
		t.Fatalf("source document ids = %#v, want %#v", roundTrip.SourceDocumentIDs, page.SourceDocumentIDs)
	}
	if !reflect.DeepEqual(roundTrip.SourceChunkIDs, page.SourceChunkIDs) {
		t.Fatalf("source chunk ids = %#v, want %#v", roundTrip.SourceChunkIDs, page.SourceChunkIDs)
	}
	if !roundTrip.CreatedAt.Equal(now) || !roundTrip.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected timestamps after round trip: %+v", roundTrip)
	}
}

func TestWikiPageModelRoundTripNilJSONB(t *testing.T) {
	t.Parallel()

	page := domain.WikiPage{
		ID:              "p-2",
		KnowledgeBaseID: "kb-1",
		Slug:            "concept/并发",
		Title:           "并发",
		PageType:        domain.WikiPageTypeConcept,
		Status:          domain.WikiPageStatusPublished,
		CreatedBy:       "u-1",
		UpdatedBy:       "u-1",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	model := toWikiPageModel(page)
	if string(model.SourceDocumentIDs) != "[]" || string(model.SourceChunkIDs) != "[]" {
		t.Fatalf("expected empty jsonb array for nil ids, got %q / %q", model.SourceDocumentIDs, model.SourceChunkIDs)
	}
}

func TestWikiLinkModelRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now()
	link := domain.WikiLink{
		ID:              "l-1",
		KnowledgeBaseID: "kb-1",
		FromPageID:      "p-1",
		ToPageID:        "p-2",
		TargetType:      domain.WikiLinkTargetTypeWiki,
		Anchor:          "并发",
		CreatedAt:       now,
	}

	model := toWikiLinkModel(link)
	roundTrip := toWikiLinkDomain(model)
	if roundTrip.ID != link.ID || roundTrip.KnowledgeBaseID != link.KnowledgeBaseID {
		t.Fatalf("unexpected wiki link round trip result: %+v", roundTrip)
	}
	if roundTrip.FromPageID != link.FromPageID || roundTrip.ToPageID != link.ToPageID {
		t.Fatalf("unexpected wiki link pages after round trip: %+v", roundTrip)
	}
	if roundTrip.TargetType != link.TargetType || roundTrip.Anchor != link.Anchor {
		t.Fatalf("unexpected wiki link metadata after round trip: %+v", roundTrip)
	}
	if !roundTrip.CreatedAt.Equal(now) {
		t.Fatalf("unexpected timestamp after round trip: %+v", roundTrip)
	}
}

func TestWikiModelTableNames(t *testing.T) {
	t.Parallel()

	if got := (models.WikiPageModel{}).TableName(); got != "t_wiki_page" {
		t.Fatalf("unexpected wiki page table name: %q", got)
	}
	if got := (models.WikiLinkModel{}).TableName(); got != "t_wiki_link" {
		t.Fatalf("unexpected wiki link table name: %q", got)
	}
}
