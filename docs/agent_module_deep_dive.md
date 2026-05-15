# Agent 模块深度解析

> 日期: 2026-05-13

---

## 一、总览：Agent 在整个 RAG 管道中的位置

Agent 是 RAG 管道中的**第四阶段**，位于检索之后、LLM 生成之前。它的职责是：当知识库检索返回的结果不够充分，或用户问的是诊断类问题（"这个文档为什么导入失败了？"）时，自动调用一系列工具来收集更深层的证据。

```
HTTP GET /rag/v3/chat?question=...
  │
  ▼
RagChatService.Chat()
  │
  ├─ Stage 1: prepareChat()
  │   ├─ Conversation (创建/获取会话)
  │   ├─ Memory (加载历史消息)
  │   ├─ UserMessage (持久化用户问题)
  │   ├─ Rewrite (改写查询，拆分子问题)
  │   └─ Retrieve (多通道检索: vector, keyword, metadata...)
  │
  ├─ Stage 2: 置信度门控 (可选)
  │   如果 top chunk score < 阈值 → 清空知识上下文，注入 fallback prompt
  │
  ├─ Stage 3: ★ Agent Tool Workflow ★
  │   调用 AgentLoop.Run() → 多轮 Plan-Act-Observe
  │
  ├─ Stage 4: Prompt 构建
  │   合并: 原始问题 + 知识上下文 + 工具上下文 + 工作流策略 + 回答指导 + 历史
  │
  └─ Stage 5: LLM 流式生成 → SSE 推送
```

---

## 二、入口：Agent 如何被触发

### 2.1 注入点

Agent Workflow 在 `RagChatService` 上通过 `SetToolWorkflow()` 注入：

```go
// bootstrap 阶段
ragChatService.SetToolWorkflow(
    assembly.BuildLocalWorkflow(db, traceRunRepo, traceNodeRepo, cfg, chatService)
)
```

如果没有设置 Workflow（`s.toolWorkflow == nil`），整个 Agent 阶段被跳过，直接进入 Prompt 构建。

### 2.2 WorkflowInput：Agent 从上游接收什么

```go
s.toolWorkflow.Run(ctx, ragtool.WorkflowInput{
    Question:         "为什么 doc-123 导入失败了？",
    UserID:           "user-1",
    ConversationID:   "conv-1",
    TraceID:          "trace-abc",
    Control:          WorkflowControl{...},    // 初始为空
    KnowledgeBaseIDs: ["kb-1"],
    SearchMode:       "hybrid",
    History:          [...],                    // 对话历史
    RewriteResult:    {                         // 查询改写结果
        RewrittenQuestion: "doc-123 ingestion task 为什么 failed",
        SubQuestions: ["哪个节点失败了", "节点错误是什么"],
    },
    RetrieveResult:   {                         // 检索结果
        Chunks: [{ID, Text, Metadata, Score}],
        KnowledgeContext: "## 知识库检索结果\n...",
        SearchChannels: ["vector_global", "keyword"],
        ChannelStats: [{Name, ChunkCount, Error}],
    },
    EventSink:        sseChatSink,             // SSE 事件推送
})
```

---

## 三、Agent Loop 核心：Plan → Act → Observe

### 3.1 整体循环

```
AgentLoop.Run(ctx, input) → WorkflowResult

初始化:
  allResults   = []    // 所有轮次的所有工具结果
  allCalls     = []    // 所有调用摘要
  rounds       = []    // 每轮摘要
  degradeReasons = []  // 降级原因
  executed     = {}    // 已执行调用集合 (去重)
  agentState   = {}    // Agent 状态 (跨轮次)

for round = 1 to maxIterations (默认3):

  ┌─ Plan ──────────────────────────────────────────┐
  │ plannedCalls = planCalls(input, agentState,     │
  │                           allResults, executed)  │
  │                                                  │
  │ planCalls 内部:                                  │
  │   1. 尝试 LLMPlanner (如果已配置)                 │
  │   2. 如果 LLM 未返回结果 → 回退到 planWithRules   │
  │      2a. 如果有 NextHintCalls → 使用它们         │
  │      2b. 否则 → planWithBaseRules (关键词匹配)   │
  │      2c. 否则 → planCallsFromResults (链式推导)  │
  │                                                  │
  │ 如果 plannedCalls 为空 → break (停止循环)        │
  └──────────────────────────────────────────────────┘
                    │
  ┌─ Act ───────────────────────────────────────────┐
  │ roundResults = executeRoundCalls(plannedCalls)   │
  │                                                  │
  │ 串行或并行 (取决于 parallelToolCalls 配置):      │
  │   for each call:                                 │
  │     executor.Execute(call)                       │
  │       → registry.GetModule(call.Name)            │
  │       → module.Invoker.Invoke(ctx, call)         │
  │       → merge ResultMeta                         │
  │       → log + eventSink.OnToolStart/OnToolResult │
  │                                                  │
  │ 结果追加到 allResults, allCalls                  │
  │ executed[callKey(call)] = {}                     │
  └──────────────────────────────────────────────────┘
                    │
  ┌─ Observe ───────────────────────────────────────┐
  │ observation = observer.Observe(observeInput)     │
  │                                                  │
  │ LLMObserver (如果已配置):                        │
  │   1. 构建 system prompt (工具定义 + 8个示例)     │
  │   2. 构建 user prompt (问题 + 改写 + 检索 +      │
  │      状态 + 当前轮结果 + 历史)                    │
  │   3. 调用 LLM → 解析 JSON                       │
  │   4. 验证 NextHintCalls 中的 ID (防幻觉)         │
  │   5. 失败/解析错误 → 回退到 RuleObserver         │
  │                                                  │
  │ RuleObserver (确定性回退):                       │
  │   1. 取最后一个非 think 结果                     │
  │   2. 查 registry 中的 module Behavior.Observe    │
  │   3. 查 inferLegacyBehavior 的 Observe           │
  │   4. 检查 diagnosisDepth (node_level /           │
  │      task_level / 默认)                           │
  │                                                  │
  │ observation = {Done, Reasoning, NextHintCalls,   │
  │                NextHint, Confidence, State}       │
  │                                                  │
  │ 如果观察失败 → Done=true, 停止                   │
  │ 如果 observation.Done → break (停止循环)         │
  │ 否则 → agentState = observation.State (继续)     │
  └──────────────────────────────────────────────────┘
                    │
              下一轮 (round++)
```

### 3.2 AgentState：跨轮次的状态传递

AgentState 是 Agent Loop 的**工作记忆**，在每轮 Plan 和 Observe 之间传递：

```go
type AgentState struct {
    Phase         string     // 当前阶段标签
    Hypothesis    string     // 当前假设
    Confidence    float64    // 置信度 0.0-1.0
    OpenQuestions []string   // 尚未解答的问题
    CheckedTools  []string   // 已检查的工具列表
    NextHintCalls []HintCall // 下一轮建议的工具调用 (结构化)
    NextHint      string     // 下一轮建议的工具调用 (传统字符串格式)
}
```

**Phase 的典型流转**：

```
"" (空，第1轮开始)
  → "initial_diagnosis" (planWithBaseRules 触发了诊断)
  → "deep_dive" (observer 判断需要更深层的证据)
  → "verification" (observer 发现任务还在 running，转为验证模式)
  → "external_search" (KB 检索不足，触发外部搜索)
  → "fetching" (web_search 完成，准备抓取网页)
  → "complete" (证据充足，可以回答)
```

**状态如何在轮次间传递**：

```
Round 1:
  agentState = {} (空)
  planCalls 使用 agentState.NextHintCalls (空) → 回退到 planWithBaseRules
  → 计划: [document_root_cause_diagnosis]
  → 执行
  → observer 看结果: document 状态是 failed, latestTaskId=task-1
    → NextHintCalls = [{ingestion_task_query, {taskId: task-1}}]
    → Phase = "deep_dive"
    → Confidence = 0.72
    → Done = false

Round 2:
  agentState = {Phase: "deep_dive", NextHintCalls: [...]}
  planCalls 使用 agentState.NextHintCalls
  → 计划: [ingestion_task_query]
  → 执行: 得到 taskNodeSummary [{nodeId: indexer, status: failed}]
  → observer: 有 failed node 但没有具体错误
    → NextHintCalls = [{ingestion_task_node_query, {taskId: task-1, nodeId: indexer}}]
    → Phase = "deep_dive"
    → Done = false

Round 3:
  agentState = {Phase: "deep_dive", NextHintCalls: [...], CheckedTools: [document_root_cause_diagnosis, ingestion_task_query]}
  planCalls 使用 agentState.NextHintCalls
  → 计划: [ingestion_task_node_query]
  → 执行: 得到 errorMessage = "connection refused: vector store unavailable"
  → observer: node_error_found
    → Done = true
    → Confidence = 1.0
    → Phase = "complete"
```

---

## 四、Plan 阶段：如何决定调用什么工具

### 4.1 三层回退

```
planCalls()
  │
  ├─ 第1层: LLM Planner
  │   如果 chatService 已配置 → planner.Plan(planInput)
  │   输入: 工具定义列表 + 问题 + agentState + 上一轮结果 + KB scope
  │   输出: PlanResult{Calls: [...]} 或 PlanResult{} (空)
  │   ↓ 验证: validateCallAgainstEvidence (ID 必须来自问题/上一轮hint/上一轮结果)
  │   ↓ 去重: 跳过 executed 集合中已执行的调用
  │
  ├─ 第2层: planWithRules (规则回退，LLM 返回空时触发)
  │   ├─ 如果 agentState.NextHintCalls 非空
  │   │   → PlanCallsFromHintCalls(hintCalls, defs)
  │   │   将 hint 转换为经过参数校验的 Call
  │   │
  │   └─ 如果 hint 转换失败 (参数校验不通过)
  │       → planCallsFromResultsWithRegistry(previousResults, input, registry)
  │       遍历每个 result 的 Behavior.Next，链式推导下一步
  │
  └─ 第3层: planWithBaseRules (无 hint、无 behavior 时的最后回退)
      从用户问题中提取 documentId / taskId / traceId (正则)
      根据中文/英文关键词选择工具:
        "失败"/"failed" + "解决"/"fix" → document_diagnose_with_search
        "失败"/"failed" (无 "fix")     → document_root_cause_diagnosis
        "chunk log"/"ingestion"        → document_chunk_log_query
        无诊断关键词                     → document_query (基础查询)
        "哪些"/"list" + "document"     → document_list
        "哪些"/"list" + "task"         → task_list
        KB 检索无结果                    → external_evidence_workflow
```

### 4.2 LLM Planner 的 Prompt 结构

`planner/planner.go` 构建的 prompt 包含：

```
System: You are the planning phase of an agentic diagnostic workflow.
  Available tools: [工具名字 + 描述 + 参数schema]
  Rules: 1-7 (不要编造ID, 最多3个调用, 串行避免同一entity等)

User:
  User question: <问题>
  Rewrite context: rewrittenQuestion=..., subQuestions=...
  Retrieve context: searchChannels=..., channelStats=..., topChunks=...
  Previous agent state: {phase, hypothesis, confidence, checkedTools, nextHintCalls}
  Knowledge base scope: kb-1
  [如果有上轮结果] Previous tool results: ...

Return strict JSON: {"calls": [{"name": "...", "arguments": {...}}]}
```

### 4.3 工具调用去重

`executed` 集合记录本轮 Agent Loop 中已经成功提交过的调用。Key 的计算：

```go
callKey(call) = call.Name + ":" + json.Marshal(sortedArguments)
```

这意味着：**同一个工具 + 完全相同的参数**不会被重复调用。但是同一个工具 + 不同参数（比如 `ingestion_task_node_query` 分别查 `nodeId=indexer` 和 `nodeId=parser`）会被视为不同的调用。

---

## 五、Act 阶段：工具如何执行

### 5.1 Executor 执行流程

```go
executor.Execute(ctx, call):
  1. call.Validate() → 校验 name 非空
  2. registry.GetModule(call.Name) → 查找工具模块
  3. module.Invoker.Invoke(ctx, call) → 执行工具
  4. 填充 result.Name (如果为空，用模块定义中的 name)
  5. 填充 result.Status (根据 err 设置 success/failed)
  6. 填充 result.ErrorMessage (如果 err != nil 且 message 为空)
  7. 合并 result.Meta ← module.Spec.resultMeta() (工具元数据)
  8. 记录日志: [tool] <name> success/failed (<duration>ms)
```

### 5.2 工具的 Result 结构

```go
type Result struct {
    Name         string            // 工具名
    Status       string            // "success" / "failed" / "skipped"
    Summary      string            // 一句话摘要
    Data         map[string]any    // 结构化数据 (文档状态, 诊断结论, 搜索结果等)
    ErrorMessage string            // 失败时的错误信息
    Meta         ResultMeta        // 能力标签 (Capability, RiskLevel, 等)
}
```

**Data 字段是工具间传递信息的核心载体**。每个工具定义了自己的 Data schema：

| 工具 | Data 关键字段 |
|------|-------------|
| `document_query` | documentId, status, processMode, chunkCount, fileSize, fileURL |
| `document_ingestion_diagnose` | documentId, conclusion, confidence, latestTaskId, latestNodeId, latestNodeError, diagnosisDepth, evidence |
| `ingestion_task_query` | taskId, status, taskNodeSummary[{nodeId, status}], nodes[...] |
| `ingestion_task_node_query` | taskId, nodeId, status, errorMessage, summary |
| `web_search` | results[{title, url, snippet, policy, sourceType}], resultCount |
| `web_fetch` | pages[{url, text, wasTruncated}], successCount, failedCount |
| `trace_node_query` | traceId, nodes[{nodeId, nodeName, nodeType, status, input, output, errorMessage}] |

### 5.3 并发执行

```go
// 串行模式 (默认)
for idx := range items {
    items[idx] = executeSingleCall(ctx, round, items[idx])
}

// 并行模式 (ParallelToolCalls=true 且 MaxConcurrency>1 且 plannedCalls>1)
sem := make(chan struct{}, maxConcurrency)
for idx := range items {
    go func(i int) {
        sem <- struct{}{}  // 获取许可
        defer func() { <-sem }()
        items[i] = executeSingleCall(ctx, round, items[i])
    }(idx)
}
wg.Wait()
```

并行只在一个 round 内生效，rounds 之间永远是串行的（observer 必须等待所有工具完成）。

---

## 六、Observe 阶段：如何判断是否结束

### 6.1 Observer 决策流程

```
observer.Observe(observeInput) → ObserveResult

observeInput 包含:
  Question: 用户原始问题
  Round: 当前轮次
  Results: 所有历史结果 (跨轮累积)
  RoundResults: 当前轮新产生的结果
  PreviousState: 上一轮的 AgentState
  MaxIterations: 最大轮次
  ReachedMaxLoop: 是否已到上限
  ToolDefinitions: 工具列表
  ToolRegistry: 工具注册表
  KnowledgeBaseIDs: KB范围
  RewriteResult + RetrieveResult: 上游上下文
```

### 6.2 LLM Observer 的决策逻辑

LLM Observer 的 system prompt 包含 8 个精心设计的示例，覆盖以下场景：

```
示例1: 证据充足 (node-level error 已获取) → done=true, confidence=0.95
示例2: 证据不够深 (只有 task-level) → done=false, hint: ingestion_task_query
示例3: 有 failed node 但无具体错误 → done=false, hint: ingestion_task_node_query
示例4: task 仍在 running → done=false, 不要过度诊断为失败
示例5: 多个独立查询可并行 → done=false, 2个 hint
示例6: KB 检索无结果 → done=false, hint: web_search
示例7: web_search 完成 → done=false, hint: web_fetch (抓取详情)
示例8: web_fetch 完成, 内容充足 → done=true, confidence=0.8
```

**关键防幻觉机制**：`validateHintAgainstEvidence()`

Observer 返回的 NextHintCalls 中如果包含 documentId / taskId / nodeId / traceId，这些 ID 必须出现在以下白名单中：
- 用户问题中提取的 ID (正则)
- 上一轮 NextHintCalls 中的 ID
- 所有工具执行结果 Data 中的 ID

如果 ID 不在白名单中 → LLM 响应被拒绝，回退到 RuleObserver。

### 6.3 Rule Observer 的回退链

```
RuleObserver.Observe():
  1. RoundResults 为空? → Done=true
  2. ReachedMaxLoop? → Done=true
  3. lastNonThinkResult() → 跳过 think 工具，看最后一个实际结果
  4. observeWithRegistry() → 查 registry 中该工具的 Behavior.Observe
  5. inferLegacyBehavior().Observe → 查兼容行为的 Observe
  6. diagnosisDepth fallback:
     "node_level"  → Done=true, confidence=0.95
     "task_level"  → Done=true, confidence=0.75
     其他          → Done=true, confidence=0.6
```

### 6.4 特定工具的 Observe 决策

以 `observeDocumentDiagnosis` 为例，根据 `nextAction` 返回的 reason 做不同决策：

```
reason="node_error_found"
  → Done=true, confidence=1.0
  → "文档诊断已经包含节点级错误，Agent 可以直接回答"

reason="failed_node_located"
  → Done=false, confidence=0.72
  → hint: ingestion_task_node_query
  → "定位到了失败节点，但还需要节点的具体错误详情"

reason="task_level_error_only"
  → Done=false, confidence=0.58
  → hint: ingestion_task_query
  → "只有任务级错误摘要，需要深入查看任务详情"

reason="still_running_or_inconsistent"
  → Done=false, confidence=0.45
  → hint: ingestion_task_query
  → "任务仍在运行或状态不一致，转为验证模式"

default (confidence="high")
  → Done=true
  → "已经获得高置信度结论"
```

---

## 七、工具链式编排：Behavior.Next

### 7.1 诊断链

工具之间的链式关系通过 `ToolBehavior.Next` 定义：

```
document_query (pipeline + failed)
  → Next: document_ingestion_diagnose(documentId)

document_ingestion_diagnose (found failed node)
  → Next: ingestion_task_node_query(taskId, nodeId)

ingestion_task_query (found failed/running node)
  → Next: ingestion_task_node_query(taskId, nodeId)

web_search (found results)
  → Next: web_fetch(urls)

document_chunk_log_query (abnormal + has taskId)
  → Next: ingestion_task_query(taskId)
```

### 7.2 为什么用 Behavior.Next 而不是让 Observer 决定？

Observer 的决策依赖 LLM（有延迟和不确定性），而 `Behavior.Next` 是**确定性的**：当工具返回了某种特定的结构化数据，下一步的操作是固定的。比如，`document_query` 返回 `status=failed, processMode=pipeline` → 下一步一定是 `document_ingestion_diagnose`。这种确定性推导不需要 LLM，响应更快且不会出错。

---

## 八、结论如何达成：从工具结果到最终回答

### 8.1 上下文构建

Agent Loop 结束后，`WorkflowResult` 包含：

```
WorkflowResult {
    Used:           true/false              // 是否有工具被调用
    Context:        "### document_ingestion_diagnose\n..."  // 工具结果渲染文本
    AnswerGuidance: "按'结论/证据/建议'组织回答\n当前结论：..."  // 回答结构指导
    Control:        {Capability, ExecutionMode, RiskLevel} // 工作流能力标签
    TraceMeta:      {Capability, EvidenceSources, ...}     // trace 元数据
    Calls:          [{Round, Name, Status, Summary, DurationMs}] // 所有调用摘要
    Rounds:         [{Round, Done, Reasoning, Confidence, State}] // 每轮摘要
    Degraded:       true/false             // 是否有降级
    DegradeReason:  "..."                  // 降级原因
}
```

### 8.2 Context 如何生成

`RenderContextWithRegistry()` 遍历所有工具结果：

```markdown
### document_ingestion_diagnose
document ingestion failed at node indexer, confidence=high
Conclusion: indexer failed with connection refused
Diagnosis depth: node_level
Chain length: 3
Latest task/node: task-1 / indexer

### web_search
Search results:
1. How to fix vector store connection refused (https://example.com/1) [example.com, policy=allow, type=official_docs]: ...
2. Vector DB troubleshooting guide (https://example.com/2) [example.com, policy=allow]: ...

### web_fetch
Fetched web content:
Connection refused errors typically mean the vector store is not running or not reachable...
```

### 8.3 AnswerGuidance 如何生成

`BuildAnswerGuidanceWithRegistry()` 采用**首个匹配胜出**的策略：

```
1. 诊断类型结果? (document_ingestion_diagnose / task_ingestion_diagnose / trace_retrieval_diagnose)
   → 中文引导: "按[结论 / 证据边界 / 建议]的顺序组织回答"
   → enrichDiagnosisWithDeeperEvidence() 交叉验证更深层证据
   → 如果深层证据与诊断结论不一致 → 修正置信度

2. 外部证据工作流? (external_evidence_workflow)
   → 引导: "先给系统内诊断结论，外部搜索只提供补充参考"
   → 标注来源 URL, 证据质量, 覆盖度

3. Web 搜索/抓取?
   → 引导: "按[来源 / 核心发现 / 局限性]组织回答，标注来源URL"

4. 上述都不匹配
   → 返回空字符串 (无特殊指导)
```

### 8.4 最终 Prompt 的合并

```go
promptService.BuildMessages({
    Question:         "为什么 doc-123 导入失败了？",
    KnowledgeContext: "## 知识库检索结果\n...",      // 来自 Retrieve 阶段
    ToolContext:      "### document_ingestion_diagnose\n...", // Agent 生成
    WorkflowPolicy:   "能力域: diagnosis\n执行模式: read_only\n...", // Agent 生成
    AnswerGuidance:   "按[结论/证据/建议]的顺序组织回答...", // Agent 生成
    History:          [...],
})
```

这些信息按后缀叠加到 system prompt 后面，最终发给 LLM。

---

## 九、错误处理与降级

### 9.1 降级层级

```
LLM Planner 返回空 → planWithRules (规则回退)
LLM Observer 失败/解析失败 → RuleObserver (规则回退)
RuleObserver 无匹配 → diagnosisDepth fallback (通用回退)
单个工具调用失败 → degradeReasons 记录，Agent 继续
Agent Loop 达到 maxIterations → Done=true + degrade 标记
Workflow.Run() 整体报错 → 构建空 WorkflowResult, pipeline 继续
```

### 9.2 关键容错点

1. **Tool 不存在** → executor 返回 `status=failed, errorMessage="tool not found"`，不中断循环
2. **Tool 执行超时** → 依赖 HTTP client timeout (30s)，返回 error
3. **Observer LLM 返回非法 JSON** → 尝试提取 \`\`\`json 块 → 失败则回退 RuleObserver
4. **Observer 编造 ID** → `validateHintAgainstEvidence` 白名单校验 → 拒绝，回退
5. **Panic 恢复** → goroutine 级别的 `recover()`，记录日志

---

## 十、需要注意的细节

### 10.1 `planWithBaseRules` 的关键词匹配是最后兜底

关键词列表硬编码了中英文各约 30 个词。它只在 LLM Planner 返回空 AND agentState.NextHintCalls 为空 AND behavior.Next 无链式推导时才触发。正常情况下，LLM Planner 或 behavior.Next 就已经决定了调用链。

### 10.2 `executed` 去重只去完全相同的调用

如果第 2 轮调用了 `ingestion_task_node_query(taskId=task-1, nodeId=indexer)`，第 3 轮 Observer 又建议同一个调用，会被去重。但如果参数不同（比如查不同的 nodeId），不会被去重。

### 10.3 think 工具被 Observer 跳过

`lastNonThinkResult()` 确保 Observer 基于最后一个**非 think** 的工具结果做决策。think 工具只是推理记录，不影响 Agent 的决策逻辑。

### 10.4 两种 NextHint 格式共存

- `NextHintCalls []HintCall` — 结构化格式：`[{Name: "web_search", Arguments: {query: "..."}}]`
- `NextHint string` — 传统字符串格式：`"tool:web_search|query=..."`

`AgentState.Normalize()` 保持两者同步：从 `NextHintCalls` 序列化到 `NextHint`（只保留第一个），从 `NextHint` 解析到 `NextHintCalls`。

### 10.5 Observer State 与 observation 字段的合并逻辑

Agent Loop 中有复杂的字段合并（`agent_loop.go:162-183`）：
- 如果 `observation.State` 为空，从 `observation` 的基本字段构建
- `Confidence` 取 `observation.Confidence` 和 `observation.State.Confidence` 中第一个非零值
- `NextHint` 取 `observation.State.NextHint` 和 `observation.NextHint` 中第一个非空值
- `NextHintCalls` 同理

这种合并是因为 LLM Observer 返回的 JSON 中，`done/reasoning/nextHintCalls/confidence` 在顶层，而 `state` 是嵌套对象。两个层级都可能有值，需要统一。

### 10.6 WorkflowControl 的推导

`deriveWorkflowControl()` 根据所有工具结果的 Meta 来推导整体的能力级别：

```
如果任一结果 Meta.Capability = "search" → 整体 Capability = "search"
否则如果任一 = "diagnosis" → 整体 Capability = "diagnosis"
否则如果任一 = "knowledge" → 整体 Capability = "knowledge"
否则如果有检索结果 → "knowledge"
否则 → "general"
```

同理推导 `ExecutionMode`, `RiskLevel`, `ApprovalRequirement`。这些信息注入到最终 prompt 的 `WorkflowPolicy` 中，影响 LLM 的回答风格。

### 10.7 工具间的 data 传递

工具的 Data 字段 (`map[string]any`) 是弱类型的。前一个工具产出的 data 通过 `NextHintCalls` 的 Arguments 传递给下一个工具。例如：

```
document_ingestion_diagnose 产出:
  Data: { "latestTaskId": "task-1", "latestNodeId": "indexer" }

Behavior.Next 从中提取 taskId 和 nodeId:
  → HintCall{Name: "ingestion_task_node_query", Arguments: {taskId: "task-1", nodeId: "indexer"}}

Agent Loop 将 HintCall 转换为 Call:
  → Call{Name: "ingestion_task_node_query", Arguments: {taskId: "task-1", nodeId: "indexer"}}

Executor 调用:
  → ingestion_task_node_query.Invoke(ctx, Call{..., Arguments: {taskId: "task-1", nodeId: "indexer"}})
```

这个传递链条依赖 Behavior.Next 正确地从前一个 Result.Data 中提取字段。如果字段名变了，链条就断了。

---

## 十一、完整示例：一次诊断的完整执行

```
用户问题: "doc-fail-01 为什么失败了？有没有修复方案参考？"

Round 1:
  Plan: planCalls → planWithLLM 返回空 → planWithBaseRules
    从问题提取 documentId="doc-fail-01"
    关键词 "失败了" + "修复方案" → 触发 document_diagnose_with_search

  Act: executor.Execute({Name: "document_diagnose_with_search", Arguments: {documentId: "doc-fail-01"}})
    内部是 Eino 编译图:
      document_root_cause_diagnosis(doc-fail-01)
        → document_ingestion_diagnose(doc-fail-01)
          → ingestion_task_query(task-1)
            → ingestion_task_node_query(task-1, indexer)
        → 结果: conclusion="indexer failed: connection refused to vector store"
      web_search("connection refused troubleshooting guide")
        → 找到 3 个结果
      → 合并结果: 系统诊断 + 外部搜索

  Observe: LLMObserver 分析
    有 node_level 错误 + web 搜索结果 → Done=true, confidence=0.95

Agent Loop 结束.

WorkflowResult:
  Context: "### document_diagnose_with_search\n..."
  AnswerGuidance: "这是一次诊断+搜索结果。请优先用中文，先给出系统内诊断结论..."
  Control: {Capability: "search", RiskLevel: "low", ...}
  Degraded: false

最终 Prompt:
  System: "你是知识库助手..."
  + KnowledgeContext: (来自检索)
  + ToolContext: (Agent 生成的上下文)
  + AnswerGuidance: "按[结论/证据边界/建议]组织回答. 当前结论: indexer failed..."
  + User: "doc-fail-01 为什么失败了？有没有修复方案参考？"
```

---

## 十二、数据流全图

```
                                     ┌─────────────┐
                                     │  LLM Model  │
                                     │  (final     │
                                     │   answer)   │
                                     └──────▲──────┘
                                            │ SSE streaming
┌──────────┐  question  ┌──────────┐  ┌─────┴─────────────┐
│  HTTP    │───────────►│ RagChat  │  │ Prompt Builder    │
│  Handler │            │ Service  │  │ • KnowledgeContext │
└──────────┘            └────┬─────┘  │ • ToolContext      │
                             │        │ • AnswerGuidance   │
                    ┌────────▼──────┐ │ • WorkflowPolicy   │
                    │  Retrieve     │ │ • History          │
                    │  (RAG search) │ └────────────────────┘
                    └───────┬───────┘           ▲
                            │ chunks            │
                    ┌───────▼───────────────────────┐
                    │     AgentLoop.Run()            │
                    │                                │
                    │  ┌── Plan ──────────────────┐  │
                    │  │ LLMPlanner → planWithRules│  │
                    │  │ → planWithBaseRules       │  │
                    │  └──────────┬───────────────┘  │
                    │             │ calls             │
                    │  ┌──────────▼───────────────┐  │
                    │  │ Executor.Execute()        │  │
                    │  │ → 16 tools in Registry    │  │
                    │  └──────────┬───────────────┘  │
                    │             │ results           │
                    │  ┌──────────▼───────────────┐  │
                    │  │ Observer (LLM → Rule)     │  │
                    │  │ → Done? / NextHintCalls   │  │
                    │  └──────────┬───────────────┘  │
                    │             │                   │
                    │      ┌──────▼──────┐            │
                    │      │ agentState   │───────────┼──► WorkflowResult
                    │      │ (next round) │            │    • Context
                    │      └─────────────┘            │    • AnswerGuidance
                    └────────────────────────────────┘    • Control
                                                          • Calls/Rounds
```
