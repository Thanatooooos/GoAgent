# Agent Runtime Scope（2026-06-11）

## 决策

自 `2026-06-11` 起，新 `Agent Runtime` 的目标收敛为：

- 外部检索
- 网页抓取
- 外部证据整合

本轮**不再**把以下能力作为新 runtime 的产品目标：

- 文档诊断
- 任务诊断
- trace 诊断
- graph 根因分析

## In Scope

当前新 runtime 应保留并继续收口的能力：

- `web_search`
- `web_fetch`
- `external_evidence_collect`

可选保留，但不作为本轮核心阻塞项：

- `memory_recall`
- `content_summarize`

## Out Of Scope

以下旧能力不再要求迁移到新 runtime：

- `think`
- `document_query`
- `document_list`
- `document_chunk_log_query`
- `document_ingestion_diagnose`
- `ingestion_task_query`
- `ingestion_task_node_query`
- `task_list`
- `task_ingestion_diagnose`
- `trace_node_query`
- `trace_retrieval_diagnose`
- `document_root_cause_diagnosis`
- `document_diagnose_with_search`

这些能力后续只能走两种处置方式：

- `legacy/frozen`
- 直接下线

## Runtime 仍缺的能力

即使去掉诊断和 trace，新 runtime 仍有这些收口项：

1. SSE 事件兼容
   需要补齐 `tool_start / tool_result / agent_think` 一类实时事件体验。

2. 审批/恢复持久化
   需要为 `SessionStore / CheckpointStore` 提供持久化实现，避免 approval/resume 只能单进程存活。

3. 生产默认路径
   需要明确生产默认 pattern，到底继续固定 `reactive`，还是逐步开放 `plan_execute`。

4. Chat 到 Agent 的上下文交接
   需要减少把结构化上下文压平为 notes 的损耗。

5. 双 runtime 收口
   需要完成灰度、回滚策略和旧 `ToolWorkflow` 退役前验证。

## Backlog 处理建议

应继续保留的 backlog：

- `P0-4` 双 runtime 收口
- `P0-5` 依赖注入收敛
- `P0-6` parity matrix，但只保留新 scope
- `P0-3` retrieve eval
- `P1-4` token-aware context budget
- `P1-5` summary 生命周期升级
- `P1-6` answer eval
- `P1-7` metrics

应从“runtime 前置条件”中移除的 backlog：

- `trace` 诊断 capability
- `document_investigation` 接入 bootstrap
- `think` capability 对齐
- graph 诊断迁移到 `plan_execute` 或 `reactive`

这些项如果以后仍保留，也应作为：

- legacy 诊断线维护任务
- 或产品下线清理任务

## 当前推荐顺序

1. 先完成 `P0-6` 的 scope 重定义与测试清单收缩。
2. 再推进 `P0-4` 的 SSE、持久化、灰度切换。
3. 同步收尾 `P0-5` 和 `P1-2`，降低 runtime 装配复杂度。
4. 然后继续做 eval、budget、summary、metrics 这些质量闭环。
