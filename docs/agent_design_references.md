# Agent Runtime 设计参考：成熟框架的模式与洞察

日期：2026-05-30

目的：为 goagent 的 Agent Runtime 建设提供外部设计参考。聚焦于与 goagent 架构方向（typed state、graph execution、capability abstraction、pattern-based orchestration）一致或可对比的成熟设计。

---

## 一、参考框架总览

| 框架 | 所属组织 | 核心抽象 | 与 goagent 的关系 |
|---|---|---|---|
| **Eino** | CloudWeGo/字节跳动 | Graph + Branch + Checkpoint | 已是执行底座 |
| **LangGraph** | LangChain | StateGraph + Node + ConditionalEdge + Checkpoint | 最接近的类比 |
| **Anthropic Patterns** | Anthropic | Augmented LLM → Workflow → Agent 三级 | 模式体系直接可映射 |
| **OpenAI Agents SDK** | OpenAI | Agent + Handoff + Guardrail + Session | handoff/delegation 参考 |
| **BeeAI / Bee Agent** | IBM | ReActAgent + DAG Workflow + Memory | workflow 编排参考 |

---

## 二、LangGraph — 最接近的类比

LangGraph 的 StateGraph 架构与 goagent 的核心设计有极高的结构相似性。

### 架构对比

| 概念 | LangGraph | goagent |
|---|---|---|
| 状态容器 | `AgentState(TypedDict)` | `StateSnapshot` (5 域 struct) |
| 状态更新 | `Annotated[list, operator.add]` 合并策略 | `Reducer.Apply(snapshot, delta)` |
| 节点 | `node(name, func(state) -> partial_state)` | `kernel.Node.Run(session) -> NodeResult{Delta}` |
| 条件分支 | `add_conditional_edges(from, router, mapping)` | `builder.AddBranch(from, BranchFunc, targets)` |
| Checkpoint | `CheckpointAt(thread_id, checkpoint_ns)` | `Runner.RunWithCheckpoint / Resume` |
| Subgraph | `add_subgraph(name, graph)` | 尚未实现 |
| Interrupt | `interrupt()` / `Command(resume=...)` | `WithInterruptBeforeNodes` |

### 值得借用的设计

**1. TypedDict 的 `Annotated[type, merge_fn]` 模式**

LangGraph 允许每个状态字段声明独立 merge 函数：

```python
class State(TypedDict):
    messages: Annotated[list[Message], add_messages]  # append + 同 ID 覆盖
    documents: Annotated[list[Doc], operator.add]       # 只追加
    answer: str                                         # 直接覆盖
```

goagent 当前所有 slice 字段都是纯 append，scalar 都是纯 overwrite。没有字段级别的 merge 策略。如果未来某个字段需要"同 ID 覆盖"（如 messages/documents），需要在 Reducer 里增加字段级策略。

**建议：** 为 `Reducer` 的字段增加策略注释（哪些是 append-only，哪些是 upsert-by-id，哪些是 overwrite）。当前 M1 的 `SearchResults` / `FetchResults` 每次 search 都想"追加还是替换"——这是 LangGraph `operator.add` vs 直赋的哲学。

**2. `Command` 对象 — 节点驱动路由**

LangGraph 允许节点通过返回 `Command(goto=..., update=...)` 直接指定下一个节点：

```python
def my_node(state):
    return Command(goto="other_node", update={"key": "value"})
```

goagent 当前路由完全由 graph 编译时的 `AddBranch` 决定，节点本身不干预路由决策。`DecisionArtifact` 部分弥补了这一点（observe 节点产出 `branch → answer|degrade`），但 `decision_emitted` 事件和实际的 graph branch 是两条逻辑路径。

**建议：** 观察 LangGraph `Command` 的演化。如果未来 reactive loop 需要节点级动态路由（如"执行 tool A 失败后直接跳到 degrade，不走正常的 branch 逻辑"），可能需要类似机制。

**3. Subgraph — 可嵌套的图**

LangGraph 的 `add_subgraph` 允许将一个图作为另一个图的节点。goagent 尚无此能力。未来的 `external_evidence_workflow` 等复合能力天然适合作为 subgraph 嵌入 reactive pattern。

**建议：** M6（Checkpoint/Replay/Interrupt）之后评估 Eino 的 `AddGraphNode` 是否可用于实现 subgraph。

---

## 三、Anthropic 6 模式 — 直接可映射

Anthropic 的 *Building Effective Agents* 定义了 6 种模式。goagent 的 `pattern/` 目录是本模式体系的直接体现。

### 模式映射

| Anthropic 模式 | goagent 当前状态 | 映射 |
|---|---|---|
| **Prompt Chaining** | 尚未有独立 pattern | 可作为 `prepare → search → fetch → answer` 的简化版 |
| **Routing** | `observe → branch(answer\|degrade)` | `reactive` 已内置简单的二路路由 |
| **Parallelization** | 尚未实现 | kernel 当前是线性图 |
| **Orchestrator-Workers** | 尚未实现 | 映射到设计文档的 Pattern C（Delegation） |
| **Evaluator-Optimizer** | 尚未实现 | observe 节点目前是规则评估，无 LLM evaluator |
| **Autonomous Agent** | `reactive` 是简化版 | 缺 LLM planner + multi-turn loop |

### 关键洞察

Anthropic 的核心主张：**80% 的任务用 workflow（预定路径），20% 用真正的 agent（动态决策）。**

goagent 的 `pattern/reactive` 目前处于一个有趣的位置——它既有 workflow 的一面（`prepare → search → fetch → observe → branch` 是预定路径），又有 agent 的一面（observe 的 branch decision 是动态决策）。

**建议：**
- 短期：`pattern/reactive` 引入 loop 后，它成为 Anthropic 意义上的"autonomous agent"（有动态决策循环）
- 中期：增加 `pattern/routing`（意图分类→路由到不同 capability 图）、`pattern/chain`（线性多步链）
- 长期：`pattern/delegation`（orchestrator-workers 模式）

---

## 四、OpenAI Agents SDK — Handoff 与 Guardrail

OpenAI Agents SDK 的三个核心概念（Agent、Handoff、Guardrail）中，**Handoff** 和 **Guardrail** 是 goagent 最值得借鉴的。

### 4.1 Handoff — 委托与上下文传递

OpenAI 的 handoff 设计：

```
Runner.run(triage_agent, input)
  → triage_agent evaluates → decides to handoff to specialist
  → conversation context automatically transferred to specialist
  → specialist runs with full history
  → specialist returns final result
```

**两种模式：**
- **Manager Pattern**：中央 orchestrator 通过 tool calls 调用 sub-agent（可并行）
- **Decentralized Pattern**：平级 agent 之间的 handoff，控制权完全转移

goagent 的设计文档 Pattern C（Delegation/Multi-Agent）与这两种模式对应：
- Manager ↔ `orchestrator → delegate(sub_agent_1, sub_agent_2)` 并行
- Decentralized ↔ agent A 产出 decision `{Kind: "delegation", Target: "agent_B"}`

**关键借用以：**

1. **Handoff 的 conversation context 传递语义**

OpenAI SDK 在 handoff 时自动传递完整 conversation history 给目标 agent。goagent 如果做 delegation，`RuntimeSession` 是天然的状态容器——sub-agent 可以共享同一份 `StateSnapshot`（或继承一份 copy of the evidence domain）。

2. **`as_tool()` — agent 也是 capability**

OpenAI 允许 `agent.as_tool()` 将一个 agent 包装为 tool，使 orchestrator 可以通过 tool call 机制调用 sub-agent。goagent 的 `Capability` 抽象可以自然支持这一点——sub-agent 就是一种 `Kind: "sub_agent"` 的 Capability。

3. **架构建议：start with one agent**

> "Start with a single agent. Add tools. Only split into multi-agent when evaluations show the single agent struggling. Multi-agent adds complexity that most early-stage workflows don't need."

这与 goagent M0-M5 的实施路径一致——目前只有一条 reactive 线，先把它做好再考虑多 agent。

### 4.2 Guardrail — 三层安全体系

```
Input Guardrail（规则 + LLM）
  → Agent Execution
    → Tool Guardrails（每次 tool call 前后）
      → Output Guardrail（合规 + 过滤 + 格式校验）
```

goagent 当前没有显式的 guardrail 体系。但 `RuntimeOptions.RequireApproval` 和 capability 的 `RequiresApproval` 已经是 guardrail 的前身——它们控制"是否允许执行"。

**建议：** 为 goagent 增加 input/output guardrail 节点类型：
- `guardrail_input` 节点（图入口）— 校验输入合法性
- `guardrail_output` 节点（图出口）— 校验输出合规性

这些可以作为 `pattern/` 的通用基础设施，每个 pattern 都自动获得。

---

## 五、BeeAI / Bee Agent Framework — Workflow 编排

IBM BeeAI 的 DAG 工作流引擎提供了更结构化的编排思路。

### 三节点类型模型

```
Tool Node    → 执行工具，产出结构化结果
Decision Node → LLM 评估，产出分支决策
Transform Node → 纯数据处理，无 LLM
```

goagent 当前 reactive pattern 的节点也可以按此分类：
| goagent 节点 | BeeAI 分类 |
|---|---|
| prepare | Transform（normalizeQuery，无 LLM） |
| search | Tool（调用 SearchCapability） |
| fetch | Tool（调用 FetchCapability） |
| observe | Decision（评估 evidence，产出 branch） |
| answer / degrade | Transform（构建答案文本） |

BeeAI 的分类法可以帮助 goagent 在文档化和标准化方面做得更好——为每种节点类型定义清晰的输入输出契约。

### Human-in-the-Loop 四种模式

| 模式 | 说明 | goagent 映射 |
|---|---|---|
| Validation | 关键操作前请求确认 | `RequiresApproval` → interrupt |
| Correction | 错误触发修正流程 | 尚未实现 |
| Clarification | 模糊输入请求澄清 | 尚未实现 |
| Decision | 多选项请求人类选择 | 尚未实现 |

goagent 的 `WithInterruptBeforeNodes` + `RequiresApproval` 已经覆盖了 Validation 模式。其他三种是未来的扩展方向。

---

## 六、总结：goagent 可以从成熟框架学到什么

### 短期（M2 期间可做）

1. **Reducer 字段级 merge 策略显式化**（借用以 LangGraph `Annotated` 思路）
   - 为每个字段注明 merge 语义：append-only / upsert-by-id / overwrite
   - 当前的 append-only + overwrite 二选一不够灵活

2. **增加 guardrail 节点类型**（借用以 OpenAI 三层 guardrail）
   - `guardrail_input` / `guardrail_output` 作为 pattern 的通用基础设施
   - 与 capability `RequiresApproval` 互补

3. **丰富 Anthropic 模式在 goagent 的落地**（借用以 6 模式体系）
   - 在 `pattern/` 下为未来的 routing / chain / evaluator-optimizer 预留目录结构
   - 当前的 reactive 就是 autonomous agent 的最小版本

### 中期（M5-M7 期间）

4. **Subgraph / 可嵌套图**（借用以 LangGraph subgraph + Eino AddGraphNode）
   - `external_evidence_workflow` 作为 subgraph 嵌入 reactive pattern
   - 引入 `pattern/composite` 用于编排多个 subgraph

5. **Delegation/Handoff with context transfer**（借用以 OpenAI handoff）
   - sub-agent 继承父 agent 的 `StateSnapshot`（或 selective copy）
   - sub-agent 返回 summary + delta，父 agent 通过 Reducer 合并

6. **Human-in-the-Loop 模式**（借用以 BeeAI 四种模式）
   - 在 Validation 基础上增加 Clarification / Decision / Correction

### 长期

7. **Agent as Capability**（借用以 OpenAI `as_tool()`）
   - sub-agent 作为 `Kind: "sub_agent"` 的 Capability 注册到 Registry
   - orchestrator 通过统一接口调用 tool 和 sub-agent

8. **跨 Agent 通信协议**（借用以 IBM Agent Communication Protocol / A2A）
   - 如果未来需要多进程甚至跨服务 agent 通信
   - Eino Graph 当前是单进程，跨服务需要协议层

---

## 七、关键参考链接

| 资源 | 链接 |
|---|---|
| Building Effective Agents (Anthropic) | `anthropic.com/engineering/building-effective-agents` |
| LangGraph Documentation | `langchain-ai.github.io/langgraph/` |
| OpenAI Agents SDK | `github.com/openai/openai-agents-python` |
| BeeAI Framework | `github.com/i-am-bee/beeai-framework` |
| Eino (CloudWeGo) | `github.com/cloudwego/eino` |
| Claude Code Best Practices | Anthropic 内部文档 |
