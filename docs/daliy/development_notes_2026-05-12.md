# Development Notes 2026-05-12

## Agent Search 能力落地（RAG 优先，KB 不足时触发搜索）

### 核心原则

**RAG 优先**：只在知识库检索结果不足时（chunks=0、低分、通道全错误）才触发联网搜索。
诊断链路（有 document/task/trace ID）不受影响。

### WebFetchTool 新增

- 新增 `internal/app/rag/tool/builtin/web_fetch_tool.go`（~230 行）
- 支持 1-3 个 URL 并发抓取，提取网页正文
- 正则剥离 `<script>/<style>/<head>` + HTML 标签，`html.UnescapeString` 解码实体
- 过滤导航/样板行（<20 字符 + <5 字母），正文截断 8KB
- 10s 超时，1MB 响应限制，标准 `goagent/1.0` User-Agent
- 支持 `urls` 数组参数（max 3），也兼容单 `url` 字符串

### SearchProvider 抽象 + Tavily 国内搜索支持

- `web_search_tool.go`：抽出 `SearchProvider` 接口
  ```go
  type SearchProvider interface {
      Search(query string) ([]SearchResult, error)
  }
  ```
- `DuckDuckGoProvider`：原有逻辑提取为独立 provider（免费，国内被墙）
- 新增 `internal/app/rag/tool/builtin/web_search_tavily.go`：`TavilyProvider`
  - Tavily Search API（`api.tavily.com`），专为 AI Agent 设计
  - 国内可访问（已验证），免费 1000 次/月
  - 返回结构化结果（title/url/content/score）
- `NewWebSearchTool(provider SearchProvider)`：接受可配置的搜索后端
- `config.go`：新增 `RagWebSearchConfig`（provider + api-key）
- `runtime.go`：`buildSearchProvider(cfg)` 根据配置选择 provider
- `application.yaml`：新增 `rag.search.web-search` 配置段

### 搜索链路集成到 AgentLoop

- `agent_loop.go`：`planWithBaseRules` 新增 KB 不足检测
  - 条件：无特定 ID + `kbInsufficient(RetrieveResult)` → 规划 `web_search`
- `next_action.go`：新增 `web_search`/`web_fetch` 分支
  - `web_search` 有结果 → `web_fetch(urls=[前3个URL])`
  - `web_fetch` → terminal
- `observer_rule.go`：
  - 新增 `observeWebSearch()` / `observeWebFetch()` 观察函数
  - `kbInsufficient()` 函数：`len(Chunks)==0` 或 `topScore<0.4` 或全通道错误
  - `document_list`/`task_list` 返回空 + KB 不足 → 触发 `web_search`
- `observer_llm.go`：
  - 新增 3 个 few-shot 示例（#6-#8）：KB 不足→搜索、搜索完成→抓取、抓取完成→回答
  - 新增规则 #10：检索质量评估引导
- `answer_guidance.go`：
  - `BuildAnswerGuidance` 支持 web 结果分支
  - `buildWebSearchGuidance()`：引导信源标注、矛盾显式化、知识库优先、局限性说明

### 全链路日志补强

- `executor.go`：`[tool] <name> started/success/failed (<N>ms)` — 每次工具调用的启动参数、结果、耗时
- `agent_loop.go`：
  - `[agent] start: question=%q maxRounds=%d parallel=%v` — 启动参数
  - `[agent] round N: M call(s) [names] (mode)` — 每轮规划的工具
  - `[agent] round N observer: DONE/CONTINUE` — Observer 决策
  - `[agent] done: N round(s), M call(s), degraded=%v` — 最终统计
- `observer_rule.go`：`[observer] kb insufficient (chunks=N), triggering web_search` — KB 不足触发原因
- 修复 `observer_rule.go` 缩进问题（第 148-151 行）

### 工具集更新

- 现有工具从 13 个增加到 **14 个**：+ WebFetchTool
- `readStringSliceArg` + `truncateText` 提取到 `args.go` 公共函数

## P0 收口（第二轮）：typed view、执行语义、capability trace

### 结果 typed view 落地

- 新增 `internal/app/rag/tool/result_views.go`
- 首批落地 3 个结果视图：
  - `WebSearchResultView`
  - `WebFetchResultView`
  - `DiagnosisResultView`
- 目标不是单纯“换个 struct”，而是把高频 `Result.Data` 消费点从散读 `map[string]any` 收成统一入口，减少 renderer / observer / nextAction / guidance 对字段名和 payload 形态的重复假设

### Guidance / Renderer 边界收口

- `answer_guidance.go`
  - `diagnose` guidance 改为通过 `DiagnosisResultView` 读取 facts / next actions / risks
  - `buildWebSearchGuidance()` 现在会显式区分“本地/知识库侧已知证据”和“外部网页来源”
- `renderer.go`
  - `web_search` / `web_fetch` 上下文渲染改为消费 typed view
- `next_action.go` / `observer_rule.go`
  - web 分支改为通过 view 提取 URL、判断 fetch 截断状态、读取 web 结果摘要

### 统一执行语义显式化

- 新增 `internal/app/rag/tool/workflow_control.go`
- 首批统一的 workflow 控制字段：
  - `ExecutionMode`: `read_only` / `proposal_only` / `guarded_write`
  - `RiskLevel`: `low` / `medium` / `high`
  - `ApprovalRequirement`: `none` / `recommended` / `required`
  - `Capability`: `knowledge` / `diagnosis` / `search` / `general`
- 新增 `WorkflowTraceMeta`
  - 用于记录能力域、证据来源和退化状态

### Workflow / Prompt / Trace 接线

- `WorkflowInput` / `WorkflowResult` 已显式携带 `Control` 与 `TraceMeta`
- 当前 tool workflow 默认注入 `read_only + low + none`
- `AgentLoop` 结束时会推导：
  - 当前 capability
  - evidence sources（`knowledge_base` / `system_records` / `rag_trace` / `external_web`）
- `prompt.Context` 新增 `WorkflowPolicy`
  - 回答阶段会额外收到一条 `## 执行约束` 系统消息
- `rag_chat_service.go`
  - trace run extraData 新增 `toolWorkflow.control` / `toolWorkflow.traceMeta` / `toolCallCount` / `roundCount`
- `chat_tracer.go`
  - `agt_round` / `agt_obs` 节点 extraData 新增 `capability` / `workflowMode` / `riskLevel` / `approvalRequirement` / `evidenceSources`

### 这轮的实际意义

- 结果契约开始从“约定俗成”走向“显式 view”
- workflow 运行边界开始从“代码里隐含”走向“上下文化”
- trace 开始具备 capability 级元数据，而不只是调用流水

### 后续还可继续补的 P0/P1 衔接点

- 为 `task_query` / `task_node_query` / `trace_node_query` 补齐 typed result view
- 继续减少 renderer / observer / guidance 对原始 `map[string]any` 的直接依赖
- 在 trace / answer rendering 中进一步利用 capability 与 evidence sources 做展示收口

## 验证状态

```
# tool 全量
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/tool/... -count=1 → PASS

# builtin
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/tool/builtin/... -count=1 → PASS

# planner
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/tool/planner/... -count=1 → PASS

# service
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/service/... -count=1 → PASS

# config
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/framework/config/... -count=1 → PASS

# 全量 internal
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/... -count=1 → 35 packages PASS, 0 FAIL

# full build
GOCACHE='d:\goagent\.gocache-agent' go build ./... → PASS

# 补充验证（P0 第二轮）
GOCACHE='d:\goagent\.gocache-agent' go test ./internal/app/rag/tool/... ./internal/app/rag/core/prompt ./internal/app/rag/service/... -count=1 → PASS
```

## 工具集总览（15 tools）

| 类别 | 工具 | 类型 |
|---|---|---|
| 诊断 | document_ingestion_diagnose, task_ingestion_diagnose, trace_retrieval_diagnose | Builtin |
| 查询 | document_query, document_chunk_log_query, ingestion_task_query, ingestion_task_node_query, trace_node_query | Builtin |
| 发现 | document_list, task_list | Builtin |
| 外部 | web_search, web_fetch | Builtin |
| 元 | think | Builtin |
| Graph | document_root_cause_diagnosis, document_diagnose_with_search, external_evidence_workflow | Eino Graph |

## 外部证据工作流收口（source review + quality + readiness）

### 新增 external_evidence_workflow

- 新增 `internal/app/rag/tool/graph/external_evidence_workflow_graph.go`
- 用 Eino Graph 固化如下链路：
  - `web_search`
  - `select`（来源筛选）
  - `web_fetch`
  - `assess`（质量审核 + 回答 readiness）
- 目标不是再造一个“web_search 包装器”，而是把“外部网页证据”提升为可被 Agent / Prompt / 前端共同消费的结构化工作流结果

### source review 结构化

- `select` 阶段新增 `sourceReview`
- 显式沉淀：
  - `selectedUrls`
  - `selectedDomains`
  - `selectedSourceTypes`
  - `sourceCoverage`
  - `selectedSources / rejectedSources`
  - `allowedCount / neutralCount / deniedCount`
  - `distinctDomains / distinctSourceTypes`
- 这样后续不管是 Answer Guidance、Tool 卡片还是 trace，都能直接看到“为什么选了这些来源”

### quality assessment 结构化

- `assess` 阶段拆成两层：
  - `qualityAssessment`
  - `readiness`
- `qualityAssessment` 当前覆盖：
  - `quality`
  - `confidence`
  - `reasoning`
  - `sourceDiversity`
  - `corroboration`
  - `successfulPages / failedPages / emptyPages / truncatedPages`
  - `notes`
- `readiness` 继续负责最终作答是否足够、缺失什么信息、应如何组织答案

### typed view / guidance / renderer 接线

- `result_views.go`
  - 新增 `ExternalEvidenceWorkflowView`
  - 统一读取 `sourceReview` 和 `qualityAssessment`
- `answer_guidance.go`
  - 新增 `buildExternalEvidenceGuidance(...)`
  - 回答引导现在会显式区分：
    - 本地/知识库证据
    - 外部来源质量
    - 最终结论
    - 局限与引用
- `renderer.go`
  - `external_evidence_workflow` 的 ToolContext 现在会包含：
    - selected sources
    - fetched content
    - quality
    - readiness
- `result_summary.go`
  - LLM 摘要新增：
    - `searchQuery`
    - `quality`
    - `sourceCoverage`
    - `sourceDiversity`
    - `corroboration`
    - `readiness`
    - `selectedUrls`
    - `citedUrls`

### 新增测试

- 新增 `internal/app/rag/tool/eino_external_evidence_workflow_graph_test.go`
- `tool_test.go`
  - 补 external evidence workflow view 解析
  - 补 external evidence guidance 断言
- `result_summary_test.go`
  - 补 external evidence 字段摘要断言

## 联调收口：Tool 卡片事件字段对齐

### 问题定位

- 前后端联调时发现 Tool 卡片只显示空白 running 占位
- 原因是：
  - 后端 `ToolCallEvent` 没有 JSON tag
  - SSE 发出的字段是 `CallID / Name / Summary / DurationMs`
  - 前端按 `callId / name / summary / durationMs` 读取
- 结果：
  - `callId` 丢失，`tool_start` 和 `tool_result` 不能合并
  - `name` 丢失，卡片标题空白
  - `summary/arguments/data` 丢失，只剩状态占位

### 后端修复

- `internal/app/rag/tool/workflow.go`
  - 为 `ToolCallEvent` 补齐 JSON tag：
    - `callId`
    - `round`
    - `sequence`
    - `name`
    - `status`
    - `summary`
    - `durationMs`
    - `arguments`
    - `data`

### 前端兼容修复

- `frontend/src/stores/chatStore.ts`
  - 新增 `normalizeToolCallPayload(...)`
  - 同时兼容 camelCase / PascalCase
  - 即便前端连的是旧后端进程，也能正确显示 Tool 卡片

### 联调意义

- Tool 卡片现在能稳定展示：
  - 工具名
  - 轮次
  - 状态
  - 参数
  - 摘要
  - 耗时
- 后续继续优化 UI 时，可以把重点放在“复杂结果的专门展示”，而不是先修基础事件字段

## external_evidence_workflow 阶段日志补齐

- `external_evidence_workflow_graph.go` 新增阶段级日志：
  - `workflow start`
  - `search start / done`
  - `select done`
  - `fetch start / done`
  - `assess done`
  - `workflow done`
- 现在从日志可以直接看出：
  - provider / resultCount / allow-neutral-deny 分布
  - 选中了哪些 URL
  - coverage / domain/sourceType 多样性
  - fetch 成功/失败/截断情况
  - 最终 quality / readiness

## 今日补充验证

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool -count=1
```

- `internal/app/rag/tool/...` PASS
- `internal/app/rag/tool` PASS

## RAG Tool 模块化长期重构推进

### 今日新增落地

- 新增长期路线文档：`docs/rag_tool_modularization_long_term_plan.md`
- 已完成 `Phase 1 Foundation`
  - 建立 `ToolModule / ToolSpec / ToolBehavior / ResultMeta / ModuleRegistry`
  - `Executor` 按 module 执行并注入 `Result.Meta`
  - runtime 改为注册 module
- 已完成 `Phase 2 Web 工具链迁移`
  - `web_search`
  - `web_fetch`
  - `external_evidence_workflow`
- 已完成 `Phase 3 中央编排瘦身` 的主体
  - `AgentLoop` / `RuleObserver` / `RenderContext` / `AnswerGuidance` 改为 module-first
- 已推进 `Phase 4 系统内工具家族迁移`
  - `document_query / document_chunk_log_query / document_list`
  - `ingestion_task_query / ingestion_task_node_query / task_list`
  - `document_ingestion_diagnose / task_ingestion_diagnose / trace_retrieval_diagnose / trace_node_query`
  - `think`
  - `document_root_cause_diagnosis / document_diagnose_with_search`

### 今日关键工程变化

- 旧 `MustRegister(tool)` 已支持对已迁移家族自动推断 behavior
  - 这样历史测试和老注册方式不用一次性全改
  - 同时已迁移工具仍然可以走 module-first 主路径
- `RuleObserver` 的中央名字分支已明显收薄
  - 当前优先级为：
  - registry behavior
  - legacy behavior inference
  - generic fallback
- `BuildAnswerGuidanceWithRegistry(...)` 已优先吃模块自带 guidance
  - graph 结果不再只能依赖中央 diagnosis guidance fallback

### 当前判断

- 这轮重构已经从“架构设计 + 底座搭建”进入“主链路已模块化、legacy fallback 清理中”的阶段
- 后续新增外部 tool 的推荐方向保持不变：
  - `api/*`
  - `github/*`
  - `db/*`

### 今日增量验证

```powershell
$env:GOCACHE=(Join-Path (Resolve-Path .).Path '.gocache'); go test ./internal/app/rag/tool/... ./internal/bootstrap/rag -count=1
```

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/builtin` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/bootstrap/rag` PASS
