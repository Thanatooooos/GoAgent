# Evolve Agent Runtime Engine Design

## Goal

将 `internal/app/agent` 从“已有多种 agent pattern 的服务装配”继续收敛为一套边界清晰的 Agent Runtime Engine，同时保留 `reactive` 与 `plan_execute` 两种 Pattern，共享 session、checkpoint、approval、resume、event、replay、capability scheduling 和 compatibility boundary。

## Scope

本设计覆盖：

- `RuntimeSession`、`runtime.Engine`、`kernel.Runner`、`Pattern`、`Capability Scheduler`、`Reducer`、`RuntimeEvent` 的职责边界
- runtime-level approval / resume 语义
- scheduler 合同与 capability policy 收口方式
- budget / error policy 的 runtime ownership
- shared state / replay / projection / event journal 的收口方式
- 渐进式迁移与验证策略

本设计不覆盖：

- 全量替换普通 RAG chat 入口
- 废弃 `internal/app/rag/tool`
- 重写 Eino graph / kernel
- 新分布式调度系统
- LLM pattern selector

## Current Baseline

当前工作区已经存在本次收口的重要在途实现，可作为开发基线继续推进：

- `internal/app/agent/runtime/engine.go` 已引入 runtime facade 雏形
- `Service` 已通过 `runtimeEngine` 调 `RunWithCheckpoint` / `Resume`
- `RuntimeSession` 已包含 request、snapshot、journal、checkpoint、metadata
- `StateSnapshot` 已拆分为 `request/context/plan/evidence/approval/execution/answer/pattern`
- `projection` / `replay` / `reducer` 已开始围绕 shared state 与 journal 收口

因此，本次工作不是从零设计平行架构，而是沿既有实现继续补齐 contract、runtime ownership 与 scheduler 收口。

## Architecture

建议明确主链路为：

```text
Service
  -> Runtime Engine
  -> Kernel Runner
  -> Pattern Nodes
  -> Reducer + Event Journal
```

### Service

职责：

- 接收外部 request
- 创建或加载 `RuntimeSession`
- 调用 `runtime.Engine`
- 将 runtime outcome 投影为现有 response / handoff / pending approval lookup

不负责：

- graph 执行细节
- capability policy 判定
- pattern 内部策略
- checkpoint / approval / event 的核心生命周期语义

### Runtime Engine

职责：

- 成为 `RunWithCheckpoint` / `Resume` 的唯一生命周期 facade
- 归一化 runtime decision / outcome
- 将 approval pending、resume completed、interrupt、complete、degrade、fail 收口为统一语义
- 拥有全局 runtime budget policy 与统一 error policy 的归口解释权
- 驱动 scheduler、checkpoint、session persistence 与 response-facing runtime outcome

不负责：

- 编译具体 pattern graph
- 自己定义 pattern 私有执行策略

### Kernel Runner

职责：

- 执行 compiled graph
- 处理 checkpoint persistence
- 通过 reducer 应用 state delta
- 追加 runtime event 到 journal

不负责：

- pattern 选择
- capability policy 决策
- service response 映射
- frontend / SSE 投影

### Pattern

职责：

- 决定“下一步做什么”
- 定义 graph / node flow
- 维护 pattern-specific strategy
- 产生 capability intent、decision hint、replan / continue / degrade / stop 策略

限制：

- 不能直接拥有 session store
- 不能直接拥有 pending approval store
- 不能绕过 runtime scheduler
- 不能定义一套独立于 shared runtime 的 externally visible event protocol
- 不能拥有独立 approval / checkpoint lifecycle

### Reducer + Event Journal

职责：

- 成为共享状态写入口
- 记录 runtime truth
- 为 replay / projection / pending approval restore / trace / SSE 提供共享基础

## Runtime Decision Model

runtime decision 必须是正式合同，而不只是零散的中间状态。

### Canonical Decisions

- `continue`
- `wait_approval`
- `resume`
- `reject`
- `retry`
- `replan`
- `degrade`
- `complete`
- `fail`

### Ownership Rule

- Pattern 可以提出 decision hint
- Scheduler 可以输出 execution decision
- 最终对外与对 journal 可见的 runtime decision 由 `runtime.Engine` 归一化
- service response、pending approval restore、replay、trace、SSE 必须能够共享这组 decision 语义，而不是依赖 pattern 私有分支解释

## Pattern Strategy

### Reactive

定位：

- 探索型、诊断型、轻量补证、短链路任务
- 可边观察边决定下一步 action

约束：

- 仍使用 shared runtime session、scheduler、approval、checkpoint、event infrastructure

### Plan Execute

定位：

- 多步骤、可展示进度、审批敏感、mixed capability 任务
- 适合显式 plan、step execution、resume、replan

约束：

- 仍使用 shared runtime session、scheduler、approval、checkpoint、event infrastructure

### Default Routing

第一阶段保持：

- `plan_execute` 作为 agent service 默认 pattern
- `reactive` 保留并继续使用同一 runtime contract
- pattern selector 使用显式配置和规则，不引入 LLM selector

## Capability Scheduler Design

Scheduler 建议落在 `internal/app/agent/runtime`，先做成统一判定层，而不是重量级执行框架。

### Scheduler Input

- `capability.Spec`
- `RuntimeSession` / `StateSnapshot`
- runtime options
- pattern 请求的 capability intent
- 当前 approval status
- retry / resume context

### Scheduler Output

- `execute`
- `wait_approval`
- `skip`
- `retry`
- `degrade`
- `fail`

### Policy Ownership

Scheduler 统一消费并解释：

- `RequiresApproval`
- `SupportsParallel`
- `SupportsResume`
- `RiskLevel`
- `Idempotency`
- `Preconditions`

全局 runtime policy 归属约束：

- capability-level execution policy 由 scheduler 统一解释
- cross-pattern budget policy 由 runtime 统一拥有
- cross-pattern error policy 由 runtime / scheduler 统一拥有
- Pattern 不得自行定义一套独立的 global budget policy 或 error policy

### Pattern Interaction

Pattern 不再自己解释 capability policy，而是只表达：

- 我想调用哪个 capability
- 当前 action 的输入是什么
- 当前失败后希望 continue / replan / degrade 的策略偏好是什么

Runtime / Scheduler 返回统一 execution decision，pattern 再据此继续分支。

## Approval And Resume

approval / resume 应定义为 runtime-owned 机制。

### Ownership Model

- Pattern 只声明某个 capability intent 需要 approval 或当前步骤受 approval gate 影响
- Scheduler 负责产出 `wait_approval` decision
- Runtime 负责写入 `ApprovalDelta`、decision event、checkpoint ref、pending approval lookup
- Resume 时由 runtime 读取 checkpoint/session，写入 approved / rejected / resume-completed 事件，并决定继续、降级或再次等待

### Unified Flow

```text
Pattern emits capability intent
  -> Scheduler evaluates capability policy
  -> wait_approval decision if approval is required
  -> Runtime applies ApprovalDelta
  -> Kernel persists checkpoint
  -> PendingApprovalStore maps conversation/user to checkpoint
  -> UI restores pending approval
  -> User approves or rejects
  -> Runtime resumes from checkpoint
  -> Runtime emits approval resolved / rejected / resume completed events
```

### Runtime Guarantee

`reactive` 与 `plan_execute` 应共享同一 approval / resume 语义，只允许在“为什么进入审批”这一点上不同，不允许在 pending persistence、resume checkpoint、rejection finalization 上出现两套机制。

## Budget And Error Policy

本次收口虽然不要求一次性做完所有 budget 逻辑，但必须明确 owner。

### Budget Ownership

- session / run 级 budget policy 由 runtime 拥有
- capability 执行时是否允许继续、降级或终止，不能由 pattern 私自定义一套全局预算语义
- scheduler 可以消费 budget context，但 budget contract 的最终 owner 是 runtime，而不是某个 pattern

### Error Policy Ownership

- runtime / scheduler 统一定义 error classification 与 error-to-decision 映射
- Pattern 可以基于统一 error class 选择 continue / replan / degrade 等策略
- Pattern 不得为 externally visible runtime behavior 维护一套私有 error taxonomy

### Standard Error Classes

- `validation`
- `permission`
- `approval_rejected`
- `external`
- `timeout`
- `budget`
- `no_progress`
- `model_output`
- `dependency`
- `unknown`

## Shared State Design

继续沿用现有 `StateSnapshot` 域划分：

- `request`
- `context`
- `plan`
- `evidence`
- `approval`
- `execution`
- `answer`
- `pattern`

### State Ownership

- `request`
  - caller input 与 runtime boundary
- `context`
  - 中间搜索、抓取、memory、notes 等执行上下文
- `plan`
  - plan-execute 共享计划状态
- `evidence`
  - 接受后的证据及 sufficiency judgment
- `approval`
  - runtime-level approval state machine
- `execution`
  - runtime control-flow progress
- `answer`
  - draft / degrade / final answer
- `pattern`
  - pattern-private 扩展区

### Pattern-Specific State Rule

`pattern` 是唯一允许存放 pattern-private 状态的扩展域。外部 projection、pending approval restore、trace、SSE、replay 不得依赖 `pattern` 私有字段。

### Snapshot Compatibility

- snapshot 必须保留 schema version
- 读取旧 checkpoint / session 时必须先 normalize / compatibility handling
- 不允许静默把不兼容旧状态当作当前合法状态继续运行

## Reducer Rules

`StateDelta + Reducer` 继续作为唯一写入口。

### Required Invariants

- `approval pending` 不能同时带 `reviewed_at`
- `approval` 状态机需支持 `none/pending/approved/rejected/expired/cancelled`
- `execution` 状态组合必须合法，不能出现互相矛盾的 interrupted / completed / degraded 语义
- `answer` 的 `draft/degrade/final` 要有确定覆盖规则
- `plan` 更新不能破坏 step sequencing
- 节点不能直接 mutate snapshot，只能通过 delta + reducer

## Runtime Event Journal

journal 应正式成为 runtime truth。

### Standard Event Families

- `session_started`
- `node_start`
- `node_finish`
- `node_error`
- `state_applied`
- `decision_emitted`
- `branch_selected`
- `capability_start`
- `capability_result`
- `capability_skipped`
- `checkpoint_recorded`
- `approval_pending`
- `approval_resolved`
- `approval_rejected`
- `interrupt`
- `resume_completed`
- `answer_finalized`
- `degraded`
- `failed`

### Event Requirements

- append-only
- sequence stable
- session-scoped
- 可被 replay / projection / pending approval restore / trace / SSE 消费
- 外部消费者不应依赖 pattern-private event types 来理解标准 runtime 行为
- 至少要覆盖 run/session started、checkpoint/interruption、approval pending/resolved、resume completed、answer/degraded/failed 这些最低必备运行事实

## Replay And Projection

Projection 与 replay 只能依赖：

- `initial_snapshot`
- `snapshot`
- `state_applied` events
- 标准 runtime lifecycle events

它们不能依赖 pattern 私有结构或 pattern 私有事件约定来恢复：

- pending approval
- checkpoint metadata
- final outcome
- branch / decision / capability 执行轨迹

## Migration Plan

采用分阶段收口，而不是强抽象重写。

### Phase 1

- 固化 contract 名称与边界
- 承认当前 `runtime.Engine`、`RuntimeSession`、`StateSnapshot` 为正式基线

### Phase 2

- 让 runtime facade 真正成为唯一 lifecycle 入口
- 收走 service 中过多的 run/resume normalization

### Phase 3

- 引入 runtime scheduler contract
- 将 capability policy interpretation 从 pattern 中抽离
- 明确 budget / error policy 的 runtime ownership

### Phase 4

- 将 approval / resume / rejection finalization 进一步 runtime-owned
- 统一 pending approval restore 与 replay semantics

### Phase 5

- 稳定 event contract
- 验证 `reactive` 与 `plan_execute` 是“同一 engine 下的两个 pattern”

## Testing Strategy

### Unit Tests

- reducer invariant tests
- snapshot compatibility / normalize tests
- scheduler decision tests
- runtime decision normalization tests
- budget / error policy ownership tests
- event sequence tests
- projection / replay reconstruction tests

### Integration Tests

- pending approval -> resume -> complete
- pending approval -> reject -> stable final/degraded outcome
- capability requires approval cannot bypass scheduler
- non-resumable capability on resume has explicit behavior

### Compatibility Tests

- `plan_execute` 仍为默认 pattern
- `reactive` 与 `plan_execute` 均经由同一 runtime contract 装配
- 普通 RAG chat 默认入口不被强制切到 agent runtime
- `internal/app/rag/tool` 继续保持可用
- frontend approval restore 所需字段与现有 contract 兼容

## Recommended Implementation Direction

推荐采用“分阶段收口”：

1. 以当前未提交实现为基线继续推进
2. 补齐 runtime facade / outcome / decision 的唯一入口语义
3. 正式引入 runtime scheduler contract
4. 把 approval / resume / event 进一步从 pattern / service 细节里抽回 runtime-owned contract

不建议本次走强抽象重写，因为那会对当前在途实现扰动过大，也容易把本次收口做成重写工程。

## Success Criteria

完成后应满足：

- `reactive` 与 `plan_execute` 被证明是 shared runtime 下的两个 Pattern
- `RuntimeSession` 是唯一运行态容器
- `runtime.Engine` 是唯一 run / resume lifecycle facade
- canonical runtime decisions 对 service / replay / trace / SSE 可共享
- capability policy 通过 runtime scheduler 统一解释
- budget / error policy 由 runtime 统一拥有，而不是分散在 pattern 私有逻辑里
- approval / resume 被视为 runtime decision 与 runtime state
- replay / projection / pending approval restore 依赖 shared state + journal，而不是 pattern 私有实现
- 普通 RAG chat 与 `rag/tool` 稳定路径保持兼容
