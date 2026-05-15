# Tool 模块深度设计文档

> 日期: 2026-05-12

---

## 一、模块定位

Tool 模块是 RAG 系统中**Agent 工具调用**的核心引擎。它在 RAG 检索之后运行，实现了一个多轮的 **Plan → Act → Observe** Agent 循环，自动决定需要调用哪些诊断/查询工具来收集足够的证据，最终为 LLM 生成回答提供结构化的上下文和指导。

一句话概括：**RAG 检索出知识块，Tool Agent 决定是否需要更深层的诊断信息（文档状态、ingestion 任务、trace 链路、外部 web 搜索），并自动编排这些工具调用。**

---

## 二、整体架构

### 2.1 分层视图

```
┌─ assembly/workflow.go ─────────────────────────┐
│  BuildLocalWorkflow() — 依赖注入 & 组装入口       │
│  注册 16+ 工具模块到 Registry                     │
└────────────────────┬───────────────────────────┘
                     │
    ┌────────────────┼──────────────────┐
    ▼                ▼                  ▼
┌───────────┐  ┌───────────┐  ┌──────────────┐
│  Planner  │  │ Executor  │  │  Observer    │
│  (规划)   │  │ (执行)    │  │  (观察)      │
└─────┬─────┘  └─────┬─────┘  └──────┬───────┘
      │              │               │
      ▼              ▼               ▼
┌──────────┐  ┌───────────┐  ┌──────────────┐
│LLMPlanner│  │ Registry  │  │ LLMObserver  │
│+ 规则回退│  │(工具注册表)│  │ +RuleObserver│
└──────────┘  └─────┬─────┘  └──────────────┘
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
   invokers/   modules/     builtin/
   (工具实现)  (行为定义)   (传统工具)
```

### 2.2 核心领域模型

```
ToolModule {
    Name     string          // "document_query", "web_search" 等
    Invoker  ToolInvoker     // 实际执行工具的组件 (func)
    Spec     ToolSpec        // 元数据: Capability, RiskLevel, EvidenceSources...
    Behavior ToolBehavior    // 策略函数: Decode, Next, Observe, RenderContext, BuildGuidance
}

ToolBehavior {
    Decode        func(Result) → (typed result, error)     // 结果类型化
    Next          func(Result, Input) → NextDecision      // 链式决策: 下一步调什么?
    Observe       func(Result, Input) → (ObserveResult, bool) // 观察评估: 证据够了吗?
    RenderContext func(Result) → string                   // 格式化为 LLM 上下文
    BuildGuidance func(Result, Input) → []GuidanceNote    // 回答指导
}
```

**设计要点**: `ToolBehavior` 使用函数字段而非接口，每个函数字段可选（nil = 无此行为），这是一种 Go 风格的策略模式。

### 2.3 核心 Agent Loop

```
Run(question, context) {
    for round := 1..maxIterations (默认3):
        ┌─ Plan ─────────────────────────────────┐
        │  LLMPlanner.Plan() → 失败? → planWithRules() │
        │  planWithRules:                         │
        │    1. 如果 agentState 有 NextHintCalls → 使用它们 │
        │    2. 否则 → planWithBaseRules (关键词匹配)  │
        │    3. 否则 → planCallsFromResults (链式推导) │
        └─────────────────────────────────────────┘
                    │
        ┌─ Act ──────────────────────────────────┐
        │  Executor.Execute(call) 串行或并行       │
        │  每个 call: invoker.Invoke() → 合并 Meta │
        └─────────────────────────────────────────┘
                    │
        ┌─ Observe ───────────────────────────────┐
        │  LLMObserver.Observe() → 失败? → RuleObserver │
        │  返回 Done=true (证据充足) 或            │
        │  Done=false + NextHintCalls (继续下一轮) │
        └─────────────────────────────────────────┘
                    │
        如果 Done → break
        否则 → 下一轮
}
```

---

## 三、工具分类

| 工具 | Family | Capability | 功能 |
|------|--------|------------|------|
| `think` | meta | general | 思考记录 (无实际操作) |
| `document_query` | system | diagnosis | 查询文档状态 |
| `document_list` | system | diagnosis | 列出文档 |
| `task_list` | system | diagnosis | 列出 ingestion 任务 |
| `document_ingestion_diagnose` | system | diagnosis | 诊断文档 ingestion 失败原因 |
| `document_chunk_log_query` | system | diagnosis | 查询文档 chunk 日志 |
| `ingestion_task_query` | system | diagnosis | 查询 ingestion 任务详情 |
| `ingestion_task_node_query` | system | diagnosis | 查询任务节点详情 |
| `task_ingestion_diagnose` | system | diagnosis | 诊断任务失败原因 |
| `trace_node_query` | trace | diagnosis | 查询 RAG trace 节点 |
| `trace_retrieval_diagnose` | trace | diagnosis | 诊断检索质量 |
| `web_search` | web | search | 外部搜索 |
| `web_fetch` | web | search | 获取网页内容 |
| `external_evidence_workflow` | web | search | 外部证据工作流 (搜索+获取) |
| `document_root_cause_diagnosis` | graph | diagnosis | 文档根因诊断图 |
| `document_diagnose_with_search` | graph | search | 文档诊断+外部搜索图 |

---

## 四、关键设计

### 4.1 AgentState — 跨轮次状态传递

```go
type AgentState struct {
    Phase         string     // "initial_diagnosis" → "deep_dive" → "verification" → "complete"
    Hypothesis    string     // 当前假设, e.g. "indexer failed because vector store unavailable"
    Confidence    float64    // 0.0 - 1.0
    OpenQuestions []string   // 尚未解答的问题
    CheckedTools  []string   // 已调用的工具
    NextHintCalls []HintCall // 下一轮工具调用提示
    NextHint      string     // 传统格式的提示串 (向后兼容)
}
```

AgentState 在每轮 Plan/Observe 之间传递，是 LLM Planner 和 Observer 理解"当前进展到什么程度"的关键上下文。

### 4.2 证据链与防幻觉机制

**核心问题**: LLM 可能虚构 ID（hallucination）。tool 模块通过 `collectEvidenceIDs` 建立**白名单机制**：

```go
func collectEvidenceIDs(question, previousHintCalls, results) map[string]struct{} {
    allowed := {} // 空集合开始
    // 1. 从用户问题中提取 ID (regex 匹配 doc-xxx, task-xxx, trace-xxx)
    // 2. 从上一轮的 NextHintCalls 中收集
    // 3. 从所有工具执行结果中收集 (documentId, taskId, nodeId, traceId, latestTaskId, latestNodeId)
    return allowed
}
```

**`validateHintAgainstEvidence`**: Observer 返回的 NextHintCalls 中的每个 ID 必须出现在白名单中，否则整组 NextHintCalls 被拒绝（回退到 RuleObserver）。

**`validateCallAgainstEvidence`**: Planner 返回的 Call 中的每个 ID 同样需要校验。这构成了**双重防 hallucination 防护**：
- Planner 的 planWithLLM 过滤（第 277 行）
- Observer 的 validateHintAgainstEvidence（第 249 行）

### 4.3 Observer 双层策略

```
LLMObserver (AI驱动)
    │
    ├─ 调用 LLM → JSON 解析
    │   ├─ 成功 → 返回 ObserveResult
    │   └─ 失败/解析失败 → 回退到 RuleObserver
    │
    └─ RuleObserver (规则驱动, 确定性)
        ├─ 先查 Registry 中模块的 Behavior.Observe
        ├─ 再查 inferLegacyBehavior 的 Observe
        └─ 最后用 diagnosisDepth 判断
```

RuleObserver 是 LLMObserver 的**安全网**——即使 LLM 服务不可用或返回非法 JSON，agent loop 仍能做出合理决策。

### 4.4 ToolBehavior.Next — 工具链编排

每个工具可以定义 `Next` 函数，根据当前结果决定下一个工具。这形成了隐含的**诊断链**：

```
document_query (failed pipeline doc)
  → Next: document_ingestion_diagnose
      → Next: ingestion_task_query (如果有 taskId)
          → Next: ingestion_task_node_query (如果有 failed node)
              → Terminal (找到节点级错误)

document_chunk_log_query (异常 chunk log)
  → Next: ingestion_task_query (如果有 taskId)
  或 Next: document_ingestion_diagnose (如果没有 taskId)

web_search (有结果)
  → Next: web_fetch (获取详情)
```

这个链式设计使得 agent 可以**自动深挖**——用户问"doc-123 为什么失败了"，agent 会自动执行 document_query → document_ingestion_diagnose → ingestion_task_query → ingestion_task_node_query 的完整链路。

### 4.5 planWithBaseRules — 关键词基线的启动逻辑

当 LLM Planner 不可用或返回空结果时，系统回退到**基于中文/英文关键词匹配**的规则规划器：

```go
// 文档 ID 匹配
if documentID := firstMatchedID(documentIDPattern, question); documentID != "" {
    if containsAny(lowered, "diagnose", "failed", "why", "排查", "诊断", "失败"...)
        → document_root_cause_diagnosis 或 document_diagnose_with_search
    if containsAny(lowered, "chunk log", "ingestion"...)
        → document_chunk_log_query
    else
        → document_query
}

// 无匹配 ID 时的开放式查询
if containsAny(lowered, "哪些", "最近", "所有", "which", "list"...)
    → document_list 或 task_list

// 知识库检索不足
if KnowledgeBaseInsufficient(retrieveResult)
    → external_evidence_workflow (web 搜索)
```

这是 system prompt engineering 之外的**确定性工具选择**，保证即使 LLM 不可用，基本的问题（"doc-xxx 为什么失败了"）也能得到诊断。

---

## 五、并行执行

```go
func executeCallsInParallel(items []roundExecutionItem) {
    sem := make(chan struct{}, maxConcurrency)  // 信号量控制并发
    var wg sync.WaitGroup
    for idx := range items {
        wg.Add(1)
        sem <- struct{}{}        // 获取许可（阻塞直到有可用）
        go func(itemIndex int) {
            defer wg.Done()
            defer func() { <-sem }()  // 释放许可
            items[itemIndex] = executeSingleCall(...)  // 写同一 slice 的不同 index
        }(idx)
    }
    wg.Wait()
}
```

**安全性分析**: 每个 goroutine 写入 `items[itemIndex]` 的不同索引，Go 的内存模型保证不同内存位置的并发写是安全的（slice 底层数组的每个元素是独立内存地址）。没有 data race。

---

## 六、双 Registry 架构（重构中）

当前代码库存在两套并行的类型系统：

| 概念 | 传统包 `package tool` | 新核心 `package core` |
|------|----------------------|----------------------|
| Tool 接口 | `tool.Tool` | `core.Tool` |
| Registry | `tool.Registry` | `core.Registry` |
| Call | `tool.Call` | `core.Call` |
| Result | `tool.Result` | `core.Result` |
| AgentLoop | `tool.AgentLoop` | `runtime.AgentLoop` |

`assembly/workflow.go` 同时向两个 registry 注册：
```go
registry := ragcore.NewRegistry()           // 新核心
legacyRegistry := legacytool.NewRegistry()  // 传统
registerLegacyModule(registry, legacyRegistry, ...)
```

`modular_wrappers.go` 提供 `fromCore*`/`toCore*` 转换函数在两者之间桥接。

**代码重复清单**:
- `agent_loop.go` — 根目录 + `runtime/` 两份
- `executor.go` — 根目录 + `runtime/` 两份
- `observer_llm.go` — 根目录 + `runtime/` 两份
- `observer_rule.go` — 根目录 + `runtime/` 两份
- `next_action.go` — 根目录 + `runtime/` 两份
- `workflow_control.go` — 根目录 + `runtime/` 两份

`runtime/` 版本使用 `. "ragcore"` dot 导入，而根目录版本是自包含的。这是一个**尚未完成的重构**。

---

## 七、已知问题

### 7.1 代码重复 (重构债务)

**文件**: 根目录 `tool/` vs `runtime/` (6 个文件重复)

**严重度**: 中。两份代码独立演进，修改一处需要同步另一处。但功能上不影响运行。

### 7.2 `serializeHintCalls` 只序列化第一个 call

**文件**: [workflow_helpers.go:114-155](internal/app/rag/tool/workflow_helpers.go#L114-L155)

```go
func serializeHintCalls(calls []HintCall) string {
    calls = normalizeHintCalls(calls)
    if len(calls) == 0 { return "" }
    call := calls[0]  // ← 只取第一个！
    // ... 序列化为 "tool:name|key=val|..."
}
```

**问题**: 当 Observer 返回多个并行的 NextHintCalls 时，`AgentState.Normalize()` 调用 `serializeHintCalls` → 只保留第一个。这导致在 `NextHint` 字符串字段（向后兼容格式）中**丢失了并行调用的信息**。不过 `NextHintCalls` 结构化字段保留了完整信息，所以当新代码路径走 `NextHintCalls` 时不受影响。

**严重度**: 低（向后兼容的字符串字段的正确性问题，结构化字段是完整的）

### 7.3 遗留的 Hint 序列化格式脆弱

**文件**: [agent_loop.go:673-704](internal/app/rag/tool/agent_loop.go#L673-L704) (`parseNextHint`)

格式: `tool:name|key1=val1|key2=val2|...`

**问题**:
- `|` 分隔符：如果参数值包含 `|`，会错误拆分
- `=` 分隔符：如果参数值包含 `=`，只有第一个 `=` 后的内容被视为 value
- 无转义机制，无版本标记
- URL 值（如 `urls` 参数）经常包含特殊字符，容易损坏

**严重度**: 低（新代码优先使用结构化的 `HintCall` JSON）

### 7.4 `truncateForLog` / `truncateScheduleStateMessage` 字节截断

**文件**: [observer_llm.go:551-557](internal/app/rag/tool/observer_llm.go#L551-L557)

```go
return raw[:300] + "..."  // byte-level truncation
```

与 schedule 模块相同的 UTF-8 截断问题。

**严重度**: 极低（仅日志显示）

### 7.5 `planWithBaseRules` 关键词匹配

**文件**: [agent_loop.go:300-418](internal/app/rag/tool/agent_loop.go#L300-L418)

**问题**:
- 中英文关键词列表硬编码且不完整——例如不支持日语、韩语等其他语言
- `containsAny(lowered, "doc", "文档")` 匹配过于宽泛（"doc" 出现在 "doctor" 中也会匹配）
- `"document"` 关键词本身是判断入口，如果用户说 "show me the status of doc-abc123" 而不包含 "document" 关键词（虽然 "doc-abc123" 包含 "doc"），会匹配。但如果用户说 "what about my file abc-123" 则不匹配
- 新工具的加入需要修改此函数

**严重度**: 低（有 LLM Planner 作为主要路径，这仅是 fallback）

### 7.6 `cloneMap` 是浅拷贝

**文件**: [agent_loop.go:642-651](internal/app/rag/tool/agent_loop.go#L642-L651)

```go
func cloneMap(input map[string]any) map[string]any {
    cloned := make(map[string]any, len(input))
    for key, value := range input {
        cloned[key] = value  // 只复制引用，不深拷贝
    }
    return cloned
}
```

**问题**: 如果 value 是 `map[string]any` 或 `[]any`，修改克隆后的嵌套结构会影响原始数据。

**严重度**: 极低（当前代码中 value 通常是 string/bool/int，没有嵌套修改场景）

### 7.7 `extractObserverJSONBlock` 脆弱性

**文件**: [observer_llm.go:415-435](internal/app/rag/tool/observer_llm.go#L415-L435)

```go
// 找 ```json → 找不到? → 找 ``` → 提取内容直到下一个 ```
```

**问题**: 如果 LLM 在推理文本中提到 ```` ``` ````（比如示例代码），会提取错误的内容。对于不以 fence 包裹的 JSON（如裸 JSON），直接 `json.Unmarshal(raw)` 也可能被前置文本干扰。

**严重度**: 低（有 fallback 到 RuleObserver，误提取只会影响单次 observer 决策）

### 7.8 `planWithBaseRules` 的 `maxCalls` 与 `maxIterations` 混淆

**文件**: [agent_loop.go:289-298](internal/app/rag/tool/agent_loop.go#L289-L298)

```go
if agentState.Empty() || len(agentState.NextHintCalls) == 0 {
    return filterNewCalls(planWithBaseRules(input, defaultMaxIterations), executed)
    //                                         ^^^^^^^^^^^^^^^^^^
    //                                         用 maxIterations(3) 作为每轮最大调用数
}
```

**问题**: `planWithBaseRules` 的第二个参数叫 `maxCalls`（限制每轮工具调用数），但传入的值是 `defaultMaxIterations`（默认 3）。两个概念语义不同，虽然数值巧合一致。应该用独立常量。

**严重度**: 极低

### 7.9 `RuleObserver.Observe` 中的 `firstNonEmpty` 位置不一致

**文件**: [observer_rule.go:89-92](internal/app/rag/tool/observer_rule.go#L89-L92)

```go
case "node_level":
    return newObserveResult(true, "...", observeState(
        "complete",
        strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
```

当 `latest.Summary == ""` 且 `latest.ErrorMessage == ""` 时，hypothesis 为空字符串。这是 `node_level` 的完成状态，但 hypothesis 为空意味着 agent 认为"已完成"但没有总结出假设。

**严重度**: 极低（错误消息为空的情况极少见）

---

## 八、架构评价

| 维度 | 评价 |
|------|------|
| **Agent 循环** | Plan-Act-Observe 三阶段设计完整，LLM + 规则双重保险 |
| **防幻觉** | 白名单证据链 + validateHintAgainstEvidence 双重防护，设计出色 |
| **可扩展性** | Registry + ToolModule + ToolBehavior 策略模式，新增工具只需注册 |
| **容错性** | LLM Planner/Observer 失败均有 fallback，degrade 逐轮记录 |
| **可观测性** | EventSink 接口 + 详细日志，每轮均有 wall clock / tool duration |
| **代码质量** | 双 Registry 架构带来的重复是重构债务，但不影响功能 |
| **测试覆盖** | 有 planner/agent_loop/behavior 等测试，但并行执行路径覆盖不足 |

---

## 九、待评估项（按优先级排序）

1. **[workflow_helpers.go:114]** — `serializeHintCalls` 只序列化第一个 call，序列化保真度不完整
2. **[agent_loop.go:673]** — `parseNextHint` 的 `|` 分隔格式无转义机制
3. **[tool/ vs runtime/ 6 文件]** — 代码重复，需完成重构去重
4. **[agent_loop.go:300]** — `planWithBaseRules` 的关键词匹配需扩展或替换为更通用的方案
5. **[observer_llm.go:551]** — `truncateForLog` 字节截断可能破坏 UTF-8
6. **[observer_llm.go:415]** — `extractObserverJSONBlock` 解析容错性一般
