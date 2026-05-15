# Trace / SSE 可观测性

更新时间：2026-05-14

## 概览

一个 RAG/Agent 系统如果只有“能回答”，但看不清回答过程中发生了什么，很快就会进入不可维护状态。这个项目在可观测性上做了两条并行链路：

- `trace`：面向后端研发、排障、复盘
- `SSE`：面向前端实时体验

这两条链路看起来都在“暴露过程”，但本质目标不同：

- `trace` 解决“事后还原发生了什么”
- `SSE` 解决“用户在请求进行中能看到什么”

一句话理解：`trace` 是结构化过程日志，`SSE` 是实时事件流。

## 这个模块在系统里的位置

它不是主链路的某个可选插件，而是贯穿聊天流程的横切能力。

主链路里的多个阶段都会写 trace：

- rewrite
- retrieve
- tool workflow
- agent round
- tool call
- observation
- final chat stream
- fallback

与此同时，主链路还会通过 `RagChatEventSink` 持续向前端发送 SSE 事件。

这意味着系统在设计上区分了两件事：

- “内部阶段已经发生了什么”由 trace 负责保存
- “用户此刻应该感知到什么”由 SSE 负责输出

## 功能

### 1. 记录一次请求级 trace run

系统会在一轮聊天真正进入运行态时创建一条 `trace run`，包括：

- `traceId`
- `conversationId`
- `taskId`
- `userId`
- `status`
- `startTime`

在请求结束时，再回写：

- 最终状态
- 错误信息
- `endTime`
- `durationMs`

这让系统可以很容易回答这种问题：

- 某次聊天成功了还是失败了？
- 它执行了多久？
- 它属于哪个会话、哪个任务、哪个用户？

### 2. 记录阶段级 trace node

除了 run，系统还会记录一系列 `trace node`，用于描述具体阶段。

每个节点会带上：

- `nodeId`
- `parentNodeId`
- `depth`
- `nodeType`
- `nodeName`
- `status`
- `errorMessage`
- `startTime`
- `endTime`
- `durationMs`
- `extraData`

这让一次请求不再只是一个“黑盒成功/失败”，而是一个可以逐层下钻的过程树。

### 3. 保存检索与工具链的结构化元数据

这个系统在 trace 的 `extraData` 里保存了不少很关键的信息，比如：

- retrieve 的 `searchMode`
- `chunkCount`
- `topScore`
- `searchChannels`
- `channelStats`

以及 tool workflow 的：

- `used`
- `degraded`
- `degradeReason`
- `toolCallCount`
- `roundCount`
- `traceMeta`

这些字段的意义是：trace 不是只记录“代码执行了”，而是记录“业务语义上发生了什么”。

### 4. 通过 SSE 实时推送关键事件

SSE 侧当前会推送这些核心事件：

- `meta`
- `fallback`
- `agent_think`
- `message`
- `tool_start`
- `tool_result`
- `tool`
- `title`
- `finish`
- `cancel`
- `error`
- `done`

这让前端可以实现：

- 聊天内容增量渲染
- thinking 展示
- tool 卡片展示
- fallback 提示
- 完成和取消反馈

## 核心代码

### 1. Trace 核心实现

- 文件：`internal/app/rag/service/chat_tracer.go`
- 类型：`type ChatTracer`

最关键的函数有 3 个：

- `startTraceRun(...)`
- `recordTraceNodeAt(...)`
- `finishTraceRun(...)`

如果只能记住 3 个名字，就记住这 3 个。

### 2. Trace 查询服务与接口

- 文件：`internal/app/rag/service/trace_service.go`
- 文件：`internal/adapter/http/rag/trace_handlers.go`

它们负责把 run 和 node 重新组织成后台可查看的 trace 详情页数据。

### 3. SSE 输出入口

- 文件：`internal/adapter/http/rag/handlers.go`
- 类型：`type sseChatSink`

`sseChatSink` 实际上是业务事件到 SSE 事件的适配层。它把业务层的：

- `SendMeta`
- `SendThinking`
- `SendMessage`
- `SendToolStart`
- `SendToolResult`
- `SendFinish`

翻译成标准 SSE 事件。

### 4. 通用 SSE sender

- 文件：`internal/framework/web/sse_emitter_sender.go`
- 类型：`type SseEmitterSender`

这个类不关心 RAG，它只关心：

- 设置 SSE 头
- 安全写出 event/data
- flush
- 写超时
- 连接关闭

所以它是一个基础设施组件，不是业务逻辑组件。

## 为什么 trace 要拆成 run 和 node

这是一个非常典型、也非常适合面试解释的设计点。

### 如果只用一张表会怎样

如果只用一张表记录所有东西，会出现两个问题：

1. 请求级信息和阶段级信息粒度不同，字段会混杂
2. 列表查询和详情查询诉求不同，很难同时优化

例如列表页通常只关心：

- traceId
- 用户
- 状态
- 耗时

而详情页才关心：

- rewrite 节点
- retrieve 节点
- tool_workflow 节点
- chat 节点

### 拆成两张表后的好处

拆成 `run + node` 后：

- `run` 用来做请求级摘要
- `node` 用来做过程级明细

这样可以：

- 列表页只查 run，更轻
- 详情页按 `TraceID` 查 node，更灵活
- 请求级和节点级可以分别扩展字段

所以最准确的表达是：

- `run` 代表“一次请求”
- `node` 代表“这次请求里的一个阶段”

## ChatTracer 是怎么工作的

### 1. `startTraceRun(...)`

这一步发生在聊天进入 runtime stage 时。

它会创建一条 `RagTraceRun`，并写入：

- `TraceName = "rag_chat"`
- `EntryMethod = "rag.v3.chat"`
- `status = running`

这相当于告诉系统：一条新的聊天运行实例开始了。

### 2. `recordTraceNodeAt(...)`

这是最核心的节点记录函数。

它的输入包括：

- `traceID`
- `ragChatTraceNode`
- `status`
- `startedAt`
- `endedAt`
- `extra`

然后它会做几件事：

1. 生成 node record id
2. 计算 duration
3. 从 `extra["error"]` 提取错误信息
4. 把 `extra` 序列化成 JSON 存进 `ExtraData`
5. 调 repo 落库

注意这里有一个很关键的点：

- 节点的结构化核心字段放在列里
- 易变、扩展性强的业务细节放在 `ExtraData`

这是一种很务实的存储设计。

### 3. `finishTraceRun(...)`

请求结束时，系统会：

- 读取 run
- 计算从 `StartTime` 到现在的总耗时
- 写入最终状态和错误信息

也就是说，trace run 的 duration 不是前面阶段累加出来的，而是直接按运行时钟计算的真实总耗时。

## 阶段节点是怎么被记录的

`rag_chat_service.go` 里用了一个很好理解的辅助函数：

- `runRagChatStage(...)`

这个函数把一个阶段抽象成：

- 一个 trace node 描述
- 一个 `run(...)` 业务函数
- 一个 `buildExtra(...)` 元数据函数

执行流程是：

1. 记录开始时间
2. 执行业务逻辑
3. 成功则写 success node
4. 失败则写 failed node

这让 rewrite、retrieve、tool_workflow 等阶段都能以相同模式接入 trace。

这是一个很漂亮的工程点，因为它统一了“阶段执行”和“阶段记录”的模式。

## tool workflow 的 trace 为什么更复杂

普通阶段例如 rewrite / retrieve，通常只对应一个节点。

但 tool workflow 不一样，它内部本身就是多轮的：

- agent round
- tool call
- observation

所以 `ChatTracer` 额外提供了：

- `recordToolCallTraceNodes(...)`
- `recordAgentWorkflowTraceNodes(...)`

在多轮 Agent 场景下，trace 结构大致是：

- `tool_workflow`
  - `agt_round_01`
    - `tool_call`
    - `tool_call`
    - `agt_obs_01`
  - `agt_round_02`
    - ...

这能很好反映 AgentLoop 的真实执行形态。

## retrieve 的过程是怎么暴露的

retrieve 阶段在 `buildExtra(...)` 中会写入：

- `chunkCount`
- `searchMode`
- `topScore`
- `searchChannels`
- `channelStats`

这几个字段非常关键，因为它们基本覆盖了检索排障最常见的问题：

- 到底是用什么模式查的？
- 命中了多少 chunk？
- 最高分高不高？
- 哪些通道参与了？
- 每个通道各自返回了什么？

所以这个项目里 trace 不是只为了记录“代码跑没跑”，而是为了记录“检索质量怎么表现”。

## SSE 是怎么接出来的

### 第一步：HTTP 层创建 sender

在 `handlers.go` 的 `Chat(...)` 里：

- 先创建 `fwweb.NewSseEmitterSender(c)`
- 再包成 `sseChatSink`

### 第二步：业务层只面向 sink

`RagChatService.Chat(...)` 不知道 HTTP 细节，它只知道自己有一个：

- `RagChatEventSink`

这是一种很好的分层：

- 业务层描述“我要发什么事件”
- HTTP 层决定“这些事件如何编码成 SSE”

### 第三步：sink 把业务事件翻译成 SSE 事件

例如：

- `SendMeta` -> `event: meta`
- `SendThinking` -> `event: message` 且 `type=think`
- `SendMessage` -> `event: message` 且 `type=response`
- `SendToolStart` -> `event: tool_start`
- `SendToolResult` -> `event: tool_result`

这说明 SSE 事件模型不是直接暴露内部对象，而是已经做过一层面向前端的整理。

## `SseEmitterSender` 做了哪些基础设施工作

### 1. 初始化标准 SSE headers

创建时会设置：

- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`
- `X-Accel-Buffering: no`

这些头保证代理层和浏览器会按 SSE 方式处理连接。

### 2. 感知客户端断连

它会监听：

- `c.Request.Context().Done()`

一旦客户端断开，就把 `closed` 置为 true。

### 3. 统一串行写出

`SendEvent(...)` 内部会加锁，避免多个 goroutine 同时向同一连接写数据。

### 4. 控制写超时

它会通过 `http.NewResponseController(...).SetWriteDeadline(...)` 尝试设置 deadline，防止写出永久卡住。

### 5. 提供幂等关闭

`Complete()` 使用 CAS 保证连接只会被真正关闭一次。

## 值得注意的设计细节

### 1. trace 和 SSE 是两个不同职责的系统

很多人会误以为“既然已经有 SSE 了，就不需要 trace”。这个项目明确没有这么做，因为两者服务的对象不同：

- SSE 服务在线用户体验
- trace 服务研发排障与后台分析

### 2. `extraData` 是高扩展性的关键

如果把所有阶段细节都做成固定列，后续演进成本很高。这里用结构化列 + JSON extra 的方式平衡了查询效率和扩展能力。

### 3. 节点是树，不是平铺

通过 `ParentNodeID + Depth` 组织成树，意味着后续做 trace 详情页、Agent 调用展开、tool 调试都更自然。

### 4. Tool 卡片事件字段对齐是一次真实的联调问题

之前 `ToolCallEvent` 没有 JSON tag，导致后端发的是 `CallID / Name / Summary`，前端按 `callId / name / summary` 读，结果卡片空白。

这个问题说明：

- 事件协议本身也是系统稳定性的一部分
- 观测链路不只是“后端记日志”，还包括“前后端事件契约是否一致”

### 5. `runRagChatStage(...)` 是一种很好的阶段执行模板

它把“阶段执行 + trace 记录”绑定起来，减少了每个阶段自己处理计时、状态、extraData 的重复代码。

## 预测面试题

1. 为什么你们要同时做 trace 和 SSE？
2. trace 为什么拆成 `run + node` 两张表？
3. `ChatTracer` 的三个核心函数分别做什么？
4. 为什么节点要做成树，而不是平铺列表？
5. retrieve 阶段为什么要记录 `searchChannels / channelStats` 这类元数据？
6. `runRagChatStage(...)` 解决了什么工程问题？
7. `SseEmitterSender` 为什么要单独抽成基础设施组件？
8. 为什么一个连接上的 SSE 写出需要串行化？
9. Tool 卡片事件字段对齐问题暴露了什么设计点？
10. 如果让你继续增强 trace 产品化，你会优先补什么？

