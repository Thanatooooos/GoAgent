# Agentic Chat 开发文档

版本：v1.1  
日期：2026-05-09

---

## 一、目标

将当前聊天链路从“单次 RAG（改写→检索→静态 tool 调用→生成）”逐步升级为 **Agentic RAG（Plan → Act → Observe 循环）**。

当前已经落地 **AgentLoop V1**，目标不再是一次性实现完整自主 Agent，而是分阶段推进：

1. **V1 已完成**：先做可循环、可终止、可观测的最小 Agent 闭环
2. **V2 待推进**：提升增量规划质量，减少重复调用和无效下钻
3. **V3 待评估**：再决定是否引入 LLM observer、前端完整思考卡片和更强的 workflow 模板化

---

## 二、开发约束

### 2.1 硬约束

1. **不改写 `RagChatService` 的业务外壳**。`Chat(ctx, input, sink)` 的签名与整体流水线结构不变，仅替换内部 tool stage 的行为。
2. **不破坏现有 SSE 协议兼容性**。已有的 `meta / message / thinking / finish / done / cancel / error` 事件语义不变。新增 `agent_think / tool_start / tool_result` 事件，前端未实现时应可安全忽略。
3. **Tool 接口不变**。`Tool.Definition() / Invoke()` 保持现有契约。
4. **所有 tool 调用必须只读**。Agent 循环内禁止执行任何有副作用的 tool。
5. **必须有迭代上限**。当前默认 `maxIterations=3`，超限后强制终止循环并基于已有结果生成回答。

### 2.2 软约束

- 优先复用而非重写。`LLMPlanner`、`Executor`、`Registry`、`RenderContext`、`AnswerGuidance` 全部保留。
- 首版优先保证“主链可跑、可验证、可解释”，而不是一步到位追求强自主。
- 不引入新的第三方依赖。

---

## 三、当前状态

### 3.1 已完成：AgentLoop V1

当前已经从单次 pass 模式升级到最小多轮 agent 模式：

```text
Question
  -> Planner
  -> Tool Calls
  -> Rule Observer
  -> Planner(下一轮，可选)
  -> Tool Calls
  -> RenderContext / AnswerGuidance
  -> Prompt
  -> LLM 生成
```

V1 的核心能力：

- `tool workflow` 已支持多轮 `Plan -> Act -> Observe`
- `PlanInput` 已支持：
  - `AgentState`
  - `PreviousResults`
- 已支持实时事件：
  - `tool_start`
  - `tool_result`
  - `agent_think`
- trace 已支持：
  - `agent_round`
  - `tool_call`
  - `agent_observation`

### 3.2 当前边界

V1 不是完整自主 Agent，而是更偏 **结构化诊断 agent**。

当前最适合的场景：

- `documentId` 已知的 ingestion 排障
- `taskId` 已知的 task / node 排障
- `traceId` 已知的 retrieval 诊断

当前不追求的场景：

- 高开放度、多意图、强模糊问题
- 复杂长链自主规划
- 全面依赖 LLM 的 observation 决策

---

## 四、当前实现范围

### 4.1 已新增文件

| 文件 | 职责 |
|---|---|
| `internal/app/rag/tool/agent_loop.go` | AgentLoop 控制器：多轮 `Plan -> Act -> Observe` |
| `internal/app/rag/tool/observer_rule.go` | V1 规则版 Observer |
| `internal/app/rag/tool/agent_loop_test.go` | AgentLoop 多轮循环与事件测试 |

### 4.2 已修改文件

| 文件 | 改动内容 |
|---|---|
| `internal/app/rag/tool/workflow.go` | 扩展 `PlanInput / WorkflowInput / WorkflowResult`，增加事件与 round 结构 |
| `internal/app/rag/tool/planner/planner.go` | planner 接入 `AgentState / PreviousResults` |
| `internal/app/rag/service/rag_chat_service.go` | `runToolWorkflowStage` 接入 AgentLoop 和 workflow event sink |
| `internal/app/rag/service/chat_tracer.go` | trace 增加 `agent_round / tool_call / agent_observation` |
| `internal/adapter/http/rag/handlers.go` | `sseChatSink` 实现 `agent_think / tool_start / tool_result` |
| `internal/bootstrap/rag/runtime.go` | runtime 默认装配 `AgentLoop` |
| `frontend/src/types/index.ts` | 扩展 tool event 类型 |
| `frontend/src/hooks/useStreamResponse.ts` | 增加新 SSE 事件分发 |
| `frontend/src/stores/chatStore.ts` | `toolCalls` 支持按 `callId` 增量更新 |
| `frontend/src/components/chat/MessageItem.tsx` | 最小展示轮次、参数、耗时和运行态 |

### 4.3 明确未做

以下内容当前 **没有** 落地，仍属于后续范围：

- `LLM observer`
- 复杂 workflow 模板库
- 完整 Agent 思考卡片 UI
- tool 并发调度与更细粒度超时控制
- Agent 参数配置化

---

## 五、V1 详细设计

### 5.1 AgentLoop 控制器

```go
type AgentLoop struct {
    executor      *Executor
    planner       Planner
    observer      Observer
    maxIterations int
}
```

当前实现特征：

- planner 优先，rule fallback 兜底
- observer 为规则版
- 已执行过的等价 call 会去重，避免循环空转
- 每轮产出：
  - call summaries
  - observation
  - next hint

### 5.2 Observer：当前是规则版

当前不是 LLM observer，而是 `RuleObserver`。

它主要根据已有 tool 结果判断：

- 是否已经足够回答
- 是否需要继续查询 `ingestion_task_query`
- 是否需要继续查询 `ingestion_task_node_query`
- 是否只需结束循环

当前重点覆盖：

- `document_ingestion_diagnose`
- `task_ingestion_diagnose`
- `document_query`
- `document_chunk_log_query`
- `ingestion_task_query`

### 5.3 Planner 增量输入

当前 `PlanInput` 已扩展为：

```go
type PlanInput struct {
    Question        string
    ToolDefinitions []Definition
    AgentState      string
    PreviousResults []Result
}
```

当前的增量规划仍然是轻量版本：

- `AgentState` 主要来自上一轮 observer 的 `NextHint`
- `PreviousResults` 以摘要形式注入 planner prompt
- 还没有专门的 few-shot 和复杂路由模板

### 5.4 SSE 事件

当前新增事件如下：

```text
event: agent_think
data: {"message":"文档诊断已经定位到失败节点，但还缺少节点详情，继续查询任务节点。"}

event: tool_start
data: {"callId":"round_1_call_01","round":1,"sequence":1,"name":"document_ingestion_diagnose","status":"running","arguments":{"documentId":"doc-1"}}

event: tool_result
data: {"callId":"round_1_call_01","round":1,"sequence":1,"name":"document_ingestion_diagnose","status":"success","summary":"...","durationMs":12,"arguments":{...},"data":{...}}
```

兼容性说明：

- 原有 `tool` 摘要事件仍保留
- 当前主要依赖 `tool_start / tool_result` 做更实时的前端展示

### 5.5 Trace 结构

当前 trace 结构已经从：

```text
tool_workflow
  -> tool_call
```

扩展为：

```text
tool_workflow
  -> agent_round
    -> tool_call
    -> tool_call
    -> agent_observation
```

每个 `tool_call` 节点当前额外记录：

- `callId`
- `round`
- `sequence`
- `summary`
- `durationMs`
- `arguments`
- `data`（若存在）

---

## 六、典型场景

### 场景：用户问“doc_abc 为什么导入失败了？”

一个典型的 V1 行为大致如下：

```text
轮次 1:
  Plan    -> planner 选择 document_ingestion_diagnose
  Act     -> 返回 latestTaskId=task_xxx, latestNodeId=indexer
  Observe -> 判断：还缺少节点详情，继续

轮次 2:
  Plan    -> planner 或 rule hint 选择 ingestion_task_node_query(task_xxx, indexer)
  Act     -> 返回具体 node error
  Observe -> 判断：信息充分，结束

最终：
  ToolContext = 多轮结果合并
  AnswerGuidance = 诊断引导
  LLM 组织最终回答
```

预期收益：

- 比单次 workflow 更像“逐步排查”
- 用户能看到每轮查了什么
- 最终回答更容易落到“根因 + 证据 + 建议”

---

## 七、当前验收状态

### 7.1 已达到

- [x] tool workflow 已支持多轮循环
- [x] 支持 `AgentState / PreviousResults`
- [x] 支持 `tool_start / tool_result / agent_think`
- [x] trace 已支持 `agent_round / agent_observation`
- [x] 后端测试通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
```

### 7.2 尚未完全验证

- [ ] **前端构建**：尝试执行 `vite build` 时，当前本地 Node 运行环境因权限问题访问 `C:\Users\1` 失败，暂未完成验证
- [x] **agent_think 前端可见性**：前端已接入 `agent_think`，Agent 推理过程对用户可见
- [x] **Hint 结构化改造**：`NextHint` 已切换为 `tool:<toolName>|param=value` 结构化格式，`planCallsFromHint` 已做精确解析
- [ ] **dead code 清理**：`local_workflow.go` 仍保留在代码库中（见问题 #1）
- [ ] 端到端聊天实测覆盖更多真实 `doc / task / trace` 场景
- [ ] trace detail 页前端展示 `agent_round` 层级
- [x] `doc_fail_01` 标准失败样例已能稳定走通 `document -> task -> node`

---

## 八、风险与当前判断

### 8.1 已识别问题

| # | 问题 | 严重程度 | 位置 | 说明 |
|---|---|---|---|---|
| 1 | dead code 未清理 | 中 | [local_workflow.go](internal/app/rag/tool/local_workflow.go) | `LocalWorkflow` 已被 `AgentLoop` 完全替代，但仍在代码库中。保留会导致新维护者误用旧的单次 pass 模式 |
| 2 | `doc_fail_01` 标准失败样例曾在 `latestLogError` 处提前结束 | 已修复 | [observer_rule.go](internal/app/rag/tool/observer_rule.go) | 之前 `observeDocumentDiagnosis(...)` 在拿到 task/chunk log 级错误摘要后就直接结束，导致无法稳定继续下钻到 `task / node`。现已改为只有拿到节点级错误才直接结束 |
| 3 | Observer hint 传递机制脆弱 | 已修复 | [observer_rule.go](internal/app/rag/tool/observer_rule.go) → [agent_loop.go](internal/app/rag/tool/agent_loop.go) | hint 已从自然语言切换为结构化格式，`planCallsFromHint` 已做精确解析 |
| 4 | planner 增量规划缺少 few-shot | 中 | [planner.go:85-127](internal/app/rag/tool/planner/planner.go#L85-L127) | `buildUserPrompt` 只是把 hint 和前序结果拼接进 user message，没有 few-shot 示例教 LLM 如何做增量规划。LLM 可能重复已执行的 tool、忽略 hint、或自由发挥 |
| 5 | `planCallsFromResults` 覆盖盲区 | 低 | [agent_loop.go:338-358](internal/app/rag/tool/agent_loop.go#L338-L358) | 只在 `latest.Name` 为 `document_ingestion_diagnose` / `task_ingestion_diagnose` 时自动推导下一步。`observeDocumentQuery` 的 hint 路径覆盖了 `document_query → document_ingestion_diagnose` 链路，但如果 observer 被替换为 nil，该链路就断了 |
| 6 | Agent trace 节点命名超出 `varchar(16)` 限制 | 已修复 | [chat_tracer.go](internal/app/rag/service/chat_tracer.go) | `agent_observation` 等命名导致 `t_rag_trace_node.node_type` 落库失败，现已压缩为数据库安全长度 |
| 7 | 最终回答可能被前一轮较弱 diagnose 结论覆盖 | 已修复 | [answer_guidance.go](internal/app/rag/tool/answer_guidance.go) | 现在最终回答会优先采用后续更深一层的 `ingestion_task_node_query` 节点级证据 |

### 8.2 整体判断

| 风险 | 当前状态 | 说明 |
|---|---|---|
| 增量规划质量不足 | 仍存在 | planner 目前只是轻量利用 `AgentState / PreviousResults`，详见问题 #4 |
| Observer 误判 Done | 可控但存在 | 当前是规则 observer，稳定性高于 LLM observer，但覆盖范围有限 |
| 重复调用 / 空转 | 已部分处理 | 已做 call 去重，但复杂场景仍需继续打磨 |
| Agent 感不足 | 明确存在 | V1 更像结构化诊断 agent，而不是完全自主 Agent。详见问题 #2 |
| 前端展示不完整 | 明确存在 | 当前是最小消费，不是最终卡片设计。详见问题 #2 |
| Hint 传递脆弱 | 明确存在 | 自然人读文本做机器解析，详见问题 #3 |

当前判断：

- 这版已经足够作为 **V1 可演示版本**
- 重点价值在于把系统从”单次 tool 调用”升级到了”可多轮追查”
- `doc_fail_01` 标准失败样例已经可以作为当前阶段的可演示链路
- 剩余重点正从“打通第一条多轮链路”转向“扩大覆盖面与稳定运行中/trace 场景”

---

## 九、下一步计划

### V2：近期优先

**P0（阻塞可解释性）：**

1. **扩大运行中场景的稳定覆盖面**
   - 让 `doc_run_01 / task_run_01` 稳定走通 `document -> task -> node`
   - 验证运行中场景不会误答成失败场景

2. **继续打磨最终回答的稳定性**
   - 继续减少“同一问题多次提问、结论漂移”的现象
   - 继续收紧 facts / inferences / riskHints 的边界

**P1（提升规划质量）：**

3. 清理 dead code：移除 `local_workflow.go`，确保所有引用已迁移到 `AgentLoop`（修复问题 #1）
4. 在 planner system prompt 中继续打磨 few-shot 示例（问题 #4 已缓解但仍需继续优化）
5. `planCallsFromResults` 补上 `document_query → document_ingestion_diagnose` / `chunk_log_query → document_ingestion_diagnose` 的回退链路（修复问题 #5）
6. 补强高频 `doc -> task -> node` 链路稳定性
7. 增加更多 AgentLoop 样本与端到端验证

### V3：评估后再做

1. 是否引入 `LLM observer`
2. 是否做更强的 workflow 模板化
3. 是否做完整前端调用链卡片与思考卡片
4. 是否将 Agent 参数移入 config

---

## 十、与其他模块的关系

```text
RagChatService
  -> retrieve        (不变)
  -> agent_loop      (已替换 V1)
  -> prompt          (不变)

agent_loop
  -> Planner         (复用 + 增量输入)
  -> Executor        (复用)
  -> RuleObserver    (V1)

后续可选：
  RuleObserver -> LLMObserver
```

当前原则保持不变：

- 不改写 `RagChatService` 外壳
- 不改 Tool 契约
- 不动 retrieve / prompt 主链
- Agent 化只发生在 tool stage 内部
