# GoAgent 结构与功能收口改进计划（弱模型执行版）

> 更新时间：2026-06-11  
> 基于：架构评估结论 + `docs/functional_improvement_todo_20260609.md`  
> 目标读者：能力较弱的代码模型、初级开发者、需要按步骤施工的执行者。  
> 当前明确约束：S1 `/health + /ready` 与 S2 Embedding 缓存暂缓，不进入本轮执行。
> 校准说明：本文已按 `2026-06-11` 仓库实际状态回写。部分章节保留原始施工步骤，供回顾和补测使用，但**不要把已完成任务重新当作待施工 backlog**。
> Scope 更新：自 `2026-06-11` 起，**新 Agent Runtime 不再承担文档诊断、trace 诊断和根因分析链路**。本计划中的 runtime 收口目标，限定为外部检索、网页抓取、外部证据整合，以及与之直接相关的审批/恢复、事件流、上下文交接能力。

---

## 0. 执行总原则

这份文档不是方向讨论稿，而是施工手册。执行者必须按任务拆分逐项完成，不允许把多个大任务混在一次改动里。

### 0.1 必须遵守

1. **一次只做一个任务编号**  
   例如只做 `P0-1`，不要顺手做 `P0-2`、`P1-3`。

2. **每个任务完成后必须跑指定测试**  
   如果测试失败，只修本任务相关文件，不要大范围重构。

3. **不允许改 S1/S2**
   - 不新增 `/health`。
   - 不新增 `/ready`。
   - 不把 `rag.memory.cache.enabled` 改成 `true`。

4. **不允许提前删除旧 `internal/app/rag/tool/`**
   只有完成 `P0-6` 等价矩阵、`P0-4` 观测和灰度条件后，才能删除旧 ToolWorkflow。

5. **不要在同一任务中做无关格式化**
   不要因为改一个函数就格式化全仓库。

6. **保持接口兼容优先**
   对已有接口优先新增字段或新增函数。不要轻易改方法签名，除非任务明确要求。

7. **文档与代码同步**
   如果新增了评估样本、治理规则或矩阵文档，必须在对应任务里说明如何运行和如何验收。

### 0.2 执行前必须先看这些文件

```text
docs/functional_improvement_todo_20260609.md
docs/structural_improvement_plan.md
configs/application.yaml
internal/framework/config/config.go
internal/bootstrap/rag/runtime.go
internal/app/rag/core/rewrite/rewrite.go
internal/app/rag/core/rewrite/llm_rewrite_service.go
internal/app/rag/core/rewrite/term_normalizer.go
internal/app/rag/core/retrieve/service.go
internal/app/rag/evaluation/evaluation.go
cmd/retrieve-eval/main.go
internal/app/rag/service/rag_chat_service.go
internal/app/rag/service/rag_chat_execute.go
internal/app/rag/service/rag_chat_agent_policy.go
```

### 0.3 推荐执行命令

PowerShell 下推荐使用：

```powershell
go test ./internal/app/rag/core/rewrite -count=1
go test ./internal/app/rag/core/retrieve -count=1
go test ./internal/app/rag/evaluation ./cmd/retrieve-eval -count=1
go test ./internal/app/rag/service -count=1
go test ./internal/bootstrap/rag -count=1
go test ./... -count=1
```

如果环境需要本地缓存，可以使用：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'
$env:GOPATH='D:\goagent\.gopath'
$env:GOMODCACHE='D:\goagent\.gomodcache'
go test ./internal/app/rag/core/rewrite -count=1
```

### 0.4 2026-06-11 状态校准

本次校准后，这份文档应按以下方式理解：

- 已完成：`P0-1`、`P0-2`、`P1-1`、`P1-3`
- 部分完成：`P0-3`、`P0-5`、`P0-6`、`P0-4`、`P1-2`、`P1-4`
- 尚未开始或仍应视为后续项：`P1-5`、`P1-6`、`P1-7`、`P2-*`

当前与仓库状态相符的剩余重点，不再是“把基础结构搭起来”，而是：

- 按新的 runtime scope 收敛 `P0-6`，只保留外部检索 / 外部证据主线相关的 parity 阻塞项
- 推进 `P0-4` 的阶段 3/4，但前提是先满足新的 parity matrix 阻塞项：SSE 事件兼容、审批/恢复持久化、生产默认路径收口
- 收尾 `P0-5` 第二阶段和 `P1-2` 的结构整理，继续减少隐式装配
- 继续 `P0-3`、`P1-6` 一类评估闭环工作，把“能力存在”推进到“质量可证明”

---

## 1. 本轮暂缓项

### D1：暂缓 `/health + /ready`

本轮不做。

禁止事项：

- 不要新增 `internal/adapter/http/health`。
- 不要在 `cmd/server/main.go` 注册 `/health` 或 `/ready`。
- 不要声称已经完成 readiness。

后续再做时至少要检查：

```text
DB
Redis
schema migration 状态
MCP manager 状态
ingestion worker 状态
metrics/health 是否绕过业务鉴权
```

### D2：暂缓直接开启 Embedding 缓存

本轮不做。

禁止事项：

- 不要把 `configs/application.yaml` 里的 `rag.memory.cache.enabled` 改成 `true`。
- 不要把 memory recall cache 表述为主检索 embedding cache。

后续再做时，应先实现统一的 `CachedEmbeddingProvider`，覆盖：

```text
vector_global query embedding
session recall embedding
long-term memory recall embedding
rewrite 后 query embedding
```

---

## 2. 改进项总表

| 编号 | 标题 | 阶段 | 优先级 | 当前状态 | 施工方式 |
|------|------|------|--------|----------|----------|
| P0-1 | Query Rewrite 术语治理收口 | 1 周内 | P0 | done | 小步代码 + 测试 + 文档 |
| P0-2 | Rewrite 强约束校验与回退 | 1 周内 | P0 | done | 小步代码 + 测试 |
| P0-3 | Retrieve 评估闭环固化 | 1 周内 | P0 | partial | 样本 + 命令 + 报告 |
| P0-5 | RagChatService 依赖注入收敛 | 1 周内 | P0 | partial | 先新增构造方式，不急删 setter |
| P0-6 | 新 runtime scope 下的 Agent Capability 等价矩阵 | 1 周内 | P0 | partial | 文档 + 测试清单 |
| P0-4 | 双 Agent Runtime 收口（限外部证据主线） | 2-4 周 | P0 | partial | 先观测，再灰度，最后删除 |
| P1-1 | 检索通道并行化 | 1-2 周 | P1 | done | 小步代码 + 顺序稳定测试 |
| P1-2 | bootstrap 函数拆分 | 1-2 周 | P1 | partial | 纯重构，不改行为 |
| P1-3 | log.FromContext 与 trace 字段收口 | 1-2 周 | P1 | done | 新增能力，渐进替换 |
| P1-4 | Token-aware 会话窗口与预算 | 1 个月内 | P1 | partial | 先估算与裁剪，再 usage 持久化 |
| P1-5 | Summary 异步化与生命周期升级 | 1 个月内 | P1 | pending | 先设计迁移，再实现 worker |
| P1-6 | Answer 层评估与引用证据命中率 | 1 个月内 | P1 | pending | 先评估格式，再接生成链路 |
| P1-7 | Prometheus metrics 接入 | 1 个月内 | P1 | pending | 低基数指标优先 |
| P2-1 | 混合检索通道贡献分析 | 中期 | P2 | pending | 基于 eval 输出扩展 |
| P2-2 | SubQuestion 策略升级 | 中期 | P2 | pending | 限制数量、记录串并原因 |
| P2-3 | State clone/merge 自动化测试 | 中期 | P2 | pending | 测试优先 |
| P2-4 | Agent/Tool 任务级评估 | 中期 | P2 | pending | 任务集 + 指标 |

---

## 3. P0-1：Query Rewrite 术语治理收口

### 3.0 状态校准（2026-06-11）

该任务已完成，不应再按本节步骤重复施工。

当前已知落地产物：

- `rewrite.Result.Metadata` 已存在
- `TermNormalizer.NormalizeTextWithReport(...)` 已存在
- `termNormalization` 命中信息已写入 metadata
- `docs/rewrite_governance.md` 已存在

本节后续内容保留为实现回顾和补测参考。

### 3.1 目标

把当前轻量术语归一化从“字符串替换工具”升级为“可治理、可观测、可测试”的 rewrite 子能力。

完成后必须能回答：

```text
术语规则在哪里配置？
哪些 alias 命中了？
命中了哪个 canonical？
规则是否支持禁用？
规则是否支持分类和版本？
命中信息是否能进入 rewrite result 的 metadata？
```

### 3.2 当前代码事实

已存在文件：

```text
internal/app/rag/core/rewrite/rewrite.go
internal/app/rag/core/rewrite/llm_rewrite_service.go
internal/app/rag/core/rewrite/term_normalizer.go
internal/app/rag/core/rewrite/term_normalizer_test.go
internal/framework/config/config.go
internal/bootstrap/rag/runtime.go
configs/application.yaml
```

当前 `rewrite.Result` 只有：

```go
type Result struct {
	RewrittenQuestion string
	SubQuestions      []string
	NeedRetrieval     bool
}
```

当前 `TermNormalizationRule` 只有：

```go
type TermNormalizationRule struct {
	Canonical string
	Aliases   []string
}
```

### 3.3 禁止事项

- 不要改变 `Service` 接口的方法签名。
- 不要把 `rewrite` 包反向依赖 `internal/app/rag/tool`。
- 不要把术语词典写死在 Go 代码里。
- 不要删除已有 `term_normalizer` 逻辑。
- 不要把术语归一化做成 LLM prompt 的一部分。术语治理必须是确定性规则。

### 3.4 需要修改的文件

```text
internal/app/rag/core/rewrite/rewrite.go
internal/app/rag/core/rewrite/term_normalizer.go
internal/app/rag/core/rewrite/term_normalizer_test.go
internal/framework/config/config.go
internal/bootstrap/rag/runtime.go
configs/application.yaml
docs/rewrite_governance.md                       新增
```

### 3.5 具体步骤

#### 步骤 1：给 `rewrite.Result` 增加 metadata

文件：`internal/app/rag/core/rewrite/rewrite.go`

把 `Result` 扩展为：

```go
type Result struct {
	RewrittenQuestion string
	SubQuestions      []string
	NeedRetrieval     bool
	Metadata          map[string]any
}
```

注意：

- 只新增字段，不改已有字段名。
- 已有代码使用 composite literal 时可以不填 `Metadata`，不会编译失败。
- 后续 trace 可以从 `Metadata` 读取治理信息。

#### 步骤 2：扩展术语规则结构

文件：`internal/app/rag/core/rewrite/term_normalizer.go`

把规则结构扩展为：

```go
type TermNormalizationRule struct {
	Canonical string
	Aliases   []string
	Category  string
	Version   int
	Enabled   *bool
}
```

新增命中结构：

```go
type TermNormalizationMatch struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Category  string `json:"category,omitempty"`
	Version   int    `json:"version,omitempty"`
}

type TermNormalizationReport struct {
	Changed bool                     `json:"changed"`
	Matches []TermNormalizationMatch `json:"matches,omitempty"`
}
```

内部 entry 扩展为：

```go
type termNormalizationEntry struct {
	canonical string
	alias     string
	category  string
	version   int
	pattern   *regexp.Regexp
}
```

规则处理要求：

- `Enabled == nil` 时默认启用。
- `Enabled != nil && *Enabled == false` 时跳过。
- `Canonical` 为空跳过。
- `Aliases` 为空跳过。
- `Version <= 0` 时可以保留 0，不要报错。

#### 步骤 3：新增带报告的归一化方法

文件：`internal/app/rag/core/rewrite/term_normalizer.go`

新增：

```go
func (n *TermNormalizer) NormalizeTextWithReport(text string) (string, TermNormalizationReport)
```

行为要求：

- 返回归一化后的文本。
- 如果没有命中，`Changed=false`。
- 如果命中，`Changed=true`，`Matches` 记录 alias、canonical、category、version。
- 多次命中同一 alias/canonical 可以只记录一次。
- 替换顺序继续保持“长 alias 优先”。

`NormalizeText(text)` 改为调用 `NormalizeTextWithReport(text)`，只返回文本。

#### 步骤 4：把命中报告写入 `Result.Metadata`

文件：`internal/app/rag/core/rewrite/term_normalizer.go`

`Apply(result Result) Result` 内部应：

1. 对 `RewrittenQuestion` 调用 `NormalizeTextWithReport`。
2. 对每个 `SubQuestions` 调用 `NormalizeTextWithReport`。
3. 合并所有 matches。
4. 返回新 `Result` 时保留原 `Metadata`，并追加：

```go
Metadata: map[string]any{
	"termNormalization": TermNormalizationReport{...},
}
```

不要覆盖已有 metadata。建议增加 helper：

```go
func cloneMetadata(metadata map[string]any) map[string]any
```

#### 步骤 5：扩展配置结构

文件：`internal/framework/config/config.go`

把：

```go
type RagTermNormalizationRule struct {
	Canonical string   `mapstructure:"canonical"`
	Aliases   []string `mapstructure:"aliases"`
}
```

改成：

```go
type RagTermNormalizationRule struct {
	Canonical string   `mapstructure:"canonical"`
	Aliases   []string `mapstructure:"aliases"`
	Category  string   `mapstructure:"category"`
	Version   int      `mapstructure:"version"`
	Enabled   *bool    `mapstructure:"enabled"`
}
```

#### 步骤 6：同步 runtime 转换函数

文件：`internal/bootstrap/rag/runtime.go`

修改 `buildTermNormalizationRules(...)`，把 `Category`、`Version`、`Enabled` 传过去。

必须保留：

```go
Canonical: strings.TrimSpace(rule.Canonical)
Aliases:   append([]string(nil), rule.Aliases...)
```

新增：

```go
Category: strings.TrimSpace(rule.Category)
Version:  rule.Version
Enabled:  rule.Enabled
```

#### 步骤 7：扩展 YAML 示例

文件：`configs/application.yaml`

在已有 `rag.query-rewrite.term-normalization.rules` 下，允许规则包含：

```yaml
- canonical: PostgreSQL
  category: component
  version: 1
  enabled: true
  aliases:
    - postgres
    - pg
```

注意：

- 不要大规模改配置。
- 只在已有规则旁补充字段即可。
- 不要改 `rag.memory.cache.enabled`。

#### 步骤 8：新增治理文档

新增文件：`docs/rewrite_governance.md`

必须包含：

```text
术语规则维护位置
字段说明：canonical / aliases / category / version / enabled
发布流程：改 YAML -> 测试 -> 上线
回滚方式：enabled=false 或恢复旧配置
命中观测：rewrite.Result.Metadata.termNormalization
边界：术语归一化只做确定性替换，不新增语义
```

### 3.6 测试要求

修改或新增：`internal/app/rag/core/rewrite/term_normalizer_test.go`

至少增加这些测试：

```text
TestTermNormalizerReportsMatches
TestTermNormalizerSkipsDisabledRules
TestTermNormalizerPreservesMetadata
TestTermNormalizerLongestAliasFirst
```

测试点：

- alias 被替换成 canonical。
- report 中包含 alias、canonical、category、version。
- disabled rule 不生效。
- 原有 `Metadata` 不丢。
- 长 alias 比短 alias 先匹配。

运行：

```powershell
go test ./internal/app/rag/core/rewrite -count=1
```

### 3.7 完成标准

满足以下条件才算完成：

```text
go test ./internal/app/rag/core/rewrite -count=1 通过
rewrite.Result 有 Metadata 字段
term normalizer 能输出命中报告
配置结构支持 category/version/enabled
docs/rewrite_governance.md 存在
没有修改 S1/S2
```

---

## 4. P0-2：Rewrite 强约束校验与回退规则

### 4.0 状态校准（2026-06-11）

该任务已完成，不应再按本节步骤重复施工。

当前已知落地产物：

- `internal/app/rag/core/rewrite/constraint_guard.go` 已存在
- `rewriteValidation` 已写入 `rewrite.Result.Metadata`
- LLM rewrite 已接入 guard/fallback 路径

本节后续内容保留为实现回顾和补测参考。

### 4.1 目标

让 LLM rewrite 不能随意丢失用户问题中的硬约束。硬约束包括：

```text
错误码
HTTP 状态码
文档 ID
任务 ID
trace ID
资源名
反引号或引号中的精确词
时间范围
用户显式限制词
```

如果模型改写结果丢失硬约束，应拒绝模型结果，回退原 query，并记录拒绝原因。

### 4.2 当前代码事实

LLM rewrite 入口：

```text
internal/app/rag/core/rewrite/llm_rewrite_service.go
```

核心函数：

```go
func (s *LLMService) RewriteWithSplit(question string) Result
func (s *LLMService) RewriteWithHistory(question string, history []convention.ChatMessage) Result
func parseRewriteResponse(raw string) Result
func fallbackResult(question string) Result
```

### 4.3 禁止事项

- 不要把校验逻辑写进 prompt 里就算完成。
- 不要让校验依赖 LLM。
- 不要依赖旧 `rag/tool` 包里的 ID regex。
- 不要因为校验失败就返回空结果。必须回退原 query。

### 4.4 需要修改的文件

```text
internal/app/rag/core/rewrite/constraint_guard.go       新增
internal/app/rag/core/rewrite/constraint_guard_test.go  新增
internal/app/rag/core/rewrite/llm_rewrite_service.go
internal/app/rag/core/rewrite/rewrite.go
```

### 4.5 具体步骤

#### 步骤 1：新增约束类型

新增文件：`internal/app/rag/core/rewrite/constraint_guard.go`

建议类型：

```go
type RewriteConstraint struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type RewriteValidationReport struct {
	Accepted           bool                `json:"accepted"`
	Reasons            []string            `json:"reasons,omitempty"`
	OriginalConstraints []RewriteConstraint `json:"originalConstraints,omitempty"`
	MissingConstraints  []RewriteConstraint `json:"missingConstraints,omitempty"`
}
```

约束类型建议使用常量：

```go
const (
	ConstraintErrorCode = "error_code"
	ConstraintHTTPCode  = "http_code"
	ConstraintID        = "id"
	ConstraintQuoted    = "quoted"
	ConstraintTimeRange = "time_range"
	ConstraintLimit     = "limit"
)
```

#### 步骤 2：实现约束抽取

新增函数：

```go
func ExtractRewriteConstraints(question string) []RewriteConstraint
```

最小实现即可，必须覆盖：

1. 反引号、单引号、双引号中的内容：

```text
`task_run_01`
"vector store"
'trace_bad_01'
```

2. 常见 ID：

```text
doc_fail_01
task_run_01
trace_bad_01
kb_123
```

可以用本地 regexp，不要 import `rag/tool`：

```go
regexp.MustCompile(`(?i)\b(?:doc|task|trace|kb)[-_][a-z0-9][a-z0-9_-]*\b`)
```

3. HTTP 状态码：

```go
regexp.MustCompile(`(?i)\b(?:http\s*)?[1-5][0-9]{2}\b`)
```

4. 错误码：

```go
regexp.MustCompile(`\b[A-Z][A-Z0-9_-]{1,20}[0-9][A-Z0-9_-]*\b`)
```

5. 时间和限制词：

```text
最近一次
昨天
今天
近 7 天
last run
latest
only
不要查外部
只看
```

#### 步骤 3：实现校验

新增函数：

```go
func GuardRewriteResult(original string, result Result) (Result, RewriteValidationReport)
```

规则：

- 从 `original` 抽取约束。
- 把 `result.RewrittenQuestion` 和 `result.SubQuestions` 拼起来作为 candidate。
- 原始约束必须出现在 candidate 中。
- 如果没有原始约束，直接 accepted。
- 如果缺失约束，返回 `fallbackResult(original)`，并在 metadata 中记录 rejected report。

注意大小写：

- 英文 ID 和错误码比较时可以 case-insensitive。
- 中文限制词直接 `strings.Contains`。

#### 步骤 4：接入 LLM rewrite

文件：`internal/app/rag/core/rewrite/llm_rewrite_service.go`

在 `RewriteWithSplit` 中：

```go
parsed, err := s.callRewriteLLM(...)
if err != nil {
	return fallbackResult(question)
}
guarded, _ := GuardRewriteResult(question, parsed)
return guarded
```

在 `RewriteWithHistory` 中做同样处理。

不要在 `parseRewriteResponse` 内部调用 guard，因为 `parseRewriteResponse` 只有 raw response，没有 original question。

#### 步骤 5：metadata 合并

如果 `P0-1` 已经给 `Result` 增加 `Metadata`，则 `GuardRewriteResult` 必须写入：

```go
Metadata["rewriteValidation"] = RewriteValidationReport{...}
```

如果 `P0-1` 还没做，先完成 `P0-1`，不要跳过 metadata。

### 4.6 测试要求

新增文件：`internal/app/rag/core/rewrite/constraint_guard_test.go`

至少覆盖：

```text
TestExtractRewriteConstraintsFindsIDsAndCodes
TestGuardRewriteResultAcceptsWhenConstraintsPreserved
TestGuardRewriteResultRejectsWhenIDDropped
TestGuardRewriteResultRejectsWhenHTTPCodeDropped
TestGuardRewriteResultKeepsSmallTalkWithoutConstraints
```

样例：

```go
original := "帮我排查 doc_fail_01 最近一次 500 错误"
result := Result{
	RewrittenQuestion: "排查导入失败原因",
	SubQuestions: []string{"导入失败原因"},
	NeedRetrieval: true,
}
guarded, report := GuardRewriteResult(original, result)
// report.Accepted == false
// guarded.RewrittenQuestion == original
```

运行：

```powershell
go test ./internal/app/rag/core/rewrite -count=1
```

### 4.7 完成标准

```text
约束抽取有独立测试
guard 接入 RewriteWithSplit / RewriteWithHistory
校验失败回退原 query
metadata 记录 rewriteValidation
go test ./internal/app/rag/core/rewrite -count=1 通过
```

---

## 5. P0-3：Retrieve 评估闭环固化

### 5.1 目标

把已有 retrieve eval 从“工具存在”升级为“稳定可重复的评估闭环”。

完成后必须能回答：

```text
评估样本在哪里？
怎么跑 baseline？
怎么跑 candidate？
指标有哪些？
每类问题提升还是退化？
失败样本怎么定位？
```

### 5.2 当前代码事实

已有模块：

```text
internal/app/rag/evaluation/evaluation.go
cmd/retrieve-eval/main.go
cmd/retrieve-inspect/main.go
testdata/retrieve_eval_samples.json
testdata/memory_fact_phase3_samples.json
```

`cmd/retrieve-eval` 支持：

```text
-input
-k
-execute
-config-dir
-json
-output
-rerank-model
-vector-topk-multiplier
-search-mode
```

已支持指标：

```text
Hit@K
Recall@K
NDCG@K
MRR
ByTag
```

### 5.3 禁止事项

- 不要重新实现一个新的 eval 框架。
- 不要删除已有 `cmd/retrieve-eval`。
- 不要把人工评估写成不可重复的临时脚本。
- 不要只给一两个样本就声称完成。

### 5.4 需要修改或新增的文件

```text
testdata/retrieve_eval_samples.json               扩充或新增同格式样本
docs/retrieve_eval_plan.md                        新增
docs/retrieve_eval_report_template.md             新增
internal/app/rag/evaluation/evaluation.go         仅在需要新增字段时修改
cmd/retrieve-eval/main.go                         尽量不改，除非必须
```

### 5.5 样本格式要求

样本可以使用已有格式：

```json
{
  "samples": [
    {
      "name": "diagnosis_latest_task_failure",
      "query": "最近一次导入失败是什么原因",
      "userId": "eval-user",
      "tags": ["diagnosis", "colloquial", "rewrite"],
      "target": "chunk",
      "expectedIds": ["chunk_001"],
      "retrieved": [],
      "knowledgeBaseIds": ["kb_eval"],
      "searchMode": "hybrid",
      "topK": 5
    }
  ]
}
```

标签必须规范，建议固定这些：

```text
colloquial
alias
abbreviation
coreference
multi_condition
diagnosis
metadata
keyword
semantic
memory_fact
```

### 5.6 具体步骤

#### 步骤 1：整理最小评测集

优先在 `testdata/retrieve_eval_samples.json` 扩充，不要另起一堆名字混乱的文件。

最小要求：

```text
总样本数 >= 20
alias 类 >= 4
diagnosis 类 >= 4
metadata 类 >= 4
coreference 类 >= 2
multi_condition 类 >= 2
keyword 类 >= 2
semantic 类 >= 2
```

如果没有真实 chunk id，不要乱填。可以先新增 `docs/retrieve_eval_plan.md` 标明“待标注”，但不能把没有 expectedIds 的样本放进可执行 eval 文件，因为当前 evaluator 要求 `expectedIds` 非空。

#### 步骤 2：跑离线评估

如果样本已经包含 `retrieved` 字段，可直接跑：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -k 1,3,5 -json -output testdata/retrieve_eval_summary.json
```

#### 步骤 3：跑真实检索评估

如果本地 DB、向量库、配置可用：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -config-dir configs -k 1,3,5 -json -output testdata/retrieve_eval_execute_summary.json
```

对比 search mode：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -search-mode semantic -json -output testdata/retrieve_eval_semantic.json
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -search-mode keyword -json -output testdata/retrieve_eval_keyword.json
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -execute -search-mode hybrid -json -output testdata/retrieve_eval_hybrid.json
```

#### 步骤 4：写评估计划文档

新增 `docs/retrieve_eval_plan.md`，必须包含：

```text
样本文件路径
样本标签定义
baseline 命令
candidate 命令
指标说明
如何判断退化
如何追加样本
```

#### 步骤 5：写报告模板

新增 `docs/retrieve_eval_report_template.md`，包含：

```text
评估日期
代码版本/commit
配置摘要
样本总数
Overall Hit@1/3/5
Overall Recall@1/3/5
Overall NDCG@1/3/5
MRR
ByTag 指标
退化样本列表
结论
下一步动作
```

### 5.7 测试要求

运行：

```powershell
go test ./internal/app/rag/evaluation ./cmd/retrieve-eval -count=1
```

如果修改了样本 JSON，还要至少跑一次：

```powershell
go run ./cmd/retrieve-eval -input testdata/retrieve_eval_samples.json -k 1,3,5
```

### 5.8 完成标准

```text
testdata/retrieve_eval_samples.json 可被 cmd/retrieve-eval 读取
docs/retrieve_eval_plan.md 存在
docs/retrieve_eval_report_template.md 存在
go test ./internal/app/rag/evaluation ./cmd/retrieve-eval -count=1 通过
能输出 Hit@K / Recall@K / NDCG@K / MRR
```

---

## 6. P0-5：RagChatService 依赖注入收敛

### 6.0 状态校准（2026-06-11）

该任务目前应视为 `partial`，其中第一阶段已落地，第二阶段仍待收尾。

当前已知状态：

- `NewRagChatServiceWithDeps(...)` 已存在
- bootstrap 已切到 `NewRagChatServiceWithDeps(...)`
- 测试辅助和主测试已出现新构造方式
- setter 和旧字段暂未全部删除，仍需结合 `P0-4` 节奏继续收口

因此本节后续步骤中，“第一阶段”主要作为已完成实现记录；真正剩余的是“第二阶段”和相关测试清理。

### 6.1 目标

把 `RagChatService` 从“构造函数 + 一堆 setter 的隐式约定”收敛为“构造时注入 + 构造时校验”。

弱模型执行时必须分两阶段，不能一步到位删除所有 setter。

### 6.2 当前代码事实

相关文件：

```text
internal/app/rag/service/rag_chat_service.go
internal/app/rag/service/rag_chat_agent_stage.go
internal/app/rag/service/rag_chat_agent_policy.go
internal/bootstrap/rag/runtime.go
internal/app/rag/service/rag_chat_service_test.go
internal/app/rag/service/rag_chat_agent_stage_test.go
```

当前 constructor：

```go
func NewRagChatService(
	conversationService *ConversationService,
	messageService *ConversationMessageService,
	historyService raghistory.Service,
	rewriteService ragrewrite.Service,
	retrieveService ragretrieve.Service,
	promptService *ragprompt.Service,
	chatService aichat.LLMService,
	tracer *ChatTracer,
) *RagChatService
```

当前 setter 包括：

```text
SetConfidenceThreshold
SetToolWorkflow
SetSessionRecallService
SetRequestCacheMaxEntries
SetParallelSubquestionRetrieval
SetLongTermMemoryRecallService
SetAgentRuntimeMode
SetAgentRuntimeService
```

### 6.3 禁止事项

- 第一阶段不要删除 setter。
- 第一阶段不要改所有测试。
- 不要在这个任务删除 `toolWorkflow`。
- 不要把 `ToolWorkflow` 迁移到新 Agent，这属于 `P0-4/P0-6`。
- 不要让构造函数 panic，必须返回 error。

### 6.4 第一阶段：新增强构造函数

修改文件：

```text
internal/app/rag/service/rag_chat_service.go
internal/bootstrap/rag/runtime.go
internal/app/rag/service/rag_chat_service_test.go
```

#### 步骤 1：新增 deps/options 类型

文件：`internal/app/rag/service/rag_chat_service.go`

新增：

```go
type RagChatDeps struct {
	ConversationService *ConversationService
	MessageService      *ConversationMessageService
	HistoryService      raghistory.Service
	RewriteService      ragrewrite.Service
	RetrieveService     ragretrieve.Service
	PromptService       *ragprompt.Service
	ChatService         aichat.LLMService
	Tracer              *ChatTracer
	AgentRuntime        AgentRuntimeService
}

type RagChatOptions struct {
	ConfidenceThreshold    float64
	ParallelSubquestions   bool
	SubquestionConcurrency int
	RequestCacheMaxEntries int
	AgentRuntimeMode       string
	SessionRecall          SessionRecallService
	LongTermMemoryRecall   longtermmemory.RecallService
	ToolWorkflow           ragtool.Workflow
}
```

注意：

- `ToolWorkflow` 暂时保留，因为旧路径尚未删除。
- `AgentRuntime` 放在 deps，因为新主线需要它。
- `RewriteService` 可以为 nil，因为当前代码允许没有 rewrite 时 fallback，但如果希望强制也可以在 validate 中允许 nil。

#### 步骤 2：新增构造函数，不替换旧函数

新增：

```go
func NewRagChatServiceWithDeps(deps RagChatDeps, opts RagChatOptions) (*RagChatService, error)
```

行为：

1. 校验必需依赖：

```text
ConversationService
MessageService
HistoryService
RetrieveService
PromptService
ChatService
Tracer
```

2. 调用旧 `NewRagChatService(...)` 创建对象。
3. 把 opts/deps 中的选配能力直接赋值到字段。
4. 归一化默认值：

```text
RequestCacheMaxEntries <= 0 -> 128
SubquestionConcurrency <= 0 -> 2
AgentRuntimeMode -> normalizeAgentRuntimeMode
```

5. 返回 `(*RagChatService, error)`。

不要在第一阶段删除旧 `NewRagChatService(...)`。

#### 步骤 3：bootstrap 切到新构造函数

文件：`internal/bootstrap/rag/runtime.go`

当前逻辑是：

```go
chatService := ragservice.NewRagChatService(...)
chatService.SetConfidenceThreshold(...)
chatService.SetParallelSubquestionRetrieval(...)
chatService.SetAgentRuntimeMode(...)
chatService.SetLongTermMemoryRecallService(...)
chatService.SetSessionRecallService(...)
chatService.SetRequestCacheMaxEntries(...)
chatService.SetToolWorkflow(...)
chatService.SetAgentRuntimeService(...)
```

第一阶段改成：

1. 先构建 `sessionRecallService`。
2. 先构建 `toolWorkflow`。
3. 先构建 `agentRuntimeService`。
4. 最后调用：

```go
chatService, err := ragservice.NewRagChatServiceWithDeps(
	ragservice.RagChatDeps{
		ConversationService: conversationService,
		MessageService:      messageService,
		HistoryService:      historyService,
		RewriteService:      rewriteService,
		RetrieveService:     retrieveService,
		PromptService:       promptService,
		ChatService:         aiRuntime.Chat,
		Tracer:              tracer,
		AgentRuntime:        agentRuntimeService,
	},
	ragservice.RagChatOptions{
		ConfidenceThreshold:    cfg.Rag.Search.Channels.VectorGlobal.ConfidenceThreshold,
		ParallelSubquestions:   cfg.Rag.Retrieve.ParallelSubquestions.Enabled,
		SubquestionConcurrency: cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency,
		RequestCacheMaxEntries: readRequestCacheMaxEntries(cfg),
		AgentRuntimeMode:       cfg.Rag.Agent.Chat.Mode,
		SessionRecall:          sessionRecallService,
		LongTermMemoryRecall:   explicitMemoryService.RecallService(),
		ToolWorkflow:           toolWorkflow,
	},
)
if err != nil {
	// 保留 ownsDB 清理逻辑
}
```

注意：

- `toolWorkflow := ragassembly.BuildLocalWorkflow(...)` 暂时还在。
- `agentRuntimeService, err := buildAgentRuntimeService(...)` 暂时还在。
- 原来的 setter 调用应从 bootstrap 删除。

#### 步骤 4：新增构造函数测试

文件：`internal/app/rag/service/rag_chat_service_test.go`

至少新增：

```text
TestNewRagChatServiceWithDepsRejectsMissingRequiredDeps
TestNewRagChatServiceWithDepsAppliesOptions
```

### 6.5 第二阶段：删除 setter

只有第一阶段合入且测试稳定后才做。

第二阶段修改：

```text
删除 bootstrap 中剩余 setter
更新测试不再依赖 setter
删除 RagChatService 的 setter 方法
```

如果测试里大量使用 setter，可以先保留测试 helper：

```go
func newTestRagChatServiceWithOptions(...)
```

### 6.6 测试要求

第一阶段运行：

```powershell
go test ./internal/app/rag/service -count=1
go test ./internal/bootstrap/rag -count=1
```

第二阶段运行：

```powershell
go test ./internal/app/rag/service ./internal/bootstrap/rag -count=1
go test ./... -count=1
```

### 6.7 完成标准

第一阶段完成标准：

```text
NewRagChatServiceWithDeps 存在
bootstrap 使用 NewRagChatServiceWithDeps
bootstrap 不再调用 chatService.SetXxx
setter 方法仍可存在
service/bootstrap 测试通过
```

第二阶段完成标准：

```text
业务代码中没有 chatService.SetXxx
测试 helper 不再依赖 setter
setter 方法被删除
go test ./... 通过
```

---

## 7. P0-6：旧 Tool 与新 Agent Capability 等价矩阵

### 7.0 状态校准（2026-06-11）

该任务目前应视为 `partial`，但目标已经发生收缩。

当前已知产物：

- `docs/agent_tool_parity_matrix.md` 已存在
- `docs/agent_tool_parity_test_plan.md` 已存在

自 `2026-06-11` 起，这个任务**不再要求**新 runtime 覆盖文档诊断、trace 诊断和 graph 根因分析链路。

当前仍明确阻塞 runtime 收口的缺口，已经收敛为：

- 外部证据主链路的 capability / workflow / SSE 兼容性
- 新 runtime 的审批/恢复持久化
- 生产默认路径和灰度切换条件

因此本节接下来应聚焦补齐 **scope 内** 的 `partial/missing`，而不是重复生成矩阵文档，更不是继续把诊断链路当成硬前置条件。

### 7.1 目标

删除旧 `internal/app/rag/tool/` 前，必须证明新 `internal/app/agent/` 至少能覆盖 **当前 scope 内** 的旧工具能力。

scope 外的旧诊断能力，只要求明确标记为：

- legacy/frozen
- 或后续下线

这个任务优先产出文档和测试清单，不要求立刻删除代码。

### 7.2 禁止事项

- 不要删除 `internal/app/rag/tool/`。
- 不要删除 `BuildLocalWorkflow`。
- 不要修改工具行为来“凑矩阵”。
- 不要只写“已覆盖”，必须写证据。

### 7.3 需要新增文件

```text
docs/agent_tool_parity_matrix.md
docs/agent_tool_parity_test_plan.md
```

### 7.4 扫描命令

先扫描旧工具：

```powershell
rg -n "Name:|Definition\\(\\)|New.*Tool|Register" internal/app/rag/tool -S
```

再扫描新 Agent capability：

```powershell
rg -n "Capability|Register|Name\\(\\)|document|task|trace|evidence|search" internal/app/agent -S
```

### 7.5 矩阵格式

`docs/agent_tool_parity_matrix.md` 必须包含表格：

```text
旧工具名
旧文件
旧能力说明
新 capability/tool
新文件
参数兼容性
返回兼容性
evidence 字段
trace 字段
SSE 事件兼容
已有测试
缺口
结论：ready / partial / out-of-scope
```

模板：

```markdown
| 旧工具名 | 旧文件 | 新 capability | 新文件 | 参数兼容 | 返回兼容 | evidence | trace | 测试 | 结论 |
|----------|--------|---------------|--------|----------|----------|----------|-------|------|------|
| web_search | internal/app/rag/tool/... | web_search | internal/app/agent/... | partial | partial | 待确认 | 待确认 | 待补 | partial |
```

### 7.6 必须覆盖的旧能力

至少覆盖：

```text
web_search
web_fetch
external_evidence_workflow
SSE 事件兼容
审批/恢复持久化
生产默认路径相关入口
```

### 7.7 测试计划

`docs/agent_tool_parity_test_plan.md` 必须列出这些测试场景：

```text
外部证据链路：web_search -> web_fetch
外部证据工作流：web_search -> web_fetch -> external_evidence_collect
SSE 事件兼容：tool_start / tool_result / agent_think
审批恢复：approval pending -> resume
失败场景：工具返回 failed
超时场景：capability timeout
空结果场景：search/fetch 无结果
不可信结果场景：外部来源被 source policy 拒绝
```

### 7.8 完成标准

```text
docs/agent_tool_parity_matrix.md 存在
docs/agent_tool_parity_test_plan.md 存在
每个核心旧工具都有 ready/partial/missing 结论
missing/partial 项有后续任务
没有删除旧 tool 代码
```

---

## 8. P0-4：双 Agent Runtime 收口与旧 ToolWorkflow 退役

### 8.0 状态校准（2026-06-11）

该任务目前应视为 `partial`。

已落地部分：

- `chat_path` / `tool_backend` 观测代码已存在
- `UseAgentRuntime` 入口需要在执行前重新扫描确认：`rg -n "UseAgentRuntime" internal cmd -S`

尚未完成部分：

- 阶段 3 的默认入口灰度切换
- 阶段 4 的旧 ToolWorkflow 删除
- 与 `P0-6` 新 scope 阻塞项对齐后的最终退役验证

因此本节的实际剩余重点是阶段 3/4，不再是阶段 1/2。

### 8.1 目标

把 RAG Chat 从新旧两套 runtime 并存，收敛到新 Agent Runtime 为唯一主线。

这里的“唯一主线”在当前阶段仅指：

- 外部检索
- 网页抓取
- 外部证据整合

不再包含文档诊断、trace 诊断和根因分析链路。

### 8.2 当前代码事实

顶层分支：

```go
if input.UseAgentRuntime {
	return s.runAgentChat(ctx, input, sink)
}
```

文件：

```text
internal/app/rag/service/rag_chat_service.go
```

工具阶段分支：

```go
if s.shouldUseAgentRuntimeForToolStage(...) {
	...
}
return s.runLegacyToolWorkflowStage(...)
```

文件：

```text
internal/app/rag/service/rag_chat_execute.go
internal/app/rag/service/rag_chat_agent_policy.go
```

旧 workflow 构建位置：

```text
internal/bootstrap/rag/runtime.go
ragassembly.BuildLocalWorkflow(...)
```

### 8.3 禁止事项

- 没有完成 P0-6 前，不要删旧 `rag/tool`。
- 没有观测 `chat_path` 前，不要声称 `mode: always` 已覆盖所有入口。
- 不要直接删除 `UseAgentRuntime` 字段。
- 不要删除 fallback 开关，除非有灰度数据证明不需要。
- 不要把“诊断 parity 未完成”继续当作 P0-4 的阻塞理由。

### 8.4 阶段 1：增加路径观测

修改文件：

```text
internal/app/rag/service/rag_chat_service.go
internal/app/rag/service/rag_chat_observability_log.go
internal/app/rag/service/chat_tracer.go
```

要求：

1. 在 `Chat()` 开始时判断：

```text
chat_path = agent_runtime_top_level    当 input.UseAgentRuntime=true
chat_path = rag_chat_legacy_main       当 input.UseAgentRuntime=false
```

2. 在 tool stage 判断：

```text
tool_backend = agent_runtime
tool_backend = tool_workflow
tool_backend = tool_workflow_fallback
```

3. 写入日志和 trace extra。

建议 trace extra：

```go
map[string]any{
	"runtimePath": map[string]any{
		"chatPath": "...",
		"toolBackend": "...",
		"useAgentRuntime": input.UseAgentRuntime,
		"agentMode": s.agentRuntimeMode,
	},
}
```

### 8.5 阶段 2：确认入口

必须搜索所有 `UseAgentRuntime`：

```powershell
rg -n "UseAgentRuntime" internal cmd -S
```

在文档或注释中确认：

```text
HTTP API 是否传 UseAgentRuntime
前端默认值是什么
测试默认值是什么
审批 resume 是否走 agent runtime
配置 rag.agent.chat.mode 是否只影响 tool stage
```

### 8.6 阶段 3：灰度切换

只有满足以下条件才能进入：

```text
P0-6 scope 内核心项 ready
外部证据主链路集成测试通过
chat_path 已有观测字段
tool_backend 已有观测字段
```

灰度策略：

1. 默认入口继续兼容 `UseAgentRuntime`。
2. 新增配置或入口默认值，使新请求默认 `UseAgentRuntime=true`。
3. 保留显式 legacy 开关用于回滚。
4. 观察 1-2 周。

### 8.7 阶段 4：删除旧 ToolWorkflow

只有满足以下条件才能删：

```text
连续观察期 legacy path 调用量为 0，或只来自显式回滚
P0-6 scope 内所有核心项 ready
go test ./internal/app/agent/... 通过
go test ./internal/app/rag/service -count=1 通过
```

删除顺序：

1. 删除 `runLegacyToolWorkflowStage`。
2. 删除 `toolWorkflow` 字段。
3. 删除 `SetToolWorkflow`。
4. 删除 `ragassembly.BuildLocalWorkflow(...)`。
5. 删除 `internal/app/rag/tool/`。
6. 修复编译错误。
7. 跑全量测试。

### 8.8 完成标准

```text
chat_path 可观测
tool_backend 可观测
UseAgentRuntime 入口已梳理
旧 tool 删除前有 parity matrix
删除旧 tool 后 go test ./... 通过
```

---

## 9. P1-1：检索通道并行化，保持结果顺序稳定

### 9.0 状态校准（2026-06-11）

该任务已完成，不应再按本节步骤重复施工。

当前代码中的 `executeChannels(...)` 已采用并行执行 + slot 聚合方式，目标语义已不再是“从串行改成并行”，而是继续补回归测试和观察性能表现。

### 9.1 目标

把 `executeChannels` 从串行改为并行，但不能改变结果语义和顺序。

### 9.2 当前代码事实

文件：

```text
internal/app/rag/core/retrieve/service.go
internal/app/rag/core/retrieve/channels.go
```

当前 `executeChannels` 串行 append 结果。并行后如果继续 append，会导致 channel 顺序不稳定。

### 9.3 禁止事项

- 不要并发 append 到同一个 slice。
- 不要让一个 channel 失败 cancel 其他 channel。
- 不要改变“部分失败可继续”的语义。
- 不要改变 `collectSearchChannels` 和 `collectChannelStats` 的输入顺序。

### 9.4 推荐实现

修改 `executeChannels`：

1. 先收集 enabled channels。
2. 按 index 分配 result slot。
3. goroutine 写自己的 slot。
4. wait 后按原 index 聚合。

推荐使用 `sync.WaitGroup`，不一定需要 `errgroup`。

伪代码：

```go
type channelExecutionResult struct {
	result SearchChannelResult
	err    error
	ok     bool
}

enabled := make([]SearchChannel, 0, len(e.channels))
for _, channel := range e.channels {
	if channel != nil && channel.Enabled(searchCtx) {
		enabled = append(enabled, channel)
	}
}
if len(enabled) == 0 {
	return nil, fmt.Errorf("no search channels enabled")
}

slots := make([]channelExecutionResult, len(enabled))
var wg sync.WaitGroup
for i, channel := range enabled {
	i, channel := i, channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := channel.Search(ctx, searchCtx)
		if err != nil {
			slots[i] = channelExecutionResult{
				result: SearchChannelResult{
					ChannelName: channel.Name(),
					Error: err.Error(),
					Metadata: map[string]any{"status": "failed"},
				},
				err: err,
			}
			return
		}
		slots[i] = channelExecutionResult{result: result, ok: true}
	}()
}
wg.Wait()
```

聚合时：

```go
results := make([]SearchChannelResult, 0, len(slots))
successCount := 0
var firstErr error
for i, slot := range slots {
	if slot.err != nil && firstErr == nil {
		firstErr = fmt.Errorf("search channel %s: %w", enabled[i].Name(), slot.err)
	}
	if slot.ok {
		successCount++
	}
	results = append(results, slot.result)
}
```

### 9.5 测试要求

新增或修改 retrieve 测试，覆盖：

```text
并行执行后结果顺序仍为 vector_global -> memory_fact -> keyword -> metadata_title 之类的原始顺序
一个 channel 失败，其他成功时整体成功
所有 channel 失败时返回 firstErr
没有 enabled channel 时返回 no search channels enabled
```

运行：

```powershell
go test ./internal/app/rag/core/retrieve -count=1
```

---

## 10. P1-2：bootstrap 函数拆分

### 10.0 状态校准（2026-06-11）

该任务目前应视为 `partial`。

当前已知信号表明拆分工作已经开始，例如 chat 装配已拆入独立 builder 文件；但是否已经把 `NewRuntime()` 主体彻底收敛为流程编排，仍应继续按本节验收标准检查，而不要假设“尚未开始”。

### 10.1 目标

拆分 `internal/bootstrap/rag/runtime.go` 的 `NewRuntime()`，降低装配复杂度。

### 10.2 禁止事项

- 不要改变行为。
- 不要删除旧 ToolWorkflow。
- 不要改配置默认值。
- 不要把 `ownsDB` 清理逻辑藏到子函数里。
- 不要引入 `interface{}` 传递中间结果。

### 10.3 推荐拆分

新增内部结构：

```go
type buildContext struct {
	cfg       *config.Config
	db        *gorm.DB
	aiRuntime *infraai.Runtime
	searcher  corevector.Searcher
}
```

推荐 builder：

```text
buildRepositories
buildConversationServices
buildMemoryServices
buildRetrieveServices
buildAgentRuntime
buildChatService
```

执行顺序：

1. 先提取 repository 创建，跑测试。
2. 再提取 conversation/message/history，跑测试。
3. 再提取 memory，跑测试。
4. 再提取 retrieve/rewrite/prompt，跑测试。
5. 最后提取 chat service。

### 10.4 验收

```text
NewRuntime 主体只保留流程编排
每个 builder 返回强类型结果
go test ./internal/bootstrap/rag -count=1 通过
go test ./... -count=1 通过
```

---

## 11. P1-3：log.FromContext 与核心 trace 字段收口

### 11.0 状态校准（2026-06-11）

该任务已完成首轮落地，不应再按本节步骤重复施工。

当前已知产物：

- `internal/framework/log/log.go` 中已有 `FromContext(...)` / `NewContext(...)`
- 相关测试已存在
- `rag chat` 相关日志路径已开始使用稳定字段

后续若继续推进，应按“扩大覆盖面”处理，而不是把它当作全新改造项。

### 11.1 目标

让一次 RAG/Agent 请求的日志可以通过稳定字段串起来。

字段名固定为：

```text
request_id
trace_id
conversation_id
user_id
task_id
agent_run_id
```

### 11.2 当前代码事实

当前 `internal/framework/log/log.go` 只有全局 logger 方法：

```text
Infof
Warnf
Errorf
Infow
Warnw
Errorw
```

### 11.3 禁止事项

- 不要每次 fallback 都 `zap.NewProduction()`。
- 不要一次性替换全仓库日志。
- 不要使用驼峰字段名混用，例如 `requestId`。

### 11.4 推荐实现

文件：`internal/framework/log/log.go`

新增：

```go
type contextKey struct{}

var fallbackSugar = zap.NewNop().Sugar()

func FromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx != nil {
		if logger, ok := ctx.Value(contextKey{}).(*zap.SugaredLogger); ok && logger != nil {
			return logger
		}
	}
	if sugar != nil {
		return sugar
	}
	return fallbackSugar
}

func NewContext(ctx context.Context, fields ...interface{}) context.Context {
	return context.WithValue(ctx, contextKey{}, FromContext(ctx).With(fields...))
}

func WithFields(fields ...interface{}) *zap.SugaredLogger {
	return FromContext(nil).With(fields...)
}
```

需要 import：

```go
import "context"
```

第一批替换位置：

```text
rag chat start
rewrite stage
retrieve stage
tool/capability stage
LLM call
```

### 11.5 验收

```text
FromContext(nil) 不 panic
request_id / trace_id 字段名统一
go test ./internal/framework/log ./internal/app/rag/service -count=1 通过
```

---

## 12. P1-4：Token-aware 会话窗口与预算控制

### 12.1 目标

从“按最近 N 条消息”升级为“按 token budget 控制上下文”。

### 12.2 当前代码事实

已有 token 估算能力：

```text
internal/app/rag/service/long_message_content_processor.go
RoughTokenEstimator
TokenEstimator
```

已有 session recall 预算：

```text
internal/app/rag/service/session_recall_service.go
MaxPromptTokens
```

LLM 接口目前主要返回 string：

```text
internal/infra-ai/chat/llm_service.go
ChatWithRequest(...) (string, error)
StreamChatWithRequest(...)
```

OpenAI-style response 目前没有解析 usage：

```text
internal/infra-ai/chat/openai_style_chat_client.go
openAIStyleChatResponse
```

### 12.3 禁止事项

- 不要第一步就大改所有 LLM 接口。
- 不要把 token usage 和裁剪策略混在一个巨大 PR。
- 不要只加字段不使用。

### 12.4 推荐分阶段

阶段 1：上下文估算与裁剪

```text
新增 ChatContextBudgetOptions
复用 RoughTokenEstimator
在 prompt build 前计算消息 token
超过预算时裁剪历史
trace 中记录 estimated_prompt_tokens
```

阶段 2：LLM usage contract

```text
新增可选接口 UsageAwareLLMService
解析 OpenAI usage
streaming usage 先记录 estimated，后续再支持 provider final usage
```

阶段 3：预算降级策略

```text
裁剪历史
压缩工具结果
降低 topK
切小模型
拒绝继续工具调用
```

### 12.5 验收

```text
长消息场景不会无限塞入 prompt
trace 能看到 estimated prompt token
token 超预算有明确裁剪记录
go test ./internal/app/rag/service -count=1 通过
```

---

## 13. P1-5：Summary 异步化与生命周期升级

### 13.1 目标

让 summary 从同步 `v1` 能力升级为可异步、可校验、可回滚、可重建的生命周期。

### 13.2 当前代码事实

相关文件：

```text
internal/app/rag/domain/conversation_summary.go
internal/adapter/repository/postgres/rag/models/conversation_summary_model.go
internal/adapter/repository/postgres/rag/conversation_summary_repo.go
internal/app/rag/core/history/service_store.go
internal/app/rag/service/conversation_message_service.go
internal/adapter/repository/postgres/migrations/20260430170000_create_rag_tables.sql
```

当前表字段：

```text
id
conversation_id
user_id
last_message_id
content
create_time
update_time
deleted
```

### 13.3 禁止事项

- 不要直接修改旧 migration 文件。
- 应新增新 migration。
- 不要把 summary worker 和表结构升级混在一个不可回滚的大改里。

### 13.4 推荐新增字段

新增 migration，例如：

```text
internal/adapter/repository/postgres/migrations/20260609000000_extend_conversation_summary_lifecycle.sql
```

字段：

```sql
ALTER TABLE t_conversation_summary
  ADD COLUMN IF NOT EXISTS summary_version INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS covered_from_message_id VARCHAR(20),
  ADD COLUMN IF NOT EXISTS covered_to_message_id VARCHAR(20),
  ADD COLUMN IF NOT EXISTS source_message_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS quality_status VARCHAR(32) NOT NULL DEFAULT 'unchecked',
  ADD COLUMN IF NOT EXISTS last_rebuild_reason TEXT;
```

同步修改：

```text
domain.ConversationSummary
models.ConversationSummaryModel
mapper.go
repository tests
```

### 13.5 异步化建议

先新增接口，不要直接重写全部 history：

```go
type SummaryJobEnqueuer interface {
	EnqueueConversationSummary(ctx context.Context, input SummaryJobInput) error
}
```

第一阶段可以用 in-memory worker，后续再落 DB 任务表。

### 13.6 验收

```text
新增 migration
domain/model/mapper 同步
旧读取 latest summary 不受影响
summary 记录能表达覆盖区间和质量状态
go test ./internal/app/rag/core/history ./internal/app/rag/service ./internal/adapter/repository/postgres/rag -count=1
```

---

## 14. P1-6：Answer 层评估与引用证据命中率

### 14.1 目标

证明 retrieve 提升最终转化成 answer 质量提升。

### 14.2 禁止事项

- 不要只评估 retrieve。
- 不要只看 LLM 自评。
- 不要生成没有 chunk/document 引用的 citation。

### 14.3 推荐新增文件

```text
internal/app/rag/evaluation/answer_evaluation.go
internal/app/rag/evaluation/answer_evaluation_test.go
testdata/answer_eval_samples.json
docs/answer_eval_plan.md
```

### 14.4 样本格式

```json
{
  "samples": [
    {
      "name": "answer_doc_failure",
      "query": "doc_fail_01 为什么失败",
      "expectedFacts": ["indexer node failed", "connection refused"],
      "expectedCitationChunkIds": ["chunk_001"],
      "forbiddenFacts": ["user cancelled the task"]
    }
  ]
}
```

### 14.5 指标

```text
answer_fact_hit_rate
citation_hit_rate
unsupported_claim_rate
forbidden_fact_rate
```

### 14.6 验收

```text
answer eval 可以离线运行
样本能表达 expected facts 和 citation chunk ids
报告能说明 retrieve 提升是否进入最终 answer
```

---

## 15. P1-7：Prometheus metrics 接入

### 15.1 目标

补齐低基数字段的业务指标。

### 15.2 禁止事项

- 不要把 query、conversation_id、document_id、trace_id 放 label。
- 不要把 metrics 接在需要业务登录的路径下，除非明确设计鉴权。
- 不要一次性加几十个指标。

### 15.3 第一批指标

```text
rag_rewrite_duration_seconds{status}
rag_retrieve_duration_seconds{search_mode}
rag_retrieve_channel_duration_seconds{channel,status}
rag_llm_call_duration_seconds{provider,model,operation}
rag_llm_call_errors_total{provider,model,error_type}
rag_agent_tool_calls_total{capability,status}
rag_summary_jobs_total{status}
ingestion_task_queue_size
```

### 15.4 推荐文件

```text
internal/framework/metrics/metrics.go
cmd/server/main.go
internal/app/rag/core/retrieve/service.go
internal/infra-ai/chat/routing_llm_service.go
```

### 15.5 验收

```text
/metrics 可访问
一次 RAG 请求后 retrieve / llm 指标变化
label cardinality 可控
go test ./... 通过
```

---

## 16. P2 任务执行边界

### P2-1：混合检索通道贡献分析

基于 `P0-3` 的 eval 输出扩展，不要另起系统。

新增指标：

```text
channel_hit_at_k
channel_unique_hit_count
channel_overlap_count
channel_first_relevant_rank
```

### P2-2：SubQuestion 策略升级

修改点：

```text
internal/app/rag/service/rag_chat_prepare.go
normalizedRetrieveSubQuestions
shouldSerializeSubQuestions
retrieveSubQuestionsParallel
retrieveSubQuestionsSerial
```

要求：

```text
最大子问题数量可配置
串并原因写入 trace
依赖型子问题走 serial
独立型子问题走 parallel
```

### P2-3：State clone/merge 自动化测试

修改点：

```text
internal/app/agent/state
```

测试方式：

```text
构造 fully populated snapshot
clone 后修改 clone 的所有 slice/map/pointer
断言 original 不变
merge 后断言 artifacts/context/capability outputs 不丢
```

不要只用 reflect 比较底层指针。

### P2-4：Agent/Tool 任务级评估

新增：

```text
docs/agent_task_eval_plan.md
testdata/agent_task_eval_samples.json
```

指标：

```text
task_success_rate
average_tool_rounds
degraded_rate
timeout_rate
unsupported_conclusion_rate
human_intervention_rate
```

---

## 17. 校准后的推荐执行顺序

以下顺序替代文档初版的 Week 1 / Week 2 施工节奏，按 `2026-06-11` 的真实剩余项推进。

### 第一优先级：先补真正阻塞 runtime 收口的项

```text
1. P0-6：补齐新 runtime scope 对应的 partial/missing 能力
2. P0-4：在 P0-6 新阻塞项缓解后推进阶段 3 灰度切换
3. P0-5：完成依赖注入收敛第二阶段，继续删残余 setter/test 隐式约定
```

### 第二优先级：补质量证明和结构整理

```text
1. P0-3：把 retrieve eval 从“有基础设施”推进到“有样本、有命令、有报告”
2. P1-2：继续拆分 bootstrap，确认 NewRuntime 主体只保留流程编排
3. P1-4：继续 token-aware 会话窗口，补 usage/裁剪闭环
```

### 第三优先级：进入中后期质量与运维能力

```text
1. P1-5：Summary 异步化与生命周期升级
2. P1-6：Answer 层评估与引用证据命中率
3. P1-7：Prometheus metrics 接入
4. P2-1 / P2-2 / P2-4：在前置评估闭环稳定后推进
```

---

## 18. 交给弱模型时的提示词模板

执行单个任务时，把下面模板贴给弱模型：

```text
你只能执行 docs/structural_improvement_plan.md 中的任务 <任务编号>。

硬性要求：
1. 不要修改 S1/S2。
2. 不要删除 internal/app/rag/tool。
3. 不要做无关重构。
4. 严格按该任务的“需要修改的文件”和“具体步骤”执行。
5. 每完成一步运行该任务指定的 go test。
6. 如果测试失败，只修本任务相关文件。
7. 最后总结：改了哪些文件、跑了哪些测试、还有哪些未完成。

开始前先读取：
<粘贴该任务列出的文件>
```

---

## 19. 最小完成定义

任何任务完成时，都必须满足：

```text
目标文件已修改或目标文档已新增
指定测试通过
没有修改 S1/S2
没有提前删除旧 tool
有明确的验收证据
有失败时的回滚方式
```

如果做不到以上条件，不要标记完成。
