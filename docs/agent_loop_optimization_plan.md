# Agent Loop 优化执行计划

## Summary

目标是在不改变现有对话、诊断、搜索和工具调用能力边界的前提下，完成一轮以 `AgentLoop` 为中心的结构收口，重点解决三类问题：

- 降低多轮循环中的冗余 LLM 调用成本
- 简化 `AgentState / ObserveResult / nextHint` 的双轨数据流
- 清理模块化迁移后的遗留控制逻辑，收紧主执行路径

本轮只做内部架构优化，不新增业务能力，不改 HTTP 对外接口，不扩展工具集，不推进多 Agent。完成后应保持现有 `doc_fail_01 / doc_run_01 / trace_bad_01 / external_evidence_workflow` 等场景行为不回退。

## Key Changes

### 1. Planner 短路优化

- 调整 `AgentLoop.planCalls()` 决策顺序：
  - Round 1 保持 `LLMPlanner -> rules`
  - Round 2+ 若 `AgentState.NextHintCalls` 非空，直接走 `PlanCallsFromHintCalls(...)`
  - 仅当结构化 hint 为空或无法解析成有效 call 时，回退到 `planCallsFromResultsWithRegistry(...)` 或 `PlanWithBaseRules(...)`
- `LLMPlanner` 不再处理“Observer 已经给出下一步”的回合
- 在 round summary / trace extraData 中补充：
  - `planningSource`
  - `llmPlannerSkipped`
  - `nextHintCallCount`

### 2. 状态流收口为单一真相源

- 以 `ObserveResult.State` 作为唯一状态来源
- `ObserveResult` 顶层仅保留：
  - `Done`
  - `Reasoning`
  - `State`
- `AgentLoop.Run()` 不再做 `Confidence / NextHintCalls / NextHint` 的双向互填
- `LLMObserver` 与 `RuleObserver` 都返回完整 `State`
- `AgentState.Normalize()` 保留 `NextHintCalls -> NextHint` 序列化兼容，但不再把 legacy 字符串反向解析回结构化 hint

### 3. RuleObserver 主路径清理

- 保留 `observeWithRegistry(...)` 作为主路径
- 保留 graph `diagnosisDepth` 的通用兜底逻辑
- 删除 runtime 层遗留的 document/task/web 中心化观察分支
- 兼容测试入口改为复用模块行为，而不是依赖 runtime 旧分支

### 4. Base Rules 保持稳定

- 本轮不引入 `ToolSpec.RouteHints`
- `PlanWithBaseRules(...)` 只承担 base rules fallback
- 关键词路由模块化、Planner/Observer 合并、更多 Graph Tool 泛化留到下一轮

## Test Plan

- 单元测试：
  - Round 2+ 存在 `NextHintCalls` 时跳过 `LLMPlanner`
  - `NextHintCalls` 为空时仍可进入 `LLMPlanner`
  - `ObserveResult.State` 成为唯一状态输入后，`AgentLoop` 多轮行为不回退
  - legacy `NextHint string` 仍能从 `NextHintCalls` 稳定序列化输出
- Observer 回归：
  - 合法 `state` 可继续下钻
  - 非法 hint / 幻觉 ID 仍会被证据校验拒绝并回退
  - registry path、graph fallback、max-loop 终止逻辑保持稳定
- 集成回归：
  - `doc_fail_01` 收敛到节点级根因
  - `doc_run_01` 保持 verification 路径
  - `trace_bad_01` 与 `external_evidence_workflow` 不回退

## Assumptions

- 不修改公开 HTTP API、SSE 事件字段或前端协议
- 保持默认 `MaxIterations=3`、并行工具执行能力、模块化 registry 行为模型不变
- `Planner/Observer` 合并、`RouteHints`、更多 Graph Tool 泛化、外部工具族扩展不在本轮范围内
