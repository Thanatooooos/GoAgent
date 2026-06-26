# evolve-agent-runtime-engine Proposal

## Summary

本变更将 `internal/app/agent` 从“已有多种 agent pattern 的服务装配”收敛为一套边界清晰的 Agent Runtime Engine。

核心方向是：

- 保留现有 `reactive` 与 `plan_execute` 两种 agent 范式。
- 不让每种范式发展成独立小引擎。
- 将它们统一为共享 Runtime Engine 下的 `Pattern`。
- 将 session、checkpoint、approval、resume、event、replay、capability scheduling、budget 和 error policy 收敛到 runtime 层。
- 保持 `rag/tool` 当前稳定生产路径不被本变更直接替换。

本变更不是从零新增 agent 能力。现有 `kernel`、`runtime`、`state`、`pattern`、`capability` 已经具备基础，本变更的重点是正式化边界、补齐共享协议，并减少后续 pattern 扩展时的重复实现。

## Why

当前 `internal/app/agent` 已经包含：

- `kernel.Runner`、node、checkpoint store
- `runtime.RuntimeSession`、session store、pending approval store、replay/projection
- `state.Snapshot`、`Delta`、`Reducer`、`RuntimeEvent`
- `pattern/reactive` 与 `pattern/planexecute`
- `capability.Spec`、registry、catalog、selector、resolver
- approval / resume / pending lookup 服务链路

这些说明 Agent Runtime 已经不是空白模块。

但当前仍存在几个架构风险：

- pattern 通过不同 graph 编译出来，外层缺少正式的统一 engine contract。
- approval / resume 虽已实现，但需要明确为 runtime-level decision，而不是 pattern 私有流程。
- capability 的 `RequiresApproval`、`SupportsParallel`、`SupportsResume`、`RiskLevel`、`Idempotency` 已存在，但缺少统一 scheduler 语义承接。
- runtime event 已存在，但需要稳定化为 trace、SSE、replay 可共享的协议。
- reactive 与 plan_execute 应继续共存，但必须共享 session、checkpoint、approval、event 和 capability execution。
- 如果未来新增 research、workflow、coding 等范式，不能复制一整套 runtime 生命周期。

因此需要一次架构收口。

## Goals

1. 定义 Agent Runtime Engine 的正式职责边界。
2. 将 `reactive` 和 `plan_execute` 保留为共享 engine 下的 Pattern。
3. 明确 Pattern 只能决定策略，不拥有 session、approval、checkpoint、event、scheduler 等通用机制。
4. 明确 `kernel.Runner` 与 `RuntimeSession` 的关系，避免引入平行 runtime 概念。
5. 引入或正式化 Capability Scheduler，消费现有 capability policy 字段。
6. 将 approval / resume 统一为 runtime-level state 与 decision。
7. 稳定 runtime event 协议，使 trace、SSE、replay 能共享同一事件语义。
8. 保持当前 chat / RAG 主链路兼容，不强制立即切换所有请求到 agent runtime。

## Non-Goals

本变更不包括：

- 不重写 `internal/app/rag`。
- 不立即废弃 `internal/app/rag/tool` 生产路径。
- 不把所有 chat 请求立即切换到 agent runtime。
- 不重新实现 `kernel` 或替换 Eino graph 底座。
- 不要求第一阶段引入分布式队列。
- 不要求第一阶段实现复杂模型自动选择 pattern。
- 不在本变更中大规模重做前端交互。

## Proposed Changes

### 1. Runtime Engine Facade

在现有 `Service -> compileRunner -> kernel.Runner` 之上明确一层 runtime engine 语义。

该层负责：

- 创建和恢复 `RuntimeSession`
- 调用 compiled pattern runner
- 统一 checkpoint / interrupt / resume 行为
- 统一 event append 与 state reducer
- 输出标准 runtime outcome

第一阶段可以通过重命名、封装或文档化现有 service/kernel 边界完成，不要求引入重量级新框架。

### 2. Pattern as Strategy

`reactive` 与 `plan_execute` 继续保留，但定位为 Pattern。

Pattern 可以决定：

- 下一步 action
- 是否需要 plan
- 是否 replan
- 是否继续、停止、降级
- 如何评估 observation
- 如何维护 pattern-specific 状态

Pattern 不应拥有：

- session store
- pending approval store
- checkpoint persistence
- SSE event protocol
- trace/replay 主格式
- capability permission/scheduling
- 全局 budget policy

### 3. Capability Scheduler

当前 `capability.Spec` 已经包含调度相关字段：

- `RequiresApproval`
- `SupportsParallel`
- `SupportsResume`
- `RiskLevel`
- `Idempotency`
- `Preconditions`

本变更要求 runtime 层正式消费这些字段，形成统一 scheduling decision。

Scheduler 负责：

- 判断是否需要 approval
- 判断是否可并行
- 判断 resume 是否允许
- 判断失败后 retry / degrade / fail
- 生成 capability start/result/skipped event
- 将 execution policy 从 pattern 私有逻辑中逐步抽离

### 4. Approval as Runtime Decision

approval 不应只是某个 pattern graph 里的节点行为。

runtime 应输出明确 decision：

- continue
- wait_approval
- resume
- reject
- retry
- replan
- degrade
- complete
- fail

pending approval、checkpoint id、rerun node、approval note、reviewed_at 等信息继续由 runtime state/session 管理。

### 5. Runtime Event Contract

现有 `state.RuntimeEvent` 继续作为基础，但需要稳定事件语义。

事件至少覆盖：

- run/session started
- node start/finish/error
- decision emitted
- branch selected
- capability start/result/skipped
- state applied
- approval pending/resolved
- interrupt
- resume completed
- answer finalized
- degraded/failed

事件必须保持 append-only、sequence stable、可用于 replay。

### 6. Compatibility

第一阶段不改变所有用户请求的入口选择。

推荐兼容策略：

- 普通 RAG chat 继续走现有稳定路径。
- 需要 approval、resume、多步骤任务、mixed capability 的请求走 agent runtime。
- 内部保留 pattern option，默认仍可使用当前 `plan_execute`。
- `reactive` 不删除，只收敛到同一 runtime contract。

## Acceptance Criteria

1. OpenSpec 明确 Agent Runtime Engine、Kernel、RuntimeSession、Pattern、Capability Scheduler 的边界。
2. `reactive` 与 `plan_execute` 均被定义为 Pattern，而不是独立 engine。
3. Pattern 不得绕过 runtime-level approval、checkpoint、event 和 scheduler。
4. approval / resume 被定义为 runtime decision 和 runtime state。
5. capability scheduling 使用现有 `capability.Spec` policy 字段。
6. runtime event contract 可支持 trace、SSE、pending approval restore 和 replay。
7. 第一阶段不破坏现有 RAG chat 和 `rag/tool` 稳定路径。
8. 后续新增 pattern 不需要复制 session、checkpoint、approval、event、scheduler 基础设施。

## Risks

- 收敛边界时可能过度抽象，导致现有 graph 实现变重。
- 如果 Scheduler 一次性抽离过多逻辑，可能影响 plan_execute 已有稳定行为。
- event contract 稳定化后会约束前后端协议，需要谨慎迁移。
- pattern selector 如果过早智能化，可能引入不可解释路由问题。

控制方式：

- 第一阶段以 facade 和 contract 收口为主。
- Scheduler 先承接 approval/parallel/resume/risk/idempotency，再逐步迁移复杂 policy。
- 保留现有 pattern 编译和 service route。
- 新事件字段兼容旧字段，先双写/映射，再清理。

## Open Questions

实现计划开始前仍需确认：

- 第一阶段是否只做 runtime contract 和内部收敛，不改变外部 chat 默认入口。
- `plan_execute` 是否继续作为 agent service 默认 pattern。
- pattern selector 第一阶段是否只使用规则，不引入 LLM selector。

默认建议：

- 不改变外部 chat 默认入口。
- 保持 `plan_execute` 为 agent service 默认 pattern。
- 第一阶段 pattern selector 使用显式配置和规则。
