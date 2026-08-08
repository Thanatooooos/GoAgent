# 文档父子切块与 LLM 增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有入库流水线中提供父子切块、LLM 文档摘要、预测问题独立索引及检索回链。

**Architecture:** 扩展 `ExecutionState` 的 chunk 描述，父块仅持久化，子块和预测问题共享现有 pgvector 表。增强节点通过可注入的 LLM 生成摘要和问题；索引器负责稳定 ID、整文档替换与补偿清理。检索命中问题后映射到源子块，再按父块扩展回答上下文。

**Tech Stack:** Go、Gin、GORM/PostgreSQL、pgvector、现有 ingestion workflow、现有 `infra-ai/chat` LLM 抽象。

---

## 文件结构

- 修改 `internal/app/core/chunk/types.go`、新增 `parent_child.go`：父/子 chunk 的纯计算模型与算法。
- 修改 `internal/app/ingestion/service/workflow/workflow_execution_state.go`：在状态中传递 chunk 类型、父引用、预测问题和摘要。
- 修改 `internal/app/ingestion/service/runner/runner_chunker.go`：读取父子切块配置并填充状态。
- 修改 `internal/app/ingestion/service/runner/runner_enhancer.go`、新增 `llm_enrichment.go`：LLM 摘要和问题生成、清洗与降级。
- 修改 knowledge domain/port/repository/migration：持久化父块关联与文档摘要。
- 修改 `runner_indexer.go`、vector store：写父子块和问题向量，支持完整补偿删除。
- 修改 RAG retrieval：将问题命中归一为子块，去重并展开父块。

### Task 1: 纯父子切块模型

**Files:**
- Modify: `internal/app/core/chunk/types.go`
- Create: `internal/app/core/chunk/parent_child.go`
- Test: `internal/app/core/chunk/test/parent_child_test.go`

- [ ] **Step 1: 写失败测试，验证父块与全局递增的子块索引**

```go
func TestSplitParentChildKeepsParentReferenceAndGlobalChildIndex(t *testing.T) {
    result, err := chunk.SplitParentChild("abcdefghij", chunk.Options{Strategy: chunk.StrategyFixedSize, ChunkSize: 6}, chunk.Options{Strategy: chunk.StrategyFixedSize, ChunkSize: 3})
    require.NoError(t, err)
    require.Len(t, result.Parents, 2)
    require.Equal(t, []int{0, 1, 2, 3}, childIndexes(result.Children))
    require.Equal(t, 0, result.Children[0].ParentIndex)
    require.Equal(t, 1, result.Children[2].ParentIndex)
}
```

- [ ] **Step 2: 运行失败测试**

Run: `go test ./internal/app/core/chunk/test -run TestSplitParentChildKeepsParentReferenceAndGlobalChildIndex -count=1`

Expected: FAIL，因为 `SplitParentChild` 尚不存在。

- [ ] **Step 3: 实现最小父子模型**

```go
type ParentChildResult struct { Parents []Chunk; Children []ChildChunk }
type ChildChunk struct { Chunk; ParentIndex int }

func SplitParentChild(text string, parent, child Options) (ParentChildResult, error) {
    parents, err := selector.Chunk(text, parent.Normalize())
    if err != nil { return ParentChildResult{}, err }
    // 对每个 parent 复用 selector，child.Index 从 0 全局递增。
}
```

- [ ] **Step 4: 增加边界测试并验证**

覆盖单父块、空文本、子块与父块相等、无效 overlap 被 Normalize；运行：`go test ./internal/app/core/chunk/test -count=1`。

### Task 2: 工作流状态和 Chunker 配置

**Files:**
- Modify: `internal/app/ingestion/service/workflow/workflow_execution_state.go`
- Modify: `internal/app/ingestion/service/runner/runner_chunker.go`
- Test: `internal/app/ingestion/service/runner/runner_chunker_test.go`

- [ ] **Step 1: 写失败测试，验证开关关闭时保持扁平输出**

```go
state, output, err := runner.Run(ctx, parsedState("abcdef"), nodeWithSettings(map[string]any{"chunkSize": 3}))
require.NoError(t, err)
require.Empty(t, state.ParentChunks)
require.Equal(t, "flat", output["chunkMode"])
```

- [ ] **Step 2: 写失败测试，验证开启后的父引用**

```go
state, _, err := runner.Run(ctx, parsedState(longText), nodeWithSettings(map[string]any{
  "enableParentChild": true, "parentChunkSize": 8, "childChunkSize": 3,
}))
require.NoError(t, err)
require.NotEmpty(t, state.ParentChunks)
require.NotEmpty(t, state.Chunks[0].ParentIndex)
```

- [ ] **Step 3: 实现状态字段与 Runner 映射**

```go
type ChunkPayload struct { Index int; Content string; Metadata map[string]any; ParentIndex *int }
type ParentChunkPayload struct { Index int; Content string }
// enableParentChild=false 仍调用 selector.Chunk；true 时调用 SplitParentChild。
```

- [ ] **Step 4: 运行验证**

Run: `go test ./internal/app/ingestion/service/runner -run Chunker -count=1`

Expected: PASS。

### Task 3: LLM 摘要和预测问题增强

**Files:**
- Create: `internal/app/ingestion/service/runner/llm_enrichment.go`
- Modify: `internal/app/ingestion/service/runner/runner_enhancer.go`
- Modify: `internal/bootstrap/ingestion/runtime.go`
- Test: `internal/app/ingestion/service/runner/llm_enrichment_test.go`

- [ ] **Step 1: 写失败测试，验证问题清洗和摘要采样**

```go
func TestNormalizeQuestionsRemovesDuplicatesAndInvalidLines(t *testing.T) {
  got := normalizeQuestions([]string{"如何配置 A？", "如何配置 A？", "", strings.Repeat("x", 101)}, 2, 100)
  require.Equal(t, []string{"如何配置 A？"}, got)
}
func TestSampleDocumentKeepsHeadMiddleAndTail(t *testing.T) { /* assert all three markers */ }
```

- [ ] **Step 2: 写失败测试，验证 LLM 失败只降级**

```go
next, _, err := runner.Run(ctx, stateWithChunks(), nodeWithSettings(map[string]any{"tasks": []any{"summary", "questions"}}))
require.NoError(t, err)
require.Equal(t, "failed", next.Enrichment.SummaryStatus)
require.Empty(t, next.Chunks[0].Questions)
```

- [ ] **Step 3: 实现可注入 LLM 生成器与状态**

```go
type DocumentEnricher interface {
  Summarize(context.Context, string, EnrichmentOptions) (string, error)
  GenerateQuestions(context.Context, string, string, EnrichmentOptions) ([]string, error)
}
// nil generator 或调用失败：记录 status/error，返回 nil error，正文继续流转。
```

- [ ] **Step 4: 注入运行时并验证**

复用已有 `infra-ai/chat` 服务构造 `DocumentEnricher`；运行：`go test ./internal/app/ingestion/service/runner -run 'Enrichment|Enhancer' -count=1`。

### Task 4: 持久化模型、迁移与索引写入

**Files:**
- Create: `internal/adapter/repository/postgres/migrations/20260711100000_add_document_enrichment.sql`
- Modify: `internal/app/knowledge/domain/knowledge_chunk.go`
- Modify: `internal/app/knowledge/domain/knowledge_document.go`
- Modify: `internal/app/knowledge/port/repository.go`
- Modify: `internal/adapter/repository/postgres/knowledge/*`
- Modify: `internal/app/ingestion/service/runner/runner_indexer.go`
- Test: `internal/app/ingestion/service/runner/runner_indexer_test.go`

- [ ] **Step 1: 写失败测试，验证索引 ID 与关联 metadata**

```go
vectors := runner.buildVectorChunks(stateWithParentAndQuestions(), "doc-1", "doc", "kb-1", nil, embedded)
require.Equal(t, "child", vectors[0].Metadata["record_type"])
require.Equal(t, "doc-1-p-0", vectors[0].Metadata["parent_chunk_id"])
require.Equal(t, "question", vectors[1].Metadata["record_type"])
require.Equal(t, "doc-1-0", vectors[1].Metadata["source_chunk_id"])
```

- [ ] **Step 2: 增加迁移**

```sql
ALTER TABLE t_knowledge_chunk ADD COLUMN IF NOT EXISTS record_type VARCHAR(16) NOT NULL DEFAULT 'child';
ALTER TABLE t_knowledge_chunk ADD COLUMN IF NOT EXISTS parent_chunk_id VARCHAR(64);
ALTER TABLE t_knowledge_document ADD COLUMN IF NOT EXISTS summary TEXT;
ALTER TABLE t_knowledge_document ADD COLUMN IF NOT EXISTS summary_status VARCHAR(16) NOT NULL DEFAULT 'none';
CREATE INDEX IF NOT EXISTS idx_knowledge_chunk_parent ON t_knowledge_chunk(parent_chunk_id);
```

- [ ] **Step 3: 扩展仓储与 Indexer**

父块 ID 采用 `documentID + "-p-" + parentIndex`，子块保持 `documentID + "-" + childIndex`，问题 ID 为 `childID + "-q-" + questionIndex`。`DeleteByDocumentID` 必须清理三种块；向量删除继续按文档 ID 完成补偿。

- [ ] **Step 4: 验证补偿行为**

Run: `go test ./internal/app/ingestion/service/runner -run Indexer -count=1`

Expected: PASS，且向量写入失败时记录的 chunk/vector 均被清理。

### Task 5: 检索问题回链与父块展开

**Files:**
- Modify: `internal/app/rag/core/retrieve/*`（以实际检索聚合器文件为准）
- Modify: `internal/app/knowledge/port/repository.go`
- Modify: `internal/adapter/repository/postgres/knowledge/*`
- Test: `internal/app/rag/core/retrieve/*_test.go`

- [ ] **Step 1: 写失败测试，验证问题命中归一和去重**

```go
hits := normalizeEnrichedHits([]SearchHit{
  hit("doc-1-0-q-0", 0.91, map[string]any{"record_type":"question", "source_chunk_id":"doc-1-0"}),
  hit("doc-1-0", 0.80, map[string]any{"record_type":"child"}),
})
require.Len(t, hits, 1)
require.Equal(t, "doc-1-0", hits[0].ChunkID)
require.Equal(t, "question", hits[0].Metadata["retrieval_source"])
```

- [ ] **Step 2: 实现归一与批量父块加载**

```go
// question -> source_chunk_id；child -> 自身 ChunkID；按最高分保留。
// 对 parent_chunk_id 非空的结果，repository 批量 GetByIDs 后将 ParentContent 填入 Prompt 上下文；引用 ID 保持 child ID。
```

- [ ] **Step 3: 写并通过兼容测试**

覆盖 metadata 缺失的旧向量、无父块的旧子块和父块不存在时的回退。运行相关 RAG 检索包测试。

### Task 6: 端到端验证与配置说明

**Files:**
- Modify: `configs/application.yaml`
- Modify: `docs/project_progress_context.md`
- Test: `internal/app/knowledge/service/test/knowledge_document_pipeline_integration_test.go`

- [ ] **Step 1: 添加集成失败测试**

使用 fake embedding、fake `DocumentEnricher` 和内存仓储构造 `parser -> chunker(parent-child) -> enricher -> indexer`，以预测问题查询，断言返回源 child 证据与 parent 上下文。

- [ ] **Step 2: 增加默认关闭的配置示例**

```yaml
ingestion:
  enrichment:
    enabled: false
    question-count: 2
    summary-max-input-chars: 12000
```

- [ ] **Step 3: 运行定向测试与静态检查**

Run: `go test ./internal/app/core/chunk/test ./internal/app/ingestion/service/runner ./internal/app/knowledge/service/test ./internal/app/rag/core/retrieve -count=1`

Run: `go vet ./...`

Expected: 定向测试全绿；若全仓 `go vet` 仍被既有临时多 main 文件阻断，记录为既有工作区问题，不作为本功能失败。

- [ ] **Step 4: 更新项目进度文档**

记录默认开关、模型成本边界、问题索引回链和降级语义，供演示与面试使用。
