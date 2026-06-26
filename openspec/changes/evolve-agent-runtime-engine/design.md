# evolve-agent-runtime-engine Design

## Overview

本设计基于现有 `internal/app/agent` 模块做架构收敛。

当前代码已经具备较完整的 agent runtime 雏形：

- `kernel` 提供 graph runner、node contract、checkpoint store。
- `runtime` 提供 `RuntimeSession`、session store、pending approval store、replay/projection。
- `state` 提供 snapshot、delta、reducer、runtime event。
- `pattern` 已有 `reactive` 和 `planexecute`。
- `capability` 已有 registry、spec、catalog、selector、resolver。
- service 层已经支持 run、approval、resume、pending approval lookup。

因此本设计不引入平行架构，而是将这些既有部件正式组织成：

```text
Agent Service
  -> Runtime Engine Facade
  -> Pattern Runtime
  -> Kernel Runner
  -> Capability Scheduler
  -> State Reducer + Event Journal
  -> Runtime Session Store / Pending Approval Store
```

## Existing Runtime Findings

### Runtime Session

`runtime.RuntimeSession` 已经是顶层运行容器，包含：

- request envelope
- initial snapshot
- current snapshot
- journal
- checkpoint ref
- metadata

这应作为 runtime state 的主容器继续保留。

### Kernel Runner

`kernel.Runner` 已经提供：

- `Run`
- `RunWithCheckpoint`
- `Resume`
- interrupt metadata update
- checkpoint ref update
- state applied event
- resume completed event

这说明底层执行引擎已经存在，不需要重新造 graph runner。

### Pattern Assembly

`service_assembly.go` 中 `compileRunner` 已经根据 pattern name 编译：

- `plan_execute`
- `reactive`

`pattern.RuntimeConfig` 已经注入跨 pattern 依赖：

- planner
- capability catalog builder
- capability selector
- capability resolver
- approval session store
- kernel config

这说明 Pattern 抽象已经在形成，但还需要更明确的 contract 和约束。

### Capability Policy Fields

`capability.Spec` 已经包含：

- `RequiresApproval`
- `SupportsParallel`
- `SupportsResume`
- `RiskLevel`
- `Idempotency`
- `Preconditions`

这些字段应成为 scheduler 的输入，不应在不同 pattern 中重复解释。

## Design Principles

### 1. One Engine, Multiple Patterns

系统保留多种 agent 范式，但只保留一套 runtime 生命周期。

### 2. Pattern Decides Strategy, Runtime Owns Mechanics

Pattern 决定“下一步做什么”，Runtime 决定“如何可靠执行、记录、审批、恢复”。

### 3. Reuse Existing Runtime Primitives

优先收敛现有 `RuntimeSession`、`kernel.Runner`、`StateDelta`、`RuntimeEvent`，避免新增同义概念。

### 4. Capability Policy is Declarative

Capability 通过 spec 描述风险、审批、并发、恢复和幂等性。Runtime/Scheduler 统一消费这些声明。

### 5. Events Are the Runtime Truth

SSE、trace、approval restore 和 replay 应尽量从同一 runtime event journal 派生。

## Component Boundaries

### Agent Service

职责：

- 接收外部请求。
- 选择或接收 pattern。
- 创建 runtime session。
- 调用 runtime engine。
- 将 runtime outcome 映射为现有 response / SSE / pending approval lookup。

不负责：

- pattern 内部策略。
- capability 调度细节。
- checkpoint reducer 细节。

### Runtime Engine Facade

职责：

- 管理 runtime session lifecycle。
- 调用对应 pattern runner。
- 统一 `Run`、`Resume`、`RunWithCheckpoint`。
- 统一处理 interrupt、approval pending、resume completed。
- 统一输出 runtime outcome。

第一阶段可作为现有 service/kernel 之间的薄封装，不要求大规模迁移。

### Kernel Runner

职责：

- 执行 compiled graph。
- 调用 node。
- 应用 checkpoint / interrupt 机制。
- 通过 reducer 应用 state delta。
- 追加 runtime event。

Kernel 不关心业务 pattern 的语义。

### Pattern

职责：

- 定义 graph / node flow。
- 决定计划、行动、观察、评估、replan、stop/degrade。
- 维护 pattern-specific state。

限制：

- 不直接持久化 approval session。
- 不定义独立 event protocol。
- 不绕过 scheduler 调用 capability。
- 不拥有全局 checkpoint store。

### Capability Scheduler

职责：

- 根据 capability spec 和 runtime options 生成 scheduling decision。
- 对 approval、parallel、resume、risk、idempotency、precondition 做统一判断。
- 负责 capability start/result/skipped event。
- 负责标准化 capability error class。

第一阶段可以先围绕现有 capability invocation 做一层调度封装。

### State Reducer and Event Journal

职责：

- 所有状态变化通过 `StateDelta` 进入 reducer。
- 所有重要运行事实通过 `RuntimeEvent` 进入 journal。
- replay/projection 不直接读取 pattern 私有状态，而从 snapshot + journal 派生。

## Pattern Model

### Reactive

定位：

- 适合探索型、诊断型、短链路任务。
- 可以边观察边决定下一步。
- 不要求先生成完整计划。

典型场景：

- 外部搜索补证。
- 轻量问题澄清。
- 单轮或短链路诊断。
- 不确定下一步能力调用的探索。

### Plan Execute

定位：

- 适合明确目标、多步骤任务。
- 适合展示计划、步骤、进度和恢复。
- 适合 approval-gated 或 mixed-capability 任务。

典型场景：

- 多步骤资料调查。
- 文档/任务诊断。
- 需要审批的外部访问。
- 失败后 retry / replan / degrade。

### Future Patterns

未来可以新增：

- `research`
- `workflow`
- `code_agent`
- `supervisor`

但新增 pattern 只能接入共享 Runtime Engine，不能复制 session、approval、checkpoint、event、scheduler。

## Pattern Selection

第一阶段建议使用简单规则：

- 显式配置优先。
- 需要 approval / resume / 多步骤计划时使用 `plan_execute`。
- 探索型、短链路、轻量补证使用 `reactive`。
- 默认保持现状，agent service 使用 `plan_execute`。

暂不引入 LLM pattern selector，避免路由不可解释。

## Runtime Decision Model

Runtime decision 建议覆盖：

- `continue`
- `wait_approval`
- `resume`
- `retry`
- `replan`
- `degrade`
- `complete`
- `fail`

这些 decision 可以由 pattern node 产生，但最终由 runtime facade 归一化并记录到 event journal。

## Approval and Resume

approval 是 runtime-level decision。

流程：

```text
Capability requires approval
  -> Scheduler emits wait_approval
  -> Runtime applies ApprovalDelta
  -> Kernel persists checkpoint
  -> PendingApprovalStore maps conversation/user to checkpoint
  -> UI restores pending approval
  -> User approves/rejects
  -> Runtime resumes from checkpoint
  -> Journal records resume_completed or rejection
```

Pattern 可以声明当前动作需要 approval，但不能绕过 runtime 的 pending store、checkpoint 和 resume 流程。

## Capability Scheduling

Scheduling input：

- capability spec
- runtime options
- current snapshot
- pattern action
- approval status
- retry/error history

Scheduling output：

- execute now
- wait approval
- skip
- retry later
- reject
- degrade
- fail

第一阶段最小能力：

- `RequiresApproval` -> approval decision
- `SupportsParallel` -> execution grouping
- `SupportsResume` -> resume guard
- `RiskLevel` -> risk display / approval reason
- `Idempotency` -> retry policy input
- `Preconditions` -> validation failure

## Event Contract

现有 `RuntimeEvent` 可以继续使用，但事件类型需要稳定。

推荐事件族：

- run/session lifecycle
- node lifecycle
- decision
- branch
- capability
- approval
- checkpoint/interruption
- resume
- state applied
- answer/degrade/fail

事件要求：

- append-only
- sequence stable
- session-scoped
- node/capability/decision 可关联
- 可被 SSE 和 replay 消费

SSE / trace / replay 映射表：

| Runtime event family | SSE consumer | Trace consumer | Replay/projection consumer |
| --- | --- | --- | --- |
| `node_start` / `node_finish` / `node_error` | 展示当前运行节点与阶段切换 | 记录节点级时序与失败点 | 重建 node timeline 与最终 node status |
| `capability_start` / `capability_result` / `capability_skipped` | 展示工具调用、跳过与结果摘要 | 记录 capability 输入输出摘要与耗时 | 重建 capability attempts、结果与跳过原因 |
| `decision_emitted` / `branch_selected` | 展示 runtime 下一步决策 | 记录 decision reasoning / confidence / target | 重建 branch path、last decision、continue/degrade/answer 走向 |
| `approval_pending` / `approval_resolved` / `approval_rejected` | 恢复 approval UI 状态与操作入口 | 记录审批生命周期与 reviewer 决策 | 重建 approval status、checkpoint 关联和 resume 前置状态 |
| `interrupt` / `checkpoint_recorded` / `resume_completed` | 展示暂停、恢复与 checkpoint 状态 | 记录中断恢复边界 | 重建 checkpoint lifecycle、resume count 与 active 状态 |
| `state_applied` | 一般不直接展示原始 delta，仅派生高层状态 | 保留 reducer 输入用于审计 | 从 snapshot + delta 重建共享 runtime state |
| `answer_finalized` / `degraded` / `failed` | 展示最终答案、降级或失败状态 | 记录终局原因 | 重建 final outcome、degrade reason 与 failure class |

## Error Handling

标准 error class：

- validation
- permission
- approval_rejected
- external
- timeout
- budget
- no_progress
- model_output
- dependency
- unknown

Pattern 可以根据 error class 做策略判断，但 error class 的生成应由 runtime/scheduler 统一。

## Rollout

### Phase 1: Contract and Documentation

- 固化 OpenSpec。
- 明确边界。
- 添加必要 contract tests。
- 不改变默认 chat 入口。

### Phase 2: Runtime Facade

- 在 service 与 kernel runner 之间明确 runtime engine facade。
- 保留现有 pattern compile 方式。
- 统一 run/resume/outcome 处理。

### Phase 3: Scheduler Extraction

- 将 approval/parallel/resume/risk/idempotency 解释收敛到 scheduler。
- pattern 改为提交 capability intent。
- scheduler 输出 execution decision。

### Phase 4: Event Stabilization

- 整理 event 类型。
- 建立 SSE/trace/replay 映射。
- 补充事件顺序测试。

### Phase 5: Pattern Extension Readiness

- 确认新增第三种 pattern 时不需要复制 session、approval、checkpoint、event、scheduler。

## Testing Strategy

### Unit Tests

- pattern selection rules
- scheduler decision by capability spec
- approval decision generation
- resume guard by `SupportsResume`
- event sequence append
- reducer delta application

### Integration Tests

- `plan_execute` approval pending -> resume -> completed
- `reactive` approval pending -> resume/reject behavior
- capability requires approval cannot bypass scheduler
- checkpoint replay reconstructs pending approval state
- event projection supports pending lookup

### Compatibility Tests

- existing RAG chat path unchanged
- existing agent service default pattern remains `plan_execute`
- existing `reactive` tests continue passing
- existing `planexecute` tests continue passing

## Open Questions

本设计默认以下选择：

- 第一阶段不切换所有 chat 到 agent runtime。
- `plan_execute` 继续作为 agent service 默认 pattern。
- pattern selector 第一阶段使用规则，不使用 LLM。

如果这些默认选择被修改，需要同步调整 rollout 和测试范围。
