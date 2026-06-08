# Agent P0 Closure Status

Date: `2026-06-06`

## 目的

这份文档用于总结 `internal/app/agent` 截至 `2026-06-06` 的 P0 收口进展。

它不是新的 backlog，也不替代 [agent_p0_closure_backlog_2026-06-05.md](./agent_p0_closure_backlog_2026-06-05.md)。

它回答的是另一件事：

- 哪些 P0 边界已经冻结
- 哪些测试已经把这些边界锁住
- 当前还不应该把哪些事情误判为“P0 已完成”

---

## 当前结论

截至 `2026-06-06`，`internal/app/agent` 在以下四条 P0 主线上已经形成一组可依赖的 focused contract：

1. `A` 对外运行契约
2. `D` service error outward contract
3. `B` approval 生命周期
4. `C` session store / checkpoint 边界

这些边界已经不再只是“代码里大概有这个能力”，而是：

- public DTO 注释已明确
- public path 行为已有 focused tests
- handoff / final-answer 的 approval 语义已对齐
- resume 输入已明确收紧为 `CheckpointID` canonical contract

但这仍然**不等于** `agent` 已经适合全面接入上层 chat 主链路。

当前更准确的判断是：

> `agent` 的 P0 outward contract 已进入“focused closure available”状态，适合继续向 transport/chat 做受控对接设计，但还不应跳过更大范围回归与非 P0 问题清理。

---

## 已完成范围

### A. 对外运行契约

已完成：

- `RunOutcome` 的三种 outward status 约束已写清：
  - `completed`
  - `degraded`
  - `awaiting_approval`
- `ApprovalPending` 中 canonical 字段与兼容字段已区分：
  - canonical:
    - `ReasonCode`
    - `CapabilityName`
    - `RerunNode`
    - `CheckpointID`
  - compatibility:
    - `Reason`
    - `Capability`
- `ResumeApprovalRequest` 的 outward contract 已明确：
  - `CheckpointID` 是 canonical lookup key
  - `Decision` 是 canonical decision field
  - `Approved bool` 仅为 compatibility fallback
- `completed / degraded` 终态不再对外暴露残留 `InterruptReason`
- approval resume 相关职责已从通用 approval projection 中拆开，单独落到：
  - `internal/app/agent/service_approval_resume.go`

对应文件：

- `internal/app/agent/request_response.go`
- `internal/app/agent/service_run.go`
- `internal/app/agent/service_approval.go`
- `internal/app/agent/service_approval_resume.go`

对应 focused tests：

- `internal/app/agent/service_contract_test.go`
- `internal/app/agent/service_resume_contract_test.go`

### D. Service Error Contract

已完成：

- service outward error 已统一使用 `code / kind / retryable`
- 关键 public error code 已有稳定 descriptor 映射
- public path 已覆盖以下典型错误：
  - `service_not_initialized`
  - `session_store_not_initialized`
  - `checkpoint_id_required`
  - `approval_decision_invalid`
  - `approval_session_not_found`
  - `approval_not_pending`
  - `approval_session_save_failed`
  - `approval_session_delete_failed`
- handoff 与 final-answer 两类 public path 均已补关键错误路径

对应文件：

- `internal/app/agent/service_error.go`
- `internal/app/agent/service_error_test.go`
- `internal/app/agent/service_error_path_test.go`

### B. Approval Lifecycle

已完成：

- 已覆盖 approval 主生命周期：
  - `run -> awaiting_approval`
  - `awaiting_approval -> approved -> completed/resumed`
  - `awaiting_approval -> rejected -> degraded`
  - `terminal -> duplicate resume`
- `rejected` 路径已明确要求：
  - `Outcome.Status == degraded`
  - `Execution.Interrupted == false`
  - `InterruptReason` 对外清空
  - journal 包含 `degraded` 事件
- approval 审计字段已覆盖：
  - `RequestedAt`
  - `ReviewedAt`
  - `DecisionNote`
  - `ApprovalDecision`
  - `ApprovalNote`
  - `ResumeCount`
  - `ResumedFrom`
- handoff approval lifecycle 已对齐：
  - approved
  - rejected
  - duplicate resume
  - not pending
- awaiting approval outward payload 已可反映 resume lineage：
  - `ResumeCount`
  - `SessionID`
  - `CheckpointID`
  - `RerunNode`

说明：

- backlog 中提到的“resume 后再次 pending approval”已在当前 focused scope 内以 outward lineage / payload contract 的方式补强
- 它没有在这轮被扩展成更大范围的 runtime pattern 行为验证

对应文件：

- `internal/app/agent/service_resume.go`
- `internal/app/agent/service_approval_resume.go`
- `internal/app/agent/service_flow_test.go`
- `internal/app/agent/service_approval_lifecycle_test.go`

### C. Session Store / Checkpoint Boundary

已完成：

- 已明确：
  - checkpoint store 负责 kernel resume bytes
  - session store 负责 approval-facing resumable session lookup
- 已明确 session alias 语义：
  - 内部可以保存 `checkpointID` 和 `sessionID` 双入口
  - outward resume contract 仍然只接受 canonical `CheckpointID`
- terminal path 清理已覆盖：
  - approved
  - rejected
  - handoff approved
- 已验证：
  - checkpoint lookup 不依赖 session alias 是否仍存在
  - `MemorySessionStore.Delete(...)` 保持幂等

这轮补上的一个关键实现收口是：

- public `ResumeAfterApproval(...)` / `ResumeHandoffAfterApproval(...)` 不再因为 session alias 存在而接受 `SessionID` 作为 outward lookup key
- 也就是说：
  - alias 仍保留给内部 lifecycle / cleanup / audit
  - outward contract 已真正收紧为 checkpoint-only

对应文件：

- `internal/app/agent/runtime/session_store.go`
- `internal/app/agent/runtime/session_store_memory.go`
- `internal/app/agent/kernel/checkpoint_store.go`
- `internal/app/agent/service_session_boundary_test.go`

---

## Focused Verification

以下 focused suites 已在本轮收口中跑通：

1. `A + D + B + C` consolidated focused suite
2. approval lifecycle focused suite
3. session/checkpoint boundary focused suite

当前可以认为以下测试文件共同构成了 P0 outward contract 的 focused safety net：

- `internal/app/agent/service_contract_test.go`
- `internal/app/agent/service_resume_contract_test.go`
- `internal/app/agent/service_error_test.go`
- `internal/app/agent/service_error_path_test.go`
- `internal/app/agent/service_approval_lifecycle_test.go`
- `internal/app/agent/service_session_boundary_test.go`

---

## 当前仍不包含的内容

以下内容**不应**被误判为“本轮 P0 已完成”：

- 新 capability family 扩展
- selector-driven capability 使用面继续扩大
- pattern router
- agent 全量接入 `RagChatService`
- approval transport / UI 产品化
- 更大范围 replay / inspection 产品化
- 非 P0 的更大范围 pattern 行为问题排查

尤其是：

- 当前 focused suites 通过
- 不代表 `go test ./internal/app/agent/... -count=1` 的所有非 P0 场景都已清零

---

## 对 chat / transport 接入的意义

截至当前阶段，后续要做 chat / transport 对接时，可以开始把下面这些视为相对稳定的输入：

- `RunOutcome`
- `ApprovalPending`
- `ResumeApprovalRequest`
- `ServiceErrorDescriptor`

同时应继续遵守以下前提：

- 对接时按 checkpoint-only resume contract 实现
- 不依赖内部 session alias
- 不依赖原始 runtime interrupt 细节
- handoff / final-answer 只允许主体 payload 不同，不允许 approval contract 分叉

---

## 下一步建议

如果继续推进，建议顺序是：

1. 将这些 outward contract 接入上层 chat / transport 的设计层
2. 对 focused P0 contract 做一次更大范围回归串联
3. 单独处理非 P0 的 pattern / plan-execute 问题
4. 再评估是否进入更高层级的正式接入

当前不建议的顺序是：

1. 跳过上述 contract，直接全面接入 chat 主链路
2. 依赖 `SessionID` 做 outward resume
3. 让 transport/UI 依赖内部 runtime 细节或错误字符串

