# Agent Runtime 入口梳理（P0-4 阶段 2）

> 生成时间：2026-06-09  
> 扫描命令：`rg -n "UseAgentRuntime" internal cmd -S`

## 1. 顶层路径分叉

| 入口 | 文件 | `UseAgentRuntime` | 实际路径 |
|------|------|-------------------|----------|
| `RagChatService.Chat` | `internal/app/rag/service/rag_chat_service.go:280` | `true` | `runAgentChat` → `agent.Service`（顶层 agent runtime） |
| `RagChatService.Chat` | 同上 | `false`（默认） | legacy 主路径：`prepareChat → retrieve → tool stage → prompt → stream` |

`chat_path` 观测值：
- `agent_runtime_top_level`：`UseAgentRuntime=true`
- `rag_chat_legacy_main`：`UseAgentRuntime=false`

## 2. HTTP API 入口

| 端点 | 文件 | `UseAgentRuntime` 传参 | 说明 |
|------|------|------------------------|------|
| `GET /rag/v3/chat` | `internal/adapter/http/rag/chat_handler.go:31` | **未传**（默认 `false`） | 走 legacy 主路径 |
| `POST resume approval` | `chat_handler.go:58` | N/A | 直接 `ResumeAfterApproval`，走 agent runtime |

HTTP 层当前**没有** query 参数控制 `UseAgentRuntime`。测试也明确期望 handler 不强制 agent 顶层路径（`chat_handler_test.go:61`）。

## 3. Tool Stage 分叉（legacy 主路径内）

| 条件 | 文件 | 路径 |
|------|------|------|
| `input.UseAgentRuntime=true` | `rag_chat_agent_policy.go:27` | `runAgentToolWorkflowStage` |
| `agentRuntimeMode=always` 且满足 tool 条件 | `rag_chat_agent_policy.go:34` | `runAgentToolWorkflowStage` |
| `agentRuntimeMode=diagnostic` 且诊断类问题 | `rag_chat_agent_policy.go:37` | `runAgentToolWorkflowStage` |
| 其他 | `rag_chat_execute.go:320` | `runLegacyToolWorkflowStage` |

配置项：`configs/application.yaml` → `rag.agent.chat.mode`（当前通常为 `always`）。

这意味着：即使 HTTP 不传 `UseAgentRuntime`，只要 `mode=always` 且问题触发 tool stage，仍会走 **agent tool backend**。

## 4. 测试入口默认值

| 位置 | 默认 `UseAgentRuntime` |
|------|------------------------|
| `rag_chat_agent_stage_test.go` 顶层 agent 测试 | `true`（显式设置） |
| `rag_chat_service_test.go` 大部分测试 | `false`（未设置） |
| HTTP handler 测试 | 期望 `false` |

## 5. 审批 Resume 入口

`ResumeAfterApproval`（`rag_chat_agent_stage.go:22`）始终走 agent runtime，不经过 `UseAgentRuntime` 判断。

## 6. 当前生产行为结论

```text
HTTP 聊天请求
  → UseAgentRuntime=false（默认）
  → legacy 主路径
  → tool stage 根据 rag.agent.chat.mode 决定 backend
     - mode=always：tool_backend=agent_runtime（常见）
     - agent 失败：fallback 到 tool_workflow_fallback

显式 UseAgentRuntime=true（仅测试/内部调用）
  → chat_path=agent_runtime_top_level
  → 全程 agent runtime
```

## 7. 灰度切换建议（P0-4 阶段 3 前置）

1. 在 HTTP handler 增加可选 query：`useAgentRuntime=true`（或读取配置默认值）。
2. 观察 `runtimePath.chatPath` / `runtimePath.toolBackend` trace 字段 1–2 周。
3. 确认 `tool_workflow_fallback` 调用量趋近 0 后再删旧 tool。

## 8. 阻塞项

- [ ] HTTP 是否应默认 `UseAgentRuntime=true`（需产品/前端确认）
- [ ] `mode=always` 下 legacy 主路径是否仍必要保留
- [ ] 审批 resume 与顶层 agent 路径的 trace 字段是否统一
