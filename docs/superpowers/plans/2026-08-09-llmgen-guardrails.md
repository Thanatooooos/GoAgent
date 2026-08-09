# LLM 生成护栏库（llmgen）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新建 `internal/framework/llmgen/` 通用 LLM 生成护栏库（句柄编解码 + 确定性引用校验 + 纯文本重写），并集成到 ingestion enricher 的 `GenerateQuestions` 作为示范，使幻觉引用 fail-closed、引用标记不污染问题文本。

**Architecture:** 三个纯 stdlib 组件：`HandleSet`（高熵 ID ↔ 短句柄，解码 fail-closed）、`RefValidator`（引用必须命中候选集，确定性拒绝原因）、`RewriteRefs`/`CleanDeadRefs`（带 forbidden spans/词边界保护的文本重写）。enricher 集成：`GenerateQuestions` 返回 `GenerateQuestionsResult{Questions, RejectedRefs}`，prompt 用句柄 `r1` 标注源 chunk，输出经解析/校验/剥离。

**Tech Stack:** Go 1.25 标准库（`regexp`, `strings`）。

**参考设计:** `docs/superpowers/specs/2026-08-09-llmgen-guardrails-design.md`

---

## 文件结构

**新建（llmgen 包）：**
- `internal/framework/llmgen/handle.go` — `HandleSet`
- `internal/framework/llmgen/validate.go` — `RefValidator` + `RejectReason`
- `internal/framework/llmgen/rewrite.go` — `RewriteRule`/`RewriteOptions`/`RewriteStats`/`RewriteRefs`/`CleanDeadRefs`
- 各 `_test.go`

**修改（enricher 集成）：**
- `internal/app/ingestion/service/runner/llm_enrichment.go` — `GeneratedQuestion`/`GenerateQuestionsResult` 类型、`DocumentEnricher` 接口签名、`GenerateQuestions` 集成 llmgen、`questionTexts` helper
- `internal/app/ingestion/service/runner/llm_enrichment_test.go` — 更新 stub 与断言、新增引用测试
- `internal/app/ingestion/service/runner/runner_enhancer.go` — 传 `SourceChunkID`、适配新返回、artifacts 上报 RejectedRefs
- `internal/app/ingestion/service/runner/runner_enhancer_test.go`（如 stub 受影响）

---

## Task 1: llmgen.HandleSet

**Files:**
- Create: `internal/framework/llmgen/handle.go`
- Test: `internal/framework/llmgen/handle_test.go`

- [ ] **Step 1: 写失败测试**

```go
package llmgen

import "testing"

func TestHandleSetEncodeDedup(t *testing.T) {
	h := NewHandleSet("r")
	h1, ok := h.Encode("chunk-abc")
	if !ok || h1 != "r1" {
		t.Fatalf("first encode = (%q,%v), want (r1,true)", h1, ok)
	}
	h2, ok := h.Encode("chunk-abc")
	if !ok || h2 != "r1" {
		t.Fatalf("re-encode same id = (%q,%v), want (r1,true)", h2, ok)
	}
	h3, ok := h.Encode("chunk-def")
	if !ok || h3 != "r2" {
		t.Fatalf("second encode = (%q,%v), want (r2,true)", h3, ok)
	}
}

func TestHandleSetEncodeEmpty(t *testing.T) {
	h := NewHandleSet("r")
	if _, ok := h.Encode("  "); ok {
		t.Fatal("empty id should not encode")
	}
}

func TestHandleSetResolveRoundTripAndFailClosed(t *testing.T) {
	h := NewHandleSet("r")
	h.Encode("chunk-abc")
	if id, ok := h.Resolve("r1"); !ok || id != "chunk-abc" {
		t.Fatalf("resolve r1 = (%q,%v)", id, ok)
	}
	if id, ok := h.Resolve("R1"); !ok || id != "chunk-abc" {
		t.Fatalf("resolve R1 (case-insensitive) = (%q,%v)", id, ok)
	}
	if _, ok := h.Resolve("r9"); ok {
		t.Fatal("resolve unknown handle should fail closed")
	}
}

func TestHandleSetResolveAll(t *testing.T) {
	h := NewHandleSet("r")
	h.Encode("chunk-a")
	h.Encode("chunk-b")
	resolved, unresolved := h.ResolveAll([]string{"r1", "r9", "r2"})
	if len(resolved) != 2 || len(unresolved) != 1 || unresolved[0] != "r9" {
		t.Fatalf("ResolveAll = (%v, %v)", resolved, unresolved)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/framework/llmgen/ -count=1`
Expected: FAIL（`undefined: NewHandleSet`）

- [ ] **Step 3: 实现 handle.go**

```go
package llmgen

import (
	"strconv"
	"strings"
)

// HandleSet encodes durable identifiers as request-local short handles (e.g.
// r1, r2) for LLM prompts, and decodes model output back fail-closed. Handles
// are per-request and never persisted.
type HandleSet struct {
	prefix   string
	byKey    map[string]string
	byHandle map[string]string
	next     int
}

func NewHandleSet(prefix string) *HandleSet {
	if prefix == "" {
		prefix = "r"
	}
	return &HandleSet{
		prefix:   prefix,
		byKey:    map[string]string{},
		byHandle: map[string]string{},
		next:     1,
	}
}

// Encode returns the handle for a durable id, reusing an existing handle on
// duplicate. Empty ids are rejected.
func (h *HandleSet) Encode(durable string) (string, bool) {
	if h == nil {
		return "", false
	}
	durable = strings.TrimSpace(durable)
	if durable == "" {
		return "", false
	}
	if handle, ok := h.byKey[durable]; ok {
		return handle, true
	}
	handle := h.prefix + strconv.Itoa(h.next)
	h.next++
	h.byKey[durable] = handle
	h.byHandle[handle] = durable
	return handle, true
}

// Resolve decodes a handle back to its durable id. Unknown handles fail closed.
func (h *HandleSet) Resolve(handle string) (string, bool) {
	if h == nil {
		return "", false
	}
	id, ok := h.byHandle[strings.ToLower(strings.TrimSpace(handle))]
	return id, ok
}

// ResolveAll decodes a batch, returning resolved ids and unresolved handles.
func (h *HandleSet) ResolveAll(handles []string) (resolved, unresolved []string) {
	for _, handle := range handles {
		if id, ok := h.Resolve(handle); ok {
			resolved = append(resolved, id)
		} else {
			unresolved = append(unresolved, handle)
		}
	}
	return resolved, unresolved
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/framework/llmgen/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/framework/llmgen/handle.go internal/framework/llmgen/handle_test.go
git commit -m "feat: add llmgen handle set"
```

---

## Task 2: llmgen.RefValidator

**Files:**
- Create: `internal/framework/llmgen/validate.go`
- Test: `internal/framework/llmgen/validate_test.go`

- [ ] **Step 1: 写失败测试**

```go
package llmgen

import "testing"

func TestRefValidatorValidateHitAndMiss(t *testing.T) {
	v := NewRefValidator([]string{"chunk-a", " chunk-b "})
	if id, reason := v.Validate("chunk-a"); id != "chunk-a" || reason != ReasonOK {
		t.Fatalf("validate chunk-a = (%q,%q)", id, reason)
	}
	if id, reason := v.Validate("chunk-b"); id != "chunk-b" || reason != ReasonOK {
		t.Fatalf("validate chunk-b (trimmed candidate) = (%q,%q)", id, reason)
	}
	if _, reason := v.Validate("chunk-zzz"); reason != ReasonNotInCandidateSet {
		t.Fatalf("validate unknown = reason %q, want not_in_candidate_set", reason)
	}
	if _, reason := v.Validate("   "); reason != ReasonEmpty {
		t.Fatalf("validate blank = reason %q, want empty", reason)
	}
}

func TestRefValidatorValidateMany(t *testing.T) {
	v := NewRefValidator([]string{"chunk-a"})
	results := v.ValidateMany([]string{"chunk-a", "nope", ""})
	if len(results) != 3 {
		t.Fatalf("ValidateMany length = %d", len(results))
	}
	if results[0].Reason != ReasonOK || results[0].ID != "chunk-a" {
		t.Fatalf("results[0] = %+v", results[0])
	}
	if results[1].Reason != ReasonNotInCandidateSet || results[1].ID != "" {
		t.Fatalf("results[1] = %+v", results[1])
	}
	if results[2].Reason != ReasonEmpty {
		t.Fatalf("results[2] = %+v", results[2])
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/framework/llmgen/ -run TestRefValidator -count=1`
Expected: FAIL（`undefined: NewRefValidator`）

- [ ] **Step 3: 实现 validate.go**

```go
package llmgen

import "strings"

type RejectReason string

const (
	ReasonOK                RejectReason = ""
	ReasonNotInCandidateSet RejectReason = "reference_not_in_candidate_set"
	ReasonMalformed         RejectReason = "malformed_reference"
	ReasonEmpty             RejectReason = "empty_reference"
)

// RefValidator deterministically checks that a reference resolves to an
// allowed candidate id. Rules are rule-based (never LLM-judged).
type RefValidator struct {
	allowed map[string]struct{}
}

func NewRefValidator(candidates []string) *RefValidator {
	allowed := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		allowed[candidate] = struct{}{}
	}
	return &RefValidator{allowed: allowed}
}

// Validate returns the canonical candidate id (ReasonOK) or a rejection reason.
func (v *RefValidator) Validate(ref string) (string, RejectReason) {
	if v == nil {
		return "", ReasonMalformed
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ReasonEmpty
	}
	if _, ok := v.allowed[ref]; ok {
		return ref, ReasonOK
	}
	return "", ReasonNotInCandidateSet
}

type RefResult struct {
	Ref    string
	ID     string
	Reason RejectReason
}

func (v *RefValidator) ValidateMany(refs []string) []RefResult {
	results := make([]RefResult, 0, len(refs))
	for _, ref := range refs {
		id, reason := v.Validate(ref)
		results = append(results, RefResult{Ref: ref, ID: id, Reason: reason})
	}
	return results
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/framework/llmgen/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/framework/llmgen/validate.go internal/framework/llmgen/validate_test.go
git commit -m "feat: add llmgen reference validator"
```

---

## Task 3: llmgen.RewriteRefs + CleanDeadRefs

**Files:**
- Create: `internal/framework/llmgen/rewrite.go`
- Test: `internal/framework/llmgen/rewrite_test.go`

- [ ] **Step 1: 写失败测试**

```go
package llmgen

import (
	"regexp"
	"strings"
	"testing"
)

func TestRewriteRefsBasic(t *testing.T) {
	got, stats := RewriteRefs("答案 [[r1]] 结束", []RewriteRule{{Find: "[[r1]]", Replace: "[来源]"}}, RewriteOptions{})
	if got != "答案 [来源] 结束" {
		t.Fatalf("rewrite = %q", got)
	}
	if stats.Rewritten != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestRewriteRefsSkipsCodeBlocks(t *testing.T) {
	text := "正文\n```\n[[r1]] not a ref\n```\n后文 [[r1]]"
	got, stats := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "X"}}, RewriteOptions{SkipCodeBlocks: true})
	if strings.Contains(got, "后文 X") == false {
		t.Fatalf("inline ref not rewritten: %q", got)
	}
	if !strings.Contains(got, "```\n[[r1]] not a ref\n```") {
		t.Fatalf("code block ref should be preserved: %q", got)
	}
	if stats.Rewritten != 1 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v (rewritten=%d skipped=%d)", stats, stats.Rewritten, stats.Skipped)
	}
}

func TestRewriteRefsSkipsInlineCodeAndLinks(t *testing.T) {
	text := "`[[r1]]` and [label](https://x/[[r1]]) and [[r1]]"
	got, _ := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "R"}}, RewriteOptions{SkipCodeBlocks: true, SkipLinks: true})
	if got != "`[[r1]]` and [label](https://x/[[r1]]) and R" {
		t.Fatalf("rewrite = %q", got)
	}
}

func TestRewriteRefsWordBoundary(t *testing.T) {
	text := "a[[r1]]b [[r1]]"
	got, _ := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "R"}}, RewriteOptions{WordBoundary: true})
	if got != "a[[r1]]b R" {
		t.Fatalf("rewrite with word boundary = %q", got)
	}
}

func TestCleanDeadRefs(t *testing.T) {
	text := "见 [[r1]] 与 [[r9]]"
	refRE := regexp.MustCompile(`\[\[(r[1-9][0-9]*)\]\]`)
	got, removed := CleanDeadRefs(text, func(id string) bool { return id == "r1" }, refRE)
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	if strings.Contains(got, "r9") {
		t.Fatalf("dead ref not removed: %q", got)
	}
	if !strings.Contains(got, "[[r1]]") {
		t.Fatalf("valid ref removed: %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/framework/llmgen/ -run 'TestRewrite|TestCleanDead' -count=1`
Expected: FAIL（`undefined: RewriteRefs`）

- [ ] **Step 3: 实现 rewrite.go**

```go
package llmgen

import (
	"regexp"
	"strings"
)

type RewriteRule struct {
	Find    string
	Replace string
}

type RewriteOptions struct {
	SkipCodeBlocks bool
	SkipLinks      bool
	WordBoundary   bool
}

type RewriteStats struct {
	Rewritten int
	Skipped   int
}

type span struct{ start, end int }

// RewriteRefs rewrites occurrences of rule.Find in text, skipping occurrences
// inside forbidden spans (code blocks / inline code / existing links) and
// optionally protecting word boundaries. Rules are applied in a single pass
// (leftmost-earliest match wins), so produced text is never re-processed.
func RewriteRefs(text string, rules []RewriteRule, opts RewriteOptions) (string, RewriteStats) {
	var stats RewriteStats
	if len(rules) == 0 || text == "" {
		return text, stats
	}
	active := make([]RewriteRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Find != "" {
			active = append(active, rule)
		}
	}
	if len(active) == 0 {
		return text, stats
	}
	forbidden := buildForbiddenSpans(text, opts)

	var b strings.Builder
	b.Grow(len(text))
	i := 0
	for i < len(text) {
		bestRule, bestIdx, bestEnd := -1, -1, -1
		for ri, rule := range active {
			idx := strings.Index(text[i:], rule.Find)
			if idx < 0 {
				continue
			}
			idx += i
			if insideSpan(forbidden, idx) {
				stats.Skipped++
				continue
			}
			if opts.WordBoundary && !wordBoundaryOK(text, idx, len(rule.Find)) {
				continue
			}
			if bestIdx < 0 || idx < bestIdx {
				bestRule, bestIdx, bestEnd = ri, idx, idx+len(rule.Find)
			}
		}
		if bestRule < 0 {
			b.WriteString(text[i:])
			break
		}
		b.WriteString(text[i:bestIdx])
		b.WriteString(active[bestRule].Replace)
		stats.Rewritten++
		i = bestEnd
	}
	return b.String(), stats
}

func buildForbiddenSpans(text string, opts RewriteOptions) []span {
	var spans []span
	if opts.SkipCodeBlocks {
		spans = append(spans, codeSpans(text)...)
	}
	if opts.SkipLinks {
		spans = append(spans, linkSpans(text)...)
	}
	return spans
}

func codeSpans(text string) []span {
	var spans []span
	// inline code: `...`
	for i := 0; i < len(text); {
		open := strings.IndexByte(text[i:], '`')
		if open < 0 {
			break
		}
		open += i
		close := strings.IndexByte(text[open+1:], '`')
		if close < 0 {
			break
		}
		close += open + 1
		spans = append(spans, span{open, close + 1})
		i = close + 1
	}
	// fenced code blocks: ``` ... ```
	lines := strings.Split(text, "\n")
	inFence := false
	fenceStart := 0
	offset := 0
	for li, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				fenceStart = offset
			} else {
				spans = append(spans, span{fenceStart, offset + len(line)})
				inFence = false
			}
		}
		offset += len(line) + 1
		_ = li
	}
	if inFence {
		spans = append(spans, span{fenceStart, len(text)})
	}
	return spans
}

func linkSpans(text string) []span {
	var spans []span
	for i := 0; i < len(text); {
		open := strings.IndexByte(text[i:], '[')
		if open < 0 {
			break
		}
		open += i
		// find "(...)" after the closing ]
		closeBracket := strings.IndexByte(text[open+1:], ']')
		if closeBracket < 0 {
			break
		}
		closeBracket += open + 1
		paren := closeBracket + 1
		if paren >= len(text) || text[paren] != '(' {
			i = closeBracket + 1
			continue
		}
		depth := 0
		end := -1
		for j := paren; j < len(text); j++ {
			switch text[j] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = j + 1
				}
			}
			if end > 0 {
				break
			}
		}
		if end > 0 {
			spans = append(spans, span{open, end})
			i = end
		} else {
			i = closeBracket + 1
		}
	}
	return spans
}

func insideSpan(spans []span, idx int) bool {
	for _, s := range spans {
		if idx >= s.start && idx < s.end {
			return true
		}
	}
	return false
}

func wordBoundaryOK(text string, idx, length int) bool {
	if idx > 0 && isWordChar(text[idx-1]) {
		return false
	}
	end := idx + length
	if end < len(text) && isWordChar(text[end]) {
		return false
	}
	return true
}

func isWordChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// CleanDeadRefs removes references matching refPattern whose id is not kept by
// the keep predicate. It returns the cleaned text and the number of removed refs.
func CleanDeadRefs(text string, keep func(id string) bool, refPattern *regexp.Regexp) (string, int) {
	if text == "" || refPattern == nil {
		return text, 0
	}
	removed := 0
	out := refPattern.ReplaceAllStringFunc(text, func(match string) string {
		m := refPattern.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		if keep != nil && keep(m[1]) {
			return match
		}
		removed++
		return ""
	})
	return out, removed
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/framework/llmgen/ -count=1`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/framework/llmgen/rewrite.go internal/framework/llmgen/rewrite_test.go
git commit -m "feat: add llmgen text rewriting"
```

---

## Task 4: enricher 接口与返回类型

**Files:**
- Modify: `internal/app/ingestion/service/runner/llm_enrichment.go`
- Test: `internal/app/ingestion/service/runner/llm_enrichment_test.go`

- [ ] **Step 1: 修改接口与类型**

`llm_enrichment.go` 中 `EnrichmentOptions` 加字段，新增类型，修改接口：

```go
type EnrichmentOptions struct {
	QuestionCount     int
	MaxQuestionLength int
	SummaryMaxChars   int
	SourceChunkID     string // 生成问题对应的 chunk ID（用于句柄编码与校验）
}

// GeneratedQuestion 是带来源的问题：Text 为剥除引用标记后的干净文本，
// SourceChunkID 为校验后解析出的来源 chunk ID（未命中时为空串）。
type GeneratedQuestion struct {
	Text          string
	SourceChunkID string
}

// GenerateQuestionsResult 返回问题列表与因引用未命中候选集而被拒绝的数量。
type GenerateQuestionsResult struct {
	Questions    []GeneratedQuestion
	RejectedRefs int
}

type DocumentEnricher interface {
	Summarize(context.Context, string, EnrichmentOptions) (string, error)
	GenerateQuestions(context.Context, string, string, EnrichmentOptions) (GenerateQuestionsResult, error)
}
```

新增 import：`"local/rag-project/internal/framework/llmgen"` 和 `"regexp"`。

- [ ] **Step 2: 更新测试 stub 编译**

`llm_enrichment_test.go` 中 `failingDocumentEnricher.GenerateQuestions` 签名改为：

```go
func (failingDocumentEnricher) GenerateQuestions(context.Context, string, string, EnrichmentOptions) (GenerateQuestionsResult, error) {
	return GenerateQuestionsResult{}, errors.New("model unavailable")
}
```

`promptCompleterStub` 的 `Chat` 返回值改为带引用标记的示例，供后续断言：

```go
func (s *promptCompleterStub) Chat(prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	return "Question one? [[r1]]\nQuestion two?", nil
}
```

- [ ] **Step 3: 更新现有测试断言**

`TestLLMDocumentEnricherGeneratesQuestionsThroughPromptCompleter` 改为：

```go
func TestLLMDocumentEnricherGeneratesQuestionsThroughPromptCompleter(t *testing.T) {
	client := &promptCompleterStub{}
	enricher := NewLLMDocumentEnricher(client)
	res, err := enricher.GenerateQuestions(context.Background(), "Guide", "Chunk content", EnrichmentOptions{QuestionCount: 2, MaxQuestionLength: 80, SourceChunkID: "chunk-1"})
	if err != nil {
		t.Fatalf("generate questions: %v", err)
	}
	if len(res.Questions) != 2 || res.Questions[0].Text != "Question one?" {
		t.Fatalf("unexpected questions: %#v", res.Questions)
	}
	if res.Questions[0].SourceChunkID != "chunk-1" {
		t.Fatalf("source chunk not resolved: %#v", res.Questions[0])
	}
	if len(client.prompts) != 1 || !strings.Contains(client.prompts[0], "Chunk content") {
		t.Fatalf("unexpected prompt: %#v", client.prompts)
	}
	if !strings.Contains(client.prompts[0], "r1") {
		t.Fatalf("prompt should mention source handle r1: %#v", client.prompts[0])
	}
}
```

`TestEnhancerNodeRunnerLLMFailureDoesNotBlockChunks` 的断言不变（失败时 Questions 仍为空）。

- [ ] **Step 4: 实现 GenerateQuestions（集成 llmgen）**

替换 `llm_enrichment.go` 的 `GenerateQuestions` 实现并新增 helper：

```go
var questionRefRE = regexp.MustCompile(`(?i)\[\[(r[1-9][0-9]*)\]\]`)

func (e *llmDocumentEnricher) GenerateQuestions(_ context.Context, title string, content string, options EnrichmentOptions) (GenerateQuestionsResult, error) {
	if e == nil || e.client == nil {
		return GenerateQuestionsResult{}, fmt.Errorf("document enrichment chat client is required")
	}
	sourceChunkID := strings.TrimSpace(options.SourceChunkID)
	sourceHint := ""
	var handles *llmgen.HandleSet
	if sourceChunkID != "" {
		handles = llmgen.NewHandleSet("r")
		if sourceHandle, ok := handles.Encode(sourceChunkID); ok {
			sourceHint = fmt.Sprintf("\n当前内容块句柄为 %s；若问题依赖本段内容，请在问题末尾追加 [[%s]]。", sourceHandle, sourceHandle)
		}
	}
	prompt := fmt.Sprintf("为下面内容生成最多 %d 个可由该段充分回答的用户问题，每行一个，不要编号。标题：%s\n\n内容：%s%s", options.QuestionCount, strings.TrimSpace(title), strings.TrimSpace(content), sourceHint)
	response, err := e.client.Chat(prompt)
	if err != nil {
		return GenerateQuestionsResult{}, err
	}
	raw := normalizeGeneratedQuestions(strings.Split(response, "\n"), options.QuestionCount, options.MaxQuestionLength)
	return resolveGeneratedQuestions(raw, handles, sourceChunkID), nil
}

// resolveGeneratedQuestions 解析问题行尾的 [[rN]] 引用：命中句柄则记录来源并剥离标记；
// 未命中句柄（幻觉引用）不落库并计入 RejectedRefs。无引用问题保持原样。
func resolveGeneratedQuestions(values []string, handles *llmgen.HandleSet, sourceChunkID string) GenerateQuestionsResult {
	validator := llmgen.NewRefValidator([]string{sourceChunkID})
	result := GenerateQuestionsResult{Questions: make([]GeneratedQuestion, 0, len(values))}
	for _, value := range values {
		question := GeneratedQuestion{Text: strings.TrimSpace(value)}
		if handles == nil {
			result.Questions = append(result.Questions, question)
			continue
		}
		rules := make([]llmgen.RewriteRule, 0)
		for _, match := range questionRefRE.FindAllStringSubmatch(question.Text, -1) {
			handle := strings.ToLower(match[1])
			rules = append(rules, llmgen.RewriteRule{Find: "[[" + handle + "]]", Replace: ""})
			if id, ok := handles.Resolve(handle); ok {
				if _, reason := validator.Validate(id); reason == llmgen.ReasonOK {
					question.SourceChunkID = id
				} else {
					result.RejectedRefs++
				}
			} else {
				result.RejectedRefs++
			}
		}
		if len(rules) > 0 {
			cleaned, _ := llmgen.RewriteRefs(question.Text, rules, llmgen.RewriteOptions{SkipLinks: true, WordBoundary: true})
			question.Text = strings.TrimSpace(cleaned)
		}
		result.Questions = append(result.Questions, question)
	}
	return result
}

// questionTexts 提取问题文本，供 runner 写回 Chunk.Questions。
func questionTexts(questions []GeneratedQuestion) []string {
	result := make([]string, 0, len(questions))
	for _, question := range questions {
		result = append(result, question.Text)
	}
	return result
}
```

- [ ] **Step 5: 新增引用处理测试**

在 `llm_enrichment_test.go` 追加：

```go
func TestResolveGeneratedQuestionsStripsValidRef(t *testing.T) {
	handles := llmgen.NewHandleSet("r")
	handles.Encode("chunk-1")
	res := resolveGeneratedQuestions([]string{"问题一 [[r1]]", "问题二"}, handles, "chunk-1")
	if len(res.Questions) != 2 {
		t.Fatalf("questions = %#v", res.Questions)
	}
	if res.Questions[0].Text != "问题一" || res.Questions[0].SourceChunkID != "chunk-1" {
		t.Fatalf("valid ref not resolved: %+v", res.Questions[0])
	}
	if res.Questions[1].SourceChunkID != "" {
		t.Fatalf("no-ref question should have empty source: %+v", res.Questions[1])
	}
	if res.RejectedRefs != 0 {
		t.Fatalf("RejectedRefs = %d", res.RejectedRefs)
	}
}

func TestResolveGeneratedQuestionsRejectsHallucinatedRef(t *testing.T) {
	handles := llmgen.NewHandleSet("r")
	handles.Encode("chunk-1")
	res := resolveGeneratedQuestions([]string{"问题三 [[r2]]"}, handles, "chunk-1")
	if len(res.Questions) != 1 {
		t.Fatalf("questions = %#v", res.Questions)
	}
	if res.Questions[0].Text != "问题三" {
		t.Fatalf("hallucinated ref should be stripped, got %q", res.Questions[0].Text)
	}
	if res.Questions[0].SourceChunkID != "" {
		t.Fatalf("hallucinated ref must not set source: %+v", res.Questions[0])
	}
	if res.RejectedRefs != 1 {
		t.Fatalf("RejectedRefs = %d, want 1", res.RejectedRefs)
	}
}

func TestResolveGeneratedQuestionsWithoutSource(t *testing.T) {
	res := resolveGeneratedQuestions([]string{"问题四 [[r1]]"}, nil, "")
	if len(res.Questions) != 1 || res.Questions[0].Text != "问题四 [[r1]]" {
		t.Fatalf("no-source mode should pass through: %#v", res.Questions)
	}
}
```

（`llm_enrichment_test.go` 需要新增 import `"local/rag-project/internal/framework/llmgen"`。）

- [ ] **Step 6: 运行测试**

Run: `go test ./internal/app/ingestion/service/runner/ -count=1`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/app/ingestion/service/runner/llm_enrichment.go internal/app/ingestion/service/runner/llm_enrichment_test.go
git commit -m "feat: make enricher question generation reference-aware"
```

---

## Task 5: runner_enhancer.go 适配

**Files:**
- Modify: `internal/app/ingestion/service/runner/runner_enhancer.go`
- Test: `internal/app/ingestion/service/runner/runner_enhancer_test.go`（如受影响）

- [ ] **Step 1: 修改 questions 分支**

`runner_enhancer.go` 中 LLM questions 分支改为：

```go
		if includesEnrichmentTask(tasks, "questions") {
			for index := range next.Chunks {
				chunkOptions := options
				chunkOptions.SourceChunkID = next.Chunks[index].ID
				result, err := r.documentEnricher.GenerateQuestions(ctx, next.Parsed.Title, next.Chunks[index].Content, chunkOptions)
				if err != nil {
					llmDegraded = true
					continue
				}
				next.Chunks[index].Questions = questionTexts(result.Questions)
				if result.RejectedRefs > 0 {
					llmDegraded = true
					next.Artifacts["enhancerRejectedQuestionRefs"] = result.RejectedRefs
				}
			}
		}
```

（`questionTexts` 在 `llm_enrichment.go` 已定义，同包直接可用。）

- [ ] **Step 2: 运行测试确认通过**

`DocumentEnricher` 的实现者仅 `llmDocumentEnricher`（Task 4 已改）与测试 stub `failingDocumentEnricher`（Task 4 已改）；`runner_enhancer_test.go`/`runner_node_test.go` 均通过 `NewEnhancerNodeRunner()`（无 enricher）或 `failingDocumentEnricher` 构造，不直接实现接口，**无需改动**。`internal/app/ingestion/service/aliases.go` 的 `DocumentEnricher = ingestionrunner.DocumentEnricher` 类型别名自动跟随接口签名变化，无需改动。

Run: `go test ./internal/app/ingestion/service/runner/ ./internal/app/ingestion/... -count=1`
Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/app/ingestion/service/runner/runner_enhancer.go
git commit -m "feat: wire llmgen guardrails into enhancer runner"
```

---

## Task 6: 全量验证

- [ ] **Step 1: 编译 + 测试**

Run:
- `go build ./internal/framework/llmgen/ ./internal/app/ingestion/...`
- `go test ./internal/framework/llmgen/ ./internal/app/ingestion/... -count=1`

Expected: 全部通过。

- [ ] **Step 2: 全仓核心测试**

Run: `go test ./cmd/... ./internal/... -count=1`
Expected: 除仓库既有的失败包（cmd/eval-runner、cmd/rewrite-eval、config 默认值）外全绿；llmgen 与 ingestion 相关包必须通过。

- [ ] **Step 3: 自检设计覆盖**

逐条对照 `docs/superpowers/specs/2026-08-09-llmgen-guardrails-design.md`：
- HandleSet 编解码/去重/fail-closed/大小写不敏感/空拒绝/ResolveAll → Task 1 ✓
- RefValidator 三拒绝原因 + ValidateMany + 候选集规范化 → Task 2 ✓
- RewriteRefs 跳过代码块/内联代码/链接/词边界 + 统计 + CleanDeadRefs → Task 3 ✓
- GeneratedQuestion/GenerateQuestionsResult/接口签名 → Task 4 ✓
- prompt 句柄 r1、解析/校验/剥离、幻觉不落库、RejectedRefs 上报 → Task 4+5 ✓
- 兼容性：无引用行为不变、indexer 落库路径不动、不改 schema ✓

---

## 自审记录

- **Spec 覆盖**：设计文档全部小节均有对应任务。
- **已知细化**：
  - 观察上报从"设计文档的 `next.Artifacts["enhancer"]` 内部 counter"改为 runner 侧 `next.Artifacts["enhancerRejectedQuestionRefs"]`（enricher 通过 `GenerateQuestionsResult.RejectedRefs` 返回计数，runner 上报）。理由：enricher 是无状态结构，无法直接写 artifacts。
  - `resolveGeneratedQuestions` 在无 `SourceChunkID`（无句柄）时完全透传（含 `[[r1]]` 文本），保证与现状一致——句柄机制只在显式提供来源时启用。
- **类型一致性**：`GeneratedQuestion`/`GenerateQuestionsResult`/`questionTexts` 在 Task 4 定义，Task 5 使用，签名一致。
- **不迁移 citation 包、不做跨 chunk 引用、不改 schema**（设计文档"明确不做"均未触碰）。
