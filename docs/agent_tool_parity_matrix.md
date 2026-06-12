# 旧 Tool 与新 Agent Capability 等价矩阵

> 生成时间：2026-06-09  
> 范围调整：2026-06-11 起，**新 Agent Runtime 不再承担文档诊断、trace 诊断和根因分析链路**。  
> 扫描范围：`internal/app/rag/tool/` vs `internal/app/agent/`  
> 当前目的：明确新 Agent Runtime 的收口范围，只要求覆盖外部检索、网页抓取、外部证据整合这条主线；诊断类旧 Tool 不再作为迁移前置条件。

## 当前 runtime scope

自 `2026-06-11` 起，本矩阵按以下原则解读：

- **新 Agent Runtime 的目标范围**：`web_search`、`web_fetch`、`external_evidence_collect`，以及后续可能保留的 `memory_recall` / `content_summarize`
- **明确不纳入本轮收口**：`document_query`、`document_list`、`document_chunk_log_query`、`document_ingestion_diagnose`、`ingestion_task_query`、`ingestion_task_node_query`、`task_list`、`task_ingestion_diagnose`
- **明确不再迁移到新 runtime**：`trace_node_query`、`trace_retrieval_diagnose`、`document_root_cause_diagnosis`
- **`think` 处理原则**：默认视为不进入新 runtime scope；只有未来重新定义产品需求时才重新评估

这意味着下面矩阵中的 `partial` / `missing` 需要区分：

- 属于 **runtime scope 内** 的能力缺口：仍然需要补齐
- 属于 **runtime scope 外** 的旧诊断能力：只需要标记为 legacy/frozen 或后续下线，不再阻塞新 runtime 收口

## 扫描命令

```powershell
rg -n "registerModule|registerGraphTools|New.*Tool" internal/app/rag/tool/assembly -S
rg -n "NameWebSearch|NameDocumentInvestigation|assembleCapabilities|NewCapability" internal/app/agent -S
```

## 新 Agent 当前已注册能力（bootstrap 实际路径）

`internal/bootstrap/rag/agent_runtime.go` 调用 `agentapp.NewService`，未注入 `DocumentInvestigator`。

因此生产环境当前仅注册：

| 新 capability | 文件 |
|---------------|------|
| `web_search` | `internal/app/agent/search/capability.go` |
| `web_fetch` | `internal/app/agent/fetch/capability.go` |
| `external_evidence_collect` | `internal/app/agent/external_evidence/capability.go` |

`document_investigation_collect` 仅在测试或显式注入 `DocumentInvestigator` 时注册（`service_assembly.go:registerOptionalWorkflowCapabilities`）。

## 等价矩阵

| 旧工具名 | 旧文件 | 新 capability | 新文件 | 参数兼容 | 返回兼容 | evidence | trace | 测试 | 结论 |
|----------|--------|---------------|--------|----------|----------|----------|-------|------|------|
| `think` | `invokers/meta/think_tool.go` | — | — | — | — | N/A | N/A | 旧有 | **out of scope** |
| `web_search` | `invokers/web/web_search_tool.go` | `web_search` | `agent/search/capability.go` | partial | partial | 有 | 有 capability 事件 | 新有单测 | **ready** |
| `web_fetch` | `invokers/web/web_fetch_tool.go` | `web_fetch` | `agent/fetch/capability.go` | partial | partial | 有 | 有 | 新有单测 | **ready** |
| `external_evidence_workflow` | `invokers/graph/external_evidence_workflow_graph.go` | `external_evidence_collect` | `agent/external_evidence/capability.go` | partial | partial | 有 | 有 | 新有单测 | **partial** |
| `document_query` | `invokers/system/document_query_tool.go` | `document_investigation_collect` | `agent/document_investigation/capability.go` | partial | partial | 有 | 有 | 新有单测 | **out of scope**（legacy 诊断链路） |
| `document_list` | `invokers/system/document_list_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `document_chunk_log_query` | `invokers/system/document_chunk_log_query_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `document_ingestion_diagnose` | `invokers/system/document_ingestion_diagnose_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `ingestion_task_query` | `invokers/system/ingestion_task_query_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `ingestion_task_node_query` | `invokers/system/ingestion_task_node_query_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `task_list` | `invokers/system/task_list_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `task_ingestion_diagnose` | `invokers/system/task_ingestion_diagnose_tool.go` | `document_investigation_collect` | 同上 | partial | partial | 有 | 有 | 缺集成 | **out of scope** |
| `trace_node_query` | `invokers/trace/trace_node_query_tool.go` | — | — | — | — | 有（旧） | 有（旧） | 旧有 | **out of scope** |
| `trace_retrieval_diagnose` | `invokers/trace/trace_retrieval_diagnose_tool.go` | — | — | — | — | 有（旧） | 有（旧） | 旧有 | **out of scope** |
| `document_root_cause_diagnosis` | `invokers/graph/diagnosis_graph.go` | — | — | — | — | 有（旧） | 有（旧） | 旧有图测试 | **out of scope** |
| `document_diagnose_with_search` | `invokers/graph/diagnose_search_graph.go` | `external_evidence_collect` + 诊断上下文 | 组合能力 | partial | partial | 有 | 有 | 新有部分 | **out of scope** |

## 阻塞删除清单

在当前 scope 下，只有与“外部检索 / 外部证据 runtime 主线”直接相关的项才阻塞收口：

| 阻塞项 | 当前状态 | 后续任务 |
|--------|----------|----------|
| SSE 事件兼容（`SendToolStart` / `SendToolResult`） | partial | P0-4 阶段 1 观测 + 前端联调 |
| 审批/恢复持久化（session/checkpoint） | partial | 为 `SessionStore / CheckpointStore` 提供持久化实现 |
| 外部证据主链路的生产默认路径 | partial | 明确生产默认 pattern，决定继续固定 `reactive` 还是逐步开放 `plan_execute` |

以下旧能力**不再是删除新 runtime 阻塞项**，但必须做出产品决策：

- 保留为 legacy/frozen 诊断路径
- 或下线对应入口/API/前端能力

这些非阻塞项包括：

- `think`
- `document_investigation_collect` 及其旧文档/任务诊断工具族
- `trace_node_query`
- `trace_retrieval_diagnose`
- `document_root_cause_diagnosis`
- `document_diagnose_with_search`

## 结论摘要

- **可认为 ready**：`web_search`、`web_fetch`（外部证据基础链路）
- **partial**：外部证据工作流、SSE 事件兼容、审批/恢复持久化、生产默认路径收口
- **out of scope**：文档诊断、trace 诊断、graph 根因诊断、`think`

**当前不建议删除旧 `rag/tool/`。** 但原因已经变化：不再是“还没补齐诊断 parity”，而是新 runtime 仍需先完成 SSE 事件兼容、审批/恢复持久化，以及生产默认路径收口。诊断类旧工具应按 legacy/frozen 或下线路径单独治理，而不是继续作为新 runtime 的迁移前置条件。
