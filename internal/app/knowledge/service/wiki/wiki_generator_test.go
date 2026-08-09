package wiki

import (
	"context"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
)

type stubChatCompleter struct {
	response string
	err      error
	calls    int
}

func (c *stubChatCompleter) Chat(prompt string) (string, error) {
	c.calls++
	if c.err != nil {
		return "", c.err
	}
	return c.response, nil
}

func validJSON() string {
	return `{"pages":[{"slug":"entity/go","title":"Go","type":"entity","summary":"Go 语言","content":"# Go\nGo 是编程语言。"},{"slug":"concept/并发","title":"并发","type":"concept","summary":"并发","content":"# 并发\n并发模型。"}],"links":[{"from":"entity/go","to":"concept/并发","anchor":"并发"}]}`
}

func TestGenerateFromDocumentParsesValidJSON(t *testing.T) {
	client := &stubChatCompleter{response: validJSON()}
	g := NewLLMWikiGenerator(client)
	result, err := g.GenerateFromDocument(context.Background(), "Go 指南", "content", WikiGenerationOptions{MaxPages: 5})
	if err != nil {
		t.Fatalf("GenerateFromDocument: %v", err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("pages = %d", len(result.Pages))
	}
	if result.Pages[0].Slug != "entity/go" || result.Pages[0].PageType != domain.WikiPageTypeEntity {
		t.Fatalf("page[0] = %+v", result.Pages[0])
	}
	if len(result.Links) != 1 || result.Links[0].FromPageID != "entity/go" || result.Links[0].ToPageID != "concept/并发" {
		t.Fatalf("links = %#v", result.Links)
	}
}

func TestGenerateFromDocumentRejectsInvalidSlugsAndLinks(t *testing.T) {
	raw := `{"pages":[{"slug":"","title":"坏","type":"entity"},{"slug":"entity/a","title":"A","type":"bad_type"},{"slug":"entity/b","title":"B","type":"entity","content":"b"}],"links":[{"from":"entity/b","to":"entity/nonexistent","anchor":"x"}]}`
	client := &stubChatCompleter{response: raw}
	g := NewLLMWikiGenerator(client)
	result, err := g.GenerateFromDocument(context.Background(), "t", "c", WikiGenerationOptions{MaxPages: 5})
	if err != nil {
		t.Fatalf("GenerateFromDocument: %v", err)
	}
	// 空 slug 页丢弃、非法 type 页丢弃 → 只剩 1 页
	if len(result.Pages) != 1 || result.Pages[0].Slug != "entity/b" {
		t.Fatalf("pages = %#v", result.Pages)
	}
	// 链接指向批外 slug → 拒绝
	if len(result.Links) != 0 {
		t.Fatalf("links should be empty, got %#v", result.Links)
	}
}

func TestGenerateFromDocumentEnforcesMaxPages(t *testing.T) {
	raw := `{"pages":[{"slug":"entity/a","title":"A","type":"entity"},{"slug":"entity/b","title":"B","type":"entity"},{"slug":"entity/c","title":"C","type":"entity"}],"links":[{"from":"entity/a","to":"entity/b","anchor":"ab"},{"from":"entity/a","to":"entity/c","anchor":"ac"}]}`
	client := &stubChatCompleter{response: raw}
	g := NewLLMWikiGenerator(client)
	result, err := g.GenerateFromDocument(context.Background(), "t", "c", WikiGenerationOptions{MaxPages: 2})
	if err != nil {
		t.Fatalf("GenerateFromDocument: %v", err)
	}
	if len(result.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(result.Pages))
	}
	if result.Pages[0].Slug != "entity/a" || result.Pages[1].Slug != "entity/b" {
		t.Fatalf("pages = %#v", result.Pages)
	}
	// 链接指向被截断的 entity/c → 拒绝；指向保留的 entity/b → 保留。
	if len(result.Links) != 1 || result.Links[0].ToPageID != "entity/b" {
		t.Fatalf("links = %#v, want only to entity/b", result.Links)
	}
}

func TestGenerateFromDocumentDegradesOnLLMError(t *testing.T) {
	client := &stubChatCompleter{err: context.DeadlineExceeded}
	g := NewLLMWikiGenerator(client)
	result, err := g.GenerateFromDocument(context.Background(), "t", "c", WikiGenerationOptions{})
	if err != nil {
		t.Fatalf("expected degrade (no error), got %v", err)
	}
	if len(result.Pages) != 0 || len(result.Links) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestGenerateFromDocumentDegradesOnBadJSON(t *testing.T) {
	client := &stubChatCompleter{response: "not json at all"}
	g := NewLLMWikiGenerator(client)
	result, err := g.GenerateFromDocument(context.Background(), "t", "c", WikiGenerationOptions{})
	if err != nil {
		t.Fatalf("expected degrade, got %v", err)
	}
	if len(result.Pages) != 0 {
		t.Fatalf("expected empty pages, got %+v", result.Pages)
	}
}
