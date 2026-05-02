# Chat 全链路说明

本文档说明当前 `chat` 功能从前端发起请求，到后端完成 RAG 编排、调用大模型流式接口、再把流式结果推回前端的完整链路。重点覆盖两部分：

- 流式调用过程：后端如何向模型发起流式请求，如何接收流式输出，如何通过 SSE 推给前端，取消句柄如何生效。
- 会话历史持久化与上下文搭建：会话、消息、摘要如何落库，模型上下文如何组装。

## 1. 整体入口

当前最小聊天闭环的主入口分成三层：

- 前端状态与请求发起：`frontend/src/stores/chatStore.ts`
- 前端 SSE 读取：`frontend/src/hooks/useStreamResponse.ts`
- 后端 HTTP 与 RAG 编排：`internal/adapter/http/rag/handlers.go` 和 `internal/app/rag/service/rag_chat_service.go`

主调用顺序可以先概括为：

1. 前端 `sendMessage()` 组装请求参数。
2. 前端通过 `createStreamResponse().start()` 发起 `GET /rag/v3/chat`。
3. 后端 `Handler.Chat()` 解析参数，创建 SSE sink，调用 `RagChatService.Chat()`。
4. `RagChatService.Chat()` 依次完成会话准备、历史加载、用户消息持久化、检索、Prompt 组装、LLM 流式调用。
5. 模型流式输出经 `StreamCallback` 回调进入 `ragChatStreamCallback`。
6. `ragChatStreamCallback` 一边累计完整回复，一边通过 `sseChatSink` 推送增量给前端。
7. 前端 SSE reader 按事件类型更新 store 中的消息、会话和流式状态。
8. 流完成、失败或取消后，后端统一收口：落 assistant 消息、更新 trace、发 `finish/cancel/error/done` 事件。

## 2. 前端请求发起链路

### 2.1 `sendMessage()`

文件：`frontend/src/stores/chatStore.ts`

主入口函数：`sendMessage(content)`

这个函数内部做了下面几件事：

1. 校验输入，过滤空消息。
2. 如果当前已经在流式生成，直接返回，避免并发发送。
3. 先在本地消息列表里插入两条消息：
   - 一条用户消息 `userMessage`
   - 一条占位 assistant 消息 `assistantMessage`
4. 根据当前状态决定这次请求是否要新建会话：
   - `snapshot.isCreatingNew && snapshot.messages.length === 0` 时，`conversationId` 为空，表示首轮新建
   - 否则优先复用 `currentSessionId -> lastResolvedSessionId -> sessions[0]?.id`
5. 调用 `buildQuery(...)` 组装查询参数：
   - `question`
   - `conversationId`
   - `deepThinking`
6. 调用 `createStreamResponse(...)` 创建一个带 `start/cancel` 的流式请求对象。
7. 执行 `await start()` 真正发起 SSE 请求。

### 2.2 前端 SSE 事件处理

`sendMessage()` 同时注册了一组 handlers：

- `onMeta`
- `onMessage`
- `onThinking`
- `onFinish`
- `onCancel`
- `onDone`
- `onTitle`
- `onError`

这些 handler 的职责如下：

- `onMeta`
  - 接收后端发回的 `conversationId` 和 `taskId`
  - 把当前会话 ID 固定下来
  - 把当前流式任务 ID 写入 `streamTaskId`
  - 如果用户已经点过停止，会立即调用 `stopTask(taskId)`
- `onMessage`
  - 处理普通回复增量，调用 `appendStreamContent(delta)`
- `onThinking`
  - 处理思考增量，调用 `appendThinkingContent(delta)`
- `onFinish`
  - 把占位 assistant 消息收口成正式消息
  - 用后端返回的 `messageId` 替换本地临时 ID
  - 更新会话标题和 `lastTime`
- `onCancel`
  - 把当前 assistant 消息标记为 `cancelled`
  - 如果后端已经持久化了部分输出，也会用真实 `messageId` 替换本地 ID
- `onDone`
  - 结束前端流式状态，清理 `streamTaskId/streamAbort/streamingMessageId`
- `onError`
  - 把当前 assistant 消息标记为失败态

## 3. 前端如何读取 SSE

文件：`frontend/src/hooks/useStreamResponse.ts`

### 3.1 `createStreamResponse()`

`createStreamResponse(options, handlers)` 返回两个函数：

- `start()`
- `cancel()`

其中：

- `start()` 调用 `streamWithRetry(...)`
- `cancel()` 调用内部 `AbortController.abort()`

这只是取消浏览器侧的 HTTP 读取，不等于后端停止模型生成。真正让后端停下来，要靠单独的 `/rag/v3/stop`。

### 3.2 `streamWithRetry()`

`streamWithRetry()` 使用 `fetch(url, { method: "GET", headers: { Accept: "text/event-stream" }})` 发起请求，然后调用 `readSseStream(response, handlers, signal)`。

如果请求失败，会按 `retryCount` 和指数退避做重试。

### 3.3 `readSseStream()`

`readSseStream()` 是前端 SSE 解析器，内部做法是：

1. 通过 `response.body.getReader()` 获取字节流 reader。
2. 用 `TextDecoder` 持续把字节解码成文本。
3. 按 SSE 协议解析：
   - `event: xxx`
   - `data: yyy`
   - 空行表示一条事件结束
4. 把 `data:` 内容拼起来后，调用 `parseData(raw)` 尝试 JSON 解析。
5. 根据事件名分发：
   - `meta`
   - `message`
   - `finish`
   - `done`
   - `cancel`
   - `title`
   - `error`

这里要注意一点：

- 后端把“思考增量”和“回复增量”都用 `event: message`
- 前端再通过 payload 里的 `type` 区分：
  - `type: "think"`
  - `type: "response"`

## 4. 后端 HTTP 入口

文件：`internal/adapter/http/rag/handlers.go`

### 4.1 `RegisterRoutes()`

聊天相关路由注册如下：

- `GET /rag/v3/chat`
- `POST /rag/v3/stop`

### 4.2 `Handler.Chat()`

`Handler.Chat(c *gin.Context)` 是后端聊天入口，内部流程如下：

1. 调用 `requireLoginUser(c)` 获取登录用户。
2. 创建 `sender := fwweb.NewSseEmitterSender(c)`。
3. 创建 `sink := &sseChatSink{sender: sender}`。
4. 组装 `ragservice.RagChatInput`：
   - `ConversationID`
   - `UserID`
   - `Question`
   - `KnowledgeBaseIDs`
   - `DeepThinking`
5. 调用 `h.chatService.Chat(c.Request.Context(), input, sink)`。

这里 `Handler.Chat()` 自己不做任何编排，只负责：

- 参数提取
- SSE 输出通道建立
- 调用 service

### 4.3 `sseChatSink`

`sseChatSink` 是 service 层到 SSE 协议层的适配器。它把 service 的抽象事件映射为具体 SSE 事件：

- `SendMeta(meta)` -> `event: meta`
- `SendThinking(delta)` -> `event: message`, body `{type:"think",delta}`
- `SendMessage(delta)` -> `event: message`, body `{type:"response",delta}`
- `SendTitle(title)` -> `event: title`
- `SendFinish(payload)` -> `event: finish`
- `SendCancel(payload)` -> `event: cancel`
- `SendError(err)` -> `event: error`
- `SendDone()` -> `event: done`，然后 `sender.Complete()`

这样 `RagChatService` 不依赖 Gin，也不直接写 HTTP。

## 5. 后端 SSE 发送器

文件：`internal/framework/web/sse_emitter_sender.go`

### 5.1 `NewSseEmitterSender()`

这个函数负责初始化 SSE 响应：

- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`
- `X-Accel-Buffering: no`

同时它启动一个 goroutine 监听 `c.Request.Context().Done()`。如果客户端断开连接，会把 sender 标记为 `closed`。

### 5.2 `SendEvent()`

`SendEvent(eventName, data)` 的行为是：

1. 如果连接已关闭，直接返回。
2. 先写 `event: xxx\n`
3. 再把 `data` 序列化成 JSON，写成 `data: {...}\n\n`
4. 调用 `Flush()` 强制把缓冲区刷给前端

这一步就是“后端把收到的模型增量，真正实时推给前端”的底层动作。

## 6. 后端聊天编排主流程

文件：`internal/app/rag/service/rag_chat_service.go`

### 6.1 `Chat()`

`Chat(ctx, input, sink)` 是整个最小 RAG 闭环的编排入口。

它的执行顺序如下：

1. `validateDependencies()` 检查服务依赖是否齐全。
2. 校验 `question` 和 `userID`。
3. 调用 `prepareChat(ctx, input)` 完成前置阶段。
4. `sink.SendMeta(state.meta)` 把 `conversationId` 和 `taskId` 先发给前端。
5. 如果已有标题，`sink.SendTitle(state.title)`。
6. 调用 `runPromptStage(...)` 组装最终模型 messages。
7. 创建 `task := newTask()`。
8. `setTaskHandle(taskID, task, nil)` 先把任务挂到内存任务表。
9. 构造 `convention.ChatRequest`。
10. 创建 `callback := newRagChatStreamCallback(task, sink)`。
11. 调用 `s.chatService.StreamChatWithRequest(request, callback)` 向模型发起流式请求。
12. 拿到 `handle` 后，再调用 `setTaskHandle(taskID, task, handle)`，把取消句柄挂进去。
13. 阻塞等待 `result := <-task.doneCh`。
14. 根据结果分别走：
   - `handleCancelledResult(...)`
   - `handleFailedResult(...)`
   - `handleSucceededResult(...)`

`Chat()` 本身只负责编排，不直接处理具体的 repository 细节。

## 7. `prepareChat()` 阶段拆分

`prepareChat()` 现在被显式拆成多个阶段，顺序如下。

### 7.1 `runConversationStage()`

职责：

- 如果前端没传 `conversationId`，生成新的会话 ID
- 调用 `conversationService.CreateOrUpdate(...)`

`CreateOrUpdate(...)` 的行为是：

- 会话不存在：创建 `t_conversation`
- 会话已存在：只刷新 `LastTime`

当会话首次创建时，`ConversationService.generateTitle(...)` 会尝试用 LLM 生成标题；如果失败，就退回到基于首轮问题的截断标题。

### 7.2 `runMemoryStage()`

职责：

- 调用 `memoryService.Load(conversationID, userID)` 读取历史上下文

这里的 `memoryService` 当前实现是 `DefaultService`，内部又会做两步：

1. `store.LoadHistory(...)`
2. `summary.LoadLatestSummary(...)`

也就是说，模型上下文里的历史消息来自：

- 最近 N 条消息历史
- 可选的一条最近摘要

### 7.3 `runUserMessageStage()`

职责：

- 调用 `messageService.AddMessage(...)`
- 把当前用户问题持久化到 `t_message`

这里特意采用的是“先取历史，再落当前用户消息”的顺序。这样可以避免当前问题同时出现在：

- `history`
- `question`

从而导致 Prompt 里重复。

### 7.4 `runRuntimeStage()`

职责：

- 生成 `traceID`
- 生成 `taskID`
- 组装 `RagChatMeta`
- 调用 `startTraceRun(...)`

这一步完成后，前端才会收到 `meta` 事件。

### 7.5 `runRetrieveStage()`

职责：

- 调用 `retrieveService.Retrieve(...)`
- 生成知识检索结果
- 记录 retrieve 阶段 trace

`retrieveService.Retrieve(...)` 的内部步骤是：

1. 对用户问题做 `embedding.Embed(query)`
2. 调用向量搜索器 `searcher.Search(...)`
3. 可选 rerank
4. 调用 `BuildKnowledgeContext(chunks)` 把召回 chunk 拼成文本上下文

最终会得到：

- `Chunks`
- `KnowledgeContext`

## 8. Prompt 如何搭建

### 8.1 `runPromptStage()`

`runPromptStage()` 会调用 `promptService.BuildMessages(...)`。

### 8.2 `promptService.BuildMessages()`

文件：`internal/app/rag/core/prompt/service.go`

它会按固定顺序构造模型输入：

1. 系统提示词 `system prompt`
2. 知识上下文 `KnowledgeContext`
3. 历史消息 `History`
4. 当前用户问题 `Question`

最终 messages 的结构是：

1. `SystemMessage(system prompt)`
2. `SystemMessage("## 知识上下文\n...")`
3. `history...`
4. `UserMessage(question)`

也就是说，当前最小闭环里的上下文是“系统规则 + 检索知识 + 历史会话 + 当前问题”。

## 9. 大模型流式调用的后端链路

这一段是整个系统的核心。

### 9.1 `RagChatService.Chat()` 调用 `LLMService.StreamChatWithRequest()`

在 prompt messages 组装完成后，`Chat()` 会构造：

- `Messages`
- `Thinking`
- 其他推理开关

然后调用：

`s.chatService.StreamChatWithRequest(request, callback)`

这里的 `s.chatService` 是 AI 层的 `LLMService`。

### 9.2 `RoutingLLmService.StreamChatWithRequest()`

文件：`internal/infra-ai/chat/routing_llm_service.go`

这个函数做了三件重要的事：

1. 选模型
   - `selector.SelectChatCandidates(request.ThinkingEnabled())`
2. 逐个 provider/client 尝试流式调用
3. 用 `ProbeStreamBridge` 等待“首包探测成功”后，才认为这次流真正启动成功

关键步骤如下：

1. 选出候选模型 `targets`
2. 对每个 `target`：
   - `resolveClient(target)` 找到对应 provider client
   - 创建 `bridge := NewProbeStreamBridge(callback)`
   - 调用 `client.StreamChat(request, bridge, target)`
   - 得到一个 `handle`
   - 调用 `bridge.AwaitFirstPacket(firstPacketTimeout)`
3. 如果在超时时间内收到了第一段内容或 thinking 增量：
   - `ProbeResultSuccess`
   - 标记 provider 健康
   - 返回 `handle`
4. 如果没有首包、报错或者超时：
   - 调用 `handle.Cancel()`
   - 换下一个模型/provider 继续尝试

这一步的意义是：只有真正收到首包，后端才把这次流式调用视为“启动成功”。

### 9.3 `ProbeStreamBridge` 的作用

文件：`internal/infra-ai/chat/probe_stream_bridge.go`

`ProbeStreamBridge` 是一个“首包探测 + 缓冲转发”桥接器。

它的行为是：

- `OnContent()` 或 `OnThinking()` 到来时：
  - 把 probe 结果记为成功
  - 在正式提交前，先把回调动作缓存在 `buffer`
- `AwaitFirstPacket()` 收到成功结果后：
  - 调用 `commit()`
  - 把之前缓存的增量一次性转发给下游 callback
- 如果最先收到的是 `OnComplete()` 且没有任何内容：
  - 返回 `ProbeResultNoContent`
- 如果最先收到的是 `OnError(err)`：
  - 返回 `ProbeResultError`

因此，真正的业务 callback 在“首包确认之前”并不会立刻收到内容。

### 9.4 `OpenAIStyleChatClient.StreamChat()`

文件：`internal/infra-ai/chat/openai_style_chat_client.go`

这是当前模型流式请求的具体 HTTP 客户端实现。它的流程如下：

1. 校验请求和模型配置
2. `marshalRequestBody(req, target, true)`，把 `stream=true` 写入请求体
3. `context.WithCancel(context.Background())`
4. `newRequest(ctx, target, body, aihttp.MediaTypeSSE)`，构造 HTTP 请求
5. 创建 `handle := &cancellableStreamHandle{cancel: cancel}`
6. 启动 goroutine：`go op.doStream(httpReq, callback, req)`
7. 立即把 `handle` 返回给上层

这就是取消句柄的来源。句柄内部只是包了一层 `context.CancelFunc`。

### 9.5 `cancellableStreamHandle.Cancel()`

`Cancel()` 的实现很简单：

- 调用 `context.CancelFunc`

一旦调用：

- 请求上下文会被取消
- 底层 HTTP streaming 请求会尽快中断
- `doStream()` 会停止继续读取模型响应

### 9.6 `OpenAIStyleChatClient.doStream()`

`doStream()` 是真正消费模型 SSE 的地方，步骤如下：

1. `client.Do(httpReq)` 发起流式 HTTP 请求
2. 校验 HTTP 响应状态
3. `reader := bufio.NewReader(resp.Body)`
4. 循环 `reader.ReadString('\n')` 按行读取上游模型的 SSE 流
5. 每读到一行，调用 `parseStreamLine(line, reasoningEnabled)`
6. `parseStreamLine()` 默认走 `ParseOpenAIStyleSseLine(...)`
7. 解析出 `ParsedEvent` 后：
   - 有 `Reasoning` -> `callback.OnThinking(...)`
   - 有 `Content` -> `callback.OnContent(...)`
   - `Completed` -> `callback.OnComplete()`

如果响应异常结束、解析失败或请求失败，则调用 `callback.OnError(err)`。

### 9.7 `ParseOpenAIStyleSseLine()`

文件：`internal/infra-ai/chat/openai_style_sse_parser.go`

这个解析器负责把模型厂商返回的 SSE 文本行转成统一事件：

- 去掉 `data:` 前缀
- 识别 `[DONE]`
- JSON 反序列化成 `openAIStyleSsePayload`
- 从 `choices[0]` 中提取：
  - `content`
  - `reasoning_content`
  - `finish_reason`

产物是统一的 `ParsedEvent`：

- `Content`
- `Reasoning`
- `Completed`

## 10. 模型流输出如何再推给前端

### 10.1 `newRagChatStreamCallback()`

文件：`internal/app/rag/service/rag_chat_service.go`

`RagChatService` 把传给 LLM 层的 callback 封装成 `ragChatStreamCallback`。这个 callback 一边接收模型增量，一边做两件事：

- 累积完整回复内容
- 通过 `sink` 把增量继续推到前端

### 10.2 `ragChatStreamCallback.OnThinking()`

行为：

1. 把 thinking 增量 append 到内部 builder
2. 调用 `sink.SendThinking(delta)`

结果：

- 前端收到 `event: message`
- payload 为 `{type:"think",delta:"..."}`

### 10.3 `ragChatStreamCallback.OnContent()`

行为：

1. 把回复增量 append 到内容 builder
2. 调用 `sink.SendMessage(delta)`

结果：

- 前端收到 `event: message`
- payload 为 `{type:"response",delta:"..."}`

### 10.4 `ragChatStreamCallback.OnComplete()`

行为：

- 把当前累计的 `content/thinking` 写入 `task.doneCh`

这时 `RagChatService.Chat()` 从 `<-task.doneCh` 取到结果，进入成功收口。

### 10.5 `ragChatStreamCallback.OnError()`

行为：

- 把错误写入 `task.doneCh`

这时 `Chat()` 会进入失败收口。

## 11. 取消句柄如何起作用

取消流程分成前端、后端 service、底层模型流三层。

### 11.1 前端触发取消

前端调用 `cancelGeneration()`：

1. 把 `cancelRequested` 设为 `true`
2. 如果已经拿到了 `streamTaskId`
3. 调用 `stopTask(streamTaskId)`

这里的 `stopTask()` 会调用后端：

- `POST /rag/v3/stop?taskId=...`

### 11.2 后端 `StopChat()`

`Handler.StopChat()` 做两件事：

1. 读取 `taskId`
2. 调用 `h.chatService.CancelTask(taskID)`

### 11.3 `RagChatService.CancelTask()`

`CancelTask(taskID)` 内部会：

1. 从 `tasks` map 里找到对应 `ragChatTask`
2. 关闭 `task.cancelCh`
3. 如果 `task.handle != nil`，调用 `task.handle.Cancel()`

这两步分别负责：

- `cancelCh`：通知业务层当前任务被取消
- `handle.Cancel()`：通知底层模型 HTTP stream 停止

### 11.4 `watchCancel()`

`ragChatStreamCallback` 内部有一个取消监听 goroutine，一直等 `cancelCh`。

一旦 `cancelCh` 被关闭，它会向 `task.doneCh` 写入一个“已取消”的结果。

这样即使底层模型流还没来得及自然结束，`Chat()` 也能尽快进入取消收口。

### 11.5 取消后的收口

`Chat()` 收到取消结果后，会进入 `handleCancelledResult(...)`：

1. 记录 chat trace node
2. 如果已经积累了部分 assistant 输出，调用 `persistAssistantMessage(...)` 落库
3. `finishTraceRun(..., cancelled)`
4. `sink.SendCancel(...)`
5. `sink.SendDone()`

所以取消不是“直接丢弃”，而是尽量保留已经生成的内容。

## 12. 会话和消息如何持久化

### 12.1 会话持久化

文件：`internal/app/rag/service/conversation_service.go`

核心函数：`CreateOrUpdate(...)`

逻辑如下：

- 根据 `conversationId + userId` 查询 `t_conversation`
- 不存在：
  - 新建一条会话记录
  - 生成标题
  - 初始化 `CreateTime/UpdateTime/LastTime`
- 已存在：
  - 只更新 `LastTime` 和 `UpdateTime`

这意味着：

- 首轮消息创建会话
- 每轮成功或取消后，后端都会刷新该会话的活跃时间

### 12.2 用户消息持久化

文件：`internal/app/rag/service/conversation_message_service.go`

核心函数：`AddMessage(...)`

用户消息在 `runUserMessageStage()` 中落库，保存字段包括：

- `ConversationID`
- `UserID`
- `Role`
- `Content`
- `ThinkingContent`
- `ThinkingDuration`
- `CreateTime`
- `UpdateTime`

### 12.3 assistant 消息持久化

文件：`internal/app/rag/service/rag_chat_service.go`

核心函数：`persistAssistantMessage(...)`

这个函数在成功收口或取消收口里被调用，做两件事：

1. `messageService.AddMessage(...)`
   - 保存 assistant 文本
   - 保存 `ThinkingContent`
   - 保存 `ThinkingDuration`
2. `conversationService.CreateOrUpdate(... LastTime: now)`
   - 刷新会话最后活跃时间

因此，一轮完整对话最终会至少新增两条消息：

- 一条 user
- 一条 assistant

## 13. 历史上下文是怎么搭起来的

### 13.1 `memoryService.Load()`

文件：`internal/app/rag/core/memory/default_service.go`

`Load(conversationID, userID)` 的顺序是：

1. 从 store 读取最近历史
2. 如果有摘要服务，再取最近摘要
3. 把摘要插到历史最前面

返回值是一个可直接给 prompt 层使用的 `[]convention.ChatMessage`。

### 13.2 `MessageServiceStore.LoadHistory()`

文件：`internal/app/rag/core/memory/service_store.go`

内部步骤如下：

1. 先校验该会话确实属于当前用户
2. 查询最近消息，按 `DESC` 取最近 N 条
3. 在内存里倒序回正序
4. 转成 `convention.ChatMessage`
5. `normalizeHistory(...)`

所以模型拿到的历史顺序是时间正序。

### 13.3 摘要如何接入

`SummaryServiceAdapter.LoadLatestSummary()` 会从摘要表读取最近一条摘要，转成 `SystemMessage`。

`DecorateIfNeeded()` 会给它补统一前缀：

- `对话摘要：...`

这样摘要被视为模型上下文里的系统信息，而不是普通用户消息。

### 13.4 当前最小版本的上下文结构

最终给模型的上下文顺序是：

1. 系统 Prompt
2. 知识上下文
3. 对话摘要，如果存在
4. 最近历史消息
5. 当前用户问题

## 14. 一次成功请求的完整时序

下面用顺序列表把一次成功请求串起来。

1. 前端 `sendMessage()` 决定 `conversationId`
2. 前端 `fetch(GET /rag/v3/chat?... )`
3. 后端 `Handler.Chat()`
4. 后端创建 `SseEmitterSender`
5. 后端创建 `sseChatSink`
6. 后端调用 `RagChatService.Chat()`
7. `runConversationStage()`
8. `runMemoryStage()`
9. `runUserMessageStage()`
10. `runRuntimeStage()`
11. `runRetrieveStage()`
12. `sink.SendMeta(meta)`
13. `sink.SendTitle(title)`，如果有
14. `runPromptStage()`
15. `LLMService.StreamChatWithRequest()`
16. `RoutingLLmService.StreamChatWithRequest()`
17. `OpenAIStyleChatClient.StreamChat()`
18. `OpenAIStyleChatClient.doStream()` 持续读取模型 SSE
19. 每段 thinking -> `ragChatStreamCallback.OnThinking()`
20. 每段 content -> `ragChatStreamCallback.OnContent()`
21. `sseChatSink.SendThinking/SendMessage()`
22. `SseEmitterSender.SendEvent()` + `Flush()`
23. 前端 `readSseStream()` 读取到事件
24. 前端更新 assistant 占位消息内容
25. 模型结束 -> `OnComplete()`
26. `Chat()` 进入成功收口
27. `persistAssistantMessage()`
28. `finishTraceRun(success)`
29. `sink.SendFinish(...)`
30. `sink.SendDone()`
31. 前端 `onFinish()` 和 `onDone()` 收口本轮状态

## 15. 当前实现的关键设计点

### 15.1 service 层不直接依赖 HTTP

`RagChatService` 只依赖 `RagChatEventSink`，不依赖 Gin 和 SSE 协议细节。这让后续改成 WebSocket 或别的传输方式时，核心编排可以保留。

### 15.2 取消是双通道生效

当前取消同时作用在两层：

- 业务层：`cancelCh`
- 模型流层：`handle.Cancel()`

这样既能尽快让编排层退出，也能尽快停止底层 HTTP streaming。

### 15.3 历史上下文和当前问题被刻意分开

当前流程是：

- 先读历史
- 再落当前用户消息
- 最后在 prompt 中单独追加当前问题

这样避免了当前问题被重复拼进 Prompt。

### 15.4 assistant 消息在收口阶段持久化

assistant 回复不是流一开始就落库，而是在：

- 成功完成
- 或取消后已有部分内容

时统一落库。这样库里保存的是完整或可展示的结果，而不是一堆中间碎片。

## 16. 当前边界与后续演进点

当前实现已经完成最小闭环，但仍然保留了后续扩展空间：

- 前置阶段已经显式拆分，后面可以继续演进成统一 stage runner
- trace 已经具备最小 run/node 结构，后续可以把更多阶段纳入观测
- memory 摘要接口已预留，但摘要压缩当前还未启用
- 流式层是 OpenAI 风格协议抽象，后续可继续挂接更多 provider

如果后续要做 pipeline 化或更复杂的 agent 编排，最值得复用的骨架就是：

- `prepareChat` 的阶段化组织
- `RagChatEventSink` 的输出抽象
- `StreamCancellationHandle` 的取消约定
- `ProbeStreamBridge` 的首包探测机制
