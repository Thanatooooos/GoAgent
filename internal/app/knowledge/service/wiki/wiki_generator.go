package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/framework/llmgen"
)

// PromptCompleter 是生成器依赖的 LLM 客户端抽象。
type PromptCompleter interface {
	Chat(prompt string) (string, error)
}

type WikiGenerationOptions struct {
	MaxPages int
	PageType string
}

type WikiGenerationResult struct {
	Pages []domain.WikiPage
	Links []domain.WikiLink
}

type WikiGenerator interface {
	GenerateFromDocument(ctx context.Context, title, content string, options WikiGenerationOptions) (WikiGenerationResult, error)
}

type llmWikiGenerator struct {
	client PromptCompleter
}

func NewLLMWikiGenerator(client PromptCompleter) WikiGenerator {
	return &llmWikiGenerator{client: client}
}

func (g *llmWikiGenerator) GenerateFromDocument(_ context.Context, title, content string, options WikiGenerationOptions) (WikiGenerationResult, error) {
	empty := WikiGenerationResult{}
	if g == nil || g.client == nil {
		return empty, nil
	}
	if options.MaxPages <= 0 {
		options.MaxPages = 5
	}
	pageType := strings.TrimSpace(options.PageType)
	if pageType == "" {
		pageType = domain.WikiPageTypeEntity
	}
	prompt := buildWikiGenerationPrompt(title, content, options.MaxPages, pageType)
	response, err := g.client.Chat(prompt)
	if err != nil {
		return empty, nil // 降级
	}
	return parseWikiGeneration(response, pageType, options.MaxPages)
}

type wikiGenerationJSON struct {
	Pages []struct {
		Slug    string `json:"slug"`
		Title   string `json:"title"`
		Type    string `json:"type"`
		Summary string `json:"summary"`
		Content string `json:"content"`
	} `json:"pages"`
	Links []struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Anchor string `json:"anchor"`
	} `json:"links"`
}

// parseWikiGeneration 解析严格 JSON 并做确定性校验：
// slug 非空且唯一、type 在 {entity, concept}、页面数量受 maxPages 限制、
// 链接 from/to 必须命中最终保留的 slug。
func parseWikiGeneration(raw, defaultType string, maxPages int) (WikiGenerationResult, error) {
	empty := WikiGenerationResult{}
	var parsed wikiGenerationJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return empty, nil // 降级
	}
	seen := map[string]bool{}
	result := WikiGenerationResult{Pages: make([]domain.WikiPage, 0, len(parsed.Pages))}
	for _, item := range parsed.Pages {
		slug := strings.TrimSpace(item.Slug)
		pageType := strings.TrimSpace(item.Type)
		if pageType == "" {
			pageType = defaultType
		}
		if slug == "" || seen[slug] {
			continue
		}
		if pageType != domain.WikiPageTypeEntity && pageType != domain.WikiPageTypeConcept {
			continue
		}
		seen[slug] = true
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = slug
		}
		result.Pages = append(result.Pages, domain.WikiPage{
			Slug:     slug,
			Title:    title,
			PageType: pageType,
			Content:  strings.TrimSpace(item.Content),
			Summary:  strings.TrimSpace(item.Summary),
		})
	}
	if maxPages > 0 && len(result.Pages) > maxPages {
		result.Pages = result.Pages[:maxPages]
	}
	if len(result.Pages) == 0 {
		return result, nil
	}
	slugSet := make([]string, 0, len(result.Pages))
	for _, page := range result.Pages {
		slugSet = append(slugSet, page.Slug)
	}
	validator := llmgen.NewRefValidator(slugSet)
	for _, item := range parsed.Links {
		from := strings.TrimSpace(item.From)
		to := strings.TrimSpace(item.To)
		if _, reason := validator.Validate(from); reason != llmgen.ReasonOK {
			continue
		}
		if _, reason := validator.Validate(to); reason != llmgen.ReasonOK {
			continue
		}
		result.Links = append(result.Links, domain.WikiLink{
			FromPageID: from,
			ToPageID:   to,
			TargetType: domain.WikiLinkTargetTypeWiki,
			Anchor:     strings.TrimSpace(item.Anchor),
		})
	}
	return result, nil
}

func buildWikiGenerationPrompt(title, content string, maxPages int, pageType string) string {
	return fmt.Sprintf(`你是知识库整理助手。请将下面文档整理为互相关联的 wiki 页面。

要求：
1. 只输出严格 JSON，禁止任何额外文本或 markdown 代码块。
2. 最多生成 %d 个页面，页面类型为 "%s"。
3. JSON 结构：
{"pages":[{"slug":"entity/xxx","title":"标题","type":"entity","summary":"一句话","content":"# 标题\nMarkdown 正文"}],"links":[{"from":"entity/xxx","to":"concept/yyy","anchor":"锚点文本"}]}
4. slug 必须低熵、可读、唯一（如 entity/go 或 concept/并发）。type 只允许 entity 或 concept。
5. links 的 from/to 必须引用本批 pages 的 slug。

文档标题：%s

## 文档内容
%s`, maxPages, pageType, title, content)
}
