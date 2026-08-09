package wiki

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
)

func TestLinkifyPageInsertsLinks(t *testing.T) {
	page := domain.WikiPage{Slug: "entity/a", Title: "A", Content: "介绍 A 与 并发。参考 Go。"}
	targets := []domain.WikiPage{
		{Slug: "concept/并发", Title: "并发"},
		{Slug: "entity/go", Title: "Go"},
	}
	b := &WikiLinkBuilder{}
	updated, links, err := b.LinkifyPage(context.Background(), "kb1", page, targets)
	if err != nil {
		t.Fatalf("LinkifyPage: %v", err)
	}
	if !strings.Contains(updated.Content, "[[concept/并发|并发]]") {
		t.Fatalf("content missing link: %s", updated.Content)
	}
	if !strings.Contains(updated.Content, "[[entity/go|Go]]") {
		t.Fatalf("content missing link: %s", updated.Content)
	}
	if len(links) != 2 {
		t.Fatalf("links = %#v", links)
	}
}

func TestLinkifyPageSkipsCodeAndExistingLinks(t *testing.T) {
	page := domain.WikiPage{Slug: "entity/a", Title: "A", Content: "`并发` 与 [Go](x) 与 并发。\n```\nGo\n```"}
	targets := []domain.WikiPage{{Slug: "concept/并发", Title: "并发"}, {Slug: "entity/go", Title: "Go"}}
	b := &WikiLinkBuilder{}
	updated, links, err := b.LinkifyPage(context.Background(), "kb1", page, targets)
	if err != nil {
		t.Fatalf("LinkifyPage: %v", err)
	}
	if got := strings.Count(updated.Content, "[[concept/并发|并发]]"); got != 1 {
		t.Fatalf("plain 并发 should be linked exactly once, got %d: %s", got, updated.Content)
	}
	if !strings.Contains(updated.Content, "`并发`") {
		t.Fatalf("inline-code 并发 should stay unlinked: %s", updated.Content)
	}
	if !strings.Contains(updated.Content, "[Go](x)") {
		t.Fatalf("existing link [Go](x) should not be replaced: %s", updated.Content)
	}
	if !strings.Contains(updated.Content, "```\nGo\n```") {
		t.Fatalf("fenced code Go should stay unlinked: %s", updated.Content)
	}
	if strings.Contains(updated.Content, "[[entity/go|Go]]") {
		t.Fatalf("code/link Go should not be linked: %s", updated.Content)
	}
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 derived link, got %#v", links)
	}
	if links[0].ToPageID != "concept/并发" {
		t.Fatalf("unexpected link target: %#v", links[0])
	}
}

func TestLinkifyPageDoesNotSelfLink(t *testing.T) {
	page := domain.WikiPage{Slug: "entity/a", Title: "A", Content: "介绍 A。"}
	targets := []domain.WikiPage{{Slug: "entity/a", Title: "A"}}
	b := &WikiLinkBuilder{}
	_, links, err := b.LinkifyPage(context.Background(), "kb1", page, targets)
	if err != nil {
		t.Fatalf("LinkifyPage: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("should not self-link: %#v", links)
	}
}
