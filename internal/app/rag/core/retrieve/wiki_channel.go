package retrieve

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/framework/convention"
)

const ChannelWikiPage = "wiki_page"

type WikiSearchRequest struct {
	UserID           string
	Query            string
	KnowledgeBaseIDs []string
	TopK             int
}

// WikiRetriever 由 bootstrap 注入；实现按关键词 + 图邻居召回 wiki 页面。
type WikiRetriever interface {
	SearchWiki(ctx context.Context, request WikiSearchRequest) ([]convention.RetrievedChunk, error)
}

type wikiPageChannel struct {
	retriever WikiRetriever
}

func NewWikiPageChannel(retriever WikiRetriever) SearchChannel {
	return &wikiPageChannel{retriever: retriever}
}

func (c *wikiPageChannel) Name() string  { return ChannelWikiPage }
func (c *wikiPageChannel) Priority() int { return 12 }
func (c *wikiPageChannel) Enabled(ctx SearchContext) bool {
	if ctx.DocumentsOnly {
		return false
	}
	if c == nil || c.retriever == nil {
		return false
	}
	switch normalizeSearchMode(ctx.SearchMode) {
	case SearchModeAuto, SearchModeKeyword, SearchModeHybrid:
		return true
	default:
		return false
	}
}

func (c *wikiPageChannel) Search(ctx context.Context, searchCtx SearchContext) (SearchChannelResult, error) {
	startedAt := time.Now()
	expandedTopK := expandChannelTopK(searchCtx.RecallBudget, defaultChannelTopKMultiplier)
	chunks, err := c.retriever.SearchWiki(ctx, WikiSearchRequest{
		UserID:           strings.TrimSpace(searchCtx.UserID),
		Query:            strings.TrimSpace(searchCtx.Query),
		KnowledgeBaseIDs: append([]string(nil), searchCtx.KnowledgeBaseIDs...),
		TopK:             expandedTopK,
	})
	if err != nil {
		return SearchChannelResult{}, fmt.Errorf("wiki page search: %w", err)
	}
	return newChannelResult(c.Name(), chunks, startedAt, map[string]any{
		"topK":         searchCtx.RecallBudget,
		"expandedTopK": expandedTopK,
		"multiplier":   defaultChannelTopKMultiplier,
		"rrfWeight":    defaultChannelRRFWeight(c.Name()),
	}), nil
}
