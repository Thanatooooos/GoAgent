# 旧 Tool 与新 Agent Capability 等价测试计划

> 关联文档：`docs/agent_tool_parity_matrix.md`  
> 生成时间：2026-06-09  
> 目的：在删除 `internal/app/rag/tool/` 前，用可重复测试证明新 Agent 覆盖旧工具能力。

---

## 1. 测试分层

| 层级 | 范围 | 运行方式 |
|------|------|----------|
| L1 单元测试 | 单个 capability/tool 输入输出 | `go test ./internal/app/agent/...` |
| L2 编排测试 | reactive / plan-execute 多步链路 | `go test ./internal/app/agent/pattern/...` |
| L3 集成测试 | bootstrap 装配 + RagChat agent 路径 | `go test ./internal/bootstrap/rag ./internal/app/rag/service` |
| L4 手工/E2E | HTTP SSE + trace 查询 | 启动服务后发诊断类问题 |

本计划以 L1–L3 为主；L4 在 P0-4 灰度阶段执行。

---

## 2. 核心场景清单

### 2.1 单工具查询

| 场景 ID | 旧工具 | 新 capability | 输入示例 | 期望 |
|---------|--------|---------------|----------|------|
| T-01 | `document_query` | `document_investigation_collect` | `document_id=doc_fail_01` | 返回文档状态、chunk 数、最新任务信息 |
| T-02 | `ingestion_task_query` | `document_investigation_collect` | 同上（通过 document 反查 task） | `latest_task_id` 非空或明确 not found |
| T-03 | `task_list` | `document_investigation_collect` / 待补 discovery | `knowledge_base_id=kb_eval` | 列表或降级说明（当前新能力无等价 list） |

**现有测试参考：**
- `internal/app/agent/document_investigation/capability_test.go`
- `internal/app/rag/tool/document_behavior_test.go`

### 2.2 诊断链路

| 场景 ID | 链路 | 问题示例 | 期望 |
|---------|------|----------|------|
| T-10 | document → task → node | `doc_fail_01 为什么导入失败` | 结论含失败节点/错误信息 |
| T-11 | task → node diagnose | `task_run_01 最近一次失败原因` | 定位到 failed node |
| T-12 | trace 诊断 | `trace_bad_01 检索为什么没命中` | **当前 missing**，旧 tool 有覆盖 |

**现有测试参考：**
- 旧：`internal/app/rag/tool/eino_diagnosis_graph_test.go`
- 新：`internal/app/agent/pattern/planexecute/pattern_test.go`（document investigation 场景）

### 2.3 Trace 诊断链路

| 场景 ID | 旧工具 | 状态 | 期望 |
|---------|--------|------|------|
| T-20 | `trace_node_query` | missing | 按 trace_id 返回节点列表 |
| T-21 | `trace_retrieval_diagnose` | missing | 分析 retrieve 阶段失败原因 |

**阻塞删除：** T-20、T-21 在新 Agent 补齐前，旧 tool 路径必须保留。

### 2.4 外部证据链路

| 场景 ID | 旧工具/图 | 新 capability | 期望 |
|---------|-----------|---------------|------|
| T-30 | `web_search` | `web_search` | 返回候选 URL 列表 |
| T-31 | `web_fetch` | `web_fetch` | 返回页面摘要/正文片段 |
| T-32 | `external_evidence_workflow` | `external_evidence_collect` | search → fetch 编排完成，evidence bundle 非空 |

**现有测试参考：**
- `internal/app/agent/external_evidence/capability_test.go`
- `internal/app/agent/service_flow_test.go`

### 2.5 失败与边界场景

| 场景 ID | 类型 | 输入 | 期望 |
|---------|------|------|------|
| T-40 | 工具 failed | 不存在的 `document_id` | 结构化错误，不 panic |
| T-41 | 超时 | 模拟慢 fetch | capability timeout，trace 记录 failed |
| T-42 | 空结果 | `doc_not_exist` | 明确 not found，不编造结论 |
| T-43 | source policy 拒绝 | 被 policy 拦截的 URL | fetch 被拒绝，metadata 含 policy 原因 |
| T-44 | approval 挂起 | `web_fetch` + `requireApproval=true` | `RunStatusAwaitingApproval`，可 resume |

**现有测试参考：**
- `internal/app/agent/service_approval_lifecycle_test.go`
- `internal/app/rag/service/rag_chat_agent_stage_test.go`

---

## 3. 推荐测试命令

```powershell
# L1：capability 单测
go test ./internal/app/agent/search ./internal/app/agent/fetch ./internal/app/agent/external_evidence ./internal/app/agent/document_investigation -count=1

# L2：pattern 编排
go test ./internal/app/agent/pattern/reactive ./internal/app/agent/pattern/planexecute -count=1

# L3：RAG chat agent 路径
go test ./internal/app/rag/service -run Agent -count=1
go test ./internal/bootstrap/rag -count=1

# 旧 tool 回归（删除前每次必跑）
go test ./internal/app/rag/tool/... -count=1
```

---

## 4. 验收标准（P0-6）

满足以下条件才算 P0-6 完成：

```text
docs/agent_tool_parity_matrix.md 存在且每个核心旧工具有 ready/partial/missing 结论
docs/agent_tool_parity_test_plan.md 存在（本文档）
missing/partial 项在矩阵「阻塞删除清单」中有后续任务
未删除 internal/app/rag/tool/
```

---

## 5. 删除旧 Tool 前的 Go/No-Go 检查表

在 P0-4 阶段 4 执行删除前，逐项勾选：

- [ ] `web_search` / `web_fetch`：T-30、T-31 通过
- [ ] `external_evidence_collect`：T-32 通过
- [ ] `document_investigation_collect` 已接入 bootstrap（`agent_runtime.go` 注入 `DocumentInvestigator`）
- [ ] T-10、T-11 在新 Agent 路径通过
- [ ] T-20、T-21 有等价新 capability 且通过（或明确接受功能降级并文档化）
- [ ] `think`：已决定保留/废弃并有对应实现
- [ ] graph 根因诊断：已迁移到 plan-execute 或接受降级
- [ ] `chat_path` / `tool_backend` 观测显示 legacy 调用量 ≈ 0
- [ ] `go test ./...` 全量通过

---

## 6. 后续任务映射

| 矩阵阻塞项 | 建议任务编号 | 说明 |
|------------|--------------|------|
| bootstrap 接入 DocumentInvestigator | P0-4 前置 | 修改 `agent_runtime.go` |
| trace 诊断 capability | 新增 P1-x | `FamilyTraceInvestigation` 已有常量，待实现 |
| think 元能力 | 评估后决定 | 可用 agent state notes 替代 |
| SSE 事件兼容 | P0-4 阶段 1 | 观测 `SendToolStart` / agent outcome 映射 |
