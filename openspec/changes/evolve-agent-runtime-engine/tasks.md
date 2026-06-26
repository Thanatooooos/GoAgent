# evolve-agent-runtime-engine Tasks

## Scope

本变更交付：

- Agent Runtime Engine 职责边界收敛
- 多 Pattern 保留策略
- Runtime-level approval / resume 语义
- Capability Scheduler 合同
- Runtime Event 合同
- 渐进式迁移计划与验证标准

本变更不交付：

- 全量替换 RAG chat 入口
- 废弃 `rag/tool`
- 重写 Eino graph/kernel
- 新分布式任务队列
- LLM pattern selector

## 1. Freeze Runtime Contracts

- [x] 明确 `RuntimeSession` 是 agent runtime 主状态容器。
- [x] 明确 `kernel.Runner` 是 compiled graph 执行层。
- [x] 明确 Runtime Engine Facade 负责 run/resume/outcome 归一化。
- [x] 明确 `StateDelta` + `Reducer` 是状态变更入口。
- [x] 明确 `RuntimeEvent` 是 journal/replay/SSE/trace 的基础事件模型。

验证：

- contract 文档与代码命名一致
- 现有 service/kernel/runtime 不出现平行同义概念

## 2. Strengthen Runtime State Model

- [x] 梳理 `StateSnapshot` 的领域分区：request、context、plan、evidence、approval、execution、answer。
- [x] 明确每个 state domain 的 owner：runtime、pattern、scheduler、capability、answer synthesizer。
- [x] 明确哪些字段属于 cross-pattern state，哪些字段属于 pattern-specific state。
- [x] 为 pattern-specific state 预留稳定扩展方式，避免 reactive/plan_execute 把私有状态塞进通用字段。
- [x] 梳理 `StateDelta` 的写入规则：节点不得直接改 snapshot，必须通过 delta + reducer。
- [x] 增强 reducer invariant 校验，防止 approval、execution、plan 等状态出现非法组合。
- [x] 明确 `ApprovalDelta` 的状态机：none/pending/approved/rejected/expired/cancelled。
- [x] 明确 `ExecutionDelta` 的状态机：running/interrupted/resuming/completed/degraded/failed。
- [x] 明确 plan state 的更新语义：replace、step patch、step result append 是否需要分层。
- [x] 明确 evidence state 的增长、去重、source ref 和 sufficiency 更新规则。
- [x] 明确 answer state 的 draft/degrade/final 互斥与覆盖规则。
- [x] 为 snapshot 增加版本/兼容策略，支持未来 replay 和 checkpoint schema 演进。
- [x] 建立 state projection 合同：pending approval、trace、SSE、replay 不读取 pattern 私有状态。

验证：

- reducer invariant 单测覆盖 approval/execution/answer 的非法状态组合
- replay 从 snapshot + journal 能恢复 pending approval、checkpoint、final outcome
- reactive 与 plan_execute 不依赖彼此私有 state 字段
- checkpoint 中的旧 snapshot 在版本演进后仍有明确兼容行为

## 3. Preserve Patterns Under One Engine

- [x] 将 `reactive` 定义为 Pattern，而不是独立 engine。
- [x] 将 `plan_execute` 定义为 Pattern，而不是独立 engine。
- [x] 明确 Pattern 只负责 strategy / graph / node flow。
- [x] 禁止 Pattern 绕过 runtime approval、checkpoint、event、scheduler。
- [x] 保持 `plan_execute` 为 agent service 默认 pattern。

验证：

- 新增 pattern contract 测试或 compile 测试
- `reactive` 与 `plan_execute` 均通过同一 runtime config 装配

## 4. Add Runtime Engine Facade

- [x] 在 service 和 kernel runner 之间增加或明确 runtime engine facade。
- [x] 统一 `Run`、`RunWithCheckpoint`、`Resume` 的外部 outcome。
- [x] 将 approval pending、resume completed、interrupt 映射为标准 runtime decision。
- [x] 保持现有 service response 兼容。

验证：

- 现有 agent run 测试通过
- approval resume 测试通过
- pending approval lookup 测试通过

## 5. Introduce Capability Scheduler Contract

- [x] 定义 scheduler input：capability spec、runtime options、snapshot、pattern action。
- [x] 定义 scheduler output：execute、wait_approval、skip、retry、degrade、fail。
- [x] 消费 `RequiresApproval`。
- [x] 消费 `SupportsParallel`。
- [x] 消费 `SupportsResume`。
- [x] 消费 `RiskLevel`。
- [x] 消费 `Idempotency`。
- [x] 消费 `Preconditions`。

验证：

- approval-required capability 不可绕过 scheduler
- non-resumable capability resume 行为明确
- parallel-safe capability 可被分组
- high-risk capability 进入 approval reason

## 6. Normalize Approval and Resume

- [x] 将 approval pending 定义为 runtime decision。
- [x] 将 approval resolved/rejected 定义为 runtime event。
- [x] 将 checkpoint id、rerun node、approval note 统一写入 runtime state/session。
- [x] 确认 reactive 与 plan_execute 使用同一 approval/resume 语义。

验证：

- approval pending 可恢复到 UI
- approved 后从 checkpoint 继续
- rejected 后产生稳定 outcome
- journal 可重建 approval 状态

## 7. Stabilize Runtime Events

- [x] 梳理当前 `RuntimeEvent` event type。
- [x] 补齐 approval pending/resolved/fail 事件。
- [x] 确认 capability start/result/skipped 事件字段。
- [x] 确认 decision emitted 事件字段。
- [x] 建立 SSE/trace/replay 映射表。
- [x] 增加事件顺序测试。

验证：

- event sequence 单调递增
- replay projection 可从 journal 恢复关键状态
- SSE 不依赖 pattern 私有事件

## 8. Keep Compatibility Boundaries

- [x] 不改变普通 RAG chat 默认入口。
- [x] 不删除 `internal/app/rag/tool`。
- [x] 保持现有 agent service API 兼容。
- [x] 保持现有 frontend approval pending restore 行为。

验证：

- 现有 RAG chat 聚焦测试通过
- 现有前端 approval 事件字段仍可消费
- agent pattern 配置保持向后兼容

## 9. Verification and Rollout

- [x] 运行 agent runtime 相关单测。
- [x] 运行 approval/resume 相关集成测试。
- [x] 运行 reactive 与 plan_execute pattern 测试。
- [x] 运行相关 RAG chat 兼容测试。
- [x] 执行 `openspec validate evolve-agent-runtime-engine --strict`。

完成标准：

- 新增或更新的 contract 测试能证明多个 Pattern 共用一套 Runtime Engine。
- approval、checkpoint、event、scheduler 不再表现为 pattern 私有机制。
- 默认生产路径没有被强制切换。
