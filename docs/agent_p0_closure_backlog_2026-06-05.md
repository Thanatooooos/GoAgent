# Agent P0 Closure Backlog

Date: `2026-06-05`

## 目标

这份文档用于收口 `internal/app/agent` 当前的 P0 工作。

这里的 P0 不是“继续加更多 capability”或“尽快接入 chat 主链路”，而是先把下面三件事做稳定：

- 对外运行契约稳定
- approval 生命周期稳定
- service 错误和 session/checkpoint 语义稳定

当前 `agent` 模块已经具备可运行骨架，但如果在这些边界没有冻结之前继续向上接入，会导致：

- transport/chat 层反复适配
- approval 行为在不同输出模式下不一致
- 外部调用方依赖内部实现细节
- session/checkpoint 恢复语义漂移

所以 P0 的核心目标只有一句话：

`先把 runtime/service 的外部边界做成可依赖契约，再继续向上接入和向外扩能力。`

## 范围

本轮 P0 聚焦以下文件和语义边界：

- `internal/app/agent/request_response.go`
- `internal/app/agent/service_run.go`
- `internal/app/agent/service_resume.go`
- `internal/app/agent/service_approval.go`
- `internal/app/agent/service_approval_resume.go`
- `internal/app/agent/service_error.go`
- `internal/app/agent/runtime/session_store.go`
- `internal/app/agent/runtime/session_store_memory.go`
- `internal/app/agent/kernel/checkpoint_store.go`

本轮不包含：

- 新 capability family 扩展
- pattern router
- 全量接入 `RagChatService`
- transport/UI 层 approval 交互实现
- 更大规模 replay/inspection 产品化

## P0 设计原则

### 1. 先冻结对外语义，再继续扩展内部实现

P0 的重点不是让内部逻辑“更聪明”，而是让外部调用方能稳定理解：

- run 结束时是什么状态
- approval 等待时应该拿到什么
- resume 时该传什么
- 失败时如何分类

### 2. approval 必须是 service contract，不只是 runtime interrupt

外部调用方不应该理解 Eino interrupt 细节。

外部应该只关心：

- `completed`
- `degraded`
- `awaiting_approval`

以及 approval pending payload 和 resume contract。

### 3. session/checkpoint 要有明确职责边界

checkpoint 是运行时恢复点。
session store 是 approval 生命周期恢复入口。

二者可以协作，但不能语义混用。

### 4. handoff 和 final-answer 不能走出两套 approval 语义

输出模式可以不同，但 approval contract 不能一条链路一种说法。

## Task Package A: 冻结对外运行契约

### 目标

冻结 `agent.Service` 暴露给外部调用方的 response/outcome shape，避免后续 chat/transport 接入时继续改动。

### 主要入口

- `internal/app/agent/request_response.go`
- `internal/app/agent/service_run.go`
- `internal/app/agent/service.go`

### 子任务 A1: 冻结 `RunOutcome` 语义

明确以下字段的稳定含义：

- `Status`
- `Interrupted`
- `InterruptReason`
- `CheckpointID`
- `Approval`

需要解决的问题：

- `Interrupted=true` 是否总是意味着 `awaiting_approval`
- `degraded` 场景下 `InterruptReason` 是否应该为空
- `completed` 场景下 `CheckpointID` 是否必须为空
- `Approval=nil` 是否是唯一合法的非 approval 状态表达

建议产出：

- 为 `RunOutcome` 所有字段补注释
- 在文档中列出三种 `Status` 的约束表

### 子任务 A2: 收口 `ApprovalPending` 字段

当前 `ApprovalPending` 信息较全，但还需要明确哪些字段是“公开稳定字段”，哪些只是补充投影。

重点审查：

- `Reason` 与 `ReasonCode` 是否需要同时保留
- `Capability` 与 `CapabilityName` 是否需要同时保留
- `Node` 与 `RerunNode` 的语义边界
- `Question / SearchQuery / CurrentStep / CandidateURLs` 是否保证始终可用

建议方向：

- 保留一个主字段，一个兼容字段时要写清“谁是规范字段”
- 对非必填字段明确 `omitempty` 风格和缺省语义

### 子任务 A3: 冻结 final-answer 与 handoff 的 outward contract

需要确认：

- `RunDetailed(...)` 与 `RunHandoffDetailed(...)` 在同一运行状态下的 `Outcome` 语义完全一致
- approval pending 时，二者只在主体 payload 上不同，不在 approval contract 上分叉
- `Response` 和 `HandoffResult` 都不要泄漏 runtime 内部实现细节

### 子任务 A4: 补 contract tests

建议新增测试覆盖：

- completed response shape
- degraded response shape
- awaiting approval response shape
- handoff awaiting approval response shape

测试目标不是跑复杂逻辑，而是锁定 outward schema 和字段约束。

### 验收标准

- `request_response.go` 的 public DTO 注释完整
- 三种 `RunStatus` 的字段约束可枚举
- approval pending payload 没有明显重复主字段
- `RunDetailed` / `RunHandoffDetailed` outward contract 有测试锁定

## Task Package B: Approval 生命周期补强

### 目标

把 approval 从“能 pause/resume”提升为“完整、可验证、可恢复、可审计”的 service 生命周期。

### 主要入口

- `internal/app/agent/service_run.go`
- `internal/app/agent/service_resume.go`
- `internal/app/agent/service_approval.go`
- `internal/app/agent/service_approval_resume.go`
- `internal/app/agent/service_test.go`

### 子任务 B1: 梳理 approval 状态转移图

至少明确以下状态迁移：

- run -> awaiting approval
- awaiting approval -> approved -> resumed
- awaiting approval -> rejected -> degraded
- resumed -> awaiting approval again
- terminal -> duplicate resume

建议产出：

- 在文档中补一张状态转移表
- 在测试命名上直接对应状态迁移

### 子任务 B2: 补 approved 主路径测试

需要覆盖：

- pending approval session 可以成功 resume
- approved 后 checkpoint/session 状态正确更新
- `ResumeCount`、`ReviewedAt`、`ApprovalDecision` 有正确审计信息
- resume 后如果 runtime 再次 pending approval，payload 仍正确

### 子任务 B3: 补 rejected 主路径测试

当前 rejected 路径会进入 `finalizeRejectedApproval(...)`。

需要锁定：

- 最终 `Outcome.Status == degraded`
- `Approval.Status == rejected`
- `Answer.DegradeReason == approval_rejected`
- `Execution.Interrupted == false`
- journal 包含 degrade 事件

### 子任务 B4: 补异常与重复操作测试

需要至少覆盖：

- `CheckpointID` 为空
- approval session 不存在
- session 存在但不是 pending
- decision 非法
- rejected 后重复 resume
- completed 后重复 resume

### 子任务 B5: 对齐 handoff approval resume 行为

确认：

- `ResumeAfterApproval(...)`
- `ResumeHandoffAfterApproval(...)`

在同一审批状态下只在输出主体不同，不在生命周期语义上分叉。

### 验收标准

- approval 生命周期主路径和异常路径都有测试
- approved/rejected/duplicate resume 行为稳定
- handoff 和 final-answer 的 approval 语义一致
- 审计字段有明确断言，不只断言“没报错”

## Task Package C: Session Store 与 Checkpoint 边界冻结

### 目标

明确 checkpoint 与 session store 的职责边界，稳定 approval pause/resume 的持久化语义。

### 主要入口

- `internal/app/agent/service_run.go`
- `internal/app/agent/service_resume.go`
- `internal/app/agent/runtime/session_store.go`
- `internal/app/agent/runtime/session_store_memory.go`
- `internal/app/agent/kernel/checkpoint_store.go`

### 子任务 C1: 写清 checkpoint/session store 职责

需要明确：

- checkpoint 保存什么
- session store 保存什么
- 为什么 approval 恢复不能只依赖 checkpoint bytes
- 为什么 session store 需要 `checkpointID` 和 `sessionID` 双入口

建议产出：

- 在文档中写一节职责说明
- 在接口附近补注释

### 子任务 C2: 锁定 alias 存储策略

当前实现会把 pending session 存在：

- `checkpointID`
- `sessionID`

需要正式确认：

- 这是规范行为，而不是内存实现偶然行为
- delete 需要清理两个 key
- 重复 put/delete 是否允许幂等

### 子任务 C3: 补终态清理测试

覆盖以下场景：

- approved 后 terminal path 清理
- rejected 后 terminal path 清理
- completed path 清理
- delete 时主 key 和 alias key 都被清掉

### 子任务 C4: 冻结 resume 输入契约

当前 outward resume 入口只接受：

- `CheckpointID`
- `Decision`
- `DecisionNote`

需要确认：

- 是否继续只接受 `CheckpointID`
- `Approved bool` 兼容路径是否保留，以及保留多久
- 未来如支持 `SessionID`，应走新字段而不是重载现有字段

### 验收标准

- checkpoint/session store 职责有文档和注释
- alias 生命周期有专门测试
- 终态后不会遗留 pending session
- resume 输入契约不含混

## Task Package D: Service Error 合约收口

### 目标

将 `agent` service 对上层暴露的错误收口成稳定的 `code/kind/retryable` 契约，避免 transport/chat 依赖 `err.Error()`.

### 主要入口

- `internal/app/agent/service_error.go`
- `internal/app/agent/service_run.go`
- `internal/app/agent/service_resume.go`
- `internal/app/agent/service_error_test.go`

### 子任务 D1: 冻结错误码清单

当前已有错误码需要审查并冻结：

- `service_not_initialized`
- `question_required`
- `session_store_not_initialized`
- `checkpoint_id_required`
- `approval_decision_invalid`
- `approval_session_load_failed`
- `approval_session_save_failed`
- `approval_session_delete_failed`
- `approval_session_not_found`
- `approval_not_pending`
- `runtime_execution_failed`

需要明确哪些是 P0 稳定 code，后续新增必须保持向后兼容。

### 子任务 D2: 冻结 `kind` 与 `retryable` 语义

至少需要保证：

- `invalid_request`
- `not_found`
- `failed_precondition`
- `unavailable`
- `internal`

对外定义明确，不允许不同调用点对同一 code 给出不同 kind。

### 子任务 D3: 排查 public path 是否仍泄漏原始错误

检查 `RunDetailed` / `RunHandoffDetailed` / `ResumeAfterApproval` / `ResumeHandoffAfterApproval` 相关路径，确认 outward error 都经过统一包装。

### 子任务 D4: 补 descriptor tests

至少断言：

- `DescribeServiceError(...)` 对每类 public error 返回稳定 descriptor
- 未包装错误回退到 `internal`
- `errors.As(...)` 能拿到 `ServiceError`

### 验收标准

- service outward error 都能映射到稳定 descriptor
- 上层无需依赖错误字符串匹配
- approval 相关失败可区分 invalid/not_found/precondition/unavailable

## 建议执行顺序

建议按下面顺序执行：

1. Task Package A
2. Task Package D
3. Task Package B
4. Task Package C

原因：

- 先冻结 outward contract
- 再冻结 outward error surface
- 然后补 approval 生命周期测试
- 最后把 session/checkpoint 语义完全钉死

这样可以减少后续重复修改测试和文档。

## P0 Exit Criteria

只有满足以下条件，才能认为 `agent` P0 真正完成：

- public run/approval/resume contract 已冻结
- approval 生命周期主路径与异常路径测试完整
- session store 与 checkpoint 职责边界明确
- service error outward contract 稳定
- handoff 与 final-answer 两种输出模式在 approval/service contract 上保持一致

若以上任意一点未完成，都不应进入“全面接入 chat 主链路”的阶段。

## 后续但不属于 P0 的工作

以下工作应明确放到 P1/P2，而不是挤进本轮：

- 扩更多 selector-driven capability family
- 扩更复杂的 pattern routing
- 把 agent 直接接入 `RagChatService` 主生产链路
- approval transport/UI 产品化
- 更重型的 replay/inspection 产品能力

P0 完成后，下一步才适合转向：

- outward contract 接入 chat/transport
- selector 使用面扩展
- 第二 pattern 的进一步强化
