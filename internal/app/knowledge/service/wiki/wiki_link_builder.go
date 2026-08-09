package wiki

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/framework/llmgen"
)

var wikiLinkRE = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]*))?\]\]`)

// WikiLinkBuilder 把页面内容中其他页面的标题/slug 精确匹配处改写为
// [[slug|标题]] markdown 链接；纯文本零 LLM（复用 llmgen.RewriteRefs）。
type WikiLinkBuilder struct{}

// LinkifyPage 改写 page.Content 并返回派生链接（from/to 为 slug）。
func (b *WikiLinkBuilder) LinkifyPage(ctx context.Context, kbID string, page domain.WikiPage, targets []domain.WikiPage) (domain.WikiPage, []domain.WikiLink, error) {
	_ = ctx
	_ = kbID
	var rules []llmgen.RewriteRule
	for _, target := range targets {
		if target.Slug == page.Slug {
			continue // 不链自身
		}
		find := strings.TrimSpace(target.Title)
		if find == "" {
			find = strings.TrimSpace(target.Slug)
		}
		if find == "" {
			continue
		}
		rules = append(rules, llmgen.RewriteRule{
			Find:    find,
			Replace: "[[" + strings.TrimSpace(target.Slug) + "|" + find + "]]",
		})
	}
	if len(rules) == 0 {
		return page, nil, nil
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return len(rules[i].Find) > len(rules[j].Find)
	})
	content, _ := llmgen.RewriteRefs(page.Content, rules, llmgen.RewriteOptions{
		SkipCodeBlocks: true,
		SkipLinks:      true,
		WordBoundary:   true,
	})
	page.Content = content
	var links []domain.WikiLink
	seen := map[string]bool{}
	for _, m := range wikiLinkRE.FindAllStringSubmatch(content, -1) {
		toSlug := strings.TrimSpace(m[1])
		anchor := strings.TrimSpace(m[2])
		if toSlug == "" || toSlug == page.Slug {
			continue
		}
		if seen[toSlug] {
			continue
		}
		seen[toSlug] = true
		links = append(links, domain.WikiLink{
			KnowledgeBaseID: page.KnowledgeBaseID,
			FromPageID:      page.Slug,
			ToPageID:        toSlug,
			TargetType:      domain.WikiLinkTargetTypeWiki,
			Anchor:          anchor,
		})
	}
	return page, links, nil
}
