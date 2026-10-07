# Wiki P3：Agent 编排能力（wiki_write）

> 2026-10-01 适用范围：历史设计保留：旧 internal/app/agent 能力接入链路已移除。本文的 wiki_write Agent 编排方案不能作为当前实现或自动写入权限的依据。

日期：2026-08-09
状态：待审

## 背景与范围

P0-P2 已建成 wiki 生成（ingestion 节点）与检索（检索通道）。**P3 让用户用自然语言触发 wiki 生成**——"把 XX 文档整理成 wiki"，由 agent 运行时编排。

**P3 交付**：
1. 新 capability `wiki_write`（`KindWorkflow`，仿 `document_investigation`）：输入文档 ID + 知识库 + 指令 → 读内容 → `WikiGenerator` 生成 → `WikiPageService` 持久化 + linkify → 返回页面/链接数。
2. capability 常量（`NameWikiWrite`/`FamilyWiki`/`RoleWriteWiki`）+ registry 白名单。
3. `DocumentContentReader` 适配器（按文档 ID 取解析内容）。
4. 装配进 agent capability registry（`service_assembly.go`）。

**明确不做**：P4 前端、wiki 页编辑/修订工具集（WeKnora 的 read/write/rename/delete 10 工具）、Agent 直接把 wiki 页作为检索上下文、多文档批量、Agent 修改已有 wiki 页（仅新建/覆盖）。

## 一、capability 常量 + 白名单

`internal/app/agent/capability/spec.go`：

```go
	NameWikiWrite = "wiki_write"
...
	FamilyWiki = "wiki"
...
	RoleWriteWiki = "write_wiki"
```

`knownFamilies` 加 `FamilyWiki: "generation"`（wiki 生成属 generation 工作流）；`knownRoles` 加 `RoleWriteWiki: {}`。

## 二、`internal/app/agent/wiki_write/capability.go`

```go
package wiki_write

// DocumentContentReader 取文档解析内容（标题 + 正文）。
type DocumentContentReader interface {
	ReadContent(ctx context.Context, documentID string) (title string, content string, err error)
}

// WikiWriter 是 wiki 持久化端口（由 *wikiservice.WikiPageService 满足）。
type WikiWriter interface {
	UpsertPagesFromDocument(ctx context.Context, kbID string, pages []domain.WikiPage, links []domain.WikiLink) error
	LinkifyAndPersist(ctx context.Context, kbID string, pages []domain.WikiPage, extraLinks []domain.WikiLink) (int, error)
}

// PromptCompleter 供 WikiGenerator 调用。
type PromptCompleter interface {
	Chat(prompt string) (string, error)
}

type CapabilityInput struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	DocumentID      string `json:"document_id"`
	MaxPages        int    `json:"max_pages,omitempty"`
	PageType        string `json:"page_type,omitempty"`
	Instruction     string `json:"instruction,omitempty"`
}

type CapabilityOutput struct {
	DocumentID string   `json:"document_id"`
	PageIDs    []string `json:"page_ids,omitempty"`
	PageCount  int      `json:"page_count"`
	LinkCount  int      `json:"link_count"`
}
```

`capabilityAdapter{spec, reader, writer, completer}`：
- `NewCapability(reader, writer, completer, options...) (agentcapability.Handle, error)`
- Spec：`KindWorkflow`、`FamilyWiki`、`Roles=[RoleWriteWiki]`、`RiskLevelMedium`（写操作）、`SupportsParallel=false`、`ProducesEvidence=true`、`IdempotencyBestEffort`、Preconditions（document_id/knowledge_base_id non_empty）。
- `Invoke`：
  1. `ReadContent(documentID)` → title/content；空内容 → `DependencyFailureResult`。
  2. `generator := wikiservice.NewLLMWikiGenerator(c.completer)` → `GenerateFromDocument(ctx, title, content, WikiGenerationOptions{MaxPages, PageType})`；空页 → 降级（返回空 output，StatusSucceeded 或 Degraded，见下）。
  3. `writer.UpsertPagesFromDocument(ctx, kbID, pages, links)` → `writer.LinkifyAndPersist(ctx, kbID, pages, links)` → 统计。
  4. 组装 `CapabilityOutput{PageIDs: 取 pages slug→id 需额外查询，P3 简化：PageIDs 留空或返回 pages 数量}` —— 简化：`PageCount=len(pages)`、`LinkCount=linkify 返回数`；PageIDs 通过 writer 扩展方法可选，P3 不强制。
  5. `InvocationResult`：`Action{Name, Summary: "generate wiki from document X"}`、`Observation`、`Delta.Notes`（页面/链接数）。

（`wikiservice` 的 `WikiGenerator`/`WikiPageService`/`WikiGenerationOptions`/`WikiGenerationResult` 已存在；`domain.WikiPage/WikiLink` 已存在。）

## 三、DocumentContentReader 适配器

`internal/app/agent/wiki_write/contentreader.go`：

```go
// ChunkLister 读取文档的 chunk 列表。
type ChunkLister interface {
	ListByDocumentID(ctx context.Context, documentID string, limit int) ([]domain.KnowledgeChunk, error)
}

type contentReader struct {
	docRepo  knowledgedomain...
	chunkLister ChunkLister
}

// ReadContent 取文档标题 + 按 chunk 顺序拼接内容（单 chunk 上限，超长截断）。
func (r *contentReader) ReadContent(ctx context.Context, documentID string) (string, string, error)
```

实现要点：
- 用 `KnowledgeDocumentRepository.GetByID` 取标题；用 `ChunkLister.ListByDocumentID` 取 chunks 并按 index 排序拼接 `Content`（截断到 ~20000 rune，参照 generator 的 `maxContentForWiki` 精神）。
- 若 chunk 内容为空，回退用文档 `Summary`。

（若 knowledge 已有按 documentID 取 chunk 列表的 service 方法，用 `KnowledgeChunkService.Page` 或 port 方法；实现者按实际可用 API 适配。）

## 四、装配

`internal/app/agent/service_assembly.go`（或 agent runtime 的 capability 注册点）：构造 `wiki_write.NewCapability(reader, writer, completer)` 并注册进 registry，使 LLM 规划器可选。`writer` 用 `*wikiservice.WikiPageService`（在 rag bootstrap 或 agent assembly 中从 wiki repos 构造，或在 knowledge runtime 暴露后传入）。

## 五、测试策略

- capability 单测：stub reader/writer/completer；`Invoke` 成功（生成 2 页 + linkify 返回链接数）、空内容降级、completer 失败降级、precondition 校验。
- registry 白名单：`FamilyWiki`/`RoleWriteWiki` 注册不报错（registry 测试）。
- 装配：build 通过。
- 回归：`go test ./internal/app/agent/... ./internal/app/knowledge/... ./internal/app/rag/... -count=1` + 全量。

## 六、明确不做（YAGNI）

- P4 前端、wiki 页编辑工具集、Agent 读 wiki 页作上下文、多文档批量、Agent 编辑已有页、PageIDs 精确返回（P3 只返回计数，slug→id 已在 service 内部）。

## 回归约束

- capability 常量只追加；`knownFamilies`/`knownRoles` 白名单只加新条目。
- 复用 `wikiservice`/`domain`；不引入新依赖。
- 不改 ingestion/rag 既有行为。
