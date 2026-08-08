# A1：RAG 聊天引用协议（Citation Protocol）设计

日期：2026-08-08
状态：已批准
范围：`rag` 聊天管线（不包含 `agent` 运行时路径）

## 背景

goagent 的 RAG 聊天管线目前只把检索片段以 `[N] (section) text` 文本形式注入 prompt，答案侧完全没有引用/溯源协议：模型无法标注"这句话出自哪条 chunk"，前端也没有来源悬浮/定位能力。真实 chunk ID 还会直接出现在模型可见的上下文中。

本项目从 WeKnora 的 `internal/modelcontext` 移植"临时句柄 + 引用协议"机制，解决三个问题：
1. 模型只接触低熵句柄（`cN`/`dN`/`bN`），真实 ID 永不进入模型可见输入。
2. 模型在答案中用 `<ref id="cN"/>` 内联标注来源，系统在流式输出时展开为公开 `<kb .../>` 标签。
3. 前端把 `<kb/>` 渲染为可点击的引用角标，点击展示来源 chunk 内容。

## 核心概念

- **句柄（handle）**：请求级临时标识，格式 `c1`/`d1`/`b1`（chunk/document/KB），永不持久化、不接受模型伪造。
- **引用注册表（Registry）**：每个聊天请求一个实例，维护 真实ID ↔ 句柄 的双向映射。
- **私有协议 → 公开标签**：模型输出 `<ref id="cN"/>`，系统展开为 `<kb doc="…" chunk_id="…" kb_id="…"/>` 持久化。
- **历史回放重压缩**：多轮对话中，历史 assistant 消息里的 `<kb/>` 折叠回 `<ref/>`，防止真实 ID 重新对模型可见。

## 架构

### 1. 新包 `internal/app/rag/core/citation/`

纯算法包，无 DB/HTTP/业务依赖，只依赖 `framework/convention`（`RetrievedChunk`）。

| 文件 | 职责 |
|---|---|
| `registry.go` | `Registry` 类型：`cN/dN/bN` 三张句柄表；`RegisterChunk(ChunkReference)`、`RegisterDocument(id)`、`RegisterKnowledgeBase(id)`、`RegisterChunks([]convention.RetrievedChunk)`；`ResolveChunk(handle)` 返回 `ChunkReference{ChunkID, DocumentID, KnowledgeBaseID, DocumentTitle}`；句柄按真实 ID 去重（同一 ID 恒定同句柄）；`ChunkReference` 元数据合并（首次非空优先） |
| `render.go` | `RenderKnowledgeContext(registry, chunks) string`：把 chunk 渲染为 `<retrieval type="knowledge"><document id="dN" title="…"><chunk id="cN" index="…" section="…">内容</chunk></document></retrieval>`；保留现有 section 元数据；无 chunk 返回空串 |
| `protocol.go` | `ProtocolPrompt(enabled bool) string`：系统协议文本（中英文混合，与 goagent 现有 prompt 语言风格一致）。`enabled=true` 时告知模型"用 `<ref id="cN"/>` 内联标注，禁止输出真实 ID、禁止自行输出 `<kb>`，规则优先于其他提示"；`enabled=false` 时告知"不要输出任何引用标记" |
| `expand.go` | `StreamExpander`（`Feed(chunk) string` + `Flush() string`，扣留尾部字节防止 provider 把 `<ref` 标签切碎泄漏）；`ExpandText(text) string`：`<ref id="cN"/>` → `<kb …/>`，未知句柄 fail-closed 消失，模型手写 `<kb>/<web>` 一律丢弃，`enabled=false` 时剥掉所有 `<ref>`；`ExpandText` 是 `StreamExpander` 的后端 |
| `compact.go` | `CompactPublicCitations(text) string`：把 `<kb/>`/`<web/>` 折叠回 `<ref id="handle"/>`，并注册对应 chunk（使历史引用在新请求内可解析） |

句柄正则：`^(c|d|b)[1-9][0-9]*$`。引用标签正则：`(?i)<ref\s+id\s*=\s*"([^"]+)"\s*/?>`。

### 2. prompt 协议注入

`internal/app/rag/core/prompt/`：
- `Context` 新增字段 `CitationProtocol string`。
- `BuildMessages` 在 `AnswerGuidance` 之后、History 之前插入 `formatCitationProtocol` 系统消息（空值不插入；agent 路径不设置该字段，零影响）。

### 3. rag 聊天服务集成

`internal/app/rag/service/chat/`：

**`prepareChat`（prepare_orchestrator.go）**：
1. 开头创建 `citation.Registry`（配置启用时），存入 `ragChatPreparedState.citation`。
2. `runMemoryStage` 之后：对 `memoryStage.history` 中 role=assistant 的消息执行 `registry.CompactPublicCitations`。
3. 检索成功后（`retrieveStage.used` 且 `Chunks` 非空）：`registry.RegisterChunks(chunks)`，并重写 `retrieveResult.KnowledgeContext = citation.RenderKnowledgeContext(registry, chunks)`。

**`service.go` 主流程**：
- 调用 `runPromptStage` 时把 `citation.ProtocolPrompt(enabled)` 作为新参数传入；`enabled = 配置启用 && retrievalUsed && retrieveResult.KnowledgeContext != ""`（回退分支会清零 KnowledgeContext，故此时自动禁用）。

**`runPromptStage`（execute_tool_workflow.go）**：
- 把 `CitationProtocol` 写进 `ragprompt.Context`。

**`runStreamingAnswer`（execute_orchestrator.go）→ `ragChatStreamCallback`（execute_streaming.go）**：
- `ragChatRuntimeState` 增加 `citation *citation.Registry` 字段，`prepareChat` 时赋值。
- `newRagChatStreamCallback` 增加参数 `expander *citation.StreamExpander`（nil 表示禁用，直接透传）。
- `OnContent`：先 `expander.Feed(content)` 再 `sink.SendMessage`；最终 `currentContent()` 返回展开后的完整内容，`buildTaskResult` 持久化即含 `<kb/>`。

**回退路径**：`applyFallbackGuard` 清零 `KnowledgeContext` 后，enabled 判定为 false，`runStreamingAnswer` 传 nil expander，协议消息不注入——模型不会产出 `<ref>`，历史压缩引入的 `<ref/>`（若有）在展开时被剥除。

### 4. 历史重压缩的数据流

```
持久化消息（含 <kb doc=… chunk_id=…/>）
  → runMemoryStage 加载
  → prepareChat: registry.CompactPublicCitations → <ref id="cN"/>（注册 chunk）
  → 进入 prompt（模型只见句柄）
  → 流式输出：expander 把模型 <ref id="cN"/> 展开为 <kb …/> 持久化
```

同一 chunk 在本轮检索和历史引用共享同一句柄，跨轮一致。

### 5. 前端

`frontend/`：
- `package.json` 新增 `rehype-raw`。
- `MarkdownRenderer.tsx`：加 `rehypePlugins={[rehypeRaw]}`、`components={{ kb: CitationChip }}`。
- 新增 `components/chat/CitationChip.tsx`：
  - 从 hast 节点属性读取 `doc`/`chunk_id`/`kb_id`。注意：`rehype-raw` 经 `parse5` 会小写化并保留下划线，属性名按原样读取；实现时对 `chunkId`/`kbId` 变体做防御性兼容（`props.chunk_id ?? props.chunkId`）。
  - 通过 `MessageItem` 提供的引用编号 Context 自动递增，渲染 `[1] [2]` 角标；Tooltip 悬浮显示文档标题。
  - 点击弹出 Popover（Radix Popover）：文档标题 + chunk 内容（从新端点拉取）。
- `MessageItem.tsx`：为单条 assistant 消息维护一个 `CitationNumberContext`（渲染 `<MarkdownRenderer>` 时提供）。

后端 chunk 详情端点：
- `internal/app/knowledge/service/chunk/knowledge_chunk_query_service.go`：`KnowledgeChunkService` 新增 `GetByID(ctx, chunkID)` 公开方法（复用 `chunkRepo.GetByID`，返回 `domain.KnowledgeChunk`）。
- `internal/adapter/http/knowledge/knowledge_chunk_handler.go`：新增路由 `GET /knowledge-base/chunks/:chunkId`，返回 `{ id, content, index, documentId, knowledgeBaseId }`。

### 6. 配置

- `internal/framework/config/config.go`：rag 根配置（`RagConfig`）新增 `CitationEnabled bool mapstructure:"citation-enabled"`。
- `configs/application.yaml`：`rag.citation-enabled: true`。
- bootstrap/装配层把 `CitationEnabled` 注入 `RagChatService`。

### 7. 明确不做（YAGNI）

- `agent` 运行时路径的引用协议（下一轮）。
- `<web>` 引用（web 搜索来源，本轮无）。
- 按知识库/会话粒度开关（本轮只做全局配置）。
- 引用抽屉（WeKnora 的 Drawer）——用 Popover 实现，减少工作面。
- `cN/dN/bN` 之外的其他句柄空间（resource/issue 等）。

## 错误处理与安全

- **fail-closed**：未知句柄展开时消失，不输出任何猜测内容。
- **句柄永不持久化**：持久化内容只含公开 `<kb/>` 标签，历史重压缩保证模型可见输入无真实 ID。
- **流式边界**：`StreamExpander` 扣留可能成为 `<ref`/`<kb`/`<web` 的尾部字节，`Flush` 时若结尾是待定标签前缀则丢弃，防止半截标签泄漏到 SSE。
- **模型伪造拒绝**：模型直接输出的 `<kb>`/`<web>` 标签在展开时被删除，公开标签只能由系统从注册表生成。
- **格式转义**：`doc`/`chunk_id` 属性值做 HTML 转义（`html.EscapeString`）。
- 引用协议开关关闭时：`ExpandText` 剥掉所有 `<ref>` 候选标签；协议 prompt 不注入。

## 测试策略

### citation 包单测（`internal/app/rag/core/citation/`）
- registry：注册去重（同一 ID 恒定句柄）、`ResolveChunk` 往返、未知句柄解析失败、元数据合并。
- render：`RenderKnowledgeContext` 输出结构、section 元数据、空 chunk 返回空。
- expand：`ExpandText` 展开、未知句柄消失、模型 `<kb>` 丢弃、`enabled=false` 剥 `<ref>`、`StreamExpander` 把 `<ref` 分片喂入仍能正确展开、Flush 丢弃半截标签。
- compact：`<kb/>` → `<ref/>` 往返、注册后同句柄复用。

### rag 聊天服务级测试（`internal/app/rag/service/chat/`）
- 检索成功后 KnowledgeContext 为 XML 且含句柄。
- prompt 注入协议消息（enabled=true/false 两分支）。
- 流回调 `OnContent` 展开 `<ref/>`，`buildTaskResult.content` 含 `<kb/>`。
- 低置信度回退：enabled=false、expander=nil、无协议消息。
- 历史消息 `<kb/>` 被重压缩为 `<ref/>`。

### 前端
- `npm run lint`、`npm run build`。

## 回归约束

- 不改 `BuildKnowledgeContext`（agent 路径继续使用）。
- 不改数据库迁移。
- `ragprompt.Context.CitationProtocol` 默认空值，agent 路径行为不变。
- 引用协议是增量行为：`citation-enabled: false` 时系统完全退化为当前行为。
