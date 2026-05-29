# Eino Graph Agent Loop 架构设计

日期：2026-05-29

状态：设计提案

---

## 一、动机

当前 `internal/app/rag/tool` 的 AgentLoop 经过多轮迭代已经是一个功能完备的生产级系统——15 个 tool、LLMPlanner + LLMObserver 双 LLM 决策、RuleObserver 降级、并行执行、module-first 架构。但它也积累了结构性负债：

1. **控制流隐藏在 for/if 里** — `runtime/agent_loop.go` 708 行，核心循环体 ~500 行，状态合并逻辑有明显的层层补丁痕迹
2. **状态传递链路不透明** — Planner 从 `WorkflowInput` + `AgentState` + `PreviousResults` 三个来源读数据，Observer 从 `Result.Data` map 读数据，参数幻觉 bug 的直接根因就是数据流不可追踪
3. **Planner 与 Observer 事实重叠** — Observer 的 `nextHintCalls` 就是下一轮的 plan，两次 LLM 调用为同一段推理付两次费（已在 `agent_loop_architecture_review.md` 中充分讨论）
4. **扩展靠加 if 分支** — 并行执行、multi-hint、evidence validation 都是往循环体里塞代码，而非在架构层面提供扩展点

与此同时，`internal/app/agent` 已经落地了 Eino-native 的双层 tool 架构（`search/websearch`、`fetch/webfetch`），证明了 Eino InferTool + 纯 Go service 层的模式比旧系统的手写 `Definition/Call/Decode` 更简洁。

本文档提出一个新 AgentLoop 架构，目标是在 `internal/app/agent` 包里建设一个**独立、成熟、可替代旧 tool 包的 agent 能力模块**，核心思路是：

- **用 Eino Graph 作为 AgentLoop 的运行时**（替代 Go for-loop）
- **用显式 Typed State 作为唯一数据源**（消除隐式状态传递）
- **用 Graph Branch 实现条件路由**（替代 binary Done/Continue）
- **保留旧系统的核心业务资产**（ToolBehavior、evidence validation、baseRules、prompt 模板）

---

## 二、核心架构

### 2.1 总览

```
                        ┌──────────────────────────────────────┐
                        │         Eino Graph[I, O]              │
                        │         I = O = *AgentState           │
                        │                                      │
      START             │                                      │
        │               │                                      │
   ┌────┴────────────┐  │                                      │
   │ PrepareContext  │  │  · rewrite / retrieve / memory 注入  │
   │                 │  │  · 判定是否需要检索 / 是否需要搜索    │
   └────────┬────────┘  │                                      │
            │           │                                      │
   ┌────────┴────────┐  │                                      │
   │   PlanTools     │  │  · LLMPlanner (主路径)               │
   │                 │  │  · baseRules 关键词路由 (冷启动)      │
   │                 │  │  · nextHintCalls 复用 (Round 2+)      │
   └────────┬────────┘  │                                      │
            │           │                                      │
   ┌────────┴────────┐  │                                      │
   │  ExecuteTools   │  │  · 并行执行 tool call                 │
   │                 │  │  · InferTool 驱动的工具执行           │
   └────────┬────────┘  │  · 结果写入 AgentState.ToolResults    │
            │           │                                      │
   ┌────────┴────────┐  │                                      │
   │    Observe      │  │  · LLMObserver (主路径)               │
   │                 │  │  · RuleObserver (降级)                │
   │                 │  │  · evidence validation                │
   └────────┬────────┘  │  · 设定 EvidenceLevel                │
            │           │                                      │
     ┌──────┴──────┐    │                                      │
     │   Branch    │    │  ← GraphMultiBranch                  │
     └──────┬──────┘    │                                      │
            │           │                                      │
     ┌──────┼──────────────┐                                   │
     │      │              │                                   │
  sufficient  partial       insufficient                       │
     │      + round<max    + round>=max                         │
     │                      │                                   │
     │    loop back to      │                                   │
     │     PlanTools        │                                   │
     │                      │                                   │
   ┌─┴──────────────┐  ┌───┴─────────────────┐                │
   │ GenerateAnswer │  │ GenerateDegradedAns │                │
   └────────┬───────┘  └──────────┬──────────┘                │
            │                     │                            │
            └──────────┬──────────┘                            │
                       │                                       │
                      END                                      │
                       │                                       │
                        └──────────────────────────────────────┘
```

### 2.2 与旧 AgentLoop 的关键差异

| | 旧 AgentLoop (for-loop) | 新 AgentLoop (Eino Graph) |
|---|---|---|
| 控制流 | Go `for` 循环 + `if/break` | Graph 节点 + Branch 条件边 |
| 状态 | `AgentState` + `WorkflowResult.Calls` (map) 隐式传递 | 单一 `*AgentState` struct，每节点读写 |
| 路由 | 只支持 binary done/continue | 多路分支：生成回答 / 继续规划 / 降级回答 |
| 工具包装 | 手写 `Definition/Call/Decode` | Eino `InferTool[I,O]` 自动生成 schema |
| 可恢复性 | 无 | `WithCheckPointStore` 原生暂停/恢复 |
| 中断/审批 | 无 | `WithInterruptBeforeNodes` 原生支持 |
| 可测试性 | 需构造完整循环 | 每个节点是 `State→State` 纯函数 |

---

## 三、AgentState — 唯一数据源

```go
// AgentState 是 agent loop 执行全过程中的唯一共享状态。
// 每个 graph node 接收 *AgentState，返回 *AgentState。
// 不存在任何隐式数据通道。
type AgentState struct {
    // === 输入（graph 启动时设置，之后只读）===
    Question        string   `json:"question"`
    UserID          string   `json:"user_id"`
    TraceID         string   `json:"trace_id"`
    KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`

    // === PrepareContext 注入 ===
    RewriteResult    *RewriteResult    `json:"rewrite_result,omitempty"`
    RetrieveResult   *RetrieveResult   `json:"retrieve_result,omitempty"`
    MemoryContext    *MemoryContext    `json:"memory_context,omitempty"`
    NeedRetrieval    bool              `json:"need_retrieval"`
    KBInsufficient   bool              `json:"kb_insufficient"`

    // === 每轮动态（PlanTools 设置）===
    Round            int               `json:"round"`
    PlannedCalls     []ToolCall        `json:"planned_calls"`

    // === 累积证据（ExecuteTools 追加）===
    ToolResults      []ToolResult      `json:"tool_results"`
    ExecutedTools    map[string]bool   `json:"executed_tools"`    // toolName → true，防重复调用
    AllToolCalls     []ToolCallSummary `json:"all_tool_calls"`    // trace/SSE 用

    // === Observe 产出 ===
    EvidenceLevel    string            `json:"evidence_level"`    // "insufficient" | "partial" | "sufficient"
    NextHints        []HintCall        `json:"next_hints"`        // 下一轮的 tool hint（原 nextHintCalls）
    Confidence       float64           `json:"confidence"`        // 0.0 ~ 1.0
    Degraded         bool              `json:"degraded"`
    DegradeReason    string            `json:"degrade_reason"`

    // === 终止（GenerateAnswer 设置）===
    FinalAnswer      string            `json:"final_answer,omitempty"`
    GuidanceNotes    []GuidanceNote    `json:"guidance_notes,omitempty"`
}
```

关键设计原则：

1. **没有 `map[string]any`** — 旧系统 `ToolResult.Data` 的类型擦除在此消除。每个 tool 的 output 是具体类型，由 InferTool 保证
2. **追加而非覆盖** — `ToolResults` 用 append 语义，不覆盖历史
3. **状态指针传递** — `*AgentState` 而非值传递，避免每个节点做深拷贝。Eino Graph 的 `FanInMergeConfig` 处理并发写入冲突

---

## 四、节点规范

每个节点遵循签名：

```go
func(ctx context.Context, state *AgentState) (*AgentState, error)
```

包装为 Eino Lambda：

```go
graph.AddLambdaNode("prepare_context", compose.InvokableLambda(prepareContext))
```

### 4.1 PrepareContext

**职责**：注入 rewrite、retrieve、memory 上下文，判定是否需要检索和搜索。

**输入**：`state.Question`、`state.KnowledgeBaseIDs`

**输出**：设置 `state.RewriteResult`、`state.RetrieveResult`、`state.MemoryContext`、`state.NeedRetrieval`、`state.KBInsufficient`

**逻辑**（从旧 `RagChatService.prepareChat` 提取）：
```go
func prepareContext(ctx context.Context, state *AgentState) (*AgentState, error) {
    // 1. rewrite（含 NeedRetrieval 判定）
    state.RewriteResult = rewriteService.Rewrite(ctx, state.Question)
    state.NeedRetrieval = state.RewriteResult.NeedRetrieval

    // 2. 长期记忆 recall
    state.MemoryContext = memoryRecallService.Recall(ctx, state.UserID, state.Question)

    // 3. retrieve（条件执行）
    if state.NeedRetrieval && len(state.KnowledgeBaseIDs) > 0 {
        state.RetrieveResult = retrieveService.Retrieve(ctx, ...)
        state.KBInsufficient = isKBInsufficient(state.RetrieveResult)
    }
    return state, nil
}
```

**图位置**：START → PrepareContext → PlanTools

### 4.2 PlanTools

**职责**：决定本轮调用哪些 tool。

**输入**：`state.Question`、`state.RetrieveResult`、`state.ToolResults`、`state.NextHints`、`state.ExecutedTools`

**输出**：设置 `state.PlannedCalls`

**决策层级**（继承旧系统分层降级策略）：

| 优先级 | 策略 | 条件 |
|---|---|---|
| 1 | nextHintCalls 复用 | Round ≥ 2 且 `state.NextHints` 非空 — 直接使用，零 LLM 调用 |
| 2 | baseRules 关键词路由 | Round = 1 且匹配已知关键词（诊断/查询/发现/搜索） |
| 3 | LLMPlanner | 以上都不满足，调 LLM |

```go
func planTools(ctx context.Context, state *AgentState) (*AgentState, error) {
    state.Round++
    state.PlannedCalls = nil

    // 优先级 1：复用上一轮的 next hints（消除 Planner 冗余调用）
    if len(state.NextHints) > 0 {
        state.PlannedCalls = hintsToCalls(state.NextHints, state.ExecutedTools)
        state.NextHints = nil
        return state, nil
    }

    // 优先级 2：baseRules 关键词路由
    if state.Round == 1 {
        if calls := routeByBaseRules(state.Question, state.KBInsufficient, state.RetrieveResult); len(calls) > 0 {
            state.PlannedCalls = calls
            return state, nil
        }
    }

    // 优先级 3：LLM Planner
    state.PlannedCalls = llmPlanner.Plan(ctx, PlanInput{
        Question:        state.Question,
        RetrieveResult:  state.RetrieveResult,
        PreviousResults: state.ToolResults,
        ExecutedTools:   state.ExecutedTools,
    })
    return state, nil
}
```

**图位置**：PrepareContext → PlanTools → ExecuteTools

### 4.3 ExecuteTools

**职责**：并发执行 tool call，收集结果。

**输入**：`state.PlannedCalls`

**输出**：追加 `state.ToolResults`、`state.AllToolCalls`、更新 `state.ExecutedTools`

**并行执行模型**（继承旧系统已验证的设计）：

```go
func executeTools(ctx context.Context, state *AgentState) (*AgentState, error) {
    if len(state.PlannedCalls) == 0 {
        return state, nil
    }

    // 按 ToolSpec.After 拓扑分层（确保 document_query 在 document_ingestion_diagnose 之前）
    levels := scheduleByAfter(state.PlannedCalls, moduleRegistry)
    for _, level := range levels {
        // 同层并行
        results := executeParallel(ctx, level, state)
        state.ToolResults = append(state.ToolResults, results...)
        for _, call := range level {
            state.ExecutedTools[call.Name] = true
            state.AllToolCalls = append(state.AllToolCalls, ToolCallSummary{...})
        }
    }
    return state, nil
}
```

**工具包装**：tools 以 Eino `InferTool[Input, Output]` 注册在 ModuleRegistry 中。ExecuteTools 通过 registry 查找 tool，调用 `InvokableRun`。

**SSE 事件**：`executeParallel` 内部发出 `tool_start` / `tool_result` SSE 事件。`tool_start` 按计划顺序发出，`tool_result` 完成后按序汇总。

**图位置**：PlanTools → ExecuteTools → Observe

### 4.4 Observe

**职责**：评估本轮结果，判定证据充分性，生成 next hints。

**输入**：`state.Question`、`state.ToolResults`、`state.ExecutedTools`、`state.RetrieveResult`

**输出**：设置 `state.EvidenceLevel`、`state.NextHints`、`state.Confidence`、`state.Degraded`

**决策层级**：

| 优先级 | 策略 | 条件 |
|---|---|---|
| 1 | 模块行为委托 | tool result 带有 module behavior 时，优先委托 |
| 2 | LLMObserver | 主路径 |
| 3 | RuleObserver | LLM 不可用 / 输出非法 JSON / 证据不一致 |

```go
func observe(ctx context.Context, state *AgentState) (*AgentState, error) {
    var result ObserveResult

    // 优先级 1：module behavior
    if result, ok := observeViaModuleBehaviors(state.ToolResults, state); ok {
        applyObserveResult(state, result)
        return state, nil
    }

    // 优先级 2：LLM Observer
    result, err := llmObserver.Observe(ctx, ObserveInput{
        Question:    state.Question,
        ToolResults: state.ToolResults,
    })
    if err == nil && validateEvidenceConsistency(result, state) {
        applyObserveResult(state, result)
        return state, nil
    }

    // 优先级 3：RuleObserver fallback
    result = ruleObserver.Observe(state.ToolResults, state)
    applyObserveResult(state, result)
    return state, nil
}
```

**图位置**：ExecuteTools → Observe → [Branch]

### 4.5 Branch（条件路由）

**职责**：根据 Observe 的输出选择下一节点。

```go
func routeAfterObserve(ctx context.Context, state *AgentState) (string, error) {
    maxRounds := adk.GetSessionValue(ctx, maxRoundsKey).(int)

    switch {
    case state.EvidenceLevel == "sufficient":
        return "generate_answer", nil
    case state.Round >= maxRounds:
        return "generate_degraded_answer", nil
    default:
        return "plan_tools", nil  // ← 循环回 PlanTools
    }
}
```

这是新旧架构最根本的差异：**循环不再由 Go `for` 语句驱动，而是由 Graph 的 branch 路由回环节点驱动**。Eino Graph 的 `WithMaxRunSteps` 提供框架级循环次数保护。

**图位置**：Observe → Branch → PlanTools | GenerateAnswer | GenerateDegradedAnswer

### 4.6 GenerateAnswer / GenerateDegradedAnswer

**职责**：基于累积证据生成最终回答。

**输入**：`state.Question`、`state.ToolResults`、`state.AllToolCalls`、`state.Confidence`

**输出**：设置 `state.FinalAnswer`

回答生成分为正常路径和降级路径：

```go
func generateAnswer(ctx context.Context, state *AgentState) (*AgentState, error) {
    state.FinalAnswer = buildAnswer(ctx, AnswerInput{
        Question:      state.Question,
        ToolResults:   state.ToolResults,
        RetrieveResult: state.RetrieveResult,
        MemoryContext: state.MemoryContext,
        GuidanceNotes: state.GuidanceNotes,
    })
    return state, nil
}

func generateDegradedAnswer(ctx context.Context, state *AgentState) (*AgentState, error) {
    state.FinalAnswer = buildDegradedAnswer(ctx, DegradedAnswerInput{
        Question:       state.Question,
        ToolResults:    state.ToolResults,
        DegradeReason:  state.DegradeReason,
        Confidence:     state.Confidence,
    })
    return state, nil
}
```

---

## 五、Tool 体系

### 5.1 双层架构

沿用 `internal/app/agent` 已验证的模式：

```
internal/app/agent/
├── search/          ← 业务内核（纯 Go，无框架依赖）
│   ├── service.go
│   ├── types.go
│   └── provider/   ← DuckDuckGo / Tavily / Tavily MCP
├── websearch/       ← Eino 工具包装（InferTool）
│   └── tool.go
├── fetch/
│   ├── service.go
│   ├── extract.go
│   └── types.go
├── webfetch/
│   └── tool.go
├── system/          ← 新增：诊断/查询 tool family
│   ├── service/     ← 从旧 invokers/system 提取业务逻辑
│   │   ├── document_query.go
│   │   ├── document_ingestion_diagnose.go
│   │   └── ...
│   ├── document_query_tool.go
│   ├── document_ingestion_diagnose_tool.go
│   └── ...
├── trace/           ← 新增
├── meta/            ← 新增（think）
└── graph/           ← 新增（Eino Graph tools → ADK SequentialAgent 适配）
```

### 5.2 ToolBehavior — 保留旧系统核心资产

ToolBehavior 的五个回调在新架构中仍然存在，但消费方式变了：

```go
type ToolBehavior struct {
    // 结果 → 类型化视图（InferTool 已覆盖此职责，保留为显式转换）
    Decode func(result any) (any, error)

    // 推导下一步 — 被 Observe 节点的 module behavior 委托路径消费
    Next func(output any, state *AgentState) NextDecision

    // 判断证据是否充足 — 被 Observe 节点消费
    Observe func(output any, state *AgentState) (ObserveResult, bool)

    // 结果 → LLM 可读文本 — 被 PlanTools（LLMPlanner prompt）消费
    RenderContext func(output any) string

    // 回答指导 — 被 GenerateAnswer 节点消费
    BuildGuidance func(output any, allResults []any) []GuidanceNote
}
```

与旧系统的差异：
- 参数从 `Result`（含 `map[string]any Data`）变为具体类型 `any`（由 InferTool 保证类型安全）
- `Next` 和 `Observe` 现在接收 `*AgentState`，可以访问完整上下文而不仅仅是当前 tool 的结果

### 5.3 ModuleRegistry

```go
type ModuleRegistry struct {
    modules map[string]*ToolModule
}

type ToolModule struct {
    Name     string
    Tool     einotool.InvokableTool    // Eino InferTool 实例
    Spec     ToolSpec                  // capability / risk / evidence / After
    Behavior ToolBehavior             // 业务语义回调
}
```

---

## 六、图的编译与运行

### 6.1 编译

```go
func NewAgentLoop(cfg AgentLoopConfig) (*AgentLoop, error) {
    graph := compose.NewGraph[*AgentState, *AgentState]()

    // 注册节点
    graph.AddLambdaNode("prepare_context", compose.InvokableLambda(prepareContext))
    graph.AddLambdaNode("plan_tools", compose.InvokableLambda(planTools))
    graph.AddLambdaNode("execute_tools", compose.InvokableLambda(executeTools))
    graph.AddLambdaNode("observe", compose.InvokableLambda(observe))
    graph.AddLambdaNode("generate_answer", compose.InvokableLambda(generateAnswer))
    graph.AddLambdaNode("generate_degraded_answer", compose.InvokableLambda(generateDegradedAnswer))

    // 注册边
    graph.AddEdge(compose.START, "prepare_context")
    graph.AddEdge("prepare_context", "plan_tools")
    graph.AddEdge("plan_tools", "execute_tools")
    graph.AddEdge("execute_tools", "observe")

    // 条件路由
    graph.AddBranch("observe", compose.NewGraphMultiBranch(routeAfterObserve, map[string]bool{
        "plan_tools":                true,
        "generate_answer":           true,
        "generate_degraded_answer":  true,
    }))

    graph.AddEdge("generate_answer", compose.END)
    graph.AddEdge("generate_degraded_answer", compose.END)

    // 编译
    runner, err := graph.Compile(context.Background(),
        compose.WithGraphName("agent_loop"),
        compose.WithMaxRunSteps(20),                      // 框架级安全阀
        compose.WithNodeTriggerMode(compose.AllPredecessor),
    )
    if err != nil {
        return nil, err
    }
    return &AgentLoop{runner: runner, cfg: cfg}, nil
}
```

### 6.2 运行

```go
func (a *AgentLoop) Run(ctx context.Context, req Request) (Response, error) {
    state := &AgentState{
        Question:         req.Question,
        UserID:           req.UserID,
        TraceID:          req.TraceID,
        KnowledgeBaseIDs: req.KnowledgeBaseIDs,
        ExecutedTools:    make(map[string]bool),
        ToolResults:      make([]ToolResult, 0),
    }

    final, err := a.runner.Invoke(ctx, state)

    return Response{
        Answer:    final.FinalAnswer,
        ToolCalls: final.AllToolCalls,
        Rounds:    final.Round,
        Degraded:  final.Degraded,
    }, err
}
```

### 6.3 Checkpoint 与恢复（后续迭代）

```go
// 启用 checkpoint 只需要编译时加一个 option
store := NewPostgresCheckpointStore(db)
runner, err := graph.Compile(ctx,
    compose.WithGraphName("agent_loop"),
    compose.WithMaxRunSteps(20),
    compose.WithCheckPointStore(store),      // ← 仅此一行
)
```

恢复时从 checkpoint 继续，不重跑已完成节点。对长链路诊断（4 跳以上）收益显著。

### 6.4 Human-in-the-Loop（后续迭代）

```go
// 在 tool 执行前暂停，等待人工审批
runner, err := graph.Compile(ctx,
    compose.WithInterruptBeforeNodes([]string{"execute_tools"}),
)
```

当 risk level = `high` 的 tool 被规划时，graph 在 ExecuteTools 前暂停，由外部系统审批后恢复。

---

## 七、SSE 事件与 Trace

### 7.1 SSE 事件

ExecuteTools 节点内部发出 SSE 事件，不改变事件契约：

```
tool_start  → { callId, round, name, summary, arguments }
tool_result → { callId, round, name, status, summary, data, duration }
agent_think → { round, planSource, plannedCalls, nextHints }
```

图节点的执行日志替代旧的 agent loop 日志：

```
[agent] round 1: plan=baseRules, calls=[document_ingestion_diagnose]
[agent] round 1: observe=LLM, evidence=insufficient, nextHints=[ingestion_task_query]
[agent] round 2: plan=hintReuse, calls=[ingestion_task_query]   ← 零 LLM
[agent] round 2: observe=LLM, evidence=sufficient
[agent] done: 2 rounds, 2 calls, confidence=0.95
```

### 7.2 Trace 节点

trace 节点类型保持不变（`agt_round` / `tool_call` / `agt_obs`），数据从 `AgentState.AllToolCalls` 提取，不再需要从 `WorkflowResult.Calls` 和 `RoundSummary` 两个来源拼凑。

---

## 八、与现有系统的关系

### 8.1 新旧并存策略

```
internal/app/rag/tool/   ← 维持现状，继续服务 RagChatService
internal/app/agent/      ← 新架构在此建设
```

新 agent 包在建设期间：
1. 不修改旧 tool 包的任何代码
2. 通过 `internal/app/agent/service.go` 暴露独立的调用入口
3. 先完成 M1（见下方里程碑），再逐步接管旧系统的调用方

### 8.2 业务代码复用策略

| 旧系统 | 复用方式 | 新系统位置 |
|---|---|---|
| `invokers/system/*.go` 工具逻辑 | 提取纯业务代码到 service 层 | `agent/system/service/` |
| `planner/planner.go` prompt 模板 | 直接复用，只改参数来源 | `agent/workflow/plan_tools.go` |
| `runtime/observer_llm_prompt.go` | 直接复用 | `agent/workflow/observe.go` |
| `runtime/base_rules.go` | 直接复用 | `agent/workflow/base_rules.go` |
| `modules/*/behavior.go` | 适配参数类型 | `agent/system/behavior.go` 等 |
| `modules/*/result_views.go` | 不再需要 — InferTool 输出直接是 typed struct | — |
| `runtime/evidence_validation.go` | 直接复用 | `agent/workflow/evidence_validation.go` |
| `invokers/web/*` search/fetch | 已在新包中重写（更简洁） | `agent/search/` + `agent/fetch/` |

---

## 九、实施路径

### M1：Graph Agent Loop 骨架（目标：图编译 + 空节点跑通）

- 定义 `AgentState` struct
- 实现 6 个节点的空壳（passthrough）
- 编译 graph，验证 `START → PrepareContext → PlanTools → ExecuteTools → Observe → [Branch] → GenerateAnswer → END` 拓扑正确
- 用 mock state 验证条件路由逻辑
- **此时不做任何 tool 执行和 LLM 调用**

### M2：接入真实 PrepareContext + PlanTools

- PrepareContext 接入 `infra-ai` 的 rewrite / retrieve / memory recall
- PlanTools 接入 baseRules（从旧系统复制关键词路由表），不做 LLMPlanner
- 此时新 agent 可以对"doc_fail_01 为什么导入失败"做出 tool 规划

### M3：接入真实 ExecuteTools + 2 个已有 tool

- ExecuteTools 接入 `web_search` / `web_fetch`（已有 InferTool 实现）
- 验证 graph node 内工具执行 + 结果写入 AgentState 的链路
- 此时新 agent 可以完整执行 `search → fetch → observe`

### M4：接入 LLMObserver + 完整 Observe 节点

- 从旧系统复制 prompt 模板和 evidence validation
- Observe 节点接入 LLM 调用
- Branch 节点按 EvidenceLevel 路由
- 此时可以跑通完整的最小闭环：Plan → Execute → Observe → [GenerateAnswer or loop]

### M5：迁移 system family（7 个 tool）

- 从旧的 `invokers/system/` 提取业务逻辑到 `agent/system/service/`
- 每个 tool 用 InferTool 包装
- 补 ToolBehavior（Next/Observe/RenderContext/BuildGuidance）
- 验证 doc_fail_01 / doc_run_01 端到端场景

### M6：全量 tool + 集成

- 迁移 trace family（trace_node_query、trace_retrieval_diagnose）
- 迁移 meta family（think）
- 适配 graph tools 为 ADK SequentialAgent
- SSE 事件发射 + trace 节点落库

### M7：切主

- `RagChatService` 新增 `SetAgentService(...)` 注入点
- 灰度：新 agent 与旧 AgentLoop 并行执行，对比输出
- 切换：旧 AgentLoop 降级为 fallback
- 最终下线旧 tool 包

---

## 十、风险评估

| 风险 | 等级 | 缓解 |
|---|---|---|
| Eino Graph cycle 节点内的状态并发写入 | 中 | 同层并行 tool 结果先收集再一次性写入 `state.ToolResults`，不在 goroutine 里直接写 |
| LLMObserver prompt 在新架构下需要重新校准 | 中 | 直接复用旧 prompt 文本，在新架构里 A/B 对比输出 |
| InferTool 的类型约束 vs 旧系统 `map[string]any` 的灵活性 | 低 | InferTool 的 typed Input/Output 覆盖了所有已知 tool 的需求场景。旧系统用 map 的历史原因是当时没有 InferTool |
| 图编译缓存失效导致每次请求重新编译 | 低 | 编译一次、缓存 runner（ingestion 服务已有此模式） |
| 旧 AgentLoop 的某些边缘场景未覆盖 | 中 | M7 切主时保留旧 AgentLoop 为 fallback 至少一个版本周期 |

---

## 十一、参考

- `docs/agent_loop_architecture_review.md` — 旧 AgentLoop 的深度评估（2026-05-15）
- `docs/tool_module_constraints.md` — Tool 模块改造约束（2026-05-18）
- `docs/project_progress_context.md` — 项目整体进度与当前能力矩阵
- [Eino Graph 文档](https://github.com/cloudwego/eino) — Graph API、Branch、CheckPoint、Interrupt
- [LangGraph Concepts](https://langchain-ai.github.io/langgraph/concepts/) — Typed State、Conditional Edges、Checkpointer
