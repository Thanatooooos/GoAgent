# 流式聊天、首包探测与取消

更新时间：2026-05-14

## 概览

这个模块讲的是：系统在真正把回答“一个字一个字”推给前端时，内部是怎么组织的；以及如果用户中途点停止，系统如何把业务层和底层模型流都停下来。

这部分很重要，因为很多系统虽然“支持流式输出”，但实现很脆弱，常见问题包括：

- 建连成功却一直没有首包
- 流其实已经坏了，但前端迟迟感知不到
- 用户点取消后，前端停了，底层 HTTP 流还在跑
- 多个 goroutine 同时写 SSE，输出打架

这个项目围绕这些问题做了比较完整的一套设计：

- `RoutingLLmService.StreamChatWithRequest(...)`
- `ProbeStreamBridge`
- `TaskRegistry`
- `ragChatStreamCallback`
- `sseChatSink`
- `SseEmitterSender`

一句话理解：这是一个“从底层模型流到前端 EventSource 的完整流式管道”。

## 这个模块在整个系统里的位置

流式聊天发生在主链路的最后一段：

1. 会话、记忆、rewrite、retrieve、tool workflow、prompt 都已经准备好
2. `RagChatService` 开始调用流式 LLM
3. 模型增量吐出内容
4. 系统把增量转发给前端
5. 系统同时累积完整 content / thinking 以备落库
6. 结束、失败或取消时做不同收尾

所以这部分不是“聊天系统的全部”，但它决定了聊天系统最后怎么把结果交付出去。

## 功能

### 1. 发起流式模型调用

主入口最终会调用：

- `chatService.StreamChatWithRequest(request, callback)`

这一步建立了“业务层回调”与“底层模型流”之间的连接。

### 2. 做首包探测

系统不会因为 `StreamChat(...)` 返回了 handle 就立刻认为这次流式调用成功，而是要等到首个真正有效的内容事件出现。

### 3. 在首包成功前先缓存事件

这样可以避免“假成功流”把半截垃圾事件暴露给业务层和前端。

### 4. 同时支持增量输出和完整结果累积

一边把 `thinking / content delta` 发给前端，一边在本地累积完整内容，用于最终保存消息。

### 5. 支持任务级取消

用户点停止时，系统会：

- 让主业务流程尽快知道“这轮已经取消”
- 同时真正取消底层模型 HTTP 流

### 6. 安全写出 SSE

最终所有事件都通过一个线程安全、支持写超时和关闭检测的 sender 写到前端。

## 核心代码

### 1. 流式入口

- 文件：`internal/infra-ai/chat/routing_llm_service.go`
- 函数：`StreamChatWithRequest(...)`

这是底层流式路由的总入口。

### 2. 首包探测桥

- 文件：`internal/infra-ai/chat/probe_stream_bridge.go`
- 类型：`type ProbeStreamBridge`

核心方法：

- `OnContent(...)`
- `OnThinking(...)`
- `OnComplete()`
- `OnError(...)`
- `AwaitFirstPacket(...)`
- `commit()`

### 3. 任务注册与取消

- 文件：`internal/app/rag/service/task_registry.go`
- 类型：`type TaskRegistry`

核心方法：

- `New()`
- `Set(...)`
- `Cancel(...)`
- `Delete(...)`

### 4. 业务层流式回调

- 文件：`internal/app/rag/service/rag_chat_service.go`
- 类型：`type ragChatStreamCallback`

核心方法：

- `OnContent(...)`
- `OnThinking(...)`
- `OnComplete()`
- `OnError(...)`
- `watchCancel()`

### 5. SSE 适配层

- 文件：`internal/adapter/http/rag/handlers.go`
- 类型：`type sseChatSink`

### 6. SSE 基础设施

- 文件：`internal/framework/web/sse_emitter_sender.go`
- 类型：`type SseEmitterSender`

## 为什么流式不能只看 `StreamChat(...)` 返回成功

这是理解整套实现的关键前提。

在非流式调用里，请求返回成功通常就说明：

- 这次调用至少拿到了一个完整响应

但流式调用不一样。

### 可能出现的几种“假成功”

#### 1. 建连成功，但一直没有首包

网络连接建立了，handle 也返回了，但模型侧没有实际输出任何 token。

#### 2. 很快报错

请求刚开始就异常，但因为 handle 已经拿到了，如果没有额外探测，业务层会误以为流已成功启动。

#### 3. 很快 complete，但没有有效内容

从协议角度它结束了，但从业务角度这次流是空的。

如果系统只看“handle 是否返回”，就会把这些情况都误判成成功。

## `ProbeStreamBridge` 是怎么解决这个问题的

`ProbeStreamBridge` 可以理解成“夹在底层 provider callback 和业务 callback 之间的一层缓冲桥”。

### 它的核心思想

在首包还没被确认之前：

- 不立即把事件放给业务层
- 先缓存下来
- 等待一个明确的 probe 结果

### 它如何判断 probe 结果

#### 收到 `OnContent(...)`

视为：

- `ProbeResultSuccess`

#### 收到 `OnThinking(...)`

也视为：

- `ProbeResultSuccess`

说明模型已经开始真实地产出有效流式内容。

#### 收到 `OnComplete()`

如果在此之前没有 content / thinking，就视为：

- `ProbeResultNoContent`

#### 收到 `OnError(err)`

视为：

- `ProbeResultError`

#### 超时

`AwaitFirstPacket(timeout)` 等不到 probe 结果，就视为：

- `ProbeResultTimeout`

### 为什么要先缓存事件

因为在首包确认前，系统还不应该承诺“这条流已经稳定可用”。

如果不缓存，可能会出现：

- 前端已经收到一点 thinking
- 结果下一秒发现整条流其实不该被判成功

这样业务语义会很混乱。

### `commit()` 的意义

一旦 `AwaitFirstPacket(...)` 收到成功结果：

- `commit()` 会把之前缓存的事件按顺序真正转发给下游 callback
- 之后新事件直接透传，不再缓存

所以 `commit()` 相当于一次“正式放行”。

## 流式路由层是怎么使用这个 bridge 的

在 `RoutingLLmService.StreamChatWithRequest(...)` 中，针对每个候选模型：

1. 先创建 `bridge := NewProbeStreamBridge(callback)`
2. 让底层 client 把流事件写进 bridge
3. 拿到 `handle`
4. 等待 `bridge.AwaitFirstPacket(firstPacketTimeout)`
5. 如果成功：
   - `MarkSuccess`
   - 返回这个 handle
6. 如果失败：
   - `handle.Cancel()`
   - `MarkFailure`
   - 切换到下一个模型

这意味着系统对流式路由的成功标准是：

- “不是建连成功”
- 而是“真正开始吐有效事件”

这是一种更符合业务语义的成功判定。

## `TaskRegistry` 在这里解决什么问题

模型流跑起来以后，业务层需要一个办法在“请求还没结束时”找到并控制它。

这就是 `TaskRegistry` 的作用。

### 一个任务包含什么

`ragChatTask` 里有：

- `handle`
- `cancelCh`
- `doneCh`

### 这三个东西分别代表什么

#### `handle`

底层模型流的取消句柄，本质上能取消 provider 的 HTTP request context。

#### `cancelCh`

业务层取消信号。谁收到它，谁就知道“这轮已经被用户取消了”。

#### `doneCh`

回调和主流程之间的结果通道。主流程最终会阻塞等待这里的结果。

### `Cancel(taskID)` 做了什么

找到任务后，会执行一次：

- 关闭 `cancelCh`
- 调 `handle.Cancel()`

注意是 `sync.Once` 控制的，避免重复取消。

这就是典型的“两层取消”：

- 业务层状态取消
- 底层模型流取消

## `ragChatStreamCallback` 是怎么串联业务层的

这个回调对象一边接收底层流事件，一边承担业务层的收敛职责。

### `OnContent(...)`

做两件事：

1. 把 content 追加到本地 `strings.Builder`
2. 调 `sink.SendMessage(content)` 把增量推给前端

### `OnThinking(...)`

同理：

1. 本地累积 thinking
2. 调 `sink.SendThinking(content)`

### `OnComplete()`

向 `doneCh` 发一个结果，带上完整：

- `content`
- `thinking`

### `OnError(err)`

同样向 `doneCh` 发结果，但附带 error。

### `watchCancel()`

这个 goroutine 会一直等：

- `<-task.cancelCh`

一旦收到取消信号，也会往 `doneCh` 发一个 `cancelled=true` 的结果。

所以从主流程视角看，不管结束原因是什么，最终都统一表现为：

- 从 `doneCh` 收到一个 `ragChatTaskResult`

然后主流程再决定走成功、失败还是取消收尾。

## 从前端视角看，事件是怎么送过来的

### 第一步：业务层只知道 `RagChatEventSink`

`RagChatService` 并不直接写 SSE，它只是调用：

- `SendMessage`
- `SendThinking`
- `SendToolStart`
- `SendToolResult`
- `SendFinish`
- `SendCancel`
- `SendError`

### 第二步：`sseChatSink` 把业务事件映射成 SSE

例如：

- `SendThinking` 发送 `event=message`，但 payload 里 `type=think`
- `SendMessage` 发送 `event=message`，payload 里 `type=response`

这说明前端看到的不是底层 callback 原样输出，而是经过协议整理后的事件流。

### 第三步：`SseEmitterSender` 负责真正写连接

它做的事情包括：

- 设置 SSE 头
- 构造标准 `event/data` payload
- 写超时控制
- flush
- 感知断连
- 幂等关闭

所以 `sseChatSink` 是业务适配层，`SseEmitterSender` 是传输基础设施层。

## 为什么 `SseEmitterSender` 要同时用 `atomic.Bool + Mutex`

这也是很适合面试讲的点。

### `atomic.Bool` 解决什么

解决：

- “连接现在还该不该继续写”

也就是一个快速、跨 goroutine 的关闭状态判断。

### `Mutex` 解决什么

解决：

- “多个 goroutine 同时写同一条连接时，写入会不会互相打断”

SSE 是有格式要求的：

```text
event: xxx
data: yyy

```

如果两个 goroutine 同时写，很容易把 payload 交叉写坏。

所以：

- `atomic.Bool` 管状态
- `Mutex` 管写入临界区

这两者职责完全不同，不是重复。

## 一次完整流式聊天的协作链

如果你想把这部分真正讲顺，可以按下面这条链描述：

1. `RagChatService` 组好 prompt
2. 创建 `ragChatTask`
3. 注册到 `TaskRegistry`
4. 构造 `ragChatStreamCallback`
5. 调 `RoutingLLmService.StreamChatWithRequest(...)`
6. 路由层对候选模型逐个尝试
7. 每个候选都通过 `ProbeStreamBridge` 做首包探测
8. 首包成功后返回底层 `handle`
9. 底层流事件进入 `ragChatStreamCallback`
10. callback 一边累积完整内容，一边通过 `sseChatSink` 推给前端
11. callback 最终把结果写入 `doneCh`
12. 主流程从 `doneCh` 收到结果后收尾
13. 如果用户中途取消，则 `TaskRegistry.Cancel(taskID)` 同时触发业务层取消和底层流取消

这条链讲清楚之后，这个模块基本就算真正理解了。

## 值得注意的设计细节

### 1. 成功标准是“首包成功”，不是“handle 返回”

这是这套设计最核心的质量点。

### 2. callback 同时承担“增量转发”和“完整结果累积”

这样最终落库和前端实时体验都能兼顾。

### 3. 取消是双层的

只取消业务层不够，因为底层 HTTP 流会继续占资源。

只取消底层流也不够，因为业务层还要知道本轮状态已经变成 cancelled。

### 4. SSE 写出被严格串行化

这是基础设施稳定性的必要条件，不是“性能优化项”。

### 5. `doneCh` 把多种结束原因统一成一个收敛点

这样主流程不需要同时监听很多异步源，只需要等一个结果通道即可。

## 预测面试题

1. 为什么流式调用不能只看 `StreamChat(...)` 返回成功？
2. `ProbeStreamBridge` 的核心作用是什么？
3. 为什么首包前要先缓存事件？
4. 流式路由层如何判断一个候选模型是真的可用？
5. `TaskRegistry` 在流式聊天里解决了什么问题？
6. 为什么取消要同时作用于业务层和底层模型流？
7. `ragChatStreamCallback` 为什么既要转发增量，又要累积完整结果？
8. `doneCh` 的设计有什么好处？
9. `SseEmitterSender` 为什么要同时用 `atomic.Bool + Mutex`？
10. 如果某个流式模型建连成功但 60 秒没首包，系统会怎么处理？
