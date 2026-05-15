# RAG / Agent 总入口与主链路

更新时间：2026-05-14

## 概览

这个模块是整个 `goagent` 聊天能力的总入口。无论外部看到的是“问答系统”“RAG 对话”还是“带 Agent 能力的聊天”，最终都要落到这里。

从架构上说，它的核心不是“某一个算法”，而是一个总编排器。这个编排器负责把一次用户提问拆成若干业务阶段，然后按顺序推进：

1. 识别或创建会话
2. 读取历史记忆
3. 保存用户消息
4. 初始化本次请求运行态
5. 改写问题
6. 检索知识
7. 必要时运行工具工作流
8. 组装 prompt
9. 调用流式大模型输出
10. 保存 assistant 消息并收尾

一句话概括：`RagChatService` 不是一个“调用 LLM 的服务”，它是一个“把会话、记忆、检索、工具、流式输出组织成完整回答链路的编排服务”。

## 这个模块在整个系统里的位置

对外入口在 HTTP 层：

- 文件：`internal/adapter/http/rag/handlers.go`
- 入口：`GET /rag/v3/chat`

HTTP 层只做比较薄的事情：

- 从 query 中取参数
- 识别当前登录用户
- 建立 SSE 输出器
- 调用 `RagChatService.Chat(...)`

真正的主链路在：

- 文件：`internal/app/rag/service/rag_chat_service.go`
- 类型：`type RagChatService struct`
- 主入口：`func (s *RagChatService) Chat(...)`

所以如果你要向别人解释这个系统，最准确的说法不是“RAG 在 handler 里处理”，而是：

- handler 是接入口
- `RagChatService` 是总编排层
- rewrite / retrieve / tool / prompt / chat 分别是它调度的阶段

## 功能

这个模块承担 6 类核心职责。

### 1. 承接一轮聊天请求

它会接收如下输入：

- `ConversationID`
- `UserID`
- `Question`
- `KnowledgeBaseIDs`
- `DeepThinking`
- `SearchMode`

对应结构：

- `RagChatInput`

这些字段决定了一轮聊天要查哪个会话、用哪个知识库、是否开启深度思考、是否带检索模式偏好等。

### 2. 把请求拆成多个业务阶段

在 `prepareChat(...)` 里，这个服务会顺序执行：

- `runConversationStage(...)`
- `runMemoryStage(...)`
- `runUserMessageStage(...)`
- `runRuntimeStage(...)`
- `runRewriteStage(...)`
- `runRetrieveStage(...)`

这里非常重要的一点是：真正的聊天不是一个大函数一次性做完，而是被明确拆成“阶段”。这样做的好处有三个：

- 方便 trace 落点
- 方便阶段失败时单独定位
- 方便后续插入新阶段，比如 tool workflow、fallback、更多 guardrail

### 3. 决定这次回答是否只依赖 RAG，还是需要 Agent 介入

在 `Chat(...)` 里，retrieve 之后还会调用：

- `runToolWorkflowStage(...)`

如果 `toolWorkflow` 没注入，那这一步就是空操作。

如果注入了，就会进入工具工作流。工具工作流会看到：

- 原始问题
- 用户 ID
- 会话 ID
- trace ID
- 知识库 ID
- 历史对话
- rewrite 结果
- retrieve 结果

也就是说，Agent 并不是脱离 RAG 独立工作的，而是建立在已有会话上下文和检索结果之上的增强层。

### 4. 在低置信度时触发 fallback

`Chat(...)` 里还有一个很实用的逻辑：

- 如果设置了 `confidenceThreshold`
- 且当前 retrieve 的最高分低于阈值

那么系统会：

- 发送 `fallback` SSE 事件
- 清空 `retrieveResult.KnowledgeContext`
- 构造一个 fallback prompt
- 在 trace 中记录这是一次低置信度回退

这代表系统不是盲目相信知识库召回结果，而是愿意在“召回证据不可靠”时退回到通用回答模式。

### 5. 组装 prompt 并发起真正的流式回答

当 retrieve 和 tool workflow 都完成后，系统会进入 `runPromptStage(...)`。

这个阶段会把下面几类信息组装成最终 prompt：

- 用户问题
- 历史消息
- 检索到的知识上下文
- 工具工作流产出的 `ToolContext`
- 工作流控制信息 `WorkflowPolicy`
- 回答引导 `AnswerGuidance`
- fallback prompt

然后再把这些消息传给：

- `chatService.StreamChatWithRequest(...)`

这里才是真正调用底层大模型进行流式输出。

### 6. 处理成功、失败、取消三种收尾路径

流式调用结束后，系统不会简单返回，而是进入不同的收尾函数：

- 成功：`handleSucceededResult(...)`
- 失败：`handleFailedResult(...)`
- 取消：`handleCancelledResult(...)`

成功和取消都会尝试把已经生成的 content / thinking 保存成 assistant 消息；失败则记录错误并结束 trace。

## 核心代码

如果要读代码，建议按下面顺序看。

### 1. HTTP 入口

- 文件：`internal/adapter/http/rag/handlers.go`
- 函数：`func (h *Handler) Chat(...)`

可以先看它如何：

- 从 query 拿到 `conversationId / question / knowledgeBaseId / deepThinking / searchMode`
- 创建 `SseEmitterSender`
- 包装成 `sseChatSink`
- 调用 `h.chatService.Chat(...)`

### 2. 主服务结构

- 文件：`internal/app/rag/service/rag_chat_service.go`
- 结构：`type RagChatService struct`

重点看它依赖了哪些子服务：

- `conversationService`
- `messageService`
- `memoryService`
- `rewriteService`
- `retrieveService`
- `promptService`
- `chatService`
- `tracer`
- `toolWorkflow`
- `taskRegistry`

这基本就把整个聊天系统的骨架暴露出来了。

### 3. 主入口函数

- `func (s *RagChatService) Chat(...)`

这一个函数是理解全链路最关键的代码。建议看它时按这个顺序理解：

1. 依赖校验
2. 输入合法性校验
3. `prepareChat(...)`
4. `SendMeta(...)`
5. fallback 判断
6. `runToolWorkflowStage(...)`
7. `runPromptStage(...)`
8. `StreamChatWithRequest(...)`
9. 等待 `task.doneCh`
10. 走成功 / 失败 / 取消收尾

### 4. 预处理阶段总入口

- `func (s *RagChatService) prepareChat(...)`

这是一个很值得背的函数，因为它体现了系统的阶段划分思想。它本身不做复杂业务，而是把前半段链路拆成明确步骤。

### 5. 核心阶段函数

- `runConversationStage(...)`
- `runMemoryStage(...)`
- `runUserMessageStage(...)`
- `runRuntimeStage(...)`
- `runRewriteStage(...)`
- `runRetrieveStage(...)`
- `runToolWorkflowStage(...)`

特别值得注意的两个函数：

- `runRewriteStage(...)`
  - 如果没有 rewriteService，就退化为“原问题即改写结果”
- `runRetrieveStage(...)`
  - 会按多个 `subQuestion` 分别调用 retrieve
  - 然后使用 `ragretrieve.MergeResults(...)` 合并

## 主链路是怎么跑起来的

下面用“用户发起一次聊天”这个视角，把阶段关系说清楚。

### 第一步：会话准备

`runConversationStage(...)` 会做两件事：

- 如果前端没传 `ConversationID`，就先生成一个新的 conversation id
- 然后调用 `conversationService.CreateOrUpdate(...)`

这一步的重点不是“创建会话记录”本身，而是把本轮请求绑定到某个会话语义上。后面的 memory、消息保存、trace、标题更新，都是围绕这个 conversation 展开的。

### 第二步：历史记忆加载

`runMemoryStage(...)` 调用：

- `memoryService.Load(ctx, conversationID, userID)`

这一步会返回历史消息列表，供 rewrite 和最终 prompt 使用。

所以系统不是把用户当前问题当作孤立输入，而是会尽量把历史上下文带进来。

### 第三步：先保存用户消息

`runUserMessageStage(...)` 会先把用户这次提问落库。

这一步有两个意义：

- 会话记录完整，不会只记录 assistant 回复
- 即使后续回答失败，也能知道用户当时问了什么

### 第四步：初始化本次运行态

`runRuntimeStage(...)` 会生成两类标识：

- `traceID`
- `taskID`

然后初始化：

- `RagChatMeta`
- 标题
- 用户消息 id
- 开始时间

同时通过 `ChatTracer.startTraceRunAt(...)` 创建 trace run。

这一步把“业务请求”和“系统运行实例”区分开了：

- 会话是长生命周期概念
- task / trace 是单次请求级概念

### 第五步：rewrite

`runRewriteStage(...)` 的目标不是生成最终回答，而是把用户问题变成更适合检索的形式。

它会产生：

- `RewrittenQuestion`
- `SubQuestions`
- `PreferredSearchMode`

这里最关键的点是 `SubQuestions`。因为后续 retrieve 不是只查一个问题，而是可以按子问题分拆召回。

### 第六步：retrieve

`runRetrieveStage(...)` 的逻辑非常值得掌握：

1. 先拿 rewrite 产生的 `SubQuestions`
2. 如果没有，就退回原始问题
3. 决定最终 `searchMode`
4. 对每个子问题单独调用 `retrieveService.Retrieve(...)`
5. 如果所有子问题都失败，再退回原问题直接查一次
6. 如果拿到了多份结果，就用 `MergeResults(...)` 合并

所以这个系统的 retrieve 不是“问一次、查一次、结束”，而是“多子问题并行概念上的多路召回，再统一归并”。

### 第七步：tool workflow

retrieve 完成后，系统可能进入 `runToolWorkflowStage(...)`。

这里会执行一个 `ragtool.Workflow`，输入包括：

- 问题
- 用户 id
- 会话 id
- trace id
- 搜索模式
- 知识库 id
- history
- rewrite 结果
- retrieve 结果
- 事件 sink

也就是说，工具工作流是建立在现成上下文之上的“增强判断层”。它不会从零开始猜，而是吃已有证据。

tool workflow 的输出包括：

- 是否使用过工具
- 是否 degraded
- 工具调用列表
- Agent rounds
- `Context`
- `AnswerGuidance`
- `Control`
- `TraceMeta`

这说明工具工作流的价值不只是“调用工具”，还包括：

- 决定回答时怎么约束
- 决定 trace 里如何标记能力域和证据来源
- 为 prompt 提供结构化上下文

### 第八步：prompt

prompt 阶段会把多路信息拼起来，让最终大模型回答时知道：

- 问题是什么
- 历史上下文是什么
- 知识库证据是什么
- 工具证据是什么
- 有没有 fallback
- 有没有显式回答指导

所以最终回答不是只看“检索结果文本”，而是综合消费多个上游阶段。

### 第九步：真正开始流式输出

这一步通过：

- `chatService.StreamChatWithRequest(...)`

配合：

- `ragChatStreamCallback`
- `TaskRegistry`

把大模型输出流接进系统。

这一步也标志着主链路从“准备阶段”进入“实时输出阶段”。

### 第十步：收尾

回调结束后，主流程阻塞在：

- `result := <-task.doneCh`

然后根据结果类型进入：

- 取消
- 失败
- 成功

成功时会保存 assistant 消息，更新会话 lastTime，发送 `finish` 和 `done`。

## 值得注意的设计细节

### 1. `RagChatService` 是编排层，不是“LLM service”

很多系统会把聊天实现写成一个大函数，内部夹杂数据库、检索、模型调用、输出逻辑。这里没有这么做，而是明确把自己定义成编排层。

这样做的好处是：

- 阶段边界清晰
- 方便 trace
- 方便插入 fallback、tool workflow、更多 guardrail

### 2. 用户消息先落库，assistant 消息后落库

这是一种很典型的对话系统设计：

- 用户提问一进来就记录
- assistant 回复等生成结束再记录

这保证即使模型失败，也不会丢掉用户输入。

### 3. trace 从 runtime stage 开始创建

这说明 trace 不是临时日志，而是被当作主链路的一等公民。系统一旦进入真正的请求处理，就会立刻生成 trace run。

### 4. fallback 是检索置信度驱动的

这不是“工具故障才回退”，而是“知识库证据不够强也要回退”。说明系统对错误答案的防御不只放在模型调用层，也放在检索质量层。

### 5. tool workflow 的输出不止是工具结果

它还输出：

- 回答 guidance
- prompt control
- trace meta

这很重要，因为它说明 Agent 并不是单纯的“执行器”，还是后续回答阶段的控制器和证据组织者。

### 6. 成功、失败、取消三条收尾路径完全分开

这比统一用一个 `defer` 粗暴收尾要健壮得多。因为三种结果对：

- trace 状态
- 消息持久化
- SSE 事件

都有不同要求。

## 预测面试题

1. 你们系统一次聊天请求的主链路是什么？
2. `RagChatService` 为什么被称为总编排层？
3. 为什么用户消息要先落库，而 assistant 消息要在回答完成后再落库？
4. `prepareChat(...)` 做了哪些事，为什么拆成这些阶段？
5. rewrite 在主链路里到底解决什么问题？
6. retrieve 为什么不是直接查一次，而是可能按多个子问题查询？
7. tool workflow 在这个系统里扮演什么角色？
8. fallback 是基于什么条件触发的？
9. 为什么成功、失败、取消要分成三条收尾路径？
10. 如果让你继续扩展这个主链路，你最想插入哪个新阶段，为什么？

