# LLM 生成护栏库（llmgen）+ ingestion enricher 示范集成

> 2026-10-01 适用范围：历史设计保留：llmgen 护栏库仍可参考；旧 ingestion enricher 示范集成已经失效，当前文档增强由 DocumentProcessService 的 chunk 处理链路承接。

日期：2026-08-09
状态：已批准
来源借鉴：WeKnora `wiki_ingest_*` 系列的 W3/W4/W5 工程护栏（句柄防幻觉、确定性拒绝规则、纯文本后处理兜底）

## 背景

WeKnora 的 LLM 批量生成（wiki 页面、实体抽取、链接建立）沉淀了一套可泛化的工程护栏，共同点是"**不信任 LLM 输出中的 ID/引用**"：

1. **句柄防幻觉**：高熵 ID（UUID、chunk ID）不出现在提示词，替换为短句柄（`r1`/`c000`），模型只能原样复制；解码时 fail-closed。
2. **确定性拒绝规则**：模型输出中的引用必须命中候选集，否则用确定性规则拒绝（非 LLM 裁决），带明确拒绝原因。
3. **纯文本后处理兜底**：用精确的文本重写（跳过代码块/已有链接、词边界保护）做零 LLM 成本的链接/标记处理，可复用、可测试。

goagent 现有 LLM 生成点（ingestion enricher、会话摘要、偏好抽取、citation）已有部分精神（citation 的句柄表、偏好抽取的拒绝规则），但没有一套**独立、可复用的通用库**。

本设计：新建通用护栏库 `internal/framework/llmgen/`，并以 ingestion enricher 的 `GenerateQuestions` 作为首个示范集成点。

## 一、新库 `internal/framework/llmgen/`（纯 stdlib，无业务依赖）

### handle.go — 句柄编解码（防幻觉）

请求级句柄表：把高熵持久 ID 编码为短句柄喂给 LLM，输出解码 fail-closed。

```go
// HandleSet 把持久 ID 编码为请求级短句柄（如 r1/r2），用于喂给 LLM 的提示词。
type HandleSet struct {
    prefix   string
    byKey    map[string]string // 持久ID -> 句柄
    byHandle map[string]string // 句柄 -> 持久ID
    next     int
}

func NewHandleSet(prefix string) *HandleSet

// Encode 返回 durable 的句柄；重复 ID 复用同一句柄。
func (h *HandleSet) Encode(durable string) (string, bool)

// Resolve 解码句柄为持久 ID；未知句柄 fail-closed（false）。
func (h *HandleSet) Resolve(handle string) (string, bool)

// ResolveAll 批量解码，返回 (已解析, 未解析)。
func (h *HandleSet) ResolveAll(handles []string) (resolved, unresolved []string)
```

行为约定：
- `Encode` 空串返回 `("", false)`。
- `Resolve` 对句柄做大小写不敏感匹配（与 citation 包一致）。
- 句柄格式 `prefix + 序号`（`r1`、`r2`...）。检测"模型回传的句柄形 ID"不在此库职责（调用方决定是否允许 echo 已有句柄）。

### validate.go — 确定性拒绝规则

校验 LLM 结构化输出中的引用是否命中候选集，拒绝原因确定且可审计。

```go
type RejectReason string

const (
    ReasonOK                RejectReason = ""
    ReasonNotInCandidateSet RejectReason = "reference_not_in_candidate_set"
    ReasonMalformed         RejectReason = "malformed_reference"
    ReasonEmpty             RejectReason = "empty_reference"
)

type RefValidator struct{ allowed map[string]struct{} }

// NewRefValidator 从候选持久 ID 集合构造校验器（做 TrimSpace + 去空规范化）。
func NewRefValidator(candidates []string) *RefValidator

// Validate 返回 (规范ID, 拒绝原因)；ReasonOK 表示命中。
func (v *RefValidator) Validate(ref string) (string, RejectReason)

type RefResult struct {
    Ref    string       // 输入引用（原样）
    ID     string       // 解析出的规范 ID（未命中时为空）
    Reason RejectReason // 拒绝原因
}

func (v *RefValidator) ValidateMany(refs []string) []RefResult
```

判定规则（确定性，全部基于输入与候选集，无 LLM）：
- 空/全空白 → `ReasonEmpty`
- 无法匹配候选集 → `ReasonNotInCandidateSet`
- 可选：调用方提供合法性谓词时用于 `ReasonMalformed`（默认 `Validate` 只做候选集判定；`ReasonMalformed` 留给需要格式校验的调用方扩展）。

### rewrite.go — 纯文本后处理（兜底）

在 markdown 风格文本中精确重写标记/链接，跳过代码块与已有链接，词边界保护；以及死引用清理。

```go
type RewriteRule struct {
    Find    string // 要替换的标记，如 "[[r1]]"
    Replace string // 替换为，如 "[标题](wiki://slug)" 或 ""（剥离）
}

type RewriteOptions struct {
    SkipCodeBlocks bool // 跳过 ``` 围栏代码块与 `inline code`
    SkipLinks      bool // 跳过已有 markdown 链接 [label](url)
    WordBoundary   bool // 词边界保护（Find 两侧不能是词字符）
}

type RewriteStats struct {
    Rewritten int
    Skipped   int // 因 forbidden span 被跳过的命中数
}

// RewriteRefs 按规则重写 text 中出现的 Find，返回新文本与统计。
func RewriteRefs(text string, rules []RewriteRule, opts RewriteOptions) (string, RewriteStats)

// CleanDeadRefs 把文本中匹配 refPattern 的引用按 keep 谓词清理：
// keep(id)=false 的引用被移除；返回清理计数。用于删除/失效后的死引用兜底。
func CleanDeadRefs(text string, keep func(id string) bool, refPattern *regexp.Regexp) (string, int)
```

行为约定：
- 同一个 `Find` 命中后不二次处理（避免重叠替换）。
- 多个规则按顺序应用，后规则不处理前规则产出物（调用方可据此设计管道）。
- `SkipLinks` 用最小化的 markdown 链接识别（`[..](..)` 括号匹配），不做完整 parser（YAGNI）。

## 二、enricher 示范集成

### 现状

`internal/app/ingestion/service/runner/llm_enrichment.go`：
```go
type DocumentEnricher interface {
    Summarize(context.Context, string, EnrichmentOptions) (string, error)
    GenerateQuestions(context.Context, string, string, EnrichmentOptions) ([]string, error)
}
```
`GenerateQuestions` 返回纯字符串；模型若在问题里输出 `[[rN]]` 引用标记会被原样存入 `Chunk.Questions`（当前 prompt 不含引用语法，风险潜伏；一旦启用就会污染）。

落库路径：`runner_indexer.go` 把每个问题写为 `record_type=question` + `source_chunk_id=<所属chunk>`。

### 改动

**1. 新增返回类型 + 修改 `GenerateQuestions` 签名**（`llm_enrichment.go`）：

```go
type GeneratedQuestion struct {
    Text          string // 剥除引用标记后的干净问题文本
    SourceChunkID string // 校验后解析出的来源 chunk ID
}

type DocumentEnricher interface {
    Summarize(context.Context, string, EnrichmentOptions) (string, error)
    GenerateQuestions(context.Context, string, string, EnrichmentOptions) ([]GeneratedQuestion, error)
}
```

`GenerateQuestions` 内部：
- 用 `llmgen.NewHandleSet("r")` 把调用方传入的 `options.SourceChunkID` 编码为 `r1`（`EncodeOptions.SourceChunkID`，见下）。
- prompt 追加说明："当前内容块句柄为 r1，若问题依赖本段内容，以 `[[r1]]` 结尾。"（`[[..]]` 语法仅在本方法内约定）
- 解析模型输出：逐行 `normalizeGeneratedQuestions`（去重/限长/去空，逻辑保留）后，用 `regexp` 提取行尾 `[[rN]]`；`HandleSet.ResolveAll` + `RefValidator.ValidateMany` 判定；`RewriteRefs(SkipLinks, WordBoundary)` 剥离标记。
- 每个问题返回 `{Text, SourceChunkID}`：引用命中 → 该 chunk ID；无引用或未命中 → 空串（调用方沿用所属 chunk 默认）。

**2. `EnrichmentOptions` 增加来源字段**（`llm_enrichment.go`）：

```go
type EnrichmentOptions struct {
    QuestionCount     int
    MaxQuestionLength int
    SummaryMaxChars   int
    SourceChunkID     string // 生成问题对应的 chunk ID（用于句柄编码与校验）
}
```

**3. runner 适配**（`runner_enhancer.go`）：

```go
if includesEnrichmentTask(tasks, "questions") {
    for index := range next.Chunks {
        chunkOptions := options
        chunkOptions.SourceChunkID = next.Chunks[index].ID
        questions, err := r.documentEnricher.GenerateQuestions(ctx, next.Parsed.Title, next.Chunks[index].Content, chunkOptions)
        if err != nil { llmDegraded = true; continue }
        next.Chunks[index].Questions = toQuestionTexts(questions) // []string，取自 Text
        // 幻觉引用（SourceChunkID 空但模型标了未解析句柄）由 enricher 内部记录并剥离，不落库
    }
}
```

`Chunk.Questions` 仍为 `[]string`（indexer 落库路径不变）；`SourceChunkID` 当前恒等于所属 chunk，`source_chunk_id` 写库行为不变。**功能向后兼容**：无引用语法时输出与现在一致。

**4. observability**：enricher 内部对 `ReasonNotInCandidateSet`/`ReasonMalformed` 的拒绝计数通过 `next.Artifacts["enhancer"]` 上报（`rejectedQuestionRefs`），便于排查模型幻觉。

### 兼容性
- `DocumentEnricher` 接口签名变化 → 同步更新：`llm_enrichment_test.go`、`runner_enhancer_test.go` 中 stub/断言。
- `EnrichmentOptions` 加字段为纯增量。
- 不改数据库、不改 indexer、不改 workflow 状态结构。

## 三、测试策略

### llmgen 包单测
- **handle**：Encode 去重（同一 ID 恒定句柄）、Resolve 往返、未知句柄 false、大小写不敏感、空 ID 拒绝、ResolveAll 分组。
- **validate**：命中/`ReasonEmpty`/`ReasonNotInCandidateSet`；`ValidateMany` 结果分组；候选集规范化（TrimSpace/去空）。
- **rewrite**：普通替换；SkipCodeBlocks 跳过围栏代码块与 inline code；SkipLinks 跳过 `[..](..)`；WordBoundary 保护（`[[r1]]` 贴词不误替换）；多重规则顺序；`CleanDeadRefs` 移除与保留。
- **集成**：RewriteRefs 先 strip 再 CleanDeadRefs 的管道组合。

### enricher 测试
- 合法 `[[r1]]` 被剥离，`Text` 干净、`SourceChunkID` = chunk ID。
- 幻觉 `[[r2]]` 被拒：`Text` 剥离、`SourceChunkID` 空、拒绝计数上报。
- 无引用问题：行为与现状一致。
- LLM 失败：降级路径不变（`llmDegraded`）。

### 回归
- `go test ./internal/app/ingestion/... ./internal/framework/... -count=1`
- `go build ./cmd/... ./internal/...`

## 四、明确不做（YAGNI）

- 不迁移/重构 citation 包（避免动已合并的 A1；两处句柄实现暂并存，未来可统一）。
- 不做跨 chunk 引用生成（`SourceChunkID` 仍为所属 chunk；管线已就绪，未来可启用）。
- 不做 W3"规划先行"批量编排框架。
- 不引入完整 Markdown parser（`RewriteRefs` 用最小化识别）。
- 不改数据库 schema。

## 回归约束

- `DocumentEnricher` 接口变化是 enricher 模块内部，外部调用方仅 runner。
- `Chunk.Questions` 类型与 indexer 落库路径不动。
- llmgen 库纯 stdlib、位于 `internal/framework/`（最底层，无业务依赖），供未来任何 LLM 生成点复用。
