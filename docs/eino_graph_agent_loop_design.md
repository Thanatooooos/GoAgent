# Eino Agent Runtime 架构设计

日期：2026-05-29

状态：设计提案（重构版），M1 spike 已完成（2026-05-29）

---

## 一、设计目标

本文档不再把目标定义为“重写一个新的 AgentLoop”，而是定义为：

> 在 `internal/app/agent` 下建设一套全新的、长期可演进的 **Agent Runtime**。

这套 runtime 的目标是：

1. **最终完整取代旧 `internal/app/rag/tool` AgentLoop 的功能**
2. **让状态的定义、消费、流转、归因都比旧系统更清晰、更可控**
3. **为未来能力预留稳定扩展边界**
   - 多 Agent 编排
   - plan-execute / plan-and-solve / delegation 等更高级范式
   - checkpoint / resume / replay
   - human-in-the-loop / interrupt / approval

本文档明确采用以下前提：

- **不以兼容旧 runtime 为设计目标**
- **旧系统只作为参考资产、迁移素材和回归样本集**
- **不为“先临时跑起来”而牺牲长期边界质量**

---

## 二、为什么需要新 runtime

旧 `AgentLoop` 已经是可工作的生产系统，但从长期演进角度看，问题已经不是“继续修补是否可行”，而是“继续修补是否值得”。

当前主要结构性问题有：

1. **控制流被埋在实现细节里**
   - `for/if/break` 驱动主循环
   - 并行、降级、hint 复用、证据校验等逻辑都塞在 loop 内
   - 扩展新范式时，容易继续堆分支

2. **状态来源不单一，归因不透明**
   - planner、observer、tool result、retrieve context 通过多个入口读写
   - 旧系统虽然已经在修复“参数幻觉”和“证据不一致”，但核心问题仍是状态传递模型不够清晰

3. **运行时语义和业务语义耦合过深**
   - loop 既负责调度，又负责推理策略，又负责证据收口
   - 很难把“执行框架”与“某一代 agent 策略”分开

4. **未来范式扩展缺少一等抽象**
   - `nextHintCalls` 适合当前 reactive loop
   - 但对“计划对象”“子任务委派”“多 agent 协作”“审批中断”并不是理想的一等模型

所以，新设计不是为了把旧 loop 改得更整洁，而是为了把系统重心转到：

- **runtime kernel**
- **typed state**
- **decision artifact**
- **capability abstraction**
- **event/replay/checkpoint**

---

## 三、核心设计原则

### 3.1 Eino Graph 是执行底座，不是架构本身

我们会使用 Eino Graph 提供：

- graph compile / run
- branch routing
- checkpoint
- interrupt
- run-step guardrail

但 **Eino Graph 不直接定义我们的状态模型、能力模型和决策模型**。

也就是说：

- Graph 负责“怎么跑”
- Runtime 负责“什么是状态、什么是计划、什么是执行、什么是证据”

### 3.2 runtime 与 agent pattern 解耦

新 runtime 不应把自己固定成单一 `Plan -> Execute -> Observe` 循环。

它至少要支持三类 pattern：

1. `reactive loop`
   - 适合当前 diagnose / search / fetch / retrieve 场景

2. `plan-execute`
   - 先生成显式 plan，再按 step 执行，再做 replan

3. `delegation / multi-agent`
   - 一个 agent 把子问题分派给其他 agent 或 subgraph

因此，**loop 只是 runtime 上的一种 strategy，而不是 runtime 本体**。

### 3.3 状态必须可追踪，而不只是可读

新 runtime 不能只解决“状态定义清楚”。

还必须解决：

- 这个状态是谁写入的
- 基于什么证据写入的
- 哪个节点覆盖了哪个旧结论
- 为什么最后走到降级回答

所以状态模型必须同时包含：

1. **State Snapshot**
   - 当前运行时消费的结构化状态

2. **Event / Delta Journal**
   - 每一步产生的事件、差量、决策、执行记录

3. **Reducer**
   - 唯一允许将 delta 合并回 snapshot 的统一入口

### 3.4 执行单元统一抽象

未来从 runtime 看，以下对象应该尽量是同类抽象：

- tool
- deterministic workflow / subgraph
- delegated sub-agent

它们都应该被视为一种 **Capability**，差别只在：

- 输入输出 schema
- 风险级别
- 是否需要审批
- 是否可并行
- 是否可 checkpoint

这样后续做多 Agent 编排时，不需要推翻 runtime，只需要扩充 capability registry。

---

## 四、整体架构

### 4.1 分层视图

```text
+-----------------------------------------------------------+
| Agent Runtime API                                         |
| Run / Resume / Interrupt / Replay / Inspect               |
+-----------------------------------------------------------+
| Runtime Kernel                                            |
| graph compile | node scheduler | branch router            |
| checkpoint | interrupt | timeout | retry | cancellation   |
+-----------------------------------------------------------+
| State System                                              |
| Snapshot | Event Journal | Delta | Reducer | Projection   |
+-----------------------------------------------------------+
| Decision Layer                                            |
| Planner | Evaluator | Router | Policy Engine              |
+-----------------------------------------------------------+
| Capability Layer                                          |
| Tool | Workflow | SubAgent | Registry | Spec              |
+-----------------------------------------------------------+
| Infra / Eino                                              |
| Eino Graph | InferTool | LLM | storage | SSE | trace      |
+-----------------------------------------------------------+
```

### 4.2 运行时职责边界

`Runtime Kernel` 负责：

- 运行 graph
- 驱动节点调度
- 管理 checkpoint / interrupt / resume
- 输出事件流和审计记录

`Decision Layer` 负责：

- 生成计划
- 评估证据
- 选择下一动作
- 执行策略降级

`Capability Layer` 负责：

- 暴露 tool / workflow / sub-agent
- 提供 schema、风险、依赖关系、执行语义

`State System` 负责：

- 统一管理 snapshot
- 接收各节点生成的 delta/event
- 通过 reducer 合并状态
- 为 trace / replay / debug 提供基础

---

## 五、核心抽象

### 5.1 RuntimeSession

每次 agent 运行对应一个 `RuntimeSession`，它是运行时最重要的容器。

```go
type RuntimeSession struct {
    SessionID   string
    Request     RequestEnvelope
    Snapshot    StateSnapshot
    Journal     []RuntimeEvent
    Checkpoint  *CheckpointRef
    Metadata    SessionMetadata
}
```

它解决的是“这次运行的全部上下文”问题，而不是只保存某个 loop 的中间变量。

### 5.2 StateSnapshot

`StateSnapshot` 是当前状态的结构化视图，但它不是一个“大而全、随处可改”的可变对象。

建议采用分域建模：

```go
type StateSnapshot struct {
    Request    RequestState
    Context    ContextState
    Plan       PlanState
    Evidence   EvidenceState
    Execution  ExecutionState
    Answer     AnswerState
    Telemetry  TelemetryState
}
```

建议拆分如下：

- `RequestState`
  - question
  - user / trace / conversation
  - target kb / runtime options

- `ContextState`
  - rewrite
  - retrieve
  - memory
  - session context

- `PlanState`
  - current strategy
  - active plan
  - plan steps
  - pending decisions

- `EvidenceState`
  - accumulated facts
  - evidence level
  - source map
  - unresolved questions

- `ExecutionState`
  - scheduled actions
  - completed actions
  - failed actions
  - retries / interrupts

- `AnswerState`
  - answer draft
  - degrade reason
  - final answer

- `TelemetryState`
  - timings
  - token usage
  - branch history
  - audit refs

### 5.3 RuntimeEvent 与 StateDelta

节点执行不直接“随手改 snapshot”，而是产出：

1. `RuntimeEvent`
   - 面向 journal / replay / 审计

2. `StateDelta`
   - 面向 reducer 合并

```go
type RuntimeEvent struct {
    ID          string
    SessionID   string
    Node        string
    EventType   string
    Timestamp   time.Time
    Payload     any
    EvidenceRef []EvidenceRef
}

type StateDelta struct {
    ContextDelta   *ContextDelta
    PlanDelta      *PlanDelta
    EvidenceDelta  *EvidenceDelta
    ExecutionDelta *ExecutionDelta
    AnswerDelta    *AnswerDelta
    TelemetryDelta *TelemetryDelta
}
```

这样做的好处是：

- 节点职责更纯
- 状态来源更可追踪
- checkpoint / replay 有天然边界
- 同一个状态字段的覆盖逻辑集中在 reducer 中，而不是散落在各节点

### 5.4 Reducer

Reducer 是唯一允许把 delta 写回 snapshot 的地方。

```go
type Reducer interface {
    Apply(snapshot StateSnapshot, delta StateDelta) (StateSnapshot, error)
}
```

这会带来几个长期收益：

- 避免节点之间偷偷共享写状态
- 可以显式定义“追加”“覆盖”“冲突拒绝”“按证据升级”
- 可以做 deterministic replay

### 5.5 Capability

Capability 是 runtime 的统一执行单元抽象。

```go
type Capability interface {
    Spec() CapabilitySpec
    Invoke(ctx context.Context, input any) (CapabilityOutput, error)
}

type CapabilitySpec struct {
    Name             string
    Kind             string // tool | workflow | sub_agent
    InputSchema      any
    OutputSchema     any
    RiskLevel        string
    SupportsParallel bool
    SupportsResume   bool
    Dependencies     []string
}
```

对 runtime 而言：

- `web_search` 是 capability
- `external_evidence_workflow` 是 capability
- 以后一个 `diagnose_agent` 也可以是 capability

这比现在把 graph tool、普通 tool、sub-agent 分别看待更适合长期扩展。

### 5.6 Decision Artifact

新 runtime 里不能再把“下一步 hint”当成最核心决策对象。

建议提升为统一的 `DecisionArtifact`：

```go
type DecisionArtifact struct {
    Kind       string // action | plan | branch | delegation | answer
    Confidence float64
    Reasoning  string
    Payload    any
}
```

这样以后：

- reactive loop 输出的是 `action`
- plan-execute 输出的是 `plan`
- branch router 输出的是 `branch`
- multi-agent 输出的是 `delegation`

runtime 本身不需要改模型。

---

## 六、节点协议

### 6.1 节点签名

新 runtime 的 graph node 建议遵循：

```go
func(ctx context.Context, session RuntimeSession) (NodeResult, error)
```

其中：

```go
type NodeResult struct {
    Events   []RuntimeEvent
    Delta    StateDelta
    Decision *DecisionArtifact
}
```

节点职责是：

- 读取 snapshot
- 产出 event
- 产出 delta
- 可选地产出 decision

而不是直接原地修改全局状态。

### 6.2 为什么不建议把 `*AgentState` 作为全局可变对象

文档上一版用单一 `*AgentState` 作为 graph 输入输出，这比旧系统已经清晰很多，但仍然有几个长期隐患：

1. 容易重新长成新的 God Object
2. 节点原地写入会让“谁改了状态”再次分散
3. 并发节点时需要额外处理共享写冲突
4. checkpoint / replay 更难保证语义稳定

因此，新版建议：

- graph 内传播 `RuntimeSession` 或 `StateSnapshot`
- 每个节点只返回 `NodeResult`
- reducer 统一合并

---

## 七、执行模式

### 7.1 Pattern A: Reactive Loop

这是第一阶段最接近旧系统的模式，但它只是 `pattern/reactive` 下的一种实现。

```text
prepare_context
  -> plan_or_route
  -> execute_capabilities
  -> evaluate_evidence
  -> branch
      -> continue
      -> answer
      -> degrade
```

这里的 `plan_or_route` 不是旧 `nextHintCalls` 的直接翻版，而是输出 `DecisionArtifact{Kind: "action"}`。

### 7.2 Pattern B: Plan-Execute

```text
prepare_context
  -> build_plan
  -> validate_plan
  -> execute_step
  -> assess_step_result
  -> replan_or_continue
  -> answer
```

关键是 `PlanState` 在 runtime 里已经有位置，所以未来不需要重构核心模型。

### 7.3 Pattern C: Delegation / Multi-Agent

```text
prepare_context
  -> decompose
  -> delegate(sub_agent_1, sub_agent_2, ...)
  -> collect_results
  -> synthesize
  -> answer
```

只要 `sub-agent` 在 runtime 中是 capability，一套状态系统和事件系统即可复用。

---

## 八、Eino Graph 在新 runtime 里的角色

### 8.1 应该使用的能力

- `compose.NewGraph`
- `AddLambdaNode`
- `AddBranch`
- `Compile`
- `WithMaxRunSteps`
- `WithCheckPointStore`
- `WithInterruptBeforeNodes`

### 8.2 不应该让 Graph 直接承担的职责

- 不让 Graph 决定业务状态结构
- 不让 Graph 决定 planner / evaluator / policy 接口
- 不让 session value 成为主状态通道
- 不让节点之间通过隐式上下文读写关键状态

当前 `internal/app/agent/workflow` 这条 PoC 线使用 `SequentialAgent + LoopAgent + session values` 已经证明最小闭环可跑；但它更像实验验证，不适合直接升级成长期 runtime 的最终形态。

新 runtime 应转向：

- **typed snapshot**
- **delta journal**
- **graph branch**
- **reducer merge**

---

## 九、能力体系设计

### 9.1 package 组织建议

建议把 `internal/app/agent` 从“PoC workflow + web tools”升级成以下结构：

```text
internal/app/agent/
├── runtime/
│   ├── kernel/
│   ├── graph/
│   ├── reducer/
│   ├── checkpoint/
│   ├── interrupt/
│   └── replay/
├── state/
│   ├── snapshot.go
│   ├── event.go
│   ├── delta.go
│   ├── reducer.go
│   └── projection.go
├── capability/
│   ├── spec.go
│   ├── registry.go
│   ├── tool/
│   ├── workflow/
│   └── subagent/
├── policy/
│   ├── planner/
│   ├── evaluator/
│   ├── router/
│   └── validation/
├── pattern/
│   ├── reactive/
│   ├── planexecute/
│   └── delegation/
├── search/
├── websearch/
├── fetch/
├── webfetch/
├── system/
├── trace/
└── meta/
```

### 9.2 旧系统资产如何使用

旧系统不作为兼容层，而作为以下来源：

- **prompt 资产**
  - planner / observer 的 prompt 模板可借鉴

- **policy 资产**
  - evidence validation
  - base rules
  - diagnose answer guidance

- **domain logic 资产**
  - `invokers/system`
  - `invokers/trace`
  - `modules/*/behavior.go`

- **回归样本集**
  - `doc_fail_01`
  - `doc_run_01`
  - `task_run_01`
  - `trace_bad_01`

原则是：

- **可以借鉴逻辑，不继承旧边界**
- **可以复制语义，不复制旧 runtime 结构**

---

## 十、状态与观测模型

### 10.1 观测不是附属能力，而是 runtime 的一部分

新 runtime 默认应该输出：

- node start / finish
- decision emitted
- capability scheduled
- capability completed
- branch selected
- degrade raised
- answer finalized

这些都应该先进入 `RuntimeEvent Journal`，再投射到：

- SSE
- trace DB
- debug log
- replay UI

### 10.2 SSE 契约建议

建议从“tool 事件”扩展为“runtime 事件”：

```text
runtime_node_start
runtime_node_finish
decision_emitted
capability_start
capability_result
branch_selected
agent_answer
agent_degraded
```

旧的 `tool_start / tool_result / agent_think` 可以在 projection 层兼容映射，但不应再作为 runtime 的原生内部模型。

### 10.3 Trace 建议

trace 也不再只记录：

- `agt_round`
- `tool_call`
- `agt_obs`

而是提升到 runtime 级别：

- `rt_node`
- `rt_decision`
- `rt_capability`
- `rt_branch`
- `rt_answer`

是否继续向下投影到旧表结构，可以单独决定，但新 runtime 内部不再被旧 trace 结构束缚。

---

## 十一、checkpoint / replay / interrupt

这是新 runtime 相比旧 loop 必须前置考虑的能力，而不是“以后再补”。

### 11.1 Checkpoint

checkpoint 应保存：

- session id
- current snapshot
- applied event offset
- current graph node
- pending decisions / pending capabilities

### 11.2 Replay

replay 基于 journal 重放：

1. load snapshot
2. replay deltas
3. rebuild projections
4. inspect branch and answer path

这对 diagnose、复杂检索和多 Agent 场景非常关键。

### 11.3 Interrupt / Approval

interrupt 不应只绑定到 `execute_tools` 这种旧式节点名，而应绑定到 capability spec：

- 当 capability risk = `high`
- 或 policy 要求审批
- runtime 自动在 capability invoke 前发出 interrupt

这样未来即使执行单元不是 tool，而是 sub-agent，也能复用同一审批框架。

---

## 十二、实施路径

这次实施路径不再以“最小 loop 临时跑通”为中心，而以“runtime 骨架定型”为中心。

### M0：Runtime Core Design Freeze

目标：

- 冻结核心抽象，而不是先写功能

产出：

- `RuntimeSession`
- `StateSnapshot`
- `RuntimeEvent`
- `StateDelta`
- `Reducer`
- `Capability`
- `DecisionArtifact`

验收：

- 核心接口、命名、职责边界达成稳定共识

### M1：Kernel + State System Skeleton

目标：

- 先建 runtime 内核，而不是先建业务 loop

产出：

- graph compile skeleton
- reducer skeleton
- in-memory journal
- snapshot projection
- basic replay harness

验收：

- 空 graph 可运行
- 节点可产出 delta
- reducer 可合并
- journal 可回放

**M1 Spike 已完成 (2026-05-29)：** 见 [十七、M1 Spike 验证结果](#十七m1-spike-验证结果)。核心结论：
- Eino Graph `NewGraphBranch` + `AddBranch`、`WithInterruptBeforeNodes` + `WithCheckPointStore`、typed state + event journal 全部验证通过
- 在 Lambda 内调用 `compose.Interrupt()` 后 resume 时节点收到 nil state（Eino v0.8.13 限制），推荐用 `WithInterruptBeforeNodes` 作为中断机制
- 自定义 state 类型必须通过 `schema.RegisterName` 注册才能支持 checkpoint 序列化

### M2：Reactive Pattern V1

目标：

- 用新 runtime 跑通第一个 reactive pattern

产出：

- `pattern/reactive`
- `prepare_context`
- `plan_or_route`
- `execute_capabilities`
- `evaluate_evidence`
- `branch`
- `answer`

限制：

- 先不接全量 system family

### M3：Capability Framework V1

目标：

- 统一 tool / workflow capability 入口

产出：

- capability registry
- typed capability spec
- `web_search`
- `web_fetch`
- `external_evidence_workflow`

验收：

- capability 能被 runtime 调度
- journal / snapshot / SSE 能完整表达一次执行

### M4：Decision Layer V1

目标：

- 接入 planner / evaluator / policy

产出：

- planner policy
- evaluator policy
- evidence validation
- degrade policy

验收：

- 不依赖旧 loop 结构，也能完成完整 reactive 闭环

### M5：System / Trace / Meta Families

目标：

- 补齐旧 agent loop 的核心功能覆盖

产出：

- `system` family
- `trace` family
- `meta` family

验收：

- `doc_fail_01`
- `doc_run_01`
- `task_run_01`
- `trace_bad_01`

### M6：Checkpoint / Replay / Interrupt

目标：

- 让 runtime 成为“可运营”的系统，而不是只可执行

产出：

- checkpoint store
- replay toolchain
- interrupt / approval path

### M7：Plan-Execute Pattern

目标：

- 验证 runtime 不是只适配 reactive loop

产出：

- 显式 `PlanArtifact`
- step execution
- replan logic

### M8：Delegation / Multi-Agent Pattern

目标：

- 验证 runtime 的长期扩展边界

产出：

- sub-agent capability
- delegation flow
- result synthesis

### M9：生产切换

目标：

- 新 runtime 完整承担旧 agent loop 的职责

原则：

- 切换时可以保留旧系统作为短期参考实现
- 但新 runtime 不引入任何“为了兼容旧框架”的内部设计让步

---

## 十三、验收标准

只有满足以下条件，才能认为新 runtime 设计成功：

1. **功能覆盖**
   - 覆盖旧 agent loop 主能力
   - 包括 diagnose、trace、external evidence、degrade、SSE、trace

2. **状态清晰**
   - 任意关键结论都能追溯：
     - 谁写入
     - 基于什么证据
     - 被谁覆盖

3. **可扩展**
   - 新增一个 capability 不需要修改 runtime 核心流程
   - 新增一个 pattern 不需要重写状态系统
   - 新增一个 sub-agent 不需要发明新的执行模型

4. **可运营**
   - 有 checkpoint
   - 有 replay
   - 有 interrupt / approval
   - 有稳定的 SSE / trace 投影

5. **可测试**
   - reducer 可单测
   - node 可单测
   - pattern 可集成测
   - 样本场景可回归测

---

## 十四、主要风险

| 风险 | 等级 | 缓解 | 状态 |
|---|---|---|---|
| 过早把 runtime 设计成某一代 loop 的专用框架 | 高 | 先冻结核心抽象，再写 reactive pattern | — |
| snapshot 重新长成新的 God Object | 高 | 强制分域状态 + delta/reducer 合并 | — |
| 节点仍然偷偷共享隐式状态 | 高 | 统一通过 session + snapshot + reducer 流转，禁止主状态走 ad-hoc session value | — |
| 为了快速落地，重新把 decision artifact 退化成 hint call | 中 | 把 `DecisionArtifact` 作为一等抽象固定下来 | — |
| capability 抽象过弱，后续 sub-agent 无法自然接入 | 中 | 一开始就把 tool/workflow/sub-agent 纳入统一 spec | — |
| 观测后置，导致 replay/checkpoint 难补 | 中 | journal 优先于 SSE/trace projection | — |
| Eino Graph 能力边界与预期不完全一致 | 中 | 先在 M1/M2 验证 compile/branch/checkpoint/interrupt，再推进全量迁移 | M1 spike 已验证 branch ✅、checkpoint ✅、interrupt(compile-time) ✅；in-node interrupt 有限制 ⚠️ |
| 自定义 state 类型序列化失败导致 checkpoint 静默丢失 | 新增→低 | 所有进入 checkpoint 的 state 类型必须在 `init()` 中调用 `schema.RegisterName[T](name)` 注册 | M1 spike 已确认并文档化 |
| In-node `compose.Interrupt()` 恢复时节点输入为 nil（Eino v0.8.13） | 新增→低 | 新 runtime 统一使用 `WithInterruptBeforeNodes`，不依赖 in-node interrupt；in-node interrupt 留给 ADK 层 | M1 spike 已确认 |

---

## 十五、结论

这次建设的目标，不是做一个“更整洁的 AgentLoop”，而是做一个能承载未来数轮 agent 演进的 **Agent Runtime**。

新的设计取向应当是：

- **runtime first**
- **state first**
- **decision artifact first**
- **capability first**
- **event journal first**

在这个前提下：

- Eino Graph 是合适的执行底座
- 旧系统是宝贵的参考资产
- 但新 runtime 的边界必须从零定义，并以长期扩展为最高优先级

---

## 十六、参考

- `docs/agent_loop_architecture_review.md`
- `docs/tool_module_constraints.md`
- `docs/project_progress_context.md`
- `internal/app/rag/tool/runtime/agent_loop.go`
- `internal/app/rag/tool/invokers/graph/diagnosis_graph.go`
- `internal/app/rag/tool/invokers/graph/external_evidence_workflow_graph.go`
- `internal/app/agent/workflow/*`
- [Eino Graph 文档](https://github.com/cloudwego/eino)
- [LangGraph Concepts](https://langchain-ai.github.io/langgraph/concepts/)

---

## Additional Update: 2026-05-31 Runtime Output Boundary, Planner Placement, and Handoff Projection Update

### Status Update

As of `2026-05-31`, the `internal/app/agent` runtime should no longer be
described only as:

- a runtime-native reactive loop
- a typed state / reducer / journal architecture
- a minimal `search -> fetch -> observe -> answer|degrade` execution path

That description is now incomplete.

The runtime line now also includes:

- a guarded post-`observe` LLM planner seam
- planner-guided next-round query and URL selection inputs
- `handoff` as a first-class terminal action
- explicit runtime `OutputMode` control
- a dedicated `handoff` projection package
- capability/profile-derived workflow policy projection

This materially changes the intended architecture boundary: the default public
runtime path is now better understood as "evidence/action orchestration with
handoff capability", not "always produce the final user-facing answer inside
agent."

### Output Boundary Is Now Explicit: `handoff` vs `final_answer`

One major design question was whether the new runtime should:

1. only gather evidence and hand off to an outer answer service
2. both gather evidence and produce the final answer itself

The current runtime now models this explicitly through `OutputMode`:

- `handoff`
- `final_answer`

The public `agent.Service` default path now prefers `handoff`.

This means the reactive runtime should now be conceptualized as:

```text
prepare
-> search
-> fetch
-> observe
-> continue | handoff | answer | degrade
```

instead of only:

```text
prepare
-> search
-> fetch
-> observe
-> continue | answer | degrade
```

So `answer` remains a supported terminal, but it is no longer the only
positive completion path, nor the default architectural assumption.

### Planner Placement Has Been Fixed After `observe`

Another architecture decision is now clearer: the first search round should not
be planner-driven.

The first round still starts from:

- the request question
- optional prepare-time normalization / rewrite

and only after `observe` has accumulated factual runtime state does the planner
participate.

So the current decision path is:

```text
prepare
-> search
-> fetch
-> observe_facts
-> planner
-> validate
-> branch
```

This placement is intentional because:

- the first round should stay deterministic and easy to explain
- planner quality should be grounded in actual fetched/search evidence
- runtime-loop correctness should stay separable from pre-search query rewriting

### Planner Contract Is Structured and Guarded

The new planner is not intended to be a free-form reasoning blob.

Its runtime role is to produce a validated structured decision artifact over a
compressed runtime summary that includes:

- request / iteration state
- search summary
- fetch summary
- evidence summary
- progress summary
- baseline rule-policy decision
- allowed actions for the current output mode

The planner output is constrained to structured fields such as:

- `decision`
- `reason`
- `confidence`
- `next_query`
- `preferred_urls`
- `avoid_urls`
- `answer_plan`

The runtime must validate planner output before accepting it. In particular:

- actions must be allowed for the current output mode
- URLs must come from known search/fetch state
- answer/handoff decisions must be evidence-grounded
- continue decisions must still respect iteration and no-progress guards

So planner integration belongs in the decision layer, but remains subordinate
to runtime guardrails rather than replacing them.

### Fetch-Text Compression Strategy Is Intentionally Conservative

The planner needed a cleaner fetch-text input surface, but the current design
intentionally stops at deterministic cleaning rather than aggressive extraction
or ranking.

For now, the fetch side should provide:

- cleaned text
- boilerplate removal
- duplicate-line removal
- whitespace normalization
- paragraph-preserving structure

but should not yet require:

- query-aware passage ranking
- LLM-generated page summaries
- relevance-scored extraction pipelines

This keeps the planner-input contract simpler and avoids mixing runtime-control
questions with a second, premature summarization architecture.

### Handoff Is Now a First-Class Projection Layer, Not an Ad-Hoc Service Return

The runtime now needs a stable contract for handing evidence and policy
constraints to an outer answer service.

That contract is currently represented by a dedicated `handoff` package rather
than ad-hoc assembly inside `service.go`.

The package boundary now separates:

- runtime execution state
- prompt-ready handoff projection

The key handoff surfaces are:

- `ToolContext`
- `AnswerGuidance`
- `WorkflowPolicy`
- `EvidenceBundle`
- `DecisionSummary`

This is important because it preserves the runtime's typed-state model while
also acknowledging that an outer answer layer may need a prompt-oriented view
of the same run.

### Workflow Policy Should Be Projected From Runtime and Capability Metadata

Earlier placeholder-style policy text is no longer sufficient once handoff
becomes a real architectural seam.

The runtime should now treat `WorkflowPolicy` as a projection over:

- runtime options
- output mode
- max iterations
- capability usage
- capability risk
- approval requirements
- execution mode

rather than a hardcoded string such as "capability: search / risk: low".

This projection model is the correct long-term direction because it lets the
handoff layer remain faithful to the actual runtime contract even as capability
families expand.

### Updated Architectural Implication

With these changes, the most accurate current architecture reading is:

1. the runtime kernel, state system, replay, checkpoint, and capability seams
   remain the foundation
2. the first reactive pattern is now no longer only a toy answer loop; it has
   an explicit handoff-oriented terminal mode
3. planner logic is now anchored after `observe`, where it can use real
   evidence/progress state
4. the default public path is converging toward:
   - agent runtime gathers evidence and decision metadata
   - outer answer service later consumes handoff output
5. final-answer generation inside `agent` is now best treated as an optional
   output mode, not the default identity of the runtime

So the runtime design is now materially closer to the long-term goal of a
general agent execution system with a clean outer integration seam, rather than
only a self-contained answer-producing loop.

## Additional Update: 2026-05-31 Capability Registry, Pattern Contract, and Handoff Projection Refinement

### Status Update

As of `2026-05-31`, the design should no longer describe the runtime's
capability layer as only:

- a typed search/fetch seam
- a minimal registry
- one reactive pattern wired directly from service assembly

That was accurate for the earlier `M1.5` closure, but it is now incomplete.

The runtime now also has:

- a unified capability registry model
- explicit `kind / family / role` capability semantics
- reusable role binding resolution
- a pattern-facing assembly contract
- registry-driven handoff/profile projection
- reactive-specific node binding projection separated back out of `service.go`

This is an important refinement because it clarifies that the current work is
not just "adding more capabilities," but defining the structural contract that
future capabilities and future patterns will live inside.

### Capability Should Now Be Read As a Runtime-Governed Task Unit

The design previously established that tool / workflow / sub-agent should all
eventually be modeled as capability.

That direction is now concretized in code.

The current capability spec now explicitly models:

- `Name`
- `Kind`
- `Family`
- `Roles`
- `RiskLevel`
- `RequiresApproval`
- `SupportsParallel`
- `SupportsResume`
- `Dependencies`

This means the capability abstraction is no longer merely:

- "a search adapter"
- "a fetch adapter"

but a runtime-governed execution unit with both:

- execution semantics
- policy / projection metadata

The first explicit `Kind` vocabulary is now:

- `tool`
- `workflow`
- `sub_agent`

This is exactly the direction the design intended, and it materially improves
the chance that later higher-level capabilities can be added without having to
redesign the registry again.

### Registry Is Now Unified, Indexed, and Validation-Oriented

The runtime design argued that capability should be centrally governable rather
than scattered across ad-hoc execution seams.

That is now more true than the earlier "minimal registry" description implied.

The registry is now responsible for:

- registering a common capability handle
- storing normalized spec metadata
- indexing capabilities by:
  - `name`
  - `role`
  - `family`
- resolving typed capability interfaces from the unified catalog
- validating:
  - required fields
  - duplicate names
  - self-dependencies
  - missing dependencies
  - role/interface consistency

This matters architecturally because it means the runtime now has a real
capability catalog, not only a pair of search/fetch maps.

### Role Binding Is Now a Shared Assembly Concept

One earlier ambiguity in the design was whether pattern assembly should keep
inventing pattern-local capability name fields.

That ambiguity is now resolved in a better direction.

Role binding is now a reusable capability-layer concept:

- patterns bind roles to capability names
- explicit bindings are validated against the registry
- implicit binding is allowed only when a role has a unique candidate
- ambiguous role resolution is rejected

So the assembly direction is now:

- pattern requests roles
- assembly supplies bindings
- registry validates and resolves them

rather than:

- each pattern grows one-off fields like `SearchCapabilityName`

This is the correct long-term shape if the runtime is meant to support more
than one pattern.

### Pattern Contract Has Been Lifted Out of Reactive-Local Configuration

The design already said runtime should be decoupled from any one pattern.

The code now takes a concrete step in that direction through a shared
pattern-facing assembly contract:

- `AssemblyContext`
- `RuntimeConfig`

This means reactive no longer has to be treated as the place where:

- registry access
- bindings
- planner seam
- output mode
- kernel config

are all mixed together in a pattern-specific config blob.

Architecturally, this is an important precondition for adding a second pattern.

Without this step, a second pattern would likely have repeated or forked the
same assembly concerns in a less controlled way.

### Handoff/Profile Projection Is Now Metadata-Driven

Another design question was whether handoff policy and capability profile
projection would remain service-local hardcoding.

That is no longer the case.

The current handoff layer now projects capability profiles from:

- registry capability spec
- explicit node-to-capability bindings

And the `family -> workflow capability` mapping now lives in one place rather
than being scattered through service assembly.

The current rule set is:

- `external_evidence -> search`
- `document_investigation -> diagnosis`
- `trace_investigation -> diagnosis`
- `discovery -> knowledge`
- fallback -> `general`

This is still a V1 projection rule, but it is now in the correct architectural
layer and can be evolved without pushing more domain mapping logic back into
`agent.Service`.

### Service Assembly Is Closer to the Intended Architecture Boundary

The service layer now looks more like a proper composition root:

- build search/fetch services
- wrap them as capabilities
- register capabilities
- declare role bindings
- compile the reactive pattern
- build handoff from registry metadata

It does less "secretly own the pattern model" work than before.

That is an architecture improvement, not just a refactor convenience.

It means the current public service path is closer to the intended design rule:

- service assembles
- pattern executes
- capability catalog governs
- handoff projects

### Updated Architectural Implication

With this refinement, the most accurate current architecture reading is:

1. the capability layer is no longer only typed; it is now also categorized and
   indexed by runtime-facing metadata
2. the registry is no longer just a thin convenience wrapper; it is becoming a
   real governance seam
3. pattern assembly is now explicit enough that a second pattern can be added
   without immediately duplicating assembly concerns
4. handoff/profile projection is now meaningfully separated from service
   assembly and tied back to capability metadata
5. the current best next step remains:
   - do **not** rush into many low-level query capabilities
   - do **not** wire the new runtime into `RagChatService` too early
   - first use the new registry/contract shape as the base for a second,
     structurally different pattern

## Additional Update: 2026-06-01 Capability V2, Runtime Approval, and Resume Closure

### Status Update

As of `2026-06-01`, the current design should no longer describe
`internal/app/agent` as only:

- a typed-state reactive runtime
- a capability registry plus role-binding catalog
- a handoff-oriented output-mode experiment

Those are still important, but they are now no longer sufficient to describe
the actual runtime boundary.

The runtime has now completed another meaningful closure around:

- capability V2 execution contract
- metadata-driven runtime policy
- approval as a first-class runtime state domain
- public approval outcome and approval resume flow

This materially changes how the capability layer and approval system should be
understood in the architecture.

### Capability Should Now Be Read as a Governed Runtime Object

Earlier versions of the design said that tool / workflow / sub-agent should all
eventually become capability.

That direction is now more concrete.

The capability layer is now centered on:

- `Spec`
- `Handle`
- `InvocationRequest`
- `InvocationResult`
- `ActionRecord`
- `ObservationRecord`

This is more important than a simple interface cleanup because it means the
runtime now has one contract that can simultaneously serve:

- registry governance
- node execution
- replay / inspection
- handoff projection
- approval / retry policy

So capability should now be understood as:

> a declared, invokable, policy-aware runtime unit

rather than only "a typed adapter around search or fetch."

### Capability Metadata Is Now Starting to Drive Runtime Policy

The design previously argued that capability metadata should exist, but not all
of it was yet operationally meaningful.

That has now changed.

The runtime now carries richer capability metadata such as:

- `InputSchema`
- `OutputSchema`
- `Preconditions`
- `ProducesEvidence`
- `Idempotency`
- `SupportsResume`

And this metadata is no longer only descriptive.

It now influences runtime behavior such as:

- rejecting invalid capability input through declared preconditions
- classifying failures into:
  - `validation_error`
  - `dependency_error`
  - `external_error`
  - `permission_error`
- deciding when retry / continue is allowed
- blocking unsafe retry when idempotency or resume support is not suitable
- validating workflow capability use in evidence-producing runtime paths

So the design should now treat capability metadata as a real policy input to
runtime decisions, not merely handoff or catalog metadata.

### Search / Fetch Are No Longer Capability-Layer Special Cases

The runtime boundary is now cleaner:

- `internal/app/agent/capability`
  - owns runtime-facing contracts and governance
- `internal/app/agent/search`
  - owns search capability adaptation
- `internal/app/agent/fetch`
  - owns fetch capability adaptation
- `internal/app/agent/external_evidence`
  - owns a higher-level workflow capability sample

This means the capability root package should now be interpreted as a contract
and registry layer, not as the place where concrete runtime abilities are
implemented.

Architecturally, that is important because it supports the intended direction:

- low-level capability
- workflow capability
- later sub-agent capability

all living under the same governed execution model.

### Approval Is Now a Dedicated Runtime State, Not Only an Interrupt Detail

One of the more important design changes is that approval should no longer be
thought of as just:

- `InterruptBeforeNodes`
- `Execution.Interrupted`
- `InterruptReason`

Those remain part of the operational implementation, but they are not enough as
the architectural model.

The runtime now has a dedicated `ApprovalState`, carrying semantics such as:

- `pending`
- `approved`
- `rejected`
- approval reason
- gated capability identity
- rerun node
- checkpoint id

This is the correct direction because approval is no longer merely a low-level
control-flow interruption. It is now a real domain of runtime state that can be:

- projected outward
- persisted across pause/resume
- reasoned about by the service layer

### Public Run Outcome Is Now an Explicit Architecture Seam

The public `agent.Service` surface now distinguishes:

- `completed`
- `degraded`
- `awaiting_approval`

through explicit detailed run-result APIs.

This is an important architecture event because the design previously had a
gap between:

- runtime internal interrupt semantics
- outer service-facing execution semantics

That gap is now meaningfully reduced.

The service layer can now expose:

- normal completion
- degraded completion
- paused pending approval

without forcing outer callers to understand Eino interrupt details directly.

### Approval Resume Now Has a Real Runtime-Service Closure

The earlier design goal for interrupt / approval was not simply "pause
execution," but "pause and later continue safely."

That closure now exists in first form.

The current approval lifecycle is now:

1. runtime reaches an approval boundary
2. service exposes `awaiting_approval`
3. service persists resumable runtime session state
4. caller later supplies approve / reject decision
5. runtime resumes from checkpoint and:
   - reruns the gated node path
   - or terminates through degrade on rejection

This is implemented through a combination of:

- checkpoint persistence
- runtime session persistence
- `ApprovalState`
- a reactive `approval` gate node
- approval-aware service resume APIs

This is a much more mature architectural state than the earlier "interrupt
exists in principle" design.

### The Approval Gate Design Clarifies a Key Eino Boundary

An earlier spike already showed that in-node interrupt/resume semantics are not
the preferred long-term path for this runtime.

The current implementation direction now reinforces that conclusion.

The preferred model remains:

- use compile-time `InterruptBeforeNodes` for stable interrupt points
- persist session state outside checkpoint bytes when outer decisions are needed
- use runtime state + service-layer decision injection to resolve approval
  before re-entering execution

So the architecture still aligns with the earlier spike conclusion:

- Eino Graph provides durable execution and checkpoint boundaries
- the runtime/service layer owns the higher-level approval semantics

### Updated Architectural Implication

With this `2026-06-01` closure, the most accurate design reading is now:

1. capability is no longer only a registry item; it is becoming the unit where
   execution, policy, replay, and approval semantics meet
2. approval is no longer just an interrupt flag; it is a dedicated runtime
   state domain and a service-facing lifecycle
3. the new runtime can now model a real human-in-the-loop pause/resume loop
   rather than only static interrupt points
4. the service boundary is now explicit enough to carry approval outcomes
   without leaking low-level Eino interrupt machinery
5. the next best design steps are:
   - stabilize approval persistence and external contract semantics
   - validate a second pattern on top of the same capability/runtime model
   - defer full `RagChatService` integration until these boundaries remain
     stable

## Additional Update: 2026-06-02 Capability Selection Before Pattern Routing

### Status Update

As of `2026-06-02`, the design direction has been refined again:

- do **not** rush to make `pattern` routing LLM-driven first
- first make capability composition and capability choice LLM-driven on top of
  the current registry/runtime model

This is an important clarification of sequencing.

The architecture goal still includes eventual decoupling between:

- `pattern`
- `capability`

but the current preferred path is now:

1. keep pattern selection explicit
2. make capability exposure / selection / validation intelligent
3. only later decide whether request-time pattern routing should also become
   model-driven

This means the runtime is currently prioritizing **capability-level
intelligence before pattern-level intelligence**.

### Capability Selection Should Now Be Modeled as Its Own Layer

The earlier design already said:

- runtime should be decoupled from one pattern
- capability should become the unified execution abstraction

The new closure adds another missing piece:

- capability choice itself should be a dedicated architecture seam

The current architecture should now be read as moving toward:

```text
registry -> catalog -> selector -> resolver -> execution
```

where:

- `registry`
  - remains the governed source of capability truth
- `catalog`
  - projects runtime metadata into an LLM-facing capability directory
- `selector`
  - chooses capability by semantics rather than direct implementation coupling
- `resolver`
  - converts model choice back into one validated executable runtime unit

This is a stronger shape than letting each pattern directly hardcode capability
names or embed capability-routing heuristics locally.

### The Capability Layer Now Has a Model-Facing Projection

The original capability design focused on runtime-governed metadata such as:

- kind
- schema
- risk
- approval
- dependencies

That remains correct, but as of `2026-06-02` it is no longer sufficient to
describe the intended architecture.

Capability metadata now also needs a **model-facing projection layer**.

That projection is intentionally smaller and more selector-oriented, exposing
fields such as:

- `name`
- `kind`
- `family`
- `roles`
- `summary`
- `input_hints`
- `requires_approval`
- `supports_resume`
- `produces_evidence`

Architecturally, this matters because the LLM should not have to consume raw
implementation detail or infer execution semantics indirectly.

Instead, the runtime now has an explicit place where:

- governed capability metadata
- model-visible capability semantics

meet cleanly.

### LLM Capability Choice Is Now a Guarded Structured Contract

The design should now explicitly distinguish two things:

1. model chooses a capability
2. runtime accepts or rejects that choice

The selector layer is not free-form planning text.

It is a structured contract that can select capability through:

- `name`
- `family`
- `role`
- optional `kind`
- structured `input`

And the runtime does not trust that output blindly.

A resolver/validator layer now sits between:

- selector output
- actual invocation

and is responsible for:

- unknown capability rejection
- ambiguous family/role rejection
- input normalization
- schema/precondition validation

So the design should now treat capability selection as:

> LLM-guided but runtime-governed

not as a direct model-to-invocation shortcut.

### Input Normalization Is Now a Required Part of the Capability Contract

One practical gap became clear during implementation:

even if the model correctly chooses a capability, selector-produced input often
arrives as generic JSON-like data rather than already-typed Go input structs.

Therefore the capability contract should now also be read as including an input
normalization seam.

This is currently expressed through a shared normalization contract that lets a
capability accept model-produced structured input and transform it into the
typed runtime input shape expected by execution and precondition validation.

Architecturally, this is important because it closes the gap between:

- model-facing structured capability planning
- typed runtime capability execution

Without this seam, capability selection would remain only half complete.

### `PlanStep` Should No Longer Be Read as Search/Fetch-Specific

The initial `plan_execute` implementation used explicit plan state, but its
first real plan shape was still strongly aligned to:

- `search`
- `fetch`

That is now no longer the right architectural reading.

`PlanStep` should now be understood as carrying capability-selection semantics,
including fields such as:

- `CapabilityName`
- `CapabilityKind`
- `CapabilityFamily`
- `CapabilityRole`
- `CapabilityInput`

This is a meaningful architecture step because the second pattern is no longer
only validating:

- explicit step sequencing

It is now also validating:

- whether plan steps can carry capability semantics without being locked to one
  hardcoded capability pair

### `plan_execute` Is Now the First Consumer of Selector-Driven Capability Semantics

The current design path deliberately uses `plan_execute` as the first place to
consume the new capability-selection stack.

This is a good fit because `plan_execute` naturally has:

- an explicit planning point
- explicit step materialization
- explicit step execution

So it is the most natural pattern for validating:

- selector-driven capability choice
- selector-to-step projection
- resolver-mediated execution

before trying to push the same behavior deeper into the reactive loop.

### `document_investigation` Is the First High-Level Validation Target

The design previously argued that the next valuable capabilities should be
higher-level task-oriented runtime units rather than many more low-level query
tools.

That direction is now reinforced by implementation.

`document_investigation` is now the first concrete capability family used to
validate the new selector path.

This is important because it proves the intended architecture path:

- capability is registered once
- metadata is surfaced to the model
- model chooses by capability semantics
- runtime resolves and executes it through the same governed contract

without requiring a new pattern or custom service-local routing logic.

### Updated Architectural Implication

With this `2026-06-02` closure, the most accurate design reading is now:

1. the runtime architecture now includes a distinct capability-selection layer,
   not only registry + execution
2. capability metadata should now be understood as serving both:
   - runtime policy
   - model-facing capability selection
3. selector output must remain constrained by a resolver/validator seam, so
   capability choice is LLM-guided but still runtime-governed
4. `plan_execute` is now validating not only explicit plans, but also
   selector-driven capability semantics inside plan steps
5. the current best next steps are:
   - keep pattern routing explicit for now
   - broaden selector-driven capability usage across more capability families
   - later decide whether and how request-time pattern routing should become
     LLM-driven on top of this same capability-selection foundation

## Additional Update: 2026-06-02 Approval Contract and Lifecycle P0 Closure

### Status Update

As of `2026-06-02`, the approval architecture should no longer be read only as:

- runtime can interrupt before approval-gated nodes
- service can expose `awaiting_approval`
- resume can continue from checkpoint

Those remain true, but they are now architecturally incomplete.

The current implementation line has now completed another important closure
around:

- outward approval payload projection
- outward approval resume decision contract
- service-facing typed error semantics
- approval persistence cleanup guarantees
- approval audit-state verification

This is important because approval is no longer merely an internal runtime
control-flow mechanism. It is now moving toward a more explicit **public
service contract**.

### Approval Pending Should Now Be Read as a Projected Service Contract

Earlier design updates correctly established that approval is a dedicated
runtime state domain.

That still matters, but the latest implementation adds another missing layer:

- approval state must also be projected outward in a caller-usable shape

The new outward approval payload is no longer only:

- capability name
- checkpoint id
- interrupt reason

It now includes richer public-facing semantics such as:

- approval status
- reason code
- readable reason message
- trigger type
- rerun node
- capability metadata
- request / query / step / candidate URL context
- approve / reject action semantics

Architecturally, this matters because the runtime state is now beginning to
have a clearer service projection rather than requiring outer layers to inspect
internal snapshot fields directly.

### Approval Resume Is Now Better Understood as an Explicit Decision Contract

Earlier implementation already supported resume after approval, but the public
call shape was still too close to an internal convenience mechanism.

The current line now treats approval resume more explicitly through:

- `Decision = approved | rejected`

with boolean compatibility retained only as a fallback path.

This is a healthier architecture shape because approval resume is now less like
"set some flag and try again" and more like:

> caller submits an explicit approval decision into a governed runtime session

That aligns better with the service-facing approval lifecycle the design has
been moving toward.

### Service Error Semantics Are Now Part of the Approval Contract Boundary

One practical gap became clear during this approval work:

even if runtime approval state was modeled correctly, outer callers still had
to infer failure semantics from free-form error strings.

That gap is now partially closed through a typed service error contract that
exposes:

- stable error code
- error kind
- retryability

The approval lifecycle can now distinguish service-level failure classes such
as:

- invalid request
- not found
- failed precondition
- unavailable
- internal

This is an architecture improvement because approval is no longer only
"runtime state + checkpoint lifecycle"; it now also has a better-defined
failure surface at the service boundary.

### Approval Persistence Must Now Be Read as a Lifecycle Guarantee

Earlier design updates already said that approval pause/resume requires:

- checkpoint persistence
- session persistence

The latest implementation hardening clarifies another important detail:

- persistence aliases must also be cleaned up correctly on terminal paths

The runtime/service path previously stored pending approval sessions under both:

- checkpoint id
- session id

but terminal cleanup only removed one alias.

That has now been corrected.

Architecturally, this reinforces that approval persistence is not merely
"runtime cache state"; it is part of the lifecycle contract and must behave
coherently across:

- pending
- approved
- rejected
- completed

### Approval Audit Metadata Is Now Better Defined

The design has long argued that runtime state should be traceable and
auditable.

The latest approval closure now validates that this principle also applies to
the approval lifecycle itself.

Approval audit metadata is now explicitly verified around fields such as:

- `RequestedAt`
- `ReviewedAt`
- `DecisionNote`
- `ApprovalDecision`
- `ApprovalNote`
- `ResumeCount`
- `ResumedFrom`

This matters because approval should now be read not only as:

- pause
- decision
- resume

but also as:

- a reviewable lifecycle with audit-relevant metadata

### Handoff Output Mode Is Now Included in Approval Lifecycle Validation

Another useful closure is that approval is no longer validated only on the
final-answer path.

The current implementation line now also explicitly covers:

- `RunHandoffDetailed`
- `ResumeHandoffAfterApproval`

This is a good architecture signal because approval is now closer to being a
runtime/service concern shared across public output modes, rather than a
behavior only proven on one answer path.

### Updated Architectural Implication

With this additional `2026-06-02` closure, the most accurate design reading is
now:

1. approval is not only a runtime state domain; it now also has a richer
   outward service projection
2. approval resume is no longer best understood as a boolean convenience path;
   it is becoming an explicit decision contract
3. the approval boundary now includes a typed service error surface, not only
   checkpoint and runtime semantics
4. approval persistence must now be treated as a full lifecycle guarantee,
   including correct alias cleanup on terminal paths
5. approval audit metadata is now better anchored in tests, which makes the
   service/runtime lifecycle more explicit
6. the next best design steps after this P0 closure are:
   - bridge these approval/runtime/service contracts into outer transport/chat
     layers
   - keep stabilizing the same outward contract before full chat-path adoption
   - continue broader post-P0 runtime evolution outside the approval P0 scope

## 十七、M1 Spike 验证结果

日期：2026-05-29

代码位置：`internal/app/agent/kernel/spike/`

### 验证目标

在投入正式 kernel 搭建之前，验证 Eino Graph 的三项未在现有代码中使用过的关键能力：

1. **Branch 路由** — `NewGraphBranch` + `AddBranch`
2. **Checkpoint 持久化** — `WithCheckPointStore`
3. **Interrupt 中断/恢复** — `WithInterruptBeforeNodes` + checkpoint resume

同时验证 typed state + event journal 在 Eino Graph 中的传播模式是否可行。

### Spike 代码结构

```text
internal/app/agent/kernel/spike/
├── spike_types.go         # 最小化的 StateSnapshot / Event / Evidence 模型
└── eino_spike_test.go     # 5 个 spike 测试
```

`SpikeState` 是设计 doc 中 `StateSnapshot` 的最小化版本，包含：
- Request 域（`Question`）
- Context 域（`SearchQuery / SearchResult / FetchResult`）
- Evidence 域（`[]EvidenceItem`）
- Answer 域（`Answer / DegradeReason`）
- Execution 域（`[]NodeEvent` journal、`Rounds / MaxRounds`）

### 验证结果总览

| 测试 | Eino 能力 | 结果 | 关键发现 |
|---|---|---|---|
| **Test 1**: Branch Routing | `NewGraphBranch` + `AddBranch` | ✅ PASS | 条件分支 `prepare → branch(hasEvidence?) → enrich / quick_answer → answer` 两个子场景均正确 |
| **Test 2**: Interrupt + Resume | `WithInterruptBeforeNodes` + `WithCheckPointStore` | ✅ PASS | 4 节点图 (prepare→search→[中断]→fetch→answer) 首次运行停在 fetch 前，checkpoint 保存；恢复后从 fetch 继续执行，journal 正确包含全部节点事件 |
| **Test 3**: In-Node Interrupt API | `compose.Interrupt(ctx)` + `ExtractInterruptInfo` | ✅ PASS | API 契约验证通过。`RerunNodes`、`RerunNodesExtra` 正确提取。`StatefulInterrupt` 签名可用 |
| **Test 4**: Reactive Loop | typed state + branch + journal | ✅ PASS | `prepare→plan→execute→evaluate→branch→(answer\|degrade)` 完整链路，answer/degrade 分支正确，journal 累积 9 个事件 |
| **Test 5**: State Preservation | checkpoint 状态保持 | ✅ PASS | 中断恢复后 `Rounds=1, Evidence=1, Journal=2 events` — checkpoint 正确保持累积状态，节点不会重复执行 |

### 发现 1：序列化注册是必需的

Eino 的 checkpoint 机制依赖内部序列化器。**任何进入 graph state 的自定义类型都必须注册**，否则 checkpoint 保存时静默失败：

```go
import "github.com/cloudwego/eino/schema"

func init() {
    schema.RegisterName[*SpikeState]("spike_SpikeState")
    schema.RegisterName[EvidenceItem]("spike_EvidenceItem")
    schema.RegisterName[NodeEvent]("spike_NodeEvent")
}
```

**对正式 runtime 的影响：**
- `StateSnapshot` 及其所有嵌套结构体都需要注册
- 如果用 `interface{}` / `any` 作为字段类型（如 `Payload any`），序列化可能失败
- 建议在 M1 kernel 搭建时就建立注册约定：每个 state 域的文件顶部写 `init()` 注册

### 发现 2：In-node Interrupt 的局限性

`compose.Interrupt(ctx, info)` 在 Lambda 节点内调用后：
- ✅ checkpoint 保存成功
- ✅ `compose.ExtractInterruptInfo(err)` 正确提取 `RerunNodes` 和 `RerunNodesExtra`
- ❌ resume 时节点收到 **nil state**（Eino v0.8.13），导致 panic

这意味着 in-node interrupt 不适合在 Lambda 内做 "暂停 → 改状态 → 继续" 的循环。正确用法是让节点检查外部状态（如 DB 中的审批标记），resume 时外部状态已变化，节点重新执行并得到不同结果。

**对正式 runtime 的影响（重要架构决策）：**
- **推荐**：新 runtime 使用 `WithInterruptBeforeNodes` 声明中断点 — 在 graph compile 时确定哪些节点前需要中断
- **配合**：capability spec 的 `risk=high` → 自动在对应 capability 节点前插入中断
- **保留**：`compose.Interrupt()` 作为 ADK 层（如 future sub-agent delegation）的动态中断机制，不在 kernel 层直接使用

### 发现 3：Eino Graph 作为执行底座的边界已探明

以下能力在 M1 spike 中已验证可以用于新 runtime：

| 能力 | Eino API | 新 runtime 中的角色 |
|---|---|---|
| 类型化图 | `NewGraph[I, O]()` | 承载 `*StateSnapshot` 的数据流 |
| Lambda 节点 | `AddLambdaNode` + `InvokableLambda` | `prepare / plan / execute / evaluate / answer` 等节点 |
| 条件分支 | `NewGraphBranch` + `AddBranch` | `evaluate → branch → (answer \| degrade \| continue)` |
| 编译时中断 | `WithInterruptBeforeNodes` | capability 审批门禁 |
| Checkpoint 持久化 | `WithCheckPointStore` | 中断恢复、replay 基础 |
| Checkpoint ID | `WithCheckPointID` | session 级别的 checkpoint 管理 |
| 强制重跑 | `WithForceNewRun` | 开发调试、异常恢复 |

以下能力仍待验证（M6 之前需要 spike）：

| 能力 | Eino API | 风险 |
|---|---|---|
| Sub-graph / nested graph | `AddGraphNode` | 未使用过，复杂度未知 |
| Stream 模式 | `StreamReader` + stream branch | 未使用过，现有系统依赖 SSE |
| Pregel 模式（循环图） | `runTypePregel` | 多重迭代的语义与 reactive loop 不同 |
| 大规模 checkpoint | checkpoint 存储性能 | 内存 store 仅适用于 spike |

### 对后续里程碑的建议

1. **M1 → M2 可以直接推进**：branch、checkpoint、interrupt(compile-time) 三条核心能力已确认可用
2. **M0 的 StateSnapshot 设计应受 spike 约束**：
   - 所有嵌套 struct 必须有 `schema.RegisterName` 注册
   - 避免 `any` 类型字段（或限定为已注册类型）
   - 建议第一个版本只定义 4 个域（Request / Context / Evidence / Execution），与 spike 的 `SpikeState` 保持一致
3. **Interrupt 机制选择已明确**：compile-time `WithInterruptBeforeNodes` 为主，in-node `Interrupt()` 留给 ADK 层
4. **`internal/app/agent/kernel/spike/` 的后续处理**：成功模式（typed state、branch、checkpoint pattern）提升到 `kernel/` 正式代码；spike 目录在 M1 完成后删除
