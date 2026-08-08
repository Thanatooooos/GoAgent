# RAG 聊天引用协议（Citation Protocol）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 goagent 的 rag 聊天管线落地"临时句柄 + `<ref/>` 内联引用"协议：模型只接触 `cN` 句柄、按句柄内联引用、系统在流式输出时展开为 `<kb/>` 标签持久化，前端渲染为可点击引用角标。

**Architecture:** 新增纯算法包 `internal/app/rag/core/citation/`（请求级注册表 + 句柄编解码 + 流式展开 + 历史重压缩）。rag 聊天服务在 `prepareChat` 创建注册表、注册检索 chunk、重渲染 KnowledgeContext 为带句柄 XML、注入协议 prompt；`ragChatStreamCallback` 用 `StreamExpander` 在流式输出时展开 `<ref/>`；历史 assistant 消息在进入 prompt 前用 `CompactPublicCitations` 折叠回 `<ref/>`。前端 `MarkdownRenderer` 加 `rehype-raw` 渲染 `<kb/>` 为角标。

**Tech Stack:** Go 1.25、Gin、react-markdown v9 + rehype-raw、Radix UI。

**参考设计:** `docs/superpowers/specs/2026-08-08-citation-protocol-design.md`

---

## 文件结构

**新建（citation 包）：**
- `internal/app/rag/core/citation/handle_table.go` — 句柄表原语（key↔handle 双向映射）
- `internal/app/rag/core/citation/registry.go` — 请求级 `Registry`（cN/dN/bN 三表，Register/Resolve）
- `internal/app/rag/core/citation/protocol.go` — 协议 prompt
- `internal/app/rag/core/citation/render.go` — `RenderKnowledgeContext` + `RenderKnowledgeContextWithBudget`
- `internal/app/rag/core/citation/expand.go` — `ExpandText` + `StreamExpander`
- `internal/app/rag/core/citation/compact.go` — `CompactPublicCitations`
- 各 `_test.go`

**修改（后端）：**
- `internal/app/rag/core/prompt/service.go` — `Context` 加 `CitationProtocol` 字段 + `BuildMessages` 格式化
- `internal/app/rag/service/chat/stage_types.go` — `ragChatRuntimeState` 加 `citation *citation.Registry`
- `internal/app/rag/service/chat/prepare_orchestrator.go` — `prepareChat` 创建注册表/重压缩/注册渲染
- `internal/app/rag/service/chat/execute_tool_workflow.go` — `runPromptStage` 加 protocol 参数、`applyRetrieveContextBudget` 感知注册表
- `internal/app/rag/service/chat/service.go` — runtimeEnabled 判定 + 传参
- `internal/app/rag/service/chat/execute_orchestrator.go` — `runStreamingAnswer` 加 expander 参数
- `internal/app/rag/service/chat/execute_streaming.go` — callback 流式展开 + Flush
- `internal/app/rag/service/chat/deps.go` — `RagChatOptions.CitationEnabled` + `RagChatService.citationEnabled`
- `internal/framework/config/config.go` — `RagConfig.CitationEnabled`
- `configs/application.yaml` — `rag.citation-enabled: true`
- `internal/bootstrap/rag/runtime_build_chat.go` — 接线
- `internal/app/knowledge/service/chunk/knowledge_chunk_query_service.go` — `GetByID`
- `internal/adapter/http/knowledge/knowledge_chunk_handler.go` — `GET /knowledge-base/chunks/:chunkId`

**修改（前端）：**
- `frontend/package.json` — 加 `rehype-raw`
- `frontend/src/components/chat/MarkdownRenderer.tsx` — rehypeRaw + `kb` 组件
- `frontend/src/components/chat/citationContext.tsx`（新建）— 引用编号 Context
- `frontend/src/components/chat/CitationChip.tsx`（新建）— 角标 + Popover
- `frontend/src/components/chat/MessageItem.tsx` — 包 `CitationNumberProvider`
- `frontend/src/services/chatService.ts` — `getChunkDetail`
- `frontend/src/types/index.ts` — `ChunkDetail` 类型

---

## Task 1: citation 包句柄表（handle_table.go）

**Files:**
- Create: `internal/app/rag/core/citation/handle_table.go`
- Test: `internal/app/rag/core/citation/handle_table_test.go`

- [ ] **Step 1: 写失败测试**

```go
package citation

import "testing"

func TestHandleTableRegisterDedup(t *testing.T) {
	table := newHandleTable[string]("c")
	if got := table.register("chunk-a", "A"); got != "c1" {
		t.Fatalf("first register = %q, want c1", got)
	}
	if got := table.register("chunk-a", "A2"); got != "c1" {
		t.Fatalf("duplicate register = %q, want same handle c1", got)
	}
	if got := table.register("chunk-b", "B"); got != "c2" {
		t.Fatalf("second register = %q, want c2", got)
	}
}

func TestHandleTableResolve(t *testing.T) {
	table := newHandleTable[string]("c")
	table.register("chunk-a", "A")
	key, value, ok := table.resolve("c1")
	if !ok || key != "chunk-a" || value != "A" {
		t.Fatalf("resolve c1 = (%q, %q, %v)", key, value, ok)
	}
	if _, _, ok := table.resolve("c99"); ok {
		t.Fatal("resolve unknown handle should fail")
	}
	// handle lookup is case-insensitive
	if key, _, ok := table.resolve("C1"); !ok || key != "chunk-a" {
		t.Fatalf("resolve C1 (case-insensitive) = (%q, %v)", key, ok)
	}
}

func TestHandleTableHas(t *testing.T) {
	table := newHandleTable[int]("b")
	table.register("kb-x", 7)
	if !table.has("b1") {
		t.Fatal("has b1 = false, want true")
	}
	if table.has("b2") {
		t.Fatal("has b2 = true, want false")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/citation/ -run TestHandleTable -count=1`
Expected: FAIL（`undefined: newHandleTable`）

- [ ] **Step 3: 实现 handle_table.go**

```go
package citation

import (
	"strconv"
	"strings"
)

// handleTable maps durable identifiers to request-local handles and back.
// Handles are per-request and never persisted.
type handleTable[M any] struct {
	prefix   string
	byKey    map[string]string
	byHandle map[string]handleEntry[M]
	next     int
}

type handleEntry[M any] struct {
	key   string
	value M
}

func newHandleTable[M any](prefix string) *handleTable[M] {
	return &handleTable[M]{
		prefix:   prefix,
		byKey:    map[string]string{},
		byHandle: map[string]handleEntry[M]{},
		next:     1,
	}
}

// register returns the handle for key, reusing an existing one on duplicate.
func (t *handleTable[M]) register(key string, value M) string {
	if handle, ok := t.byKey[key]; ok {
		return handle
	}
	handle := t.prefix + strconv.Itoa(t.next)
	t.next++
	t.byKey[key] = handle
	t.byHandle[handle] = handleEntry[M]{key: key, value: value}
	return handle
}

func (t *handleTable[M]) has(handle string) bool {
	_, ok := t.byHandle[strings.ToLower(strings.TrimSpace(handle))]
	return ok
}

// resolve returns (durableKey, value, ok). Handle matching is case-insensitive.
func (t *handleTable[M]) resolve(handle string) (string, M, bool) {
	entry, ok := t.byHandle[strings.ToLower(strings.TrimSpace(handle))]
	if !ok {
		var zero M
		return "", zero, false
	}
	return entry.key, entry.value, true
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/app/rag/core/citation/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/rag/core/citation/handle_table.go internal/app/rag/core/citation/handle_table_test.go
git commit -m "feat: add citation handle table"
```

---

## Task 2: citation 注册表（registry.go）

**Files:**
- Create: `internal/app/rag/core/citation/registry.go`
- Test: `internal/app/rag/core/citation/registry_test.go`

- [ ] **Step 1: 写失败测试**

```go
package citation

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestRegistryRegisterAndResolveChunk(t *testing.T) {
	r := NewRegistry()
	handle := r.RegisterChunk(ChunkReference{ChunkID: "chunk-1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", DocumentTitle: "标题"})
	if handle != "c1" {
		t.Fatalf("handle = %q, want c1", handle)
	}
	if got := r.RegisterChunk(ChunkReference{ChunkID: "chunk-1"}); got != "c1" {
		t.Fatalf("re-register = %q, want same c1", got)
	}
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-1" || ref.DocumentID != "doc-1" || ref.KnowledgeBaseID != "kb-1" || ref.DocumentTitle != "标题" {
		t.Fatalf("resolve c1 = %+v, %v", ref, ok)
	}
	if _, ok := r.ResolveChunk("c9"); ok {
		t.Fatal("resolve unknown handle should fail")
	}
}

func TestRegistryHandleShapedIDsAreNotRegistered(t *testing.T) {
	r := NewRegistry()
	if got := r.RegisterChunk(ChunkReference{ChunkID: "c1"}); got != "" {
		t.Fatalf("handle-shaped id registered as %q, want empty", got)
	}
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	if got := r.RegisterChunk(ChunkReference{ChunkID: "c1"}); got != "c1" {
		t.Fatalf("existing c1 not echoed back, got %q", got)
	}
}

func TestRegistryRegisterChunks(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunks([]convention.RetrievedChunk{
		{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a"},
		{ID: "chunk-b", DocumentID: "doc-b", KnowledgeBaseID: "kb-b"},
	})
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-a" {
		t.Fatalf("first chunk resolve = %+v, %v", ref, ok)
	}
	ref, ok = r.ResolveChunk("c2")
	if !ok || ref.ChunkID != "chunk-b" || ref.DocumentID != "doc-b" {
		t.Fatalf("second chunk resolve = %+v, %v", ref, ok)
	}
}

func TestRegistryEmptyChunkIDNotRegistered(t *testing.T) {
	r := NewRegistry()
	if got := r.RegisterChunk(ChunkReference{}); got != "" {
		t.Fatalf("empty chunk id registered as %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/citation/ -run TestRegistry -count=1`
Expected: FAIL（`undefined: NewRegistry`）

- [ ] **Step 3: 实现 registry.go**

```go
package citation

import (
	"regexp"
	"strings"

	"local/rag-project/internal/framework/convention"
)

// ChunkReference carries the durable identity and display metadata for a
// registered knowledge chunk. Handles never persist; this struct is the
// registry-side mirror used to expand <ref/> into public <kb/> tags.
type ChunkReference struct {
	ChunkID         string
	DocumentID      string
	KnowledgeBaseID string
	DocumentTitle   string
}

var shortHandleRE = regexp.MustCompile(`(?i)^[cdb][1-9][0-9]*$`)

// Registry maps durable identifiers to request-local handles for one chat
// request. It is never persisted or accepted across requests.
type Registry struct {
	chunks *handleTable[ChunkReference]
	docs   *handleTable[struct{}]
	kbs    *handleTable[struct{}]
}

func NewRegistry() *Registry {
	return &Registry{
		chunks: newHandleTable[ChunkReference]("c"),
		docs:   newHandleTable[struct{}]("d"),
		kbs:    newHandleTable[struct{}]("b"),
	}
}

// RegisterChunk returns the cN handle for the chunk. A model-emitted
// handle-shaped ID is echoed back only when it already exists.
func (r *Registry) RegisterChunk(ref ChunkReference) string {
	if r == nil {
		return ""
	}
	ref.ChunkID = strings.TrimSpace(ref.ChunkID)
	if ref.ChunkID == "" {
		return ""
	}
	if shortHandleRE.MatchString(ref.ChunkID) {
		return r.knownChunk(ref.ChunkID)
	}
	return r.chunks.register(ref.ChunkID, ref)
}

func (r *Registry) knownChunk(handle string) string {
	if r.chunks.has(handle) {
		return handle
	}
	return ""
}

func (r *Registry) RegisterDocument(id string) string {
	if r == nil {
		return ""
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if shortHandleRE.MatchString(id) {
		return r.knownDocument(id)
	}
	return r.docs.register(id, struct{}{})
}

func (r *Registry) knownDocument(handle string) string {
	if r.docs.has(handle) {
		return handle
	}
	return ""
}

func (r *Registry) RegisterKnowledgeBase(id string) string {
	if r == nil {
		return ""
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if shortHandleRE.MatchString(id) {
		return r.knownKB(id)
	}
	return r.kbs.register(id, struct{}{})
}

func (r *Registry) knownKB(handle string) string {
	if r.kbs.has(handle) {
		return handle
	}
	return ""
}

// RegisterChunks registers retrieved chunks together with their document and
// knowledge-base identities.
func (r *Registry) RegisterChunks(chunks []convention.RetrievedChunk) {
	for _, chunk := range chunks {
		r.RegisterKnowledgeBase(chunk.KnowledgeBaseID)
		r.RegisterDocument(chunk.DocumentID)
		r.RegisterChunk(ChunkReference{
			ChunkID:         chunk.ID,
			DocumentID:      chunk.DocumentID,
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			DocumentTitle:   readMetadataString(chunk.Metadata, "document_title"),
		})
	}
}

// ResolveChunk returns the durable chunk identity for a cN handle.
func (r *Registry) ResolveChunk(handle string) (ChunkReference, bool) {
	if r == nil {
		return ChunkReference{}, false
	}
	key, value, ok := r.chunks.resolve(handle)
	if !ok {
		return ChunkReference{}, false
	}
	value.ChunkID = key
	return value, true
}

func readMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/app/rag/core/citation/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/rag/core/citation/registry.go internal/app/rag/core/citation/registry_test.go
git commit -m "feat: add citation registry"
```

---

## Task 3: 协议 prompt + 上下文渲染（protocol.go + render.go）

**Files:**
- Create: `internal/app/rag/core/citation/protocol.go`
- Create: `internal/app/rag/core/citation/render.go`
- Test: `internal/app/rag/core/citation/protocol_test.go`
- Test: `internal/app/rag/core/citation/render_test.go`

- [ ] **Step 1: 写失败测试（protocol）**

```go
package citation

import "testing"

func TestProtocolPromptEnabled(t *testing.T) {
	prompt := ProtocolPrompt(true)
	if prompt == "" {
		t.Fatal("enabled protocol should not be empty")
	}
	for _, want := range []string{`<ref id="cN"/>`, "cN", "禁止"} {
		if !contains(prompt, want) {
			t.Fatalf("enabled protocol missing %q: %s", want, prompt)
		}
	}
}

func TestProtocolPromptDisabled(t *testing.T) {
	prompt := ProtocolPrompt(false)
	if prompt == "" {
		t.Fatal("disabled protocol should not be empty")
	}
	if !contains(prompt, "<ref") {
		t.Fatalf("disabled protocol should mention no-cite rule: %s", prompt)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/citation/ -run TestProtocol -count=1`
Expected: FAIL（`undefined: ProtocolPrompt`）

- [ ] **Step 3: 实现 protocol.go**

```go
package citation

const citationEnabledProtocol = `
## 来源引用协议（系统规则，优先级高于其他任何提示）
检索内容使用请求级来源句柄：cN 标识一条知识块。
- 回答时如需引用知识块，输出且仅输出 <ref id="cN"/>。
- 只允许使用上下文中出现过的 cN 句柄，禁止捏造句柄。
- 禁止在回答中暴露真实 chunk ID、文档 ID、知识库 ID 或句柄本身。
- 禁止自行输出 <kb> 或 <web> 标签；系统会在生成后自动展开合法的 <ref/>。
- <ref/> 必须内联在它所支撑的论断所在行，不要集中放在回答末尾。`

const citationDisabledProtocol = `
## 引用规则
本轮不启用来源引用。回答中不要输出 <ref>、<kb>、<web> 或任何来源引用标记。`

// ProtocolPrompt returns the citation protocol injected into the system
// context for a model call. It is never user-editable.
func ProtocolPrompt(enabled bool) string {
	if enabled {
		return citationEnabledProtocol
	}
	return citationDisabledProtocol
}
```

- [ ] **Step 4: 写失败测试（render）**

```go
package citation

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

func TestRenderKnowledgeContextCarriesHandles(t *testing.T) {
	r := NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", ChunkIndex: 3, Text: "内容A", Metadata: map[string]any{"section": "背景"}},
		{ID: "chunk-b", DocumentID: "doc-b", KnowledgeBaseID: "kb-b", Text: "内容B"},
	}
	rendered := RenderKnowledgeContext(r, chunks)
	for _, want := range []string{`<retrieval type="knowledge">`, `id="c1"`, `id="c2"`, "内容A", "内容B", `section="背景"`, `index="3"`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q:\n%s", want, rendered)
		}
	}
	if _, ok := r.ResolveChunk("c1"); !ok {
		t.Fatal("render should register chunks so they are resolvable")
	}
}

func TestRenderKnowledgeContextEmpty(t *testing.T) {
	if got := RenderKnowledgeContext(NewRegistry(), nil); got != "" {
		t.Fatalf("empty render = %q, want empty", got)
	}
}

func TestRenderKnowledgeContextEscapesText(t *testing.T) {
	r := NewRegistry()
	rendered := RenderKnowledgeContext(r, []convention.RetrievedChunk{
		{ID: "chunk-a", Text: "a<b>&c"},
	})
	if !strings.Contains(rendered, "a&lt;b&gt;&amp;c") {
		t.Fatalf("text not escaped: %s", rendered)
	}
}

func TestRenderKnowledgeContextWithBudgetTruncates(t *testing.T) {
	r := NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", Text: strings.Repeat("很长的内容", 200)},
		{ID: "chunk-b", Text: "短的"},
	}
	rendered, stats := RenderKnowledgeContextWithBudget(r, chunks, 30, tokenbudget.NewDefaultEstimator())
	if stats.RetainedChunks == 0 {
		t.Fatal("budget render retained zero chunks")
	}
	if !strings.Contains(rendered, "</retrieval>") {
		t.Fatalf("budget render must close retrieval tag: %s", rendered)
	}
	if stats.CandidateChunks != 2 {
		t.Fatalf("candidate chunks = %d, want 2", stats.CandidateChunks)
	}
}
```

- [ ] **Step 5: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/citation/ -run TestRender -count=1`
Expected: FAIL（`undefined: RenderKnowledgeContext`）

- [ ] **Step 6: 实现 render.go**

```go
package citation

import (
	"fmt"
	"html"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

const retrievalHeader = "<retrieval type=\"knowledge\">"
const retrievalFooter = "</retrieval>"

// RenderStats reports how a budget-aware render behaved.
type RenderStats struct {
	CandidateChunks int `json:"candidateChunks"`
	RetainedChunks  int `json:"retainedChunks"`
	TokensBefore    int `json:"tokensBefore"`
	TokensAfter     int `json:"tokensAfter"`
	Truncated       bool `json:"truncated"`
}

// RenderKnowledgeContext renders retrieved chunks as a handle-carrying
// <retrieval> block. Every chunk is registered so the model can cite
// <ref id="cN"/>.
func RenderKnowledgeContext(registry *Registry, chunks []convention.RetrievedChunk) string {
	if registry == nil || len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(retrievalHeader)
	for _, chunk := range chunks {
		b.WriteString("\n  ")
		b.WriteString(renderChunkElement(registry, chunk, chunk.Text))
	}
	b.WriteString("\n")
	b.WriteString(retrievalFooter)
	return b.String()
}

// RenderKnowledgeContextWithBudget renders the same block, stopping (or
// truncating chunk text) once the token budget is exhausted.
func RenderKnowledgeContextWithBudget(registry *Registry, chunks []convention.RetrievedChunk, budget int, estimator tokenbudget.Estimator) (string, RenderStats) {
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	stats := RenderStats{CandidateChunks: len(chunks)}
	full := RenderKnowledgeContext(registry, chunks)
	stats.TokensBefore = estimator.EstimateTokens(full)
	if len(chunks) == 0 || budget <= 0 {
		stats.Truncated = len(chunks) > 0
		return "", stats
	}
	headerTokens := estimator.EstimateTokens(retrievalHeader)
	footerTokens := estimator.EstimateTokens(retrievalFooter)
	newlineTokens := estimator.EstimateTokens("\n")

	var b strings.Builder
	b.WriteString(retrievalHeader)
	used := headerTokens + footerTokens
	for _, chunk := range chunks {
		fullPart := renderChunkElement(registry, chunk, chunk.Text)
		if used+newlineTokens+estimator.EstimateTokens(fullPart) <= budget {
			b.WriteString("\n  ")
			b.WriteString(fullPart)
			used += newlineTokens + estimator.EstimateTokens(fullPart)
			stats.RetainedChunks++
			continue
		}
		prefix := renderChunkPrefix(registry, chunk)
		prefixTokens := estimator.EstimateTokens(prefix)
		if used+newlineTokens+prefixTokens+estimator.EstimateTokens("</chunk>") <= budget {
			textBudget := budget - used - newlineTokens - prefixTokens - estimator.EstimateTokens("</chunk>")
			truncatedText, _ := tokenbudget.TruncateText(chunk.Text, textBudget, estimator)
			if strings.TrimSpace(truncatedText) != "" {
				part := prefix + escapeText(truncatedText) + "</chunk>"
				b.WriteString("\n  ")
				b.WriteString(part)
				used += newlineTokens + estimator.EstimateTokens(part)
				stats.RetainedChunks++
			}
		}
		stats.Truncated = true
		break
	}
	b.WriteString("\n")
	b.WriteString(retrievalFooter)
	stats.TokensAfter = estimator.EstimateTokens(b.String())
	stats.Truncated = stats.Truncated || stats.RetainedChunks < stats.CandidateChunks
	return b.String(), stats
}

func renderChunkElement(registry *Registry, chunk convention.RetrievedChunk, content string) string {
	prefix := renderChunkPrefix(registry, chunk)
	return prefix + escapeText(strings.TrimSpace(content)) + "</chunk>"
}

func renderChunkPrefix(registry *Registry, chunk convention.RetrievedChunk) string {
	handle := registry.RegisterChunk(ChunkReference{
		ChunkID:         chunk.ID,
		DocumentID:      chunk.DocumentID,
		KnowledgeBaseID: chunk.KnowledgeBaseID,
		DocumentTitle:   readMetadataString(chunk.Metadata, "document_title"),
	})
	var b strings.Builder
	b.WriteString("<chunk id=\"")
	b.WriteString(handle)
	b.WriteString("\"")
	if chunk.ChunkIndex > 0 {
		fmt.Fprintf(&b, " index=\"%d\"", chunk.ChunkIndex)
	}
	if section := readMetadataString(chunk.Metadata, "section"); section != "" {
		b.WriteString(" section=\"")
		b.WriteString(escapeAttr(section))
		b.WriteString("\"")
	}
	if title := readMetadataString(chunk.Metadata, "document_title"); title != "" {
		b.WriteString(" title=\"")
		b.WriteString(escapeAttr(title))
		b.WriteString("\"")
	}
	b.WriteString(">")
	return b.String()
}

func escapeText(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func escapeAttr(value string) string { return html.EscapeString(value) }
```

- [ ] **Step 7: 运行测试确认通过**

Run: `go test ./internal/app/rag/core/citation/ -count=1`
Expected: PASS

- [ ] **Step 8: 提交**

```bash
git add internal/app/rag/core/citation/protocol.go internal/app/rag/core/citation/protocol_test.go internal/app/rag/core/citation/render.go internal/app/rag/core/citation/render_test.go
git commit -m "feat: add citation protocol and context renderer"
```

---

## Task 4: 流式展开 + 展开器（expand.go）

**Files:**
- Create: `internal/app/rag/core/citation/expand.go`
- Test: `internal/app/rag/core/citation/expand_test.go`

- [ ] **Step 1: 写失败测试**

```go
package citation

import (
	"strings"
	"testing"
)

func TestExpandTextExpandsKnownRef(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", DocumentTitle: "标题"})
	got := r.ExpandText("根据 <ref id=\"c1\"/> 说明。", true)
	want := `<kb doc="标题" chunk_id="chunk-a" kb_id="kb-a" />`
	if !strings.Contains(got, want) {
		t.Fatalf("expanded output missing %q: %s", want, got)
	}
	if strings.Contains(got, "c1") {
		t.Fatalf("handle leaked in output: %s", got)
	}
}

func TestExpandTextDropsUnknownRef(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	got := r.ExpandText("x <ref id=\"c9\"/> y", true)
	if strings.Contains(got, "ref") || strings.Contains(got, "c9") {
		t.Fatalf("unknown ref not dropped: %s", got)
	}
}

func TestExpandTextStripsModelKBAndDisabledRefs(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	got := r.ExpandText("a <kb doc=\"x\" chunk_id=\"chunk-a\"/> b", true)
	if strings.Contains(got, "<kb") {
		t.Fatalf("model-written <kb> not stripped: %s", got)
	}
	got = r.ExpandText("a <ref id=\"c1\"/> b", false)
	if strings.Contains(got, "ref") {
		t.Fatalf("refs not stripped when disabled: %s", got)
	}
}

func TestStreamExpanderHandlesSplitTags(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a", DocumentTitle: "标题"})
	d := NewStreamExpander(r, true)
	var out strings.Builder
	out.WriteString(d.Feed("前文 <re"))
	out.WriteString(d.Feed("f id=\"c1\"/> 后文"))
	out.WriteString(d.Flush())
	if !strings.Contains(out.String(), `<kb doc="标题" chunk_id="chunk-a" />`) {
		t.Fatalf("split tag not expanded: %s", out.String())
	}
	if strings.Contains(out.String(), "c1") {
		t.Fatalf("handle leaked: %s", out.String())
	}
}

func TestStreamExpanderFlushDropsPartialTag(t *testing.T) {
	r := NewRegistry()
	d := NewStreamExpander(r, true)
	got := d.Feed("abc <ref id=\"c")
	got += d.Flush()
	if strings.Contains(got, "<ref") || strings.Contains(got, "c") && strings.Contains(got, "<") {
		t.Fatalf("partial tag leaked: %q", got)
	}
	if got != "abc " {
		t.Fatalf("flush output = %q, want %q", got, "abc ")
	}
}

func TestStreamExpanderPassthroughWhenNilRegistry(t *testing.T) {
	d := NewStreamExpander(nil, true)
	if got := d.Feed("plain <ref id=\"c1\"/>"); got != "plain <ref id=\"c1\"/>" {
		t.Fatalf("nil registry should pass through, got %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/citation/ -run TestExpand -count=1`
Expected: FAIL（`undefined: ExpandText` / `undefined: NewStreamExpander`）

- [ ] **Step 3: 实现 expand.go**

```go
package citation

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	refTagRE       = regexp.MustCompile(`(?i)<ref\s+id\s*=\s*"([^"]+)"\s*/?>`)
	refCandidateRE = regexp.MustCompile(`(?is)<ref(?:\s|$)[^>]*(?:>|$)`)
	modelKBTagRE   = regexp.MustCompile(`(?is)<kb(?:\s|$)[^>]*(?:>|$)`)
)

// ExpandText converts the private <ref/> protocol into the public <kb/> tag
// contract. Unknown handles fail closed and disappear; model-written <kb>
// tags are dropped because public tags are output-only.
func (r *Registry) ExpandText(text string, enabled bool) string {
	if r == nil || text == "" {
		return text
	}
	text = modelKBTagRE.ReplaceAllString(text, "")
	if !enabled {
		return refCandidateRE.ReplaceAllString(text, "")
	}
	return refCandidateRE.ReplaceAllStringFunc(text, func(tag string) string {
		match := refTagRE.FindStringSubmatch(tag)
		if len(match) != 2 {
			return ""
		}
		ref, ok := r.ResolveChunk(match[1])
		if !ok {
			return ""
		}
		return renderKBTag(ref)
	})
}

func renderKBTag(ref ChunkReference) string {
	attrs := fmt.Sprintf(`doc="%s" chunk_id="%s"`, escapeAttr(ref.DocumentTitle), escapeAttr(ref.ChunkID))
	if ref.KnowledgeBaseID != "" {
		attrs += fmt.Sprintf(` kb_id="%s"`, escapeAttr(ref.KnowledgeBaseID))
	}
	return "<kb " + attrs + " />"
}

// StreamExpander expands <ref/> tags while streaming, holding back tail bytes
// that could be part of a partial tag so SSE never leaks a half tag.
type StreamExpander struct {
	registry *Registry
	enabled  bool
	pending  string
}

func NewStreamExpander(registry *Registry, enabled bool) *StreamExpander {
	return &StreamExpander{registry: registry, enabled: enabled}
}

func (d *StreamExpander) Feed(chunk string) string {
	if d == nil || d.registry == nil {
		return chunk
	}
	data := d.pending + chunk
	d.pending = ""
	var out strings.Builder
	for data != "" {
		idx := strings.Index(data, "<")
		if idx < 0 {
			out.WriteString(data)
			break
		}
		out.WriteString(data[:idx])
		data = data[idx:]
		lower := strings.ToLower(data)
		if isSourceTagPending(lower) && !strings.Contains(data, ">") {
			d.pending = data
			break
		}
		if isRefTagStart(lower) {
			end := strings.IndexByte(data, '>')
			if end < 0 {
				d.pending = data
				break
			}
			tag := data[:end+1]
			if refTagRE.MatchString(tag) {
				out.WriteString(d.registry.ExpandText(tag, d.enabled))
			}
			data = data[end+1:]
			continue
		}
		if isNamedTagStart(lower, "kb") {
			end := strings.IndexByte(data, '>')
			if end < 0 {
				d.pending = data
				break
			}
			data = data[end+1:]
			continue
		}
		out.WriteByte('<')
		data = data[1:]
	}
	return out.String()
}

func (d *StreamExpander) Flush() string {
	if d == nil {
		return ""
	}
	pending := d.pending
	d.pending = ""
	if isSourceTagPending(strings.ToLower(pending)) {
		return ""
	}
	return d.registry.ExpandText(pending, d.enabled)
}

func isRefTagStart(value string) bool {
	return isNamedTagStart(value, "ref")
}

func isNamedTagStart(value, name string) bool {
	prefix := "<" + name
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	if len(value) == len(prefix) {
		return true
	}
	next := value[len(prefix)]
	return next == ' ' || next == '\t' || next == '\r' || next == '\n' || next == '>'
}

func isSourceTagPending(value string) bool {
	for _, name := range []string{"ref", "kb"} {
		prefix := "<" + name
		if (len(value) <= len(prefix) && strings.HasPrefix(prefix, value)) || isNamedTagStart(value, name) {
			return true
		}
	}
	return false
}

// escapeAttrHTML is used by renderKBTag; kept next to html import for clarity.
var _ = html.EscapeString
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/app/rag/core/citation/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/rag/core/citation/expand.go internal/app/rag/core/citation/expand_test.go
git commit -m "feat: add citation stream expander"
```

> 注：`html` 导入如果实际未用（renderKBTag 用的 `escapeAttr` 已在 render.go 定义），删除 `html` 导入和 `var _ = html.EscapeString`。运行 `go test` 确认编译通过。

---

## Task 5: 历史重压缩（compact.go）

**Files:**
- Create: `internal/app/rag/core/citation/compact.go`
- Test: `internal/app/rag/core/citation/compact_test.go`

- [ ] **Step 1: 写失败测试**

```go
package citation

import (
	"strings"
	"testing"
)

func TestCompactPublicCitationsFoldsKBTag(t *testing.T) {
	r := NewRegistry()
	history := `根据 <kb doc="标题" chunk_id="chunk-a" kb_id="kb-a" /> 说明。`
	got := r.CompactPublicCitations(history)
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("kb tag not folded to ref: %s", got)
	}
	if strings.Contains(got, "chunk-a") || strings.Contains(got, "<kb") {
		t.Fatalf("durable id leaked after compaction: %s", got)
	}
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-a" || ref.DocumentTitle != "标题" || ref.KnowledgeBaseID != "kb-a" {
		t.Fatalf("compact should register chunk: %+v, %v", ref, ok)
	}
}

func TestCompactPublicCitationsReusesHandle(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	got := r.CompactPublicCitations(`<kb doc="标题" chunk_id="chunk-a" />`)
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("same chunk should reuse c1: %s", got)
	}
}

func TestCompactPublicCitationsKeepsNonKBTag(t *testing.T) {
	r := NewRegistry()
	got := r.CompactPublicCitations("普通文本 <ref id=\"c1\"/> 保持原样")
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("existing ref changed: %s", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/citation/ -run TestCompact -count=1`
Expected: FAIL（`undefined: CompactPublicCitations`）

- [ ] **Step 3: 实现 compact.go**

```go
package citation

import (
	"html"
	"regexp"
)

var (
	publicKBTagRE = regexp.MustCompile(`(?is)<kb\b[^>]*>`)
	chunkAttrRE   = regexp.MustCompile(`(?i)\bchunk_id\s*=\s*"([^"]+)"`)
	kbAttrRE      = regexp.MustCompile(`(?i)\bkb_id\s*=\s*"([^"]*)"`)
	docAttrRE     = regexp.MustCompile(`(?i)\bdoc\s*=\s*"([^"]*)"`)
)

// CompactPublicCitations folds canonical <kb/> tags from prior assistant
// turns back into this request's private <ref/> handles, registering each
// chunk so historical citations stay resolvable for the current request.
func (r *Registry) CompactPublicCitations(text string) string {
	if r == nil || text == "" {
		return text
	}
	return publicKBTagRE.ReplaceAllStringFunc(text, func(tag string) string {
		chunkID := attr(chunkAttrRE, tag)
		if chunkID == "" {
			return tag
		}
		handle := r.RegisterChunk(ChunkReference{
			ChunkID:         chunkID,
			KnowledgeBaseID: attr(kbAttrRE, tag),
			DocumentTitle:   attr(docAttrRE, tag),
		})
		if handle == "" {
			return tag
		}
		return `<ref id="` + handle + `"/>`
	})
}

func attr(expression *regexp.Regexp, tag string) string {
	match := expression.FindStringSubmatch(tag)
	if len(match) != 2 {
		return ""
	}
	return html.UnescapeString(match[1])
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/app/rag/core/citation/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/rag/core/citation/compact.go internal/app/rag/core/citation/compact_test.go
git commit -m "feat: add citation history recompaction"
```

---

## Task 6: prompt 上下文注入协议（prompt.Service）

**Files:**
- Modify: `internal/app/rag/core/prompt/service.go`
- Test: `internal/app/rag/core/prompt/service_test.go`

- [ ] **Step 1: 写失败测试（追加到 service_test.go）**

```go
func TestBuildMessagesAppendsCitationProtocol(t *testing.T) {
	service := NewService(nil)
	messages, err := service.BuildMessages(Context{
		Question:         "问题",
		CitationProtocol: "## 协议\n不要引用。",
	})
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	var found bool
	for _, m := range messages {
		if m.Role == SystemRole && strings.Contains(m.Content, "Citation Protocol") {
			found = true
		}
	}
	if !found {
		t.Fatalf("citation protocol not injected, got %+v", messages)
	}
}

func TestBuildMessagesSkipsEmptyCitationProtocol(t *testing.T) {
	service := NewService(nil)
	messages, err := service.BuildMessages(Context{Question: "问题"})
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	for _, m := range messages {
		if strings.Contains(m.Content, "Citation Protocol") {
			t.Fatalf("empty citation protocol should not be injected: %+v", messages)
		}
	}
}
```

（`service_test.go` 已 import `strings`，若没有则补上。）

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/app/rag/core/prompt/ -count=1`
Expected: FAIL

- [ ] **Step 3: 修改 service.go**

在 `Context` 结构体加字段：

```go
type Context struct {
	Question         string
	MemoryContext    string
	SessionContext   string
	KnowledgeContext string
	ToolContext      string
	WorkflowPolicy   string
	AnswerGuidance   string
	CitationProtocol string
	History          []convention.ChatMessage
	SystemPromptKey  string
	SystemPrompt     string
}
```

在 `BuildMessages` 中，`AnswerGuidance` 之后、`History` 之前插入：

```go
	if strings.TrimSpace(ctx.CitationProtocol) != "" {
		messages = append(messages, convention.SystemMessage(formatCitationProtocol(ctx.CitationProtocol)))
	}
```

在文件末尾（`formatAnswerGuidance` 之后）加：

```go
func formatCitationProtocol(protocol string) string {
	return "## Citation Protocol\n" + strings.TrimSpace(protocol)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/app/rag/core/prompt/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/rag/core/prompt/service.go internal/app/rag/core/prompt/service_test.go
git commit -m "feat: inject citation protocol into prompt context"
```

---

## Task 7: 配置 + 装配接线

**Files:**
- Modify: `internal/framework/config/config.go`
- Modify: `internal/app/rag/service/chat/deps.go`
- Modify: `internal/bootstrap/rag/runtime_build_chat.go`
- Modify: `configs/application.yaml`

- [ ] **Step 1: config.go 加字段**

在 `RagConfig` 结构体（约 105 行）加一行：

```go
type RagConfig struct {
	Vector       RagVectorConfig       `mapstructure:"vector"`
	Default      RagDefaultConfig      `mapstructure:"default"`
	Agent        RagAgentConfig        `mapstructure:"agent"`
	Retrieve     RagRetrieveConfig     `mapstructure:"retrieve"`
	QueryRewrite RagQueryRewriteConfig `mapstructure:"query-rewrite"`
	RateLimit    RagRateLimitConfig    `mapstructure:"rate-limit"`
	Memory       RagMemoryConfig       `mapstructure:"memory"`
	Knowledge    RagKnowledgeConfig    `mapstructure:"knowledge"`
	MCP          RagMCPConfig          `mapstructure:"mcp"`
	Search       RagSearchConfig       `mapstructure:"search"`
	Trace        RagTraceConfig        `mapstructure:"trace"`
	CitationEnabled bool               `mapstructure:"citation-enabled"`
}
```

- [ ] **Step 2: application.yaml 加配置**

在 `rag:` 段落（顶层）加：

```yaml
rag:
  citation-enabled: true
```

（保留现有字段，只加这一行。）

- [ ] **Step 3: deps.go 加选项与字段**

`RagChatOptions` 加：

```go
type RagChatOptions struct {
	ConfidenceThreshold     float64
	ParallelSubquestions    bool
	SubquestionConcurrency  int
	RequestCacheMaxEntries  int
	AgentRuntimeMode        string
	SessionRecall           SessionRecallService
	LongTermMemoryRecall    longtermmemory.RecallService
	LongTermMemoryWriteback LongTermMemoryWriteback
	ToolWorkflow            ragtool.Workflow
	ChatContextBudget       ChatContextBudgetOptions
	CitationEnabled         bool
}
```

`RagChatService` 结构体加字段：

```go
	citationEnabled     bool
```

`NewRagChatServiceWithDeps` 中 `opts = normalizeRagChatOptions(opts)` 之后加：

```go
	service.citationEnabled = opts.CitationEnabled
```

- [ ] **Step 4: bootstrap 接线**

`runtime_build_chat.go` 的 `RagChatOptions{...}` 中加：

```go
		RagChatOptions{
			ConfidenceThreshold:     confidenceThreshold,
			ParallelSubquestions:    cfg.Rag.Retrieve.ParallelSubquestions.Enabled,
			SubquestionConcurrency:  cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency,
			RequestCacheMaxEntries:  readRequestCacheMaxEntries(cfg),
			AgentRuntimeMode:        cfg.Rag.Agent.Chat.Mode,
			CitationEnabled:         cfg.Rag.CitationEnabled,
			SessionRecall:           retrieve.sessionRecallService,
			LongTermMemoryRecall:    memory.explicitMemoryService.RecallService(),
			LongTermMemoryWriteback: adaptLongTermMemoryWriteback(buildLongTermMemoryWriteback(buildCtx, memory)),
			ToolWorkflow:            toolWorkflow,
			ChatContextBudget:       buildChatContextBudgetOptions(cfg),
		},
```

- [ ] **Step 5: 验证编译**

Run: `go build ./internal/app/rag/service/chat/ ./internal/bootstrap/rag/ ./internal/framework/config/`
Expected: 成功

- [ ] **Step 6: 提交**

```bash
git add internal/framework/config/config.go configs/application.yaml internal/app/rag/service/chat/deps.go internal/bootstrap/rag/runtime_build_chat.go
git commit -m "feat: wire citation-enabled config through rag chat service"
```

---

## Task 8: rag 聊天服务集成（核心）

**Files:**
- Modify: `internal/app/rag/service/chat/stage_types.go`
- Modify: `internal/app/rag/service/chat/prepare_orchestrator.go`
- Modify: `internal/app/rag/service/chat/execute_tool_workflow.go`
- Modify: `internal/app/rag/service/chat/service.go`
- Modify: `internal/app/rag/service/chat/execute_orchestrator.go`
- Modify: `internal/app/rag/service/chat/execute_streaming.go`

- [ ] **Step 1: stage_types.go — state 加注册表**

```go
type ragChatRuntimeState struct {
	meta          RagChatMeta
	title         string
	userMessageID string
	traceID       string
	startTime     time.Time
	citation      *citation.Registry
}
```

加 import：`ragcitation "local/rag-project/internal/app/rag/core/citation"`

- [ ] **Step 2: prepare_orchestrator.go — prepareChat 集成**

顶部加 import：`"local/rag-project/internal/framework/convention"` 和 `ragcitation "local/rag-project/internal/app/rag/core/citation"`。

`prepareChat` 改为：函数最顶部声明注册表（仅启用时创建）；`runMemoryStage` 之后重压缩历史；`retrieveStage` 之后注册并重渲染；返回值挂到 state：

```go
func (s *RagChatService) prepareChat(ctx context.Context, input RagChatInput) (ragChatPreparedState, error) {
	var registry *ragcitation.Registry
	if s.citationEnabled {
		registry = ragcitation.NewRegistry()
	}
	conversationStage, err := s.runConversationStage(ctx, input)
	...
	memoryStage, err := s.runMemoryStage(ctx, conversationStage.conversationID, strings.TrimSpace(input.UserID))
	if err != nil {
		return ragChatPreparedState{}, err
	}
	if s.citationEnabled {
		for i := range memoryStage.history {
			if memoryStage.history[i].Role == convention.AssistantRole {
				memoryStage.history[i].Content = registry.CompactPublicCitations(memoryStage.history[i].Content)
			}
		}
	}
	...
	retrieveStage, err := s.runRetrieveStage(ctx, input, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}
	if s.citationEnabled && retrieveStage.used && len(retrieveStage.result.Chunks) > 0 {
		registry.RegisterChunks(retrieveStage.result.Chunks)
		retrieveStage.result.KnowledgeContext = ragcitation.RenderKnowledgeContext(registry, retrieveStage.result.Chunks)
	}
	state := runtimeStage.result.state
	state.citation = registry
	return ragChatPreparedState{
		state:          state,
		history:        memoryStage.history,
		userMessage:    userMessageStage.message,
		rewriteResult:  rewriteStage.result,
		memoryContext:  longTermMemoryStage.result.Context,
		sessionRecall:  sessionRecallStage.result,
		sessionContext: sessionRecallStage.result.Context,
		retrieveResult: retrieveStage.result,
		retrievalUsed:  retrieveStage.used,
	}, nil
}
```

（`var registry *ragcitation.Registry` 声明在函数顶部，保证 `state.citation` 在禁用时保持 nil。）

- [ ] **Step 3: execute_tool_workflow.go — runPromptStage 加 protocol、applyRetrieveContextBudget 感知注册表**

`runPromptStage` 签名末尾加参数（`traceID` 之前）：

```go
	systemPromptOverride string,
	citationProtocol string,
	traceID string,
```

在函数体 `promptContext` 中加：

```go
			promptContext := ragprompt.Context{
				Question:         question,
				MemoryContext:    memoryContext,
				SessionContext:   sessionContext,
				KnowledgeContext: promptCtx.KnowledgeContext,
				ToolContext:      toolContext,
				WorkflowPolicy:   workflowPolicy,
				AnswerGuidance:   answerGuidance,
				CitationProtocol: citationProtocol,
				History:          history,
				SystemPrompt:     systemPromptOverride,
			}
```

`applyRetrieveContextBudget` 签名加参数并改为感知注册表：

```go
func (s *RagChatService) applyRetrieveContextBudget(ctx context.Context, traceID string, result ragretrieve.Result, registry *ragcitation.Registry) ragretrieve.Result {
	if s == nil || s.chatContextBudget.RetrieveTokens <= 0 || len(result.Chunks) == 0 {
		return result
	}
	if registry != nil {
		contextText, stats := ragcitation.RenderKnowledgeContextWithBudget(
			registry,
			result.Chunks,
			s.chatContextBudget.RetrieveTokens,
			s.chatContextBudget.Estimator,
		)
		result.KnowledgeContext = contextText
		if s.tracer != nil {
			s.tracer.appendTraceRunExtra(ctx, traceID, map[string]any{
				"retrieveContextBudget": stats,
			})
		}
		return result
	}
	contextText, stats := ragretrieve.BuildKnowledgeContextWithinBudget(
		result.Chunks,
		s.chatContextBudget.RetrieveTokens,
		s.chatContextBudget.Estimator,
	)
	result.KnowledgeContext = contextText
	if s.tracer != nil {
		s.tracer.appendTraceRunExtra(ctx, traceID, map[string]any{
			"retrieveContextBudget": stats,
		})
	}
	return result
}
```

- [ ] **Step 4: service.go — runtimeEnabled 判定与传参**

`applyFallbackGuard`/`applyRetrieveContextBudget` 调用处（65-66 行）改为：

```go
	retrieveResult, fallbackPrompt := s.applyFallbackGuard(ctx, prepared, question, sink)
	retrieveResult = s.applyRetrieveContextBudget(ctx, prepared.state.traceID, retrieveResult, prepared.state.citation)

	runtimeEnabled := s.citationEnabled && prepared.retrievalUsed && strings.TrimSpace(retrieveResult.KnowledgeContext) != ""
	citationProtocol := ""
	var expander *ragcitation.StreamExpander
	if s.citationEnabled {
		citationProtocol = ragcitation.ProtocolPrompt(runtimeEnabled)
		expander = ragcitation.NewStreamExpander(prepared.state.citation, runtimeEnabled)
	}
```

`runPromptStage` 调用（121 行）在 `effectiveFallbackPrompt(...)` 之后、`prepared.state.traceID` 之前加 `citationProtocol`。

`runStreamingAnswer` 调用（142 行）在 `input.DeepThinking` 之后、`sink` 之前加 `expander`。

顶部加 import：`ragcitation "local/rag-project/internal/app/rag/core/citation"`

- [ ] **Step 5: execute_orchestrator.go — runStreamingAnswer 加 expander**

签名：

```go
func (s *RagChatService) runStreamingAnswer(
	ctx context.Context,
	state ragChatRuntimeState,
	messages []convention.ChatMessage,
	promptTokensEstimate int,
	deepThinking bool,
	expander *ragcitation.StreamExpander,
	sink RagChatEventSink,
) (ragChatTaskResult, error) {
```

callback 构造加 expander：

```go
	callback := newRagChatStreamCallback(
		task,
		sink,
		s.chatContextBudget.normalized().Estimator,
		promptTokensEstimate,
		expander,
	)
```

顶部加 import：`ragcitation "local/rag-project/internal/app/rag/core/citation"`

- [ ] **Step 6: execute_streaming.go — callback 流式展开 + Flush**

`ragChatStreamCallback` 加字段与构造参数：

```go
type ragChatStreamCallback struct {
	task *ragChatTask
	sink RagChatEventSink

	estimator            TokenEstimator
	promptTokensEstimate int
	expander             *ragcitation.StreamExpander

	mu       sync.Mutex
	content  strings.Builder
	thinking strings.Builder
}
```

构造：

```go
func newRagChatStreamCallback(
	task *ragChatTask,
	sink RagChatEventSink,
	estimator TokenEstimator,
	promptTokensEstimate int,
	expander *ragcitation.StreamExpander,
) *ragChatStreamCallback {
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}
	callback := &ragChatStreamCallback{
		task:                 task,
		sink:                 sink,
		estimator:            estimator,
		promptTokensEstimate: promptTokensEstimate,
		expander:             expander,
	}
	go callback.watchCancel()
	return callback
}
```

`OnContent` 改为先展开再写入/发送：

```go
func (c *ragChatStreamCallback) OnContent(content string) {
	c.mu.Lock()
	expanded := content
	if c.expander != nil {
		expanded = c.expander.Feed(content)
	}
	c.content.WriteString(expanded)
	c.mu.Unlock()
	_ = c.sink.SendMessage(expanded)
}
```

`buildTaskResult` 用 `finalizeContent`：

```go
func (c *ragChatStreamCallback) buildTaskResult(err error) ragChatTaskResult {
	content := c.finalizeContent()
	thinking := c.currentThinking()
	completionTokens := c.estimator.EstimateTokens(content) + c.estimator.EstimateTokens(thinking)
	return ragChatTaskResult{
		content:     content,
		thinking:    thinking,
		err:         err,
		tokenUsage:  aichat.EstimatedTokenUsage(c.promptTokensEstimate, completionTokens),
		usageSource: "estimated",
	}
}

// finalizeContent flushes any expander tail and returns the fully expanded
// content for persistence.
func (c *ragChatStreamCallback) finalizeContent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expander != nil {
		rest := c.expander.Flush()
		if rest != "" {
			c.content.WriteString(rest)
		}
	}
	return c.content.String()
}
```

删除不再使用的 `currentContent()`（或保留但改为调用 finalizeContent）。顶部加 import：`ragcitation "local/rag-project/internal/app/rag/core/citation"`。

- [ ] **Step 7: 更新 runPromptStage 测试调用点**

在 `rag_chat_service_test.go`（约 1596/1633/1726/1901 行）和 `chat_context_budget_test.go`（约 219 行）的 `runPromptStage(...)` 调用中，`systemPromptOverride` 参数之后加 `""`（citationProtocol）。

- [ ] **Step 8: 编译 + 运行现有测试**

Run: `go build ./internal/app/rag/service/chat/`
Run: `go test ./internal/app/rag/service/chat/ -count=1`
Expected: 通过（若个别测试因签名变化失败，按 Step 7 修正调用点）

- [ ] **Step 9: 提交**

```bash
git add internal/app/rag/service/chat/
git commit -m "feat: integrate citation protocol into rag chat pipeline"
```

---

## Task 9: rag 聊天服务级测试

**Files:**
- Test: `internal/app/rag/service/chat/citation_integration_test.go`（新建）

- [ ] **Step 1: 写集成测试**

```go
package chat

import (
	"context"
	"strings"
	"testing"

	ragcitation "local/rag-project/internal/app/rag/core/citation"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

func TestPrepareChatRegistersAndRendersCitations(t *testing.T) {
	retrieve := &retrieveServiceStub{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", Text: "内容A", Metadata: map[string]any{"document_title": "标题A"}},
			},
		},
	}
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, retrieve, func(_ *RagChatDeps, opts *RagChatOptions) {
		opts.CitationEnabled = true
	})
	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		UserID:           "user-1",
		Question:         "问题",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("prepareChat() error = %v", err)
	}
	if !strings.Contains(prepared.retrieveResult.KnowledgeContext, `id="c1"`) {
		t.Fatalf("knowledge context missing c1 handle: %s", prepared.retrieveResult.KnowledgeContext)
	}
	if prepared.state.citation == nil {
		t.Fatal("state.citation should be set")
	}
	ref, ok := prepared.state.citation.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-a" || ref.DocumentTitle != "标题A" {
		t.Fatalf("chunk not registered correctly: %+v, %v", ref, ok)
	}
}

func TestPrepareChatSkipsCitationRenderWhenDisabled(t *testing.T) {
	retrieve := &retrieveServiceStub{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{ID: "chunk-a", Text: "内容A"},
			},
		},
	}
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, retrieve)
	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		UserID:           "user-1",
		Question:         "问题",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("prepareChat() error = %v", err)
	}
	if strings.Contains(prepared.retrieveResult.KnowledgeContext, "retrieval") {
		t.Fatalf("citation disabled should keep plain context: %s", prepared.retrieveResult.KnowledgeContext)
	}
	if prepared.state.citation != nil {
		t.Fatal("state.citation should be nil when disabled")
	}
}

func TestPrepareChatCompactsHistoryCitations(t *testing.T) {
	history := []convention.ChatMessage{
		convention.AssistantMessage(`根据 <kb doc="标题" chunk_id="chunk-a" /> 说明。`),
	}
	retrieve := &retrieveServiceStub{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{ID: "chunk-a", Text: "内容A"},
			},
		},
	}
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, retrieve, func(deps *RagChatDeps, opts *RagChatOptions) {
		opts.CitationEnabled = true
		deps.HistoryService = memoryServiceStub{history: history}
	})
	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		UserID:           "user-1",
		Question:         "问题",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("prepareChat() error = %v", err)
	}
	if len(prepared.history) != 1 || !strings.Contains(prepared.history[0].Content, `<ref id="c1"/>`) {
		t.Fatalf("history not compacted: %+v", prepared.history)
	}
	if strings.Contains(prepared.history[0].Content, "chunk-a") {
		t.Fatalf("durable id leaked in history: %s", prepared.history[0].Content)
	}
}

func TestRenderThenExpandRoundTrip(t *testing.T) {
	registry := ragcitation.NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", Text: "内容", Metadata: map[string]any{"document_title": "标题"}},
	}
	rendered := ragcitation.RenderKnowledgeContext(registry, chunks)
	if !strings.Contains(rendered, `id="c1"`) {
		t.Fatalf("render missing handle: %s", rendered)
	}
	expanded := registry.ExpandText(`答案是 <ref id="c1"/>。`, true)
	if !strings.Contains(expanded, `<kb doc="标题" chunk_id="chunk-a" kb_id="kb-a" />`) {
		t.Fatalf("expand round trip failed: %s", expanded)
	}
}

func TestProtocolPromptInjectedWhenEnabled(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{CitationEnabled: true})
	promptCtx := ragprompt.Context{
		Question:         "问题",
		KnowledgeContext: "<retrieval>…</retrieval>",
		CitationProtocol: ragcitation.ProtocolPrompt(true),
	}
	messages, err := service.promptService.BuildMessages(promptCtx)
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	var found bool
	for _, m := range messages {
		if m.Role == convention.SystemRole && strings.Contains(m.Content, "<ref id=\"cN\"/>") {
			found = true
		}
	}
	if !found {
		t.Fatalf("citation protocol not injected: %+v", messages)
	}
}

func TestApplyRetrieveContextBudgetUsesCitationRenderer(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		CitationEnabled:   true,
		ChatContextBudget: ChatContextBudgetOptions{RetrieveTokens: 1000},
	})
	registry := ragcitation.NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", Text: strings.Repeat("内容", 500)},
	}
	// 传入已渲染的句柄上下文（空上下文会被回退守卫跳过重建，见 citation_fallback_test.go）
	rendered := ragcitation.RenderKnowledgeContext(registry, chunks)
	result := service.applyRetrieveContextBudget(
		context.Background(),
		"trace-1",
		ragretrieve.Result{Chunks: chunks, KnowledgeContext: rendered},
		registry,
	)
	if !strings.Contains(result.KnowledgeContext, `<chunk id="c1"`) {
		t.Fatalf("budget render missing handle: %s", result.KnowledgeContext)
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/app/rag/service/chat/ -run 'Citation|Protocol|Fallback|RenderThenExpand' -count=1`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/app/rag/service/chat/citation_integration_test.go
git commit -m "test: cover citation integration in rag chat service"
```

---

## Task 10: chunk 详情端点

**Files:**
- Modify: `internal/app/knowledge/service/chunk/knowledge_chunk_query_service.go`
- Modify: `internal/adapter/http/knowledge/knowledge_chunk_handler.go`
- Test: `internal/adapter/http/knowledge/test/knowledge_chunk_handler_test.go`

- [ ] **Step 1: 服务方法 GetByID**

在 `knowledge_chunk_query_service.go` 末尾加：

```go
func (s *KnowledgeChunkService) GetByID(ctx context.Context, chunkID string) (domain.KnowledgeChunk, error) {
	if s == nil || s.chunkRepo == nil {
		return domain.KnowledgeChunk{}, exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return domain.KnowledgeChunk{}, exception.NewClientException("knowledge chunk id is required", nil)
	}
	chunk, err := s.chunkRepo.GetByID(ctx, chunkID)
	if err != nil {
		return domain.KnowledgeChunk{}, exception.NewServiceException("failed to get knowledge chunk", err)
	}
	return chunk, nil
}
```

（`domain`、`strings` 已 import。）

- [ ] **Step 2: handler 接口 + 路由 + 方法**

接口加：

```go
type KnowledgeChunkService interface {
	Page(ctx context.Context, input service.PageKnowledgeChunkInput) (service.KnowledgeChunkPageResult, error)
	Create(ctx context.Context, input service.CreateKnowledgeChunkInput) (domain.KnowledgeChunk, error)
	Update(ctx context.Context, input service.UpdateKnowledgeChunkInput) error
	Delete(ctx context.Context, input service.DeleteKnowledgeChunkInput) error
	Enable(ctx context.Context, input service.EnableKnowledgeChunkInput) error
	BatchToggleEnabled(ctx context.Context, input service.BatchToggleKnowledgeChunksInput) error
	GetByID(ctx context.Context, chunkID string) (domain.KnowledgeChunk, error)
}
```

路由加：

```go
	r.GET("/knowledge-base/chunks/:chunkId", handler.Get)
```

方法加：

```go
func (h *KnowledgeChunkHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		_ = c.Error(exception.NewServiceException("knowledge chunk service is required", nil))
		return
	}
	chunk, err := h.service.GetByID(c.Request.Context(), c.Param("chunkId"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	writeSuccess(c, toKnowledgeChunkVO(chunk))
}
```

- [ ] **Step 3: 更新 handler 测试 stub 与用例**

在 `knowledgeChunkServiceStub` 加字段与方法：

```go
	getByIDFn func(ctx context.Context, chunkID string) (domain.KnowledgeChunk, error)
```

```go
func (s knowledgeChunkServiceStub) GetByID(ctx context.Context, chunkID string) (domain.KnowledgeChunk, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, chunkID)
	}
	return domain.KnowledgeChunk{}, nil
}
```

新增用例：

```go
func TestKnowledgeChunkHandlerGetByID(t *testing.T) {
	router := newKnowledgeChunkRouter(knowledgeChunkServiceStub{
		getByIDFn: func(ctx context.Context, chunkID string) (domain.KnowledgeChunk, error) {
			if chunkID != "chunk-1" {
				t.Fatalf("unexpected chunk id: %q", chunkID)
			}
			return domain.KnowledgeChunk{
				ID:              "chunk-1",
				KnowledgeBaseID: "kb-1",
				DocumentID:      "doc-1",
				ChunkIndex:      2,
				Content:         "来源内容",
				Enabled:         true,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/knowledge-base/chunks/chunk-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Code string `json:"code"`
		Data struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Code != "0" || result.Data.ID != "chunk-1" || result.Data.Content != "来源内容" {
		t.Fatalf("unexpected get response: %+v", result.Data)
	}
}
```

- [ ] **Step 4: 编译 + 测试**

Run: `go build ./internal/app/knowledge/... ./internal/adapter/http/knowledge/...`
Run: `go test ./internal/adapter/http/knowledge/... -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/app/knowledge/service/chunk/knowledge_chunk_query_service.go internal/adapter/http/knowledge/knowledge_chunk_handler.go internal/adapter/http/knowledge/test/knowledge_chunk_handler_test.go
git commit -m "feat: add chunk detail endpoint for citation popover"
```

---

## Task 11: 前端依赖 + Markdown 渲染

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/src/components/chat/MarkdownRenderer.tsx`

- [ ] **Step 1: 安装 rehype-raw**

Run: `cd frontend && npm install rehype-raw`
Expected: `rehype-raw` 出现在 package.json dependencies

- [ ] **Step 2: MarkdownRenderer 集成**

在 `MarkdownRenderer.tsx` 顶部加：

```tsx
import rehypeRaw from "rehype-raw";
import { CitationChip } from "@/components/chat/CitationChip";
```

`<ReactMarkdown>` 加 `rehypePlugins` 和组件：

```tsx
<ReactMarkdown
  remarkPlugins={[remarkGfm]}
  rehypePlugins={[rehypeRaw]}
  components={{
    kb: CitationChip,
    code({ inline, className, children, node, ...props }) {
    ...
```

- [ ] **Step 3: 编译验证**

Run: `cd frontend && npm run build`
Expected: 成功（`kb` 组件类型引用会在 Task 12 定义后通过；若此时报错，先建一个最小 `CitationChip` 占位导出）

- [ ] **Step 4: 提交**

```bash
git add frontend/package.json frontend/package-lock.json frontend/src/components/chat/MarkdownRenderer.tsx
git commit -m "feat: render kb citation tags in markdown"
```

---

## Task 12: 前端引用角标组件

**Files:**
- Create: `frontend/src/components/chat/citationContext.tsx`
- Create: `frontend/src/components/chat/CitationChip.tsx`
- Modify: `frontend/src/components/chat/MessageItem.tsx`
- Modify: `frontend/src/services/chatService.ts`
- Modify: `frontend/src/types/index.ts`

- [ ] **Step 1: citationContext.tsx**

```tsx
import * as React from "react";

interface CitationNumberContextValue {
  nextIndex: () => number;
}

const CitationNumberContext = React.createContext<CitationNumberContextValue>({
  nextIndex: () => 0
});

export function CitationNumberProvider({ children }: { children: React.ReactNode }) {
  const counter = React.useRef(0);
  const value: CitationNumberContextValue = React.useMemo(
    () => ({
      nextIndex: () => {
        counter.current += 1;
        return counter.current;
      }
    }),
    [counter]
  );
  return <CitationNumberContext.Provider value={value}>{children}</CitationNumberContext.Provider>;
}

export function useCitationNumber() {
  return React.useContext(CitationNumberContext);
}
```

- [ ] **Step 2: types/index.ts 加 ChunkDetail**

```ts
export interface ChunkDetail {
  id: string;
  content: string;
  chunkIndex: number;
  docId: string;
  kbId: string;
}
```

- [ ] **Step 3: chatService.ts 加 getChunkDetail**

```ts
import type { ChunkDetail } from "@/types";

export async function getChunkDetail(chunkId: string) {
  return api.get<ChunkDetail>(`/knowledge-base/chunks/${encodeURIComponent(chunkId)}`);
}
```

- [ ] **Step 4: CitationChip.tsx**

```tsx
import * as React from "react";
import { FileText } from "lucide-react";
import * as Popover from "@radix-ui/react-popover";

import { useCitationNumber } from "@/components/chat/citationContext";
import { getChunkDetail } from "@/services/chatService";
import type { ChunkDetail } from "@/types";

interface CitationChipProps {
  node?: any;
}

export function CitationChip({ node }: CitationChipProps) {
  const { nextIndex } = useCitationNumber();
  const indexRef = React.useRef<number | null>(null);
  if (indexRef.current === null) {
    indexRef.current = nextIndex();
  }
  const index = indexRef.current;

  const properties = node?.properties ?? {};
  const doc = String(properties.doc ?? "");
  const chunkId = String(properties.chunk_id ?? properties.chunkId ?? "");
  const kbId = String(properties.kb_id ?? properties.kbId ?? "");

  const [detail, setDetail] = React.useState<ChunkDetail | null>(null);
  const [open, setOpen] = React.useState(false);

  const handleOpenChange = async (next: boolean) => {
    setOpen(next);
    if (next && chunkId && !detail) {
      try {
        setDetail(await getChunkDetail(chunkId));
      } catch {
        setDetail(null);
      }
    }
  };

  return (
    <Popover.Root open={open} onOpenChange={handleOpenChange}>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="mx-0.5 inline-flex items-center rounded bg-blue-50 px-1.5 py-0.5 align-super text-[11px] font-semibold text-blue-700 hover:bg-blue-100 dark:bg-blue-950 dark:text-blue-300"
          title={doc || "来源"}
        >
          <FileText className="mr-0.5 h-3 w-3" />
          {index}
        </button>
      </Popover.Trigger>
      <Popover.Content
        side="top"
        className="z-50 w-80 rounded-lg border border-gray-200 bg-white p-3 shadow-lg dark:border-gray-700 dark:bg-gray-800"
      >
        <div className="text-sm font-medium text-gray-900 dark:text-gray-100">
          {doc || (detail?.docId ? `文档 ${detail.docId}` : "来源文档")}
        </div>
        {kbId ? <div className="mt-1 text-xs text-gray-500">知识库：{kbId}</div> : null}
        {detail ? (
          <div className="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-xs leading-5 text-gray-700 dark:bg-gray-900 dark:text-gray-300">
            {detail.content}
          </div>
        ) : (
          <div className="mt-2 text-xs text-gray-400">加载中…</div>
        )}
      </Popover.Content>
    </Popover.Root>
  );
}
```

- [ ] **Step 5: MessageItem 包 Provider**

`MessageItem.tsx` 中，`hasContent` 分支改为：

```tsx
{hasContent ? (
  <CitationNumberProvider>
    <MarkdownRenderer content={message.content} />
  </CitationNumberProvider>
) : null}
```

加 import：`import { CitationNumberProvider } from "@/components/chat/citationContext";`

- [ ] **Step 6: 构建 + 类型检查**

Run: `cd frontend && npm run build`
Run: `cd frontend && npm run lint`
Expected: 成功

- [ ] **Step 7: 提交**

```bash
git add frontend/src/components/chat/citationContext.tsx frontend/src/components/chat/CitationChip.tsx frontend/src/components/chat/MessageItem.tsx frontend/src/services/chatService.ts frontend/src/types/index.ts
git commit -m "feat: render citation chips with source popover"
```

---

## Task 13: 全量验证

- [ ] **Step 1: 后端全量测试**

Run: `go test ./cmd/... ./internal/... -count=1`
Expected: PASS（若偶发与本地基础设施相关失败，以 `./internal/app/rag/core/citation/ ./internal/app/rag/core/prompt/ ./internal/app/rag/service/chat/ ./internal/app/knowledge/...` 定向为准）

- [ ] **Step 2: 前端构建**

Run: `cd frontend && npm run build && npm run lint`
Expected: 成功

- [ ] **Step 3: 手工冒烟**

- 启动 `go run ./cmd/server`。
- 向一个启用知识库的会话提问，确认回答中模型使用了 `<ref id="cN"/>`，SSE 消息流里出现 `<kb doc=… chunk_id=…/>`。
- 刷新页面重新进入该会话，确认历史 assistant 消息内容为 `<kb/>`（持久化），再次提问时模型输入中无真实 chunk ID。
- 前端回答里出现 `[1]` 角标，点击弹出文档标题与 chunk 内容。

- [ ] **Step 4: 总结提交**

确保所有任务已提交，`git status --short` 中仅剩与本次无关的既有改动。

---

## 自审记录

- **Spec 覆盖**：注册表（Task 1-2）、协议/渲染（Task 3）、展开（Task 4）、历史重压缩（Task 5）、prompt 注入（Task 6）、配置/装配（Task 7）、聊天服务集成（Task 8）、服务级测试（Task 9）、chunk 端点（Task 10）、前端（Task 11-12）、全量验证（Task 13）。设计文档 7 节全部有对应任务。
- **已知细化**：设计文档中"按文档分组渲染"简化为扁平 `<chunk>` 列表（chunk 数少、预算截断更可靠）；协议 prompt 仅保留 cN 句柄（RAG 聊天无工具参数解码需求）。`dN/bN` 仍注册（为下一轮 agent 路径预留）。
- **关键风险已覆盖**：`applyRetrieveContextBudget` 会用普通格式重建 KnowledgeContext 覆盖句柄 XML —— 通过 `RenderKnowledgeContextWithBudget`（Task 8 Step 3）解决。
