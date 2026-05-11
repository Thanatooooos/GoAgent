# Agent 状态评估

版本：v1.3
日期：2026-05-11

---

## 一、架构全景

```
RagChatService.Chat()
  │
  ├─ prepareChat()          ← 会话/记忆/改写/检索（不变）
  │
  ├─ runToolWorkflowStage() ← AgentLoop 入口
  │   │
  │   └─ AgentLoop.Run()    ← Plan → Act → Observe 循环
  │       │
  │       ├─ planCalls()    ─┬─ LLMPlanner（主路径，LLM 决策）
  │       │                  └─ planWithRules（回退，HintCall 泛化 + 开放问题处理）
  │       │
  │       ├─ Executor       ← 10 个 tool，支持串行/并行可配
  │       │
  │       └─ LLMObserver    ← LLM 决策 Done / NextHintCalls（支持多 hint）
  │           └─ RuleObserver（LLM 异常或 ReachedMaxLoop 时兜底，跳过 think 结果）
  │
  ├─ runPromptStage()       ← 检索上下文 + 工具上下文注入 prompt
  │
  └─ StreamChat             ← SSE 推送最终回答
```

---

## 二、工具集（10 个 Tool）

| 类型 | Tool | 用途 |
|------|------|------|
| 诊断 | `document_ingestion_diagnose` | 文档级诊断，含 live task node 补齐，一步到节点错误 |
| 诊断 | `task_ingestion_diagnose` | 任务级诊断 |
| 诊断 | `trace_retrieval_diagnose` | 检索链诊断 |
| 查询 | `document_query` | 按 ID 查文档 |
| 查询 | `document_chunk_log_query` | 查文档 chunk log |
| 查询 | `ingestion_task_query` | 查任务详情（含 node 摘要） |
| 查询 | `ingestion_task_node_query` | 查任务节点详情 |
| 查询 | `trace_node_query` | 查 trace 节点 |
| 发现 | `document_list` | 按 status/query 分页查文档列表 |
| 发现 | `task_list` | 按 status/pipelineId 分页查任务列表 |
| 外部 | `web_search` | DuckDuckGo 搜索，补充知识库外信息 |
| 元工具 | `think` | 记录推理过程，不产生副作用 |

---

## 三、长项

### 3.1 决策链路完整

Planner（LLM）→ Executor → Observer（LLM）→ 下一轮 Planner，全部 LLM 参与。RuleObserver 仅兜底。

### 3.2 上下文传递充分

`AgentState`（phase / hypothesis / confidence / openQuestions / checkedTools / nextHintCalls）跨轮次传递。Planner 和 Observer prompt 均含改写摘要、检索摘要、知识库范围、历史工具结果。

### 3.3 可观测性

| 层级 | 机制 |
|------|------|
| SSE | `tool_start` / `tool_result` / `agent_think` |
| Trace | `agt_round` / `tool_call` / `agt_obs` |
| RoundSummary | Done/Reasoning/Confidence/State + ExecutionMode / WallClockDurationMs / ToolCallCount / TotalToolDurationMs |
| Log | LLMObserver 解析失败时 Warning 记录原始响应（截断 300 字符） |

### 3.4 兜底机制分层

```
LLM Observer 异常/无效 JSON → RuleObserver（跳过 think 结果取 lastNonThinkResult）
LLM Planner 无结果 → planWithRules（HintCall 泛化 → results → baseRules）
Observer error → 强制 Done
ReachedMaxLoop → 标记 Degraded
LLM 幻觉 ID → validateHintAgainstEvidence / validateCallAgainstEvidence 拦截
```

### 3.5 Answer guidance 双重保障

- **深度证据升级**：节点级成功/失败证据自动覆盖较弱的 diagnose 结论
- **状态冲突归一**：document.status=failed 与 task/node.status=running 冲突时，以 task/node 实际状态为准

---

## 四、v1.2 → v1.3 新增能力

### 4.1 工具集从 8 个扩展到 10 个

| 新增 | 说明 |
|------|------|
| `document_list` | 按 status/query 分页查文档。baseRules 处理开放问题（"最近哪些文档失败了？"），自动注入 KnowledgeBaseIDs |
| `task_list` | 按 status/pipelineId 分页查任务 |
| `web_search` | DuckDuckGo Instant Answer API，免费无需 Key，返回最多 5 条结果 |
| `think` | Planner 推理记录工具，产出对用户可见但不干扰 Observer 决策 |

### 4.2 Observer 多 hint 放开

Observer prompt 规则 #3 从 `exactly one` 改为 `one or more`，新增 few-shot 示例 #4 演示并行 hint。`parseResponse` 改为遍历校验所有 hint name。

### 4.3 Observer 规则强化

- **规则 #4**：task/ingestion_task_query 的 taskNodeSummary/nodes 中含 failed/running node 时，MUST 继续到 node query
- **规则 #9**：think 结果仅用于推理可见性，Observer 决策跳过 think 看 `lastNonThinkResult`
- **新增 few-shot 示例 #3**：taskQuery 显示 indexer(status=failed) 但无 errorMessage → 继续

### 4.4 diagnose 一步到位

`document_ingestion_diagnose` 新增可选 `ingestionTaskNodeReader` 依赖。chunk log 节点数据不完整时，直接查 `taskService.ListNodes()` 补齐节点错误。大部分场景下 diagnose 本身就能返回完整结论，Agent 不需要多轮下钻。

### 4.5 baseRules 开放问题处理

当无具体 ID 匹配时，检测开放关键词（"哪些"/"最近"/"所有"/"列表"/"which"/"list"/"recent"），自动触发 `document_list` 或 `task_list`。自动从 WorkflowInput 注入 KnowledgeBaseIDs。

### 4.6 Answer guidance 状态冲突归一

`enrichDiagnosisWithDeeperEvidence` 同时查 task 和 node 两级证据。当 diagnose 结论说 failed 但 task/node 实际 running 时，`resolveStatusConflict` 覆盖结论为"仍在处理中"，显式标注状态不一致来源和风险提示。

---

## 五、当前短板

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| 1 | LLMObserver 对模糊场景仍可能过早 Done | 中 | 虽然加了规则和 few-shot，LLM 仍可能非确定性地判 Done。最可靠的兜底是 diagnose 本身返回完整数据 |
| 2 | Planner/Observer LLM 调用冗余 | 低 | 每轮分别调 LLM，上下文高度重叠 |
| 3 | 无 Agent 级 A/B 评估 | 低 | `cmd/retrieve-eval` 只覆盖检索质量，不覆盖 Agent 决策质量 |
| 4 | 并发模式 SSE 事件顺序无测试 | 低 | 并行 tool 执行时 SSE 事件序列无端到端验证 |

---

## 六、测试覆盖

| 模块 | 测试数 | 覆盖要点 |
|------|--------|----------|
| Planner | 16 | Plan/多 tool/空结果/JSON/retrieve context/prompt |
| AgentLoop | 19 | 多轮、hint 泛化、运行中、并行、think tool、open-ended |
| LLMObserver | 7 | 多 hint/空名拒绝、决策、fallback、Reasoning≠Hypothesis |
| Tool 基础 | 8 | answer guidance 冲突归一、AgentState、registry、executor |
| ResultSummary | 3 | 诊断字段、taskNodeSummary、噪音过滤 |
| Builtin Tools | 16 | 10 个 tool × Invoke + diagnose 增强 |
| **合计** | **71** | |

---

## 七、成熟度矩阵

| 维度 | 评级 | 说明 |
|------|------|------|
| 决策正确性 | ★★★★☆ | diagnose 一步到位大幅减少对 LLM Observer 的依赖 |
| 决策泛化性 | ★★★★☆ | HintCall 泛化，新增 tool 无需改 Observer/Planner |
| 可观测性 | ★★★★★ | SSE + trace + round 性能 + 解析失败日志 |
| 鲁棒性 | ★★★★★ | 分层兜底 + think 隔离 + ID 防幻觉 |
| 执行效率 | ★★★★☆ | 并行执行 + diagnose 减少轮次 |
| 场景覆盖 | ★★★★☆ | 诊断 + 开放发现 + web 搜索 |
| 测试覆盖 | ★★★★★ | 71 测试，覆盖主要路径和边界 |
| 可配置性 | ★★★★☆ | maxIterations + parallelToolCalls 配置化 |

---

## 八、整体判断

Agent 已从"结构化诊断助手"演进为"诊断 + 发现 + 外部搜索"的综合 Agent。关键进展：

- diagnose 一步到位减少了多轮下钻的不确定性
- 开放问题处理填补了"无 ID 时束手无策"的空白
- web_search 让 Agent 能回答知识库外的通用问题
- think 工具让推理过程对用户可见
- 状态冲突归一解决了回答被浅层结论带偏的问题

---

## 九、与相关文档的关系

- [project_progress_context.md](project_progress_context.md) — 项目整体进度
- [agentic_chat_dev_plan.md](agentic_chat_dev_plan.md) — Agent 开发的 V1/V2/V3 路线
- [agent_evolution_plan.md](agent_evolution_plan.md) — 从诊断 Agent 向自主 Agent 演进路线
- [observer_llm_improvements.md](observer_llm_improvements.md) — LLMObserver 薄弱点与改进跟踪
