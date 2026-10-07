# Conversation Agent Runtime 收敛规格

## 1. 状态

- 状态：分阶段实施中。`internal/app/runtime` 已接入；本文包含原始提案与后续冻结语义，具体交付范围以代码和项目进展上下文为准。
- 范围：将普通 RAG 对话收敛为 Agent Runtime 驱动的对话主链路。
- 非目标：新增 coding-agent、替换知识库检索算法、重写前端 UI、删除既有路径。

## 2. 背景与问题

当前 `RagChatService.Chat` 有两条并存路径：

1. 默认路径：对话服务预先完成历史、记忆、rewrite、retrieve、tool workflow、prompt assembly，最后作一次流式回答。
2. `UseAgentRuntime` 路径：对话服务将问题交给 `internal/app/agent`，由 plan-execute 或 reactive runtime 运行 capability，并投影结果。

两条路径的职责重叠：同一个用户问题可能在 RAG 对话层被决定“是否检索”，又在 agent runtime 中被决定“是否搜索”。这使策略、预算、trace、错误语义和 SSE 事件难以统一。

目标不是给产品增加一个 coding agent，而是让 **Agent Runtime 成为对话的唯一决策与执行内核**。对话层只负责身份、会话、输入接纳、持久化、SSE 投影和取消；runtime 决定是否以及如何使用知识库、长期记忆、外部服务与其他能力，然后生成最终回答。

## 3. 目标架构

```text
HTTP / SSE Chat API
  -> authenticate + validate request
  -> create/find conversation, persist user message
  -> admit user input into runtime session
  -> ConversationAgent.Run
       -> load visible conversation state
       -> assemble system policy + history + capability catalog
       -> model/runtime decision loop
            -> answer directly
            -> recall_memory
            -> retrieve_knowledge
            -> web_search / web_fetch
            -> approved business capability
       -> persist capability lifecycle and evidence
       -> synthesize final answer
  -> persist assistant message
  -> project runtime events to existing SSE contract
```

边界如下：

| 层 | 负责 | 不负责 |
| --- | --- | --- |
| `rag/service/chat` | HTTP/SSE、用户身份、conversation/message 持久化、取消、对外响应投影 | rewrite/retrieve/tool 的决策和编排 |
| `agent/runtime` | 回合调度、capability 选择/执行、批准暂停恢复、预算、状态/证据归并 | Gin/SSE、数据库 HTTP DTO |
| `agent/capability` | 一个可验证的领域动作及其输入、输出、state delta、evidence | 对话生命周期与最终 UI |
| `rag/core/*` | 检索、rewrite、prompt/citation 等领域算法和数据模型 | 顶层流程控制 |

## 4. 核心原则

1. **Runtime owns decisions.** 是否检索、检索顺序和是否转向外部证据必须由 runtime 决定，而不是由 chat service 提前固定。
2. **Policy constrains decisions.** “模型决定”不等于无限制：用户权限、知识库 scope、外网开关、token/轮数预算和引用协议始终是硬约束。
3. **Capabilities are typed.** 所有可调用能力必须经过 `Spec -> NormalizeInput -> Invoke`；不得让模型直接拼接数据库或外部服务调用。
4. **One durable source of truth.** conversation/message 是用户对话事实；runtime session/journal 是本次 agent 执行事实。二者通过稳定 ID 关联，不能分别伪造历史。
5. **Event projection is one-way.** runtime 产出结构化事件，chat 层投影到现有 SSE；前端不推断 runtime 状态。
6. **Progressive migration.** 在新路径达到等价验收前，保留 legacy RAG 主链路作为受控回退。

## 5. 对话运行契约

### 5.1 输入

`ConversationAgent.Run` 的输入必须至少含：

```go
type ConversationRunRequest struct {
    ConversationID    string
    UserID            string
    UserMessageID     string
    Question          string
    KnowledgeBaseIDs  []string
    RuntimePolicy     RuntimePolicy
    TraceID           string
}

type RuntimePolicy struct {
    AllowKnowledgeRetrieval bool
    AllowMemoryRecall       bool
    AllowWebSearch          bool
    RequireApproval         bool
    MaxTurns                int
    MaxToolCalls            int
    ContextTokenBudget      int
}
```

`KnowledgeBaseIDs` 是 caller 指定的访问范围，不是模型可扩大的建议值。空值的语义必须由产品策略明确：全局可访问范围或完全禁止知识库检索，二者不可混淆。

### 5.2 输出

```go
type ConversationRunResult struct {
    Status              string // completed | awaiting_approval | degraded | failed | cancelled
    AssistantContent    string
    Evidence            []EvidenceRef
    RuntimeSessionID    string
    CheckpointID        string
    DegradeReason       string
}
```

`awaiting_approval` 不创建伪造的 assistant final answer；chat 层应发送已有的 `approval_pending` 和 `done` SSE 事件。恢复后继续同一 runtime session，并仅在真正完成时写入 assistant message。

### 5.3 Runtime 状态

runtime session 至少需要持有：原始问题、用户/KB scope、可见历史引用、能力调用 journal、证据、当前计划或回合、预算消耗、approval/checkpoint 和最终回答草稿。不得只传递一个截断的 `HistorySummary` 来替代对话历史；摘要可以是压缩投影，但应可追溯到 conversation message 边界。

## 6. Capability 目录

首期由 runtime 暴露以下能力：

| Capability | 用途 | 现状 | 必要约束 |
| --- | --- | --- | --- |
| `memory_recall` | 读取当前用户长期记忆 | 已有 | 仅当前用户与授权 scope |
| `retrieve_knowledge` | 从指定知识库检索 chunk/evidence | 新增 | 必须携带受限 KB scope、返回 citation 元数据 |
| `knowledge_discovery` | 列知识库/文档、按名称查找 | 已有 | 不是 `retrieve_knowledge` 的替代品 |
| `web_search` | 获取外部候选来源 | 已有 | 受 `AllowWebSearch`、来源策略与预算限制 |
| `web_fetch` | 获取已许可 URL 的正文 | 已有 | URL/source policy、大小和次数限制 |
| `content_summarize` | 压缩长证据或历史 | 已有 | 仅 runtime 内部按预算使用 |
| `final_answer` | 用问题、历史和已接受证据生成回答 | 作为 runtime finalize 节点 | 不是向模型公开的任意工具 |

`retrieve_knowledge` 是本次收敛的前置缺口。它应复用既有 RAG retrieve service，而不是重新实现向量、关键词、hybrid、rerank 或 citation 算法。其输出应包含可展示文本、chunk/document 标识、来源信息、相关性和引用所需字段，并将已接受项写入 `EvidenceDelta`。

## 7. 调度模型

首期采用“受 runtime 管理的模型回合循环”，而非 chat service 预先串联阶段。每一个回合：

```text
load runtime snapshot + visible history
-> assemble policy and capability catalog
-> request model decision
-> direct answer: finalize
-> capability call: validate -> policy/scheduler -> execute -> apply delta/evidence -> next turn
-> approval required: checkpoint -> awaiting_approval
```

实现可以继续使用现有 plan-execute / reactive graph，也可以在 `agent/runtime` 引入一个专用于 conversation 的 native-tool-call runner；两者都必须遵守上述输入、状态与事件契约。

推荐优先复用现有 runtime 的 registry、resolver、scheduler、approval、checkpoint、state reducer 和 journal。若引入 native-tool-call runner，Miso 的可迁移价值是其“单回合、工具先记录/再执行/再结算、失败结果回喂、步数和循环限制”的 harness 语义；不可直接复制 Miso 的 SQLite store、workspace 工具或 permission 实现。

## 8. 预算、证据与回答规则

### 8.1 预算

独立记录并强制以下预算：

- 最大 runtime turn；
- 最大 capability 调用数；
- 外部搜索次数、抓取 URL 数和抓取字节数；
- context token budget、保留 output budget 和 summary budget；
- 单 capability timeout 与总任务 deadline。

预算耗尽时 runtime 进入 `degraded` 或直接基于已有可靠证据回答；不得静默继续调用外部服务。

### 8.2 证据

每个产生事实性内容的 capability 应写入结构化 evidence，而非只追加自由文本 note。最终回答节点只能引用 runtime snapshot 中接受的 evidence；知识库引用和外部网页引用必须可区分。

### 8.3 回答

`final_answer` 读取：用户问题、已压缩且有边界的历史、用户授权的记忆、已接受 evidence、citation policy 和降级原因。它不应看到未授权的能力输出、被拒绝的调用详情或系统内部错误堆栈。

## 9. SSE 与持久化投影

保留已有前端事件契约，并从 runtime journal 投影：

| Runtime 事件 | SSE 事件 |
| --- | --- |
| capability start | tool running |
| capability result / skipped / degraded | tool completed / failed |
| approval pending | approval_pending |
| final answer delta | message |
| terminal completed/degraded | finish + agent_outcome + done |
| cancelled/failed | cancel/error + done |

对话层只持久化用户和最终 assistant 消息；runtime journal、checkpoint、capability 调用与 evidence 由 runtime 存储负责。两边均保存 `conversation_id`、`user_message_id`、`trace_id` 和 `runtime_session_id` 作为关联键。

## 10. 迁移计划

### Slice 0：契约冻结

- 定义 `ConversationRunRequest/Result`、runtime event 映射和状态表。
- 明确 KB 空 scope、外网开关、approval 和 cancelled 的产品语义。
- 增加 contract tests；不改变线上路径。

### Slice 1：知识库检索能力

- 新增 `retrieve_knowledge` capability，适配现有 RAG retrieve service。
- 覆盖 typed input、scope、evidence/citation、预算和失败降级。
- 不改变默认 chat 路径。

### Slice 2：完整 runtime 对话试验路径

- `UseAgentRuntime` 改为完整 `ConversationAgent.Run`，不再只传 `ToolStageContext` 摘要。
- 通过 SSE 投影真实 journal；保留现有 approval/resume。
- 覆盖：直接回答、仅 KB、仅 web、KB 后 web、memory、拒绝 approval、取消、降级。

### Slice 3：行为对齐与灰度

- 对相同问题运行 legacy 和 runtime 路径，比较回答、引用、工具次数、token、错误与延迟。
- 通过配置和按请求灰度；legacy 仅作为明确 fallback，不能在一次请求内双重执行产生重复副作用。

### Slice 4：默认路径切换与收尾

- runtime 路径达到验收后成为默认。
- 删除 chat service 中已迁移的 rewrite/retrieve/tool 编排，不删除仍被 ingestion、evaluation 或其他业务使用的领域服务。
- 完成 trace/dashboard 和运维文档更新。

## 11. 验收标准

1. 一次普通问题可由 runtime 直接回答，且不调用任何能力。
2. 限定 KB 问题中，runtime 能自主调用 `retrieve_knowledge`，最终回答含可验证的知识库引用。
3. 无内部证据且策略允许时，runtime 可调用 `web_search`/`web_fetch`；策略禁止时绝不调用。
4. runtime 能在有用时调用记忆，且不会跨用户或跨 scope 读取。
5. 所有 capability 调用有 journal、trace、SSE 生命周期和可关联的 conversation/runtime ID。
6. approval pending 能跨请求恢复；拒绝、取消和 capability 失败均有确定终态，且不丢失已持久化对话。
7. context、外网、工具和总任务预算均可测试地生效。
8. runtime 路径的 citation、SSE、取消和持久化回归通过后，才允许扩大默认流量。

## 12. 明确不做

- 不将 `rag/tool` 与 `agent` registry 直接合并。
- 不在本次重构中引入 coding workspace/read/write/shell 工具。
- 不让 LLM 绕过 typed capability、source policy、KB scope 或 approval。
- 不以单个 `HistorySummary` 取代可追溯的 conversation history。
- 不在未完成 Slice 2/3 验收前删除 legacy RAG 路径。

## 13. Slice 0 已冻结的语义

- 新对话执行内核位于 `internal/app/runtime`，不得依赖未来会移除的 `internal/app/agent`。
- 每条已接纳的用户消息对应一个 runtime session，并以 `conversation_id`、`user_message_id`、`trace_id` 关联。
- chat 层只持久化用户消息和最终 assistant 消息；runtime 持久化自己的 append-only journal。
- runtime 是工具 catalog、模型回合、工具结算和模型可见上下文投影的唯一所有者。chat 层不预先编排 rewrite、retrieve、memory 或 web。
- 工具调用必须先写入 `pending`，再转为 `executing`，最终结算为 `completed`、`denied` 或 `failed`。服务恢复时未结算调用失败化，不重复执行。
- 空知识库 scope 表示禁止知识库检索，不表示全局可检索。
- 首版 runtime 只暴露无副作用工具；不实现 approval、checkpoint 或 approval resume。
- 浏览器断线不取消 runtime；只有显式停止才产生 `cancelled`。
- `degraded` 仅表示存在完整且可信、但受限的最终回答；模型流中断得到的半截文本属于 `failed` 的可选历史保留，不得伪装为正常或 degraded 回答。
- 标题生成、会话摘要和长期记忆写回不属于首版 runtime 主回合，作为回答完成后的独立后处理保留。
