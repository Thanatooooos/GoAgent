# RAG Service 包拆分方案

更新时间：`2026-06-10`

**实施状态（2026-06-13）**：Phase 0 ~ Phase 4（整体迁入）已完成并验证通过。

- Phase 0：`port/session_chunk.go`、`port/conversation_transaction.go` 已落地；adapter 层已改为只依赖 `port`
- Phase 1：`service/conversation/` 子包已创建；根包 `service/aliases.go` 保持对外 import 稳定
- Phase 2：`service/sessionrecall/` 已拆分（含 `tokens.go` 共享 token 估算）；`SessionRecallResult` 内部追踪字段已导出供 chat 观测使用
- Phase 3：`service/trace/` 已拆分；`ChatTracer` 仍留 chat 包（属 chat 写路径）
- Phase 4：`service/chat/` 已创建，RAG chat 编排代码整体迁入；根包 facade 转发 `RagChatService`、`NewRagChatServiceWithDeps` 等
- Phase 4b（chat 内部拆分，2026-06-13）：`chat/` 按职责拆为多文件（`prepare_*`、`execute_*`、`stage_*`、`agent_*`、`observability_*`、`budget_*`）；无单文件 >500 行
- **待做（可选）**：`chat/` 子包化（`budget/`、`task/` 等独立 package）；集成测试 `rag_chat_service_test.go`（1731 行）拆分到 `testutil/`
- **注意**：`AgentRuntimeService` 暂保留在 `service/agent_runtime_port.go`（若放入 `port` 会与 `agent → longtermmemory → port` 形成 import cycle）

目标：在不改变行为的前提下，把 `internal/app/rag/service` 从"上帝包"拆成职责清晰的子包，降低变更耦合，并为后续 chat / memory / agent 独立演进留出边界。

原则（与 `project-structure-discipline` 一致）：

- 小步迁移，每步 `go test` 全绿再进入下一步
- 先修依赖方向，再搬文件
- 根包 `service` 保留为**稳定 public facade**（复用 `longtermmemory` 已验证的模式）
- 不做"顺手重构"——只搬职责，不改业务逻辑

---

## 1. 现状诊断

### 1.1 体量

| 范围 | 生产文件 | 生产行数 | 主要问题 |
|------|----------|----------|----------|
| `service/` 根目录 | 28 | ~5,776 | 多 bounded context 混居一包 |
| `service/longtermmemory/` | 32 | ~3,932 | 已分包，结构相对合理 |
| **合计** | **60** | **~9,708** | 根目录是主要债务 |

### 1.2 根目录混居的职责（6 条独立变更理由）

1. **Chat 主编排**：`rag_chat_*.go`（prepare / execute / agent / runtime path）
2. **Agent 桥接**：`agent_runtime_*.go`
3. **会话内记忆**：`session_recall_*.go`、`long_message_content_processor.go`
4. **上下文预算**：`chat_context_budget.go`、`chat_token_usage.go`
5. **会话 CRUD**：`conversation_*.go`、`message_feedback_service.go`
6. **Trace 查询**：`trace_service.go`、`chat_tracer.go`

### 1.3 依赖方向问题（拆分前必须先修）

以下类型定义在 `service` 包，但被 **adapter 层反向依赖**——违反 port/adapter 方向：

| 类型 | 定义位置 | 引用方 |
|------|----------|--------|
| `ConversationMessageCreateTransaction` | `conversation_message_service.go` | `adapter/repository/postgres/rag/conversation_message_create_transaction.go` |
| `ConversationMessageChunkSink` | `conversation_message_service.go` | 同上 + `session_chunk_sink.go` |
| `ProcessedConversationMessageChunk` | `conversation_message_service.go` | `session_chunk_sink.go` |

**Phase 0 必须把这些契约迁到 `internal/app/rag/port`**，否则拆包后 adapter 会继续 import 各个子包，方向更乱。

### 1.4 越线大文件（>500 行）

| 文件 | 行数 | 拆分后归属 |
|------|------|------------|
| `rag_chat_prepare.go` | 645 | `service/chat/prepare/` |
| `rag_chat_execute.go` | 510 | `service/chat/execute/` |
| `long_message_content_processor.go` | 472 | `service/sessionrecall/` |
| `conversation_message_service.go` | 450 | `service/conversation/` |

---

## 2. 目标结构

```text
internal/app/rag/
├── port/
│   ├── repository.go                          # 已有
│   ├── session_chunk.go                       # 新增：ChunkSink / CreateTransaction / ProcessedChunk
│   ├── chat_agent.go                          # 新增：AgentRuntimeService 接口（从 service 迁出）
│   └── ...
└── service/
    ├── doc.go                                 # public facade 说明
    ├── aliases.go                             # 类型别名，保持旧 import 路径可用
    ├── chat/
    │   ├── service.go                         # RagChatService 入口 + Chat()
    │   ├── types.go                           # RagChatInput / EventSink / payload DTOs
    │   ├── deps.go                            # RagChatDeps / RagChatOptions
    │   ├── prepare/
    │   │   ├── orchestrator.go                # prepareChat 主流程
    │   │   ├── conversation_stage.go
    │   │   ├── memory_stage.go
    │   │   ├── rewrite_stage.go
    │   │   ├── session_recall_stage.go        # 调用 sessionrecall 端口
    │   │   ├── long_term_memory_stage.go
    │   │   └── retrieve_stage.go              # 含 subquestion 串并行
    │   ├── execute/
    │   │   ├── orchestrator.go                # 生成 / stream / finish
    │   │   └── tool_workflow.go               # 旧 tool 路径（退役前保留）
    │   ├── agent/
    │   │   ├── stage.go                       # runAgentChat
    │   │   ├── policy.go
    │   │   ├── pending.go                     # approval pending / resume
    │   │   ├── request_adapter.go
    │   │   └── projection/                    # SSE / trace 投影
    │   │       ├── tool_events.go
    │   │       └── trace.go
    │   ├── budget/
    │   │   ├── context_budget.go
    │   │   └── token_usage.go
    │   ├── observability/
    │   │   ├── runtime_path.go
    │   │   ├── tracer.go                      # ChatTracer（写 trace node）
    │   │   └── logging.go
    │   ├── stage/
    │   │   └── types.go                       # ragChat*StageResult 共享类型
    │   └── task/
    │       └── registry.go
    ├── conversation/
    │   ├── conversation_service.go
    │   ├── message_service.go
    │   ├── feedback_service.go
    │   └── transaction.go                     # delete tx 类型（若仍需）
    ├── sessionrecall/
    │   ├── service.go                           # SessionRecallService 实现
    │   ├── cache.go
    │   └── long_message_processor.go
    ├── trace/
    │   └── trace_service.go                     # Trace 列表/详情查询（HTTP 用）
    └── longtermmemory/                          # 保持现状，继续内部分包
        ├── governance/
        ├── recall/
        ├── extraction/
        └── types/
```

### 2.1 设计要点

1. **`RagChatService` 只做编排**，各 stage 是独立小模块，通过 `RagChatDeps` 注入——与 `agent.NewService(ServiceOptions)` 同模式。
2. **`prepare/retrieve_stage.go` 仍调用 `core/retrieve`**，不把它再包一层——retrieve 引擎已在 `core/retrieve`，service 层只负责 orchestration。
3. **`ChatTracer` 放在 `chat/observability`**，`TraceService`（HTTP 查询）放在 `trace/`——写 trace vs 读 trace 分离。
4. **根包 `service` 保留 facade**：

```go
// service/aliases.go — 示例
package service

import (
    ragchat "local/rag-project/internal/app/rag/service/chat"
    ragconversation "local/rag-project/internal/app/rag/service/conversation"
    // ...
)

type RagChatService = ragchat.Service
type RagChatInput = ragchat.Input
type RagChatDeps = ragchat.Deps
// ...

func NewRagChatServiceWithDeps(deps RagChatDeps, opts RagChatOptions) (*RagChatService, error) {
    return ragchat.NewService(deps, opts)
}
```

HTTP handler / bootstrap **第一阶段无需改 import 路径**，降低合并风险。

---

## 3. 文件迁移对照表

### 3.1 Phase 0 → port（契约下沉）

| 迁出 | 迁入 |
|------|------|
| `ConversationMessageCreateTransaction` | `port/session_chunk.go` |
| `ConversationMessageChunkSink` | `port/session_chunk.go` |
| `ProcessedConversationMessageChunk` | `port/session_chunk.go` 或 `domain/session_chunk.go` |
| `AgentRuntimeService` | `port/chat_agent.go` |
| `SessionRecallService`（interface） | `port/session_recall.go` |

### 3.2 Phase 1 → conversation/

| 现有文件 | 目标 |
|----------|------|
| `conversation_service.go` | `conversation/conversation_service.go` |
| `conversation_service_test.go` | `conversation/conversation_service_test.go` |
| `conversation_message_service.go` | `conversation/message_service.go` |
| `conversation_message_service_test.go` | `conversation/message_service_test.go` |
| `conversation_message_transaction.go` | `conversation/transaction.go` |
| `conversation_delete_transaction.go` | `conversation/delete_transaction.go` |
| `message_feedback_service.go` | `conversation/feedback_service.go` |

### 3.3 Phase 2 → sessionrecall/

| 现有文件 | 目标 |
|----------|------|
| `session_recall_service.go` | `sessionrecall/service.go` |
| `session_recall_service_test.go` | `sessionrecall/service_test.go` |
| `session_recall_cache.go` | `sessionrecall/cache.go` |
| `long_message_content_processor.go` | `sessionrecall/long_message_processor.go` |
| `long_message_content_processor_test.go` | `sessionrecall/long_message_processor_test.go` |

### 3.4 Phase 3 → trace/

| 现有文件 | 目标 |
|----------|------|
| `trace_service.go` | `trace/trace_service.go` |
| `trace_service_test.go` | `trace/trace_service_test.go` |

`chat_tracer.go` **不迁入 trace/**——它是 chat 写路径的一部分，归 `chat/observability/tracer.go`。

### 3.5 Phase 4 → chat/（最大块，内部再切）

| 现有文件 | 目标 |
|----------|------|
| `rag_chat_service.go` | `chat/service.go` + `chat/types.go` + `chat/deps.go` |
| `rag_chat_prepare.go` | 拆为 `chat/prepare/*.go`（见 §2） |
| `rag_chat_execute.go` | `chat/execute/orchestrator.go` + 必要时 `tool_workflow.go` |
| `rag_chat_stage_runner.go` | `chat/stage/types.go`（stage result 类型） |
| `rag_chat_agent_stage.go` | `chat/agent/stage.go` |
| `rag_chat_agent_policy.go` | `chat/agent/policy.go` |
| `rag_chat_agent_request.go` | `chat/agent/request_adapter.go` |
| `rag_chat_agent_pending.go` | `chat/agent/pending.go` |
| `rag_chat_runtime_path.go` | `chat/observability/runtime_path.go` |
| `rag_chat_observability.go` | `chat/observability/logging.go` |
| `rag_chat_observability_log.go` | 合并进 `logging.go` |
| `chat_context_budget.go` | `chat/budget/context_budget.go` |
| `chat_token_usage.go` | `chat/budget/token_usage.go` |
| `chat_tracer.go` | `chat/observability/tracer.go` |
| `task_registry.go` | `chat/task/registry.go` |
| `agent_runtime_port.go` | DTO 留 `chat/types.go`；接口已在 port |
| `agent_runtime_request_adapter.go` | `chat/agent/request_adapter.go` |
| `agent_runtime_tool_event_projection.go` | `chat/agent/projection/tool_events.go` |
| `agent_runtime_trace_projection.go` | `chat/agent/projection/trace.go` |
| 全部 `*_test.go` | 跟随源文件目录 |

### 3.6 根包保留

| 文件 | 处理 |
|------|------|
| `service/doc.go` | 新建，说明 facade 职责 |
| `service/aliases.go` | 新建，类型别名 + 构造函数转发 |
| `service/rag_chat_test_helpers_test.go` 等 | 暂留根包或迁 `chat/testutil/`，Phase 4 一并处理 |

---

## 4. 分阶段执行计划

每阶段结束运行：

```powershell
go test ./internal/app/rag/service/... ./internal/bootstrap/rag ./internal/adapter/http/rag/... -count=1
go build ./...
```

### Phase 0：契约下沉到 port（1~2 天）

**做什么：**

1. 新增 `port/session_chunk.go`、`port/chat_agent.go`、`port/session_recall.go`
2. `conversation/message_service` 改用 port 类型
3. 更新 `adapter/repository/postgres/rag/session_chunk_sink.go` 等引用
4. `service` 根包加 deprecated 类型别名（可选，过渡期）

**验收：**

- adapter 不再 `import service` 只为拿 transaction/sink 类型
- 测试全绿，无行为变更

### Phase 1：拆 conversation（2~3 天）

**做什么：**

1. 创建 `service/conversation/`，按 §3.2 搬文件
2. 根包 `aliases.go` 转发 `ConversationService`、`NewConversationService` 等
3. 更新 `bootstrap/rag/runtime_build_conversation.go` 可继续 `ragservice.NewConversationService`（via alias）

**验收：**

- `conversation/` 包独立可测
- handler import 路径不变

### Phase 2：拆 sessionrecall（2~3 天）

**做什么：**

1. 创建 `service/sessionrecall/`
2. `SessionRecallService` 实现迁入；接口已在 port
3. `runtime_build_retrieve.go::buildSessionRecallService` 改为构造 `sessionrecall.NewService(...)`
4. 根包 alias 保留 `SessionRecallService`、`NewLongMessageContentProcessor`

**验收：**

- session recall 单测可在子包独立运行
- chat prepare 的 session recall stage 行为不变

### Phase 3：拆 trace 查询（1 天）

**做什么：**

1. `trace_service.go` → `service/trace/`
2. `ChatTracer` 暂不移动（仍属 chat 写路径，Phase 4 处理）

**验收：**

- `trace_handlers.go` 通过 alias 继续工作

### Phase 4：拆 chat（5~7 天，最大块）

**建议子步骤（每步一个 PR）：**

| 子步骤 | 内容 | 风险 |
|--------|------|------|
| 4a | 建 `chat/deps.go`、`chat/types.go`，`RagChatService` 骨架迁入 | 低 |
| 4b | 迁 `budget/`、`task/`、`observability/`（除 tracer） | 低 |
| 4c | 迁 `agent/` + projection | 中 |
| 4d | 拆 `prepare/`：`orchestrator.go` 先搬，再按 stage 拆文件 | 中 |
| 4e | 迁 `execute/` | 中 |
| 4f | 迁 `chat_tracer.go` → `observability/tracer.go` | 低 |
| 4g | 根包 facade 完成；删除根目录已迁走的源文件 | 低 |

**`rag_chat_prepare.go` 拆分建议（645 行 → 6 文件，每个 <200 行）：**

```text
prepare/orchestrator.go      # prepareChat() 主顺序
prepare/conversation_stage.go
prepare/memory_stage.go      # history load
prepare/rewrite_stage.go
prepare/recall_stages.go     # session + long-term memory
prepare/retrieve_stage.go    # retrieve + subquestion 串并行
```

**验收：**

- `rag_chat_service_test.go` 全绿
- `chat_handler_test.go` E2E 样例全绿
- 根目录生产文件 ≤ 5 个（doc + aliases + 测试 helper）

### Phase 5：清理 facade（可选，1~2 天）

**做什么：**

1. 逐步把 handler / bootstrap 的 import 从 `service` 改为具体子包（`service/chat`、`service/conversation`）
2. 文档标记根包 alias 为 deprecated
3. 一个 major 版本后删除 alias

**是否必须：** 否。长期保留 facade 也可接受（`longtermmemory` 就是这样做的）。

---

## 5. bootstrap 改动要点

当前接线集中在：

- `runtime_build_conversation.go` → conversation 三件套 + long message processor
- `runtime_build_retrieve.go` → trace + session recall + tracer
- `runtime_build_chat.go` → `NewRagChatServiceWithDeps`

Phase 1~4 期间 **bootstrap 签名可不变**（通过 facade）。若希望 bootstrap 更清晰，可在 Phase 4 后改为：

```go
// runtime_build_chat.go — 目标形态
import (
    ragchat "local/rag-project/internal/app/rag/service/chat"
    ragconversation "local/rag-project/internal/app/rag/service/conversation"
    ragsessionrecall "local/rag-project/internal/app/rag/service/sessionrecall"
)

chatService, err := ragchat.NewService(ragchat.Deps{
    Conversation: conversationSvc,
    Messages:     messageSvc,
    History:      historySvc,
    SessionRecall: sessionRecallSvc,
    // ...
}, ragchat.Options{...})
```

---

## 6. 测试策略

1. **每阶段：先搬测试文件，再搬实现**——保证 RED→GREEN 可感知
2. **保留现有集成测试路径**：`rag_chat_service_test.go` 是对 prepare 全链路最重要的回归，Phase 4 完成前不要删
3. **新增子包单测**：
   - `conversation/message_service_test.go` 已在子包
   - Phase 4 后补 `chat/prepare/retrieve_stage_test.go` 专注 subquestion 串并行（从 prepare 大文件抽离时顺带做）
4. **禁止**：拆包同时改业务逻辑；参数/阈值变更单独 PR

---

## 7. 明确不做

| 不做的事 | 原因 |
|----------|------|
| 把 `core/history` 并入 service | 已是独立 core 包，方向正确 |
| 把 `longtermmemory` 提升到 `app/rag/longtermmemory` | 改动面过大；当前子包结构已够用 |
| 拆包时重写 `RagChatService.Chat()` 主流程 | 行为变更风险高 |
| 一次性删根包所有文件、全量改 import | 难以 review，回滚成本高 |
| 为拆包引入新的 `util/helper` 包 | 违反 project-structure-discipline |

---

## 8. 完成标准

拆分完成后应满足：

1. 根包 `service/` 生产文件 ≤ 5 个（facade + doc）
2. 无单文件 > 500 行（`prepare/retrieve_stage.go` 若接近阈值再拆）
3. adapter 层不 import 任何 `service/chat` 等子包（只 import `port` + 必要时 `service` facade）
4. 每个子包有单一变更理由：
   - `chat/`：一次用户提问的编排
   - `conversation/`：会话与消息 CRUD
   - `sessionrecall/`：会话内长消息召回
   - `trace/`：trace 查询
   - `longtermmemory/`：长期记忆读写
5. `go test ./internal/... -count=1` 全绿

---

## 9. 与现有计划文档的关系

| 文档 | 关系 |
|------|------|
| `structural_improvement_plan.md` P0-5 | RagChatService 构造注入收敛 — **在 Phase 4a 一并完成**，deps 进 `chat/deps.go` |
| `memory_improvement_plan.md` | sessionrecall / longtermmemory 拆分后，memory 改动不再碰 chat 根包 |
| `agent_module_evaluation_20260610.md` | agent 桥接代码归 `chat/agent/`，bootstrap 注入更清晰 |
| `project-structure-discipline` | 本方案的执行实例 |

---

## 10. 建议执行顺序（总结）

```text
Week 1:  Phase 0 (port) + Phase 1 (conversation)
Week 2:  Phase 2 (sessionrecall) + Phase 3 (trace)
Week 3~4: Phase 4a~4g (chat 拆分，每步一个 PR)
Later:   Phase 5 (可选，逐步去掉 facade alias)
```

**第一刀最值得切的是 Phase 0 + Phase 1**：修依赖方向 + 把 conversation 从 chat 主编排里剥离，立刻减少"改消息入库却 diff 到 rag_chat_prepare"的噪音。
