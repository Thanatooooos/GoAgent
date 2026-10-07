# Wiki P1：链接建立

> 2026-10-01 适用范围：历史设计保留：wiki 链接规则仍可参考；基于旧 ingestion runner 的自动编排不再适用，当前文档入库不自动运行 wiki 生成。

日期：2026-08-09
状态：待审
来源借鉴：WeKnora `wiki_linkify.go`、`wiki_lint.go`（W5 链接完整性与纯文本后处理）

## 背景与范围

P0 已打通"文档 → LLM 生成 wiki 页面"闭环，但链接是 P0 简化：`t_wiki_link.from/to` 直接存 slug、无交叉链接、无链接统计。**P1 把 wiki 变成真正互链的知识库**。

**P1 交付**：
1. **slug→页面 ID 映射**（修正 P0 欠账）：持久化链接前把 from/to slug 解析为真实 `page_id`。
2. **交叉链接注入（linkify，核心）**：纯文本零 LLM，用 `llmgen.RewriteRefs` 扫描页面内容，把其他页面的标题/slug 精确匹配处插入 `[[slug|标题]]` markdown 链接，跳过代码块/已有链接。
3. **In/Out 链接统计**：`t_wiki_page` 加 `in_links`/`out_links` 列，写入后重算。
4. **死链清理**：移除指向不存在页面的链接。

**明确不做（后续轮次）**：P2 图谱检索、P3 Agent 能力、P4 前端、wiki 进检索索引、锚点语义匹配（只做标题/slug 精确匹配）、链接版本历史。

## 一、迁移

`internal/adapter/repository/postgres/migrations/20260810000000_add_wiki_link_counts.sql`：

```sql
ALTER TABLE t_wiki_page ADD COLUMN IF NOT EXISTS in_links INT NOT NULL DEFAULT 0;
ALTER TABLE t_wiki_page ADD COLUMN IF NOT EXISTS out_links INT NOT NULL DEFAULT 0;
```

（P0 的 `t_wiki_page` 迁移已合并，按 AGENT.md 新增迁移而非修改历史迁移。）

## 二、repo 扩展

`port.WikiPageRepository` 加 `ListBySlugs(ctx, kbID string, slugs []string) ([]domain.WikiPage, error)`；`port.WikiLinkRepository` 加 `DeleteMissingTargets(ctx, kbID string, validPageIDs []string) (int, error)` 与 `CountLinksByPage(ctx, kbID string) (map[string]int, map[string]int, error)`（in/out）。

Postgres 实现按既有 gorm 模式。

`models.WikiPageModel` 加 `InLinks int`/`OutLinks int`（`column:in_links`/`column:out_links`）。

## 三、WikiPageService 扩展

### 1. slug→id 映射（`UpsertPagesFromDocument`）

```go
// UpsertPagesFromDocument 按 slug 逐页 upsert，并把链接的 from/to slug 解析为
// 真实页面 ID 后持久化（本批页面 ID 来自 upsert 返回；批外 slug 走 ListBySlugs）。
func (s *WikiPageService) UpsertPagesFromDocument(ctx, kbID, pages, links) error
```

- upsert 每页并收集 `slugToID[slug] = created.ID`。
- 对 links：`from`/`to` 先在 slugToID 命中，未命中则用 `ListBySlugs` 批量查 KB 已有页面；仍未知的链接丢弃。
- 改写后 `FromPageID`/`ToPageID` 为真实 ID，再按源页 `ReplaceByKBAndFrom`。

### 2. 交叉链接注入（`WikiLinkBuilder`）

`internal/app/knowledge/service/wiki/wiki_link_builder.go`：

```go
type WikiLinkBuilder struct {
	pageRepo port.WikiPageRepository
}

// LinkifyPage 把 page.Content 中其他页面的标题/slug 精确匹配处改写为
// [[slug|标题]] markdown 链接；返回改写后的页面与派生链接（slug→页）。
// 跳过代码块 / 内联代码 / 已有链接（llmgen.RewriteRefs SkipCodeBlocks+SkipLinks+WordBoundary）。
// 规则按 Find 长度降序（长标题优先），避免子串误替换。
func (b *WikiLinkBuilder) LinkifyPage(ctx, kbID string, page domain.WikiPage, targets []domain.WikiPage) (domain.WikiPage, []domain.WikiLink, error)
```

实现：
- 从 `targets` 构建规则：每个目标页（排除自身）`Find = title`（title 空则用 slug）、`Replace = "[[slug|title]]"`；`Find` 长度降序。
- `llmgen.RewriteRefs(page.Content, rules, {SkipCodeBlocks:true, SkipLinks:true, WordBoundary:true})` → 新内容。
- 用 `llmgen` 的 `[[slug|title]]` 正则（`\[\[([^\]|]+)(?:\|([^\]]*))?\]\]`）解析改写后内容中的链接 → 派生 `WikiLink`（from=page.Slug, to=目标 slug, anchor=title）。
- 返回改写后的 page（Content 更新）与 links。

### 3. In/Out 统计（`RebuildLinkCounts`）

```go
// RebuildLinkCounts 从 wiki_link 重算每页 in/out 计数并写回 t_wiki_page。
func (s *WikiPageService) RebuildLinkCounts(ctx, kbID) error
```

- `CountLinksByPage` → 每组 (pageID, in/out) → `pageRepo.UpdateLinkCounts(ctx, counts)`（新增 repo 方法）。

### 4. 死链清理（`CleanDeadLinks`）

```go
// CleanDeadLinks 删除指向不存在页面的链接，返回清理数。
func (s *WikiPageService) CleanDeadLinks(ctx, kbID) (int, error)
```

- 取全部 links + 全部 pages → `DeleteMissingTargets(kbID, validIDs)`。

## 四、runner 编排

`runner_wiki_generator.go` 的 `Run` 在 `UpsertPagesFromDocument` 之后追加：

```go
// P1：交叉链接 + 统计
pages, err := wikiPageService.ListByKB(ctx, kbID, 1, 1000) // 全 KB 页面作 targets
updated, links, err := wikiLinkBuilder.LinkifyPages(ctx, kbID, result.Pages, pages)
// upsert updated pages 内容 + 替换 links + RebuildLinkCounts + CleanDeadLinks
```

runner 依赖从 `WikiPageService` 扩展为"service + linkBuilder"（或把 linkify 作为 `WikiPageService` 方法，runner 只依赖 service）。

**实现决策**：把 linkify 集成进 `WikiPageService`（新增 `LinkifyAndPersist(ctx, kbID, pages, extraLinks) (int, error)` 方法，内部用 `WikiLinkBuilder`），runner 在 `UpsertPagesFromDocument` 后调一次（传入 `result.Links` 作 extraLinks），再 `RebuildLinkCounts` + `CleanDeadLinks`。runner 只依赖 `WikiPageServicePort`（接口加 `LinkifyAndPersist`）。

**链接合并**：generator 的 LLM 语义链接（`result.Links`，可能引用正文未出现标题的页面）必须存活——`LinkifyAndPersist` 按 from slug 分组后与 linkify 派生链接做并集，按目标 slug 去重（派生优先，保留正文锚文本），再统一 `resolveLinkIDs` 并 `ReplaceByKBAndFrom`。每页仍无条件 replace：空组清除该源旧链接，保证 re-ingest 收敛。

## 五、测试策略

- **slug→id**：服务测试——批内链接解析、批外 slug 走 ListBySlugs、未知 slug 丢弃。
- **linkify**：标题匹配插入 `[[slug|title]]`；跳过代码块/已有链接；WordBoundary 防子串误替；长标题优先；派生 links 正确；自身不链自身。
- **统计**：RebuildLinkCounts 从链接重算 in/out。
- **死链**：CleanDeadLinks 移除指向缺失页面的链接。
- **runner**：Run 全流程（upsert→linkify→统计→清理）输出 pageCount/linkCount。
- **迁移**：token 断言新迁移含 `in_links`/`out_links`。
- **回归**：`go test ./internal/app/knowledge/... ./internal/app/ingestion/... ./internal/adapter/http/knowledge/... -count=1`。

## 六、明确不做（YAGNI）

- P2-P4、wiki 进检索索引、Neo4j、锚点语义匹配、链接版本历史、wiki_link 唯一索引（软删累积，P2 处理）。

## 回归约束

- 迁移只新增（不修改 P0 迁移）。
- 复用 llmgen（RewriteRefs）；不新造正则/文本工具。
- 不改既有知识/ingestion 行为；`WikiPageServicePort` 只加方法。
- 无新依赖。
