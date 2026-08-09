# Wiki P0：wiki 页面数据模型 + wiki_generator 节点

日期：2026-08-09
状态：待审
来源借鉴：WeKnora `internal/types/wiki_page.go`、`wiki_ingest_*`（W1 数据模型 + W4 防幻觉护栏）

## 背景与范围

WeKnora 的 Wiki 模式是"文档 → LLM 抽取 → 自动生成互链 Markdown 知识库 → 图谱"。goagent 采用分阶段落地。本轮 **P0**：打通"文档 → 自动生成 wiki 页面"的最小闭环。

**P0 交付**：
1. 数据模型 `wiki_page` / `wiki_link` + 迁移 + repo。
2. `WikiPageService`（持久化 + 查询）+ `WikiGenerator`（LLM → 结构化页面，复用 llmgen 护栏）。
3. ingestion `wiki_generator` 节点（接入现有 DAG）。
4. 最小读 API。

**明确不做（后续轮次）**：P1 链接建立、P2 图谱检索源、P3 Agent 能力、P4 前端、wiki 版本管理（revisions）、wiki 独立任务队列、wiki 页面进入向量/关键词索引。

## 一、数据模型

### wiki_page（表 `t_wiki_page`）

`internal/app/knowledge/domain/wiki_page.go`：

```go
const (
	WikiPageTypeSummary = "summary" // 每文档一页，聚合文档要点
	WikiPageTypeEntity  = "entity"  // 实体页
	WikiPageTypeConcept = "concept" // 概念页
	WikiPageTypeIndex   = "index"   // 每 KB 一页的目录页
)

const (
	WikiPageStatusDraft     = "draft"
	WikiPageStatusPublished = "published"
)

type WikiPage struct {
	ID                string
	KnowledgeBaseID   string
	Slug              string // (kb_id, slug) 唯一；如 entity/go-kit
	Title             string
	PageType          string
	Status            string
	Content           string // Markdown 正文
	Summary           string
	SourceDocumentIDs []string // 来源文档 ID（JSONB），删除溯源
	SourceChunkIDs    []string // 来源 chunk ID（JSONB，P0 可空）
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
```

### wiki_link（表 `t_wiki_link`）

```go
const (
	WikiLinkTargetTypeWiki     = "wiki"
	WikiLinkTargetTypeExternal = "external"
)

type WikiLink struct {
	ID              string
	KnowledgeBaseID string
	FromPageID      string // 源页
	ToPageID        string // 目标页（external 时可为空）
	TargetType      string
	Anchor          string // 展示文本/锚点
	CreatedAt       time.Time
}
```

## 二、迁移

`internal/adapter/repository/postgres/migrations/20260809000000_create_wiki_tables.sql`（纯 SQL，`--` 注释跳过；沿用 `migration_runner.go` 的语句切分）：

```sql
-- Wiki 页面表
CREATE TABLE IF NOT EXISTS t_wiki_page (
    id              VARCHAR(20) PRIMARY KEY,
    kb_id           VARCHAR(20) NOT NULL,
    slug            VARCHAR(256) NOT NULL,
    title           VARCHAR(256) NOT NULL,
    page_type       VARCHAR(16) NOT NULL DEFAULT 'entity',
    status          VARCHAR(16) NOT NULL DEFAULT 'published',
    content         TEXT NOT NULL DEFAULT '',
    summary         TEXT NOT NULL DEFAULT '',
    source_document_ids JSONB NOT NULL DEFAULT '[]',
    source_chunk_ids    JSONB NOT NULL DEFAULT '[]',
    created_by      VARCHAR(20) NOT NULL,
    updated_by      VARCHAR(20) NOT NULL,
    create_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT NOT NULL DEFAULT 0,
    CONSTRAINT uk_wiki_page_kb_slug UNIQUE (kb_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_wiki_page_kb ON t_wiki_page (kb_id);

-- Wiki 页面间链接表
CREATE TABLE IF NOT EXISTS t_wiki_link (
    id              VARCHAR(20) PRIMARY KEY,
    kb_id           VARCHAR(20) NOT NULL,
    from_page_id    VARCHAR(20) NOT NULL,
    to_page_id      VARCHAR(20),
    target_type     VARCHAR(16) NOT NULL DEFAULT 'wiki',
    anchor          VARCHAR(256) NOT NULL DEFAULT '',
    create_time     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted         SMALLINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_wiki_link_from ON t_wiki_link (from_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_link_to ON t_wiki_link (to_page_id);
```

## 三、Repo（port + postgres）

`internal/app/knowledge/port/repository.go` 追加接口（或新增 `wiki_port.go`）：

```go
type WikiPageRepository interface {
	Upsert(ctx context.Context, page domain.WikiPage) (domain.WikiPage, error) // 按 (kb_id, slug) upsert
	GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error)
	ListByKB(ctx context.Context, kbID string, offset, limit int) ([]domain.WikiPage, int, error)
	DeleteByKB(ctx context.Context, kbID string) error
}

type WikiLinkRepository interface {
	CreateBatch(ctx context.Context, links []domain.WikiLink) error
	ReplaceByKBAndFrom(ctx context.Context, kbID, fromPageID string, links []domain.WikiLink) error // 事务内：删旧写新
	ListByKB(ctx context.Context, kbID string) ([]domain.WikiLink, error)
}
```

Postgres 实现（`internal/adapter/repository/postgres/knowledge/wiki_page_repo.go` + `wiki_link_repo.go` + `models/wiki_page_model.go` + `models/wiki_link_model.go`），沿用 `KnowledgeDocumentRepository` 的 gorm 模式（`TableName()`、soft_delete flag、JSONB 用 `datatypes.JSONSlice[string]` 或 `[]byte`+手写序列化——沿用 `ChunkConfig []byte jsonb` 的先例，domain 层 []string ↔ model 层 JSON 字节互转）。

## 四、服务层

### WikiPageService（`internal/app/knowledge/service/wiki/wiki_page_service.go`）

```go
type WikiPageService struct {
	pageRepo  port.WikiPageRepository
	linkRepo  port.WikiLinkRepository
}

// UpsertPagesFromDocument 事务内：按 slug upsert 页面 + 替换该文档来源页面的链接。
func (s *WikiPageService) UpsertPagesFromDocument(ctx context.Context, kbID string, pages []domain.WikiPage, links []domain.WikiLink) error

func (s *WikiPageService) GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error)
func (s *WikiPageService) ListByKB(ctx context.Context, kbID string, page, pageSize int) ([]domain.WikiPage, int, error)
```

### WikiGenerator（`internal/app/knowledge/service/wiki/wiki_generator.go`）

```go
type WikiGenerationOptions struct {
	MaxPages    int // 每文档生成页数上限，默认 5
	PageType    string // 生成主类型：entity（默认）/ concept
}

type WikiGenerationResult struct {
	Pages []domain.WikiPage // SourceDocumentIDs 已填文档 ID
	Links []domain.WikiLink // from/to 为页面 slug（持久化时映射为 page id）
}

type WikiGenerator interface {
	GenerateFromDocument(ctx context.Context, title, content string, options WikiGenerationOptions) (WikiGenerationResult, error)
}
```

`llmWikiGenerator` 实现：
- **prompt**：要求输出严格 JSON（禁止额外文本）：`{"pages":[{"slug":"entity/go-kit","title":"Go Kit","type":"entity","summary":"…","content":"# 标题\n…Markdown…"}],"links":[{"from":"entity/go-kit","to":"concept/微服务","anchor":"微服务"}]}`。
- **解析 + 确定性校验（复用 llmgen）**：
  - 每页：`slug` 非空且唯一、`title` 非空、`type` 在 {entity, concept} 内；违规页**丢弃**并计入 rejected。
  - 链接：`from`/`to` 必须命中本批页面 slug 集合 —— 用 `llmgen.NewRefValidator(本批slug)` 校验，未命中**拒绝**（fail-closed）。
  - 使用 `llmgen.RewriteRefs` 无需（纯 JSON 场景）；`HandleSet` 本轮不需要（slug 是低熵人工可读值，模型直接复制即可）。
- **失败降级**：LLM 错误 / JSON 解析失败 → 返回空 `WikiGenerationResult`（不阻断 ingestion，与 enricher 的 `llmDegraded` 一致）。

## 五、ingestion 节点

`internal/app/ingestion/domain/pipeline.go` 追加常量：

```go
	// PipelineNodeTypeWikiGenerator 表示由 LLM 生成 wiki 页面的节点。
	PipelineNodeTypeWikiGenerator = "wiki_generator"
```

`internal/app/ingestion/service/runner/runner_wiki_generator.go`（实现 `NodeRunner`）：

```go
type WikiGeneratorNodeRunner struct {
	service   *wiki.WikiPageService
	generator wiki.WikiGenerator
}

func (r *WikiGeneratorNodeRunner) NodeType() string { return domain.PipelineNodeTypeWikiGenerator }

func (r *WikiGeneratorNodeRunner) Run(ctx, state, node) (state, output, error) {
	// 读取 node.Settings: maxPages, pageType, wiki 目标 kbID（默认取 state.Task.Metadata["knowledgeBaseId"]）
	// 调 generator.GenerateFromDocument(state.Parsed.Title, state.Parsed.Content, options)
	// 成功且页数>0 → service.UpsertPagesFromDocument(kbID, pages, links)
	// 失败/为空 → 降级（不报错）
	// output: pageCount, rejectedCount, degraded
}
```

`internal/bootstrap/ingestion/runtime.go` 的 `NewNodeRunnerRegistry` 追加注册（在 indexer 之后）：

```go
		ingestionservice.NewWikiGeneratorNodeRunner(wikiPageService, ingestionservice.NewLLMWikiGenerator(aiRuntime.Chat)),
```

其中 `wikiPageService` 由 ingestion bootstrap 内构造（`postgresknowledge.NewWikiPageRepository(db)` + `NewWikiLinkRepository(db)`）。

## 六、读 API

`internal/adapter/http/knowledge/wiki_page_handler.go`：

```go
func RegisterWikiPageRoutes(r gin.IRoutes, service *wiki.WikiPageService) {
	r.GET("/knowledge-base/:kbId/wiki/pages", handler.List)
	r.GET("/knowledge-base/:kbId/wiki/pages/:slug", handler.Get)
}
```

- `List`：分页 `current`/`size`，返回 `{records, total, size, current, pages}`（沿用 knowledge VO 风格）。
- `Get`：返回页面详情（含 content）。
- `cmd/server/main.go` 的 admin 组追加 `knowledgehttp.RegisterWikiPageRoutes(admin, runtime.WikiPageService)`。

## 七、测试策略

- **wiki 服务**：`UpsertPagesFromDocument`（同 slug 二次写入覆盖、链接替换、事务回滚）；`GetBySlug`/`ListByKB` 分页。
- **generator**：合法 JSON → 页面/链接正确映射；`slug` 缺失/重复/type 非法 → 丢弃计数；链接指向批外 slug → 拒绝；LLM 失败/坏 JSON → 空结果不报错。
- **runner**：`Run` 成功写页、失败降级不阻断、output 字段。
- **handler**：List/Get 响应形状。
- **迁移**：断言 `20260809000000_create_wiki_tables.sql` 含建表语句（沿用 `knowledge_base_repo_test.go` 的迁移 token 断言先例）。
- **回归**：`go test ./internal/app/knowledge/... ./internal/app/ingestion/... ./internal/adapter/http/knowledge/... -count=1` + 全量 `go test ./cmd/... ./internal/... -count=1`。

## 八、明确不做（YAGNI）

- P1-P4（链接建立、图谱检索、Agent、前端）。
- wiki 版本管理/revisions、wiki 任务队列、wiki 页面进检索索引、wiki 页面权限。
- GraphRAG / Neo4j。
- 目录文件夹（wiki_folders）。

## 回归约束

- 只新增（wiki 域/服务/节点/迁移/API），不改既有 knowledge/ingestion 行为；`PipelineNodeType*` 常量只追加。
- 新迁移不修改历史迁移。
- 复用 llmgen 护栏库（RefValidator），不新造。
- 不引入新依赖（JSON 解析用 stdlib）。
