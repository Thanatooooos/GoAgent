# Development Notes 2026-05-11

## 检索模式简化

- 删除 `internal/app/rag/core/retrieve/search_mode_decision.go`（~289 行启发式打分规则）
- 删除 `cmd/retrieve-debug/`（依赖已废弃的 AnalyzeSearchMode）
- `channels.go`：三个 `Enabled()` 改为纯基础设施检查，hybrid 模式始终启用全部 3 通道
- `search_types.go`：移除 `SearchContext.ResolvedMode / ModeDecision / QueryHints`
- `rag_chat_service.go`：`resolveRetrieveSearchMode()` 直接返回 `"hybrid"`
- `service_test.go`：删除 8 个 AnalyzeSearchMode/ResolveSearchMode 测试，更新通道期望值
- 净删除 ~350 行，全量测试 PASS（retrieve 18/18）

## 状态机去重

- 新增 `internal/app/rag/tool/next_action.go`（120 行）—— `nextAction(result)` 单一决策源
- 覆盖 5 种工具类型的 "结果 → 下一步" 映射
- `agent_loop.go`：`planCallsFromResults` 从 60 行 switch 退化为 9 行薄适配层
- `observer_rule.go`：5 个 `observe*` 函数改为 `switch reason` 模式，决策委托给 nextAction
- 全量测试 PASS

## Result 类型安全读取

- `tool.go`：Result 新增 `GetString(key)`, `GetInt(key)`, `GetStringSlice(key)`, `PreferStringSlice(primary, fallback)` 方法
- 替换范围：`next_action.go`(20), `observer_rule.go`(15), `answer_guidance.go`(21), `query_tools_test.go`(18)
- 独立 helper 函数保留给原生 `map[string]any` 场景

## Eino Graph as Tool 接入

- 引入 `github.com/cloudwego/eino v0.8.13` 依赖（compose 包）
- 新增 `eino_diagnosis_graph.go`：`DiagnosisGraphTool` 封装 3 跳诊断链（diagnose → task_query → node_query）
- Eino `NewGraph + AddLambdaNode + AddEdge + Compile` 模式，Lambda 闭包捕获 `*Executor`
- 新增 `eino_diagnose_search_graph.go`：`DiagnoseSearchGraphTool`（diagnose → web_search）
- `extractSearchKeyword()`：20 个技术错误模式 + `looksLikeTechnicalError` 启发式，零 LLM 关键词提取
- `runtime.go`：两个 graph tool 注册到 Registry
- `agent_loop.go`：`planWithBaseRules` 新增路由规则
  - 诊断关键词 → `document_root_cause_diagnosis`
  - 诊断 + 修复/解决关键词 → `document_diagnose_with_search`
- 新增 `agent_loop_graph_test.go`：9 个集成测试（路由场景 + 真实 Eino 链执行 + Observer 置信度验证）

## AgentState 合并简化

- `observer_llm.go`：`llmObserverResponse` 去掉顶层 `confidence/nextHintCalls/nextHint`
- LLM JSON schema 统一走 `state` block，5 个 few-shot 示例 + prompt template 同步更新
- `parseResponse`：state block 成为单一数据源
- `agent_loop.go`：合并逻辑从 20 行减到 11 行

## Graph Tool Observer 可读性

- `DiagnosisGraphTool.Invoke()` 产出新增 `diagnosisDepth` 字段
- `diagnosisDepthLabel()`：chainLength≥3→node_level, ≥2→task_level, 其他→diagnose_only
- `observer_rule.go` default 分支：node_level→confidence 0.95, task_level→0.75, 其他→0.6
- `result_summary.go`：`diagnosisDepth` 加入 LLM summary priority keys

## 验证状态

```
# retrieve
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/core/retrieve/... -count=1 → PASS

# tool + builtin + planner + graph tests
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/tool/... -count=1 → PASS

# service
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/service/... -count=1 → PASS

# full build
GOCACHE='d:\goagent\.gocache-agent' go build ./... → PASS
```

## 工具集总览（13 tools）

| 类别 | 工具 | 类型 |
|---|---|---|
| 诊断 | document_ingestion_diagnose, task_ingestion_diagnose, trace_retrieval_diagnose | Builtin |
| 查询 | document_query, document_chunk_log_query, ingestion_task_query, ingestion_task_node_query, trace_node_query | Builtin |
| 发现 | document_list, task_list | Builtin |
| 外部 | web_search | Builtin |
| 元 | think | Builtin |
| Graph | document_root_cause_diagnosis, document_diagnose_with_search | Eino Graph |
