# Wiki P2：图谱检索源

日期：2026-08-09
状态：待审
来源借鉴：WeKnora `chat_pipeline/search_entity.go`（实体扩展检索）、goagent `SetFactMemoryRetriever` 注入先例

## 背景与范围

P0/P1 已建 wiki 页面 + 链接。**P2 让聊天检索能命中 wiki 页面及其图邻居**——把 wiki 作为 rag 引擎的第 5 个检索通道。

**P2 交付**：
1. `WikiRetriever`：给定查询，按标题/slug/摘要/content 关键词打分找到相关 wiki 页面（Postgres `ILIKE`，零 LLM），再经 `wiki_link` 邻接表展开 1 跳邻居（图检索）。
2. `wikiPageChannel`（`rag/core/retrieve`）：实现 `SearchChannel`，命中结果进 RRF 融合；仿 `SetFactMemoryRetriever` 用 `SetWikiRetriever` 注入。
3. 装配：bootstrap 构造 wiki repos + retriever 注入引擎。

**明确不做**：P3 Agent 能力、P4 前端、wiki 内容向量化（先用关键词 + 图邻居，向量通道后续）、Neo4j、GraphRAG 深度推理、wiki 页权限。

## 一、repo 扩展

`port.WikiPageRepository` 加两个方法：

```go
	// Search 按标题/slug/摘要/content 的 ILIKE 命中打分（title>slug>summary>content），返回 top limit。
	Search(ctx context.Context, kbID, query string, limit int) ([]domain.WikiPage, error)
	// ListByIDs 批量按 ID 取页面（图邻居回填）。
	ListByIDs(ctx context.Context, kbID string, ids []string) ([]domain.WikiPage, error)
```

Postgres 实现（`wiki_page_repo.go`）：

```go
func (r *WikiPageRepository) Search(ctx context.Context, kbID, query string, limit int) ([]domain.WikiPage, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	like := "%" + query + "%"
	var rows []models.WikiPageModel
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND (title ILIKE ? OR slug ILIKE ? OR summary ILIKE ? OR content ILIKE ?)", kbID, like, like, like, like).
		Order("CASE WHEN title ILIKE ? THEN 0 WHEN slug ILIKE ? THEN 1 WHEN summary ILIKE ? THEN 2 ELSE 3 END ASC, update_time DESC", like, like, like).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("search wiki pages: %w", err)
	}
	pages := make([]domain.WikiPage, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, toWikiPageDomain(row))
	}
	return pages, nil
}

func (r *WikiPageRepository) ListByIDs(ctx context.Context, kbID string, ids []string) ([]domain.WikiPage, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []models.WikiPageModel
	if err := r.db.WithContext(ctx).Where("kb_id = ? AND id IN ?", kbID, ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list wiki pages by ids: %w", err)
	}
	pages := make([]domain.WikiPage, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, toWikiPageDomain(row))
	}
	return pages, nil
}
```

## 二、rag 侧接口与通道

### `rag/core/retrieve/wiki_channel.go`

```go
package retrieve

import (
	"context"
	"time"

	"local/rag-project/internal/framework/convention"
)

const (
	ChannelWikiPage = "wiki_page"
)

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
	expandedTopK := expandChannelTopK(searchCtx.TopK, defaultChannelTopKMultiplier)
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
		"topK":         searchCtx.TopK,
		"expandedTopK": expandedTopK,
		"multiplier":   defaultChannelTopKMultiplier,
		"rrfWeight":    defaultChannelRRFWeight(c.Name()),
	}), nil
}
```

`defaultChannelRRFWeight` 加 `case ChannelWikiPage: return 0.75`。

### `Engine.SetWikiRetriever` + `rebuildChannels`

`service.go` 的 `Engine` 加 `wikiRetriever WikiRetriever` 字段 + `SetWikiRetriever(r)`；`rebuildChannels` 在 `SetFactMemoryRetriever` 同位置追加 `NewWikiPageChannel(r)`。

## 三、WikiRetriever 实现

`internal/app/rag/service/wikiretrieval/retriever.go`：

```go
package wikiretrieval

import (
	"context"
	"sort"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
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

// SearchWiki 召回命中页（score 递减）+ 1 跳图邻居（低分）。
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
		// 图邻居：与命中页有直接链接的页面（双向）
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
```

## 四、装配

`internal/bootstrap/rag/runtime.go`（或 `runtime_build_retrieve.go`）：构造 wiki repos + retriever，注入 `retrieve.Engine`：

```go
	wikiPageRepo := postgresknowledge.NewWikiPageRepository(db)
	wikiLinkRepo := postgresknowledge.NewWikiLinkRepository(db)
	wikiRetriever := wikiretrieval.NewRetriever(wikiPageRepo, wikiLinkRepo)
	retrieveEngine.SetWikiRetriever(wikiRetriever)
```

（位置参照现有 `SetFactMemoryRetriever` 的装配点。）

## 五、测试策略

- **repo**：`Search`（ILIKE 命中 title 优先于 content、空 query 返回 nil、limit）、`ListByIDs`（批量、空输入）。
- **channel**：Enabled 模式（auto/keyword/hybrid 开，semantic 关）、nil retriever 禁用、Search 调 retriever 返回 chunks。
- **retriever**：命中页 score 0.6 + metadata `record_type=wiki_page`；邻居页 score 0.3 + `wiki_neighbor=true`；去重；`ListByIDs` 回填。
- **装配**：build 通过。
- **回归**：`go test ./internal/app/knowledge/... ./internal/app/rag/core/retrieve/... ./internal/bootstrap/... -count=1` + 全量。

## 六、明确不做（YAGNI）

- P3 Agent 能力、P4 前端、wiki 内容向量化、Neo4j、GraphRAG 深度推理、wiki 页权限、wiki 邻居多跳（仅 1 跳）。
- 不把 wiki 页面写入 chunk 索引/向量表。

## 回归约束

- `SearchChannel` 接口不变；新增常量/通道/注入点均为增量。
- 复用 `convention.RetrievedChunk`；不引入新依赖。
- 迁移不动（repo 方法纯查询）。
