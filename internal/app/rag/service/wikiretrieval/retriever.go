package wikiretrieval

import (
	"context"
	"sort"

	"local/rag-project/internal/app/knowledge/domain"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

// WikiPageSearcher 是 retriever 依赖的最小 repo 接口。
type WikiPageSearcher interface {
	Search(ctx context.Context, kbID, query string, limit int) ([]domain.WikiPage, error)
	ListByIDs(ctx context.Context, kbID string, ids []string) ([]domain.WikiPage, error)
}

type WikiLinkLister interface {
	ListByKB(ctx context.Context, kbID string) ([]domain.WikiLink, error)
}

type Retriever struct {
	pages WikiPageSearcher
	links WikiLinkLister
}

func NewRetriever(pages WikiPageSearcher, links WikiLinkLister) *Retriever {
	return &Retriever{pages: pages, links: links}
}

// SearchWiki 召回命中页（score 0.6）+ 1 跳图邻居（score 0.3）。
func (r *Retriever) SearchWiki(ctx context.Context, request ragretrieve.WikiSearchRequest) ([]convention.RetrievedChunk, error) {
	if r == nil || r.pages == nil {
		return nil, nil
	}
	topK := request.TopK
	if topK <= 0 {
		topK = 10
	}
	var chunks []convention.RetrievedChunk
	seen := map[string]bool{}
	for _, kbID := range request.KnowledgeBaseIDs {
		pages, err := r.pages.Search(ctx, kbID, request.Query, topK)
		if err != nil {
			return nil, err
		}
		matchedIDs := make([]string, 0, len(pages))
		for _, page := range pages {
			chunks = append(chunks, toRetrievedChunk(page, true))
			seen[page.ID] = true
			matchedIDs = append(matchedIDs, page.ID)
		}
		neighborIDs := r.collectNeighbors(ctx, kbID, matchedIDs)
		if len(neighborIDs) == 0 {
			continue
		}
		neighbors, err := r.pages.ListByIDs(ctx, kbID, neighborIDs)
		if err != nil {
			return nil, err
		}
		for _, page := range neighbors {
			if seen[page.ID] {
				continue
			}
			seen[page.ID] = true
			chunks = append(chunks, toRetrievedChunk(page, false))
		}
	}
	return chunks, nil
}

func (r *Retriever) collectNeighbors(ctx context.Context, kbID string, matchedIDs []string) []string {
	if len(matchedIDs) == 0 || r.links == nil {
		return nil
	}
	links, err := r.links.ListByKB(ctx, kbID)
	if err != nil {
		return nil
	}
	matched := map[string]bool{}
	for _, id := range matchedIDs {
		matched[id] = true
	}
	neighborSet := map[string]bool{}
	for _, link := range links {
		if matched[link.FromPageID] && link.ToPageID != "" {
			neighborSet[link.ToPageID] = true
		}
		if matched[link.ToPageID] && link.FromPageID != "" {
			neighborSet[link.FromPageID] = true
		}
	}
	var ids []string
	for id := range neighborSet {
		if !matched[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func toRetrievedChunk(page domain.WikiPage, matched bool) convention.RetrievedChunk {
	metadata := map[string]any{
		"record_type":  "wiki_page",
		"wiki_slug":    page.Slug,
		"wiki_title":   page.Title,
		"wiki_page_id": page.ID,
	}
	if !matched {
		metadata["wiki_neighbor"] = true
	}
	score := float32(0.6)
	if !matched {
		score = 0.3
	}
	return convention.RetrievedChunk{
		ID:              page.ID,
		Text:            page.Content,
		Score:           score,
		DocumentID:      page.ID,
		KnowledgeBaseID: page.KnowledgeBaseID,
		Metadata:        metadata,
	}
}
