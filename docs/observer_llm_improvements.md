# LLMObserver 薄弱点与加强方向

版本：v1.2
日期：2026-05-10

---

## 一、当前状态

`LLMObserver` 已替换 `RuleObserver` 作为默认 Observer（`runtime.go:260`），且 `doc_fail_01` 的诊断路径不弱于原 RuleObserver。

自 v1.0 以来，经过两轮迭代，6 个已识别问题中 5 个已解决。当前仅剩 1 个中长期问题（决策质量评估闭环）未落地。

---

## 二、已修复问题

### 2.1 数据摘要：从白名单改为黑名单

**位置：** [result_summary.go:74-87](internal/app/rag/tool/result_summary.go#L74-L87)

**修复内容：** 显式提取 14 个高频 key + 7 个语义字段后，遍历 `data` 中剩余的所有 key，仅排除已知噪音字段（`rawBody`、`fullText`、`rawText`、`rawContent`、`originalText`），其余全部透传。

```go
remainingKeys := make([]string, 0, len(data))
for key := range data {
    if _, exists := seen[key]; exists { continue }
    if isLLMSummaryNoiseKey(key) { continue }
    remainingKeys = append(remainingKeys, key)
}
```

配合 `summarizeGenericValue` 的递归泛型处理（string / []string / []any / map[string]any），以及截断控制（每类最多 3 项、每条最多 160 字符、总计最多 18 个 part），未来新增工具产出的任意字段都不会被静默丢弃。

### 2.2 检索和改写上下文：Observer 和 Planner 均已接入

**位置：**
- Observer — [observer_llm.go:139-147](internal/app/rag/tool/observer_llm.go#L139-L147)
- Planner — [planner.go:120-127](internal/app/rag/tool/planner/planner.go#L120-L127)

**修复内容：** `SummarizeRewriteResultForLLM` 和 `SummarizeRetrieveResultForLLM` 提升为 `tool` 包的导出函数，Observer 和 Planner 共用同一套实现。

- 改写摘要：rewrittenQuestion、subQuestions、preferredSearchMode
- 检索摘要：searchChannels、channelStats（含各通道命中数和错误）、top-3 chunk 的 section/source_file_name/text

Planner 不再"盲于检索结果"，可以据此避免规划与检索阶段重复的工具调用。

### 2.3 Observer prompt：few-shot 示例 + 动态工具定义

**位置：** [observer_llm.go:15-47](internal/app/rag/tool/observer_llm.go#L15-L47)、[observer_llm.go:130](internal/app/rag/tool/observer_llm.go#L130)

**修复内容：**
- 3 个 few-shot 示例：证据充分→Done / 证据不足→继续 / 运行中→不深入
- `buildSystemPrompt(input.ToolDefinitions)` 动态注入完整工具定义（名称、描述、参数名、类型、必填），和 Planner 使用相同的 `renderToolDefinitionsForPrompt` 渲染逻辑
- 规则 #6：运行中状态优先验证而非假设失败

### 2.4 nextHint 扁平字符串协议：升级为结构化 HintCall

**位置：**
- 类型定义 — [workflow.go:30-34](internal/app/rag/tool/workflow.go#L30-L34)
- 序列化/反序列化 — [workflow_helpers.go:64-130](internal/app/rag/tool/workflow_helpers.go#L64-L130)
- AgentState — [workflow.go:41](internal/app/rag/tool/workflow.go#L41)
- ObserveResult — [observer_rule.go:36](internal/app/rag/tool/observer_rule.go#L36)
- 规划调用 — [agent_loop.go:351](internal/app/rag/tool/agent_loop.go#L351)

**修复内容：** 新增 `HintCall` 结构体：

```go
type HintCall struct {
    Name      string
    Arguments map[string]any
}
```

改动链：
- `AgentState.NextHintCalls []HintCall` — 结构化的下一步工具调用
- `ObserveResult.NextHintCalls []HintCall` — Observer 输出使用结构化数组
- 旧 `NextHint string` 保留，在 `Normalize()` 中自动双向同步（`parseHintCallsFromLegacyString` / `serializeHintCalls`）
- `planCallsFromHintCalls` — 新的规划函数直接消费结构化 hint，不再解析字符串
- `validateHintAgainstEvidence` / `collectEvidenceIDs` 全部改为接受 `[]HintCall`
- Observer prompt 示例和 Planner prompt 示例全部改用 `nextHintCalls` JSON 数组格式

**能力提升：**
- 可以表达多个下一步工具（当前仍限制为 1 个，接口已支持扩展）
- 参数保留原始类型（bool 的 `true` 不再是字符串 `"true"`）
- 不再需要 `tool:name|key=value` 这种脆弱的序列化-反序列化

### 2.5 Hypothesis 回退逻辑：移除 Reasoning 混用

**位置：** [observer_llm.go:222-223](internal/app/rag/tool/observer_llm.go#L222-L223)

**修复内容：** 去掉了 `Reasoning` → `Hypothesis` 的回退链：

```go
// 修复前（语义混淆）
state.Hypothesis = strings.TrimSpace(firstNonEmpty(parsed.Reasoning, input.PreviousState.Hypothesis))

// 修复后（仅从 PreviousState 继承）
if strings.TrimSpace(state.Hypothesis) == "" {
    state.Hypothesis = strings.TrimSpace(input.PreviousState.Hypothesis)
}
```

`Reasoning`（行为描述："Task evidence is not enough; inspect the task detail next"）和 `Hypothesis`（状态假设："the task failed but the concrete node is still unknown"）不再混淆。

---

## 三、仍存在的问题

### 3.1 Observer 决策质量反馈闭环

**现状：** `cmd/retrieve-eval` 评估基础设施只覆盖检索质量（Hit@K、Recall@K、MRR），没有覆盖 Agent 决策质量。

**影响：**
- 无法量化过早终止率（该继续但停了）
- 无法量化空转率（该停但继续了）
- 无法判断 Confidence 是否校准（0.9 高置信 Done 的实际正确率是否高于 0.6）
- 修改 prompt 后无法 A/B 对比效果

**加强方向：**
1. 利用已有的 `cmd/retrieve-eval` 模式建立 Agent 决策离线评估
2. 对样本标注 `expectedMinRounds`、`expectedStopEvidence`、`shouldNotExceedRounds`
3. 回放 Agent 执行，统计过早终止率、空转率、Confidence 校准曲线
4. 需要先积累标注样本，短期不推进

---

## 四、演进总结

| 版本 | 问题数 | 状态 |
|------|--------|------|
| v1.0 | 4 个薄弱点 | 初始分析 |
| v1.1 | 3 已修复 + 3 新发现 + 1 未开始 | 中期评估 |
| v1.2 | 6 个全部修复 | 当前版本 |

唯一遗留的"决策质量评估"属于基础设施建设项目，需要标注样本积累，不影响当前 Agent 的功能完整性和在线质量。

---

## 五、约束

以下约束在迭代过程中保持不变：

1. `Observer` 接口签名不变（`Observe(ctx, ObserveInput) (ObserveResult, error)`）
2. `RuleObserver` 作为 fallback 的机制不变
3. `validateHintAgainstEvidence` 的安全校验不变（防止 LLM 幻觉 ID）
4. 不引入新的第三方依赖
