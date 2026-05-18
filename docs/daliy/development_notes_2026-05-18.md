# Development Notes 2026-05-18

## Tool 模块 P0 收口 + Tavily MCP 接入

### 一、Tool 模块结构收口

- 新增 `docs/tool_module_constraints.md`
  - 统一约束后续 `tool` 模块改造方向：
    - `module-first`
    - 显式依赖注入优先
    - 顶层 compat 只保留兼容职责
    - assembly 失败优先降级
- 主路径进一步摆脱包级全局状态
  - `AgentLoop` / workflow control 推断链路优先走显式 registry
  - assembly 默认 workflow 组装不再依赖主路径全局 setter
- graph tool 注册失败从 `panic` 调整为 warning + skip
  - 让单个 graph family 构造失败不再直接拖垮整个 workflow

### 二、Compat 层瘦身

- 已删除：
  - `internal/app/rag/tool/agent_loop_forward.go`
  - `internal/app/rag/tool/runtime_forward.go`
- 顶层测试改为直接依赖真源：
  - `runtime`
  - `modules/*`
  - `core`
- graph invoker 对执行器的依赖收口到 `runtime.Executor`
  - `diagnosis_graph`
  - `diagnose_search_graph`
  - `external_evidence_workflow_graph`

### 三、通用 MCP 底座首版

- 新增 `internal/infra-mcp/manager.go`
- 职责固定为：
  - 管理按名称配置的 MCP server
  - 懒启动 stdio session
  - 统一暴露 `ListTools / CallTool / Close`
- 当前只支持 `stdio`
- 错误语义已显式区分：
  - `server not configured`
  - `server disabled`
  - `unsupported transport`
  - `command missing`
  - `tool not found`
  - `call timeout`
  - `manager closed`

### 四、Tavily MCP 接入 web_search

- `web_search` provider 层新增：
  - `TavilyMCPProvider`
  - `FallbackSearchProvider`
- 当前 provider 装配逻辑：
  - `duckduckgo`
  - `tavily`
  - `tavily-mcp`
- `tavily-mcp` 路径下支持：
  - MCP 主路
  - Tavily direct API fallback
  - 也可改成 DuckDuckGo fallback
- Tavily MCP 结果会被归一化成现有 `SearchResult`
  - `Title`
  - `URL`
  - `Snippet`
  - `Provider`
  - `ProviderScore`
- 对上层 `web_search` 契约不做 breaking change
- 新增 result metadata：
  - `provider`
  - `providerActual`
  - `providerFallbackUsed`

### 五、配置与生命周期

- `config.go` 新增：
  - `RagMCPConfig`
  - `RagMCPServerConfig`
  - `RagWebSearchMCPConfig`
- `application.yaml` 更新为：
  - 默认 `rag.search.web-search.provider=tavily-mcp`
  - 默认 `fallback-provider=tavily`
  - 新增 `rag.mcp.servers.tavily`
- `bootstrap/rag/runtime.go`
  - 启动时构建 `mcpManager`
  - 将 manager 注入 `BuildLocalWorkflow(...)`
  - `Runtime.Close()` 负责回收 MCP 连接/子进程
- `web-search.api-key` 现在可自动注入到 Tavily MCP server 的 `TAVILY_API_KEY`

### 六、测试补齐

- `internal/infra-mcp/manager_test.go`
  - 真实 stdio helper server 验证：
    - `ListTools`
    - `CallTool`
    - timeout
    - tool not found
    - idempotent close
- `internal/app/rag/tool/invokers/web/web_search_provider_test.go`
  - Tavily MCP 结果归一化
  - malformed response 失败
  - fallback 生效
  - empty success 不触发 fallback
- `internal/app/rag/tool/assembly/workflow_test.go`
  - `tavily-mcp` provider 装配
- `internal/bootstrap/rag/runtime_test.go`
  - API key 注入 MCP env
  - `Runtime.Close()` 回收 manager
- `internal/framework/config/config_test.go`
  - 新配置字段加载断言

### 七、验证结果

```powershell
$env:GOCACHE='D:\goagent\.gocache'; $env:GOMODCACHE='D:\goagent\.gomodcache'; $env:GOPATH='D:\goagent\.gopath'; go test ./internal/infra-mcp ./internal/app/rag/tool/... ./internal/bootstrap/rag ./internal/framework/config -count=1

$env:GOCACHE='D:\goagent\.gocache'; $env:GOMODCACHE='D:\goagent\.gomodcache'; $env:GOPATH='D:\goagent\.gopath'; go test ./... -run Test^$ -count=1
```

- 均已通过

### 八、当前结论

- `tool` 模块已经从“兼容层偏厚”继续向“真源收口”推进了一步
- Tavily MCP 已经不是孤立接入，而是落在一层可复用的 MCP 基础设施上
- 对现有 `web_search -> web_fetch -> external_evidence_workflow` 语义未造成回退
- 后续若接 `Brave / GitHub / Playwright MCP`，可以优先复用 `infra-mcp.Manager`，再在各自 family 做显式适配
