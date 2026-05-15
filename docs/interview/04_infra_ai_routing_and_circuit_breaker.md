# Infra-AI 模型选择、路由执行与熔断

更新时间：2026-05-14

## 概览

这个模块属于 AI 基础设施层，解决的问题不是“如何回答业务问题”，而是“系统在有多个模型候选时，应该选谁、怎么试、失败后怎么切、连续异常时怎么保护自己”。

一句话理解：它负责把“模型配置”变成“稳定可执行的模型调用链”。

如果没有这一层，业务代码通常会退化成这种脆弱写法：

- 直接写死一个模型
- 调失败就报错
- 不知道当前模型是否健康
- 没有主备顺序
- 流式和非流式没有统一路由逻辑

这个项目把这些问题抽成了几个独立组件：

- `ModelSelector`
- `ModelRoutingExecutor`
- `ModelHealthStore`
- `RoutingLLmService`

## 这个模块在整个系统里的位置

业务层例如 `RagChatService` 并不关心“用哪个 provider、哪个模型、当前是否熔断”，它只依赖一个更高层的接口：

- `aichat.LLMService`

而 `RoutingLLmService` 就是这个接口的一种实现。它位于：

- 业务编排层之下
- 具体 provider client 之上

可以把这层理解成一个 AI 调用网关。

调用链大致是：

- `RagChatService`
  - `RoutingLLmService`
    - `ModelSelector`
    - `ModelRoutingExecutor`
    - `ModelHealthStore`
    - provider-specific `ChatClient`

## 功能

### 1. 从配置生成有序模型候选链

系统配置里可能会定义多个模型候选，例如：

- 默认模型
- 深度思考模型
- 不同 provider 的备选模型
- 各自优先级

`ModelSelector` 的作用是把这些静态配置转成当前请求可用的、有顺序的候选链。

### 2. 在调用失败时自动 fallback

`ModelRoutingExecutor` 会按顺序尝试候选模型：

1. 解析对应 provider client
2. 检查该模型是否允许调用
3. 执行调用
4. 失败则切下一个
5. 成功则返回

这使系统具备了主备模型切换能力。

### 3. 对连续失败模型做熔断保护

`ModelHealthStore` 负责记录每个模型的健康状态，避免系统在一个明显有问题的模型上不断重试。

### 4. 统一 chat / embedding / rerank 的路由框架

虽然这里我们重点复习的是 chat，但实际上：

- chat
- embedding
- rerank

都在复用同一套路由执行框架。

这说明这个设计不是“为聊天特化的补丁”，而是整个 infra-ai 层的通用基础设施。

## 核心代码

### 1. 配置结构

- 文件：`internal/framework/config/config.go`

这里能看到和选择逻辑直接相关的字段，比如：

- `default-model`
- `deep-thinking-model`
- `supports-thinking`
- `priority`

### 2. 模型选择器

- 文件：`internal/infra-ai/model/model_selector.go`
- 类型：`type ModelSelector`

重点函数：

- `SelectChatCandidates(deepThinking bool)`
- `SelectEmbeddingCandidates()`
- `SelectRerankCandidates()`

### 3. 路由执行器

- 文件：`internal/infra-ai/model/model_routing_executor.go`
- 类型：`type ModelRoutingExecutor`
- 核心函数：`ExecuteWithFallback[...]`

### 4. 熔断状态存储

- 文件：`internal/infra-ai/model/model_health_store.go`
- 类型：`type ModelHealthStore`

### 5. chat 路由服务

- 文件：`internal/infra-ai/chat/routing_llm_service.go`
- 类型：`type RoutingLLmService`

重点函数：

- `ChatWithRequest(...)`
- `ChatWithModel(...)`
- `StreamChatWithRequest(...)`

## 模型选择是怎么做的

### 1. 先决定 first choice

`ModelSelector.resolveFirstChoiceModel(...)` 会先决定当前请求的首选模型：

- 如果是深度思考请求，且配置了 `DeepThinkingModel`，优先它
- 否则使用 `DefaultModel`

这体现了一个很重要的设计原则：

- “适合当前请求”的优先级高于“全局默认顺序”

### 2. 过滤候选

`filterAndSortCandidates(...)` 会先把不该参与本次请求的模型过滤掉。

典型过滤条件：

- 明确被禁用
- 当前请求要求深度思考，但该模型不支持 `supports-thinking`

这一步避免了无意义尝试。

### 3. 排序

排序逻辑大致是：

1. first choice 模型最优先
2. 然后按 `priority`
3. 最后按 id 稳定排序

所以这里不是简单的“按配置顺序遍历”，而是一个有显式规则的稳定候选链生成过程。

### 4. 构建可用目标

`buildAvailableTargets(...)` 会进一步把 `ModelCandidate` 转成 `ModelTarget`，同时：

- 校验 provider 配置是否存在
- 检查 health store 是否认为当前模型不可用

也就是说，`Selector` 最终返回的不是“配置里写了什么”，而是“当前真的能拿去调什么”。

## 路由执行是怎么做的

`ExecuteWithFallback[...]` 是整个框架的核心。

### 它解决什么问题

给它一组有序候选 target，它负责：

- 逐个尝试
- 解析对应 client
- 调用实际模型
- 处理失败并回退
- 更新健康状态

### 执行流程

按代码顺序看，大致是：

1. 校验 executor / targets / resolver / caller 是否为空
2. 逐个遍历 `targets`
3. `clientResolver(target)` 拿到 provider client
4. `healthStore.allowCall(target.Id)` 判断熔断是否允许本次调用
5. `caller(client, target)` 真正发起调用
6. 失败则 `markFailure`
7. 成功则 `markSuccess`
8. 全部失败则返回聚合错误

### 为什么是泛型函数

`ExecuteWithFallback[C any, T any]` 是泛型函数，说明设计者不想把它绑死在 chat 结果类型上。

它只抽象两件事：

- client 是什么类型
- 调用返回什么类型

于是同一套路由逻辑就能用于：

- chat 返回 string
- embedding 返回向量
- rerank 返回重排结果

这是一种非常典型的“把变化点参数化、把稳定流程抽出来”的写法。

## 熔断是怎么做的

`ModelHealthStore` 是这个模块里最值得被问深的部分。

### 状态机

每个模型都有一个 `modelHealth`，内部状态有 3 种：

- `Closed`
- `Open`
- `HalfOpen`

#### `Closed`

表示模型正常可调用。

#### `Open`

表示模型已被熔断，在 `openUntil` 到期前直接拒绝调用。

#### `HalfOpen`

表示熔断冷却期结束后，允许一个探测请求进入，看模型是否恢复。

### `allowCall(...)` 的逻辑

当有一次调用到来时：

- 如果是 `Open`
  - 且冷却时间还没结束，直接拒绝
  - 如果已经结束，则切到 `HalfOpen` 并放一个探测请求进去
- 如果是 `HalfOpen`
  - 且已经有探测请求在飞，就拒绝其他请求
  - 否则放行一个探测请求
- 如果是 `Closed`
  - 正常放行

这里的关键思想是：

- 不让恢复探测变成并发洪峰

### `markSuccess(...)` 的逻辑

成功后会：

- 连续失败数清零
- `halfOpenInFlight = false`
- `openUntil` 清空
- 状态回到 `Closed`

### `markFailure(...)` 的逻辑

失败时分两种情况：

- 如果当前是 `HalfOpen`
  - 说明恢复探测失败
  - 立即重新 `Open`
- 否则
  - `consecutiveFailures++`
  - 达到阈值后进入 `Open`

这个实现是一个标准且足够实用的三态熔断器。

## 为什么 `sync.Map` 外面还要再加一层 `Mutex`

这是面试非常爱问的点。

`ModelHealthStore` 使用：

- 外层 `sync.Map`
  - 管理 `modelID -> *modelHealth`
- 内层 `modelHealth.mu`
  - 保证单个模型状态机转移原子化

很多人会以为用了 `sync.Map` 就够了，但其实不够。

### `sync.Map` 能保证什么

它只能保证：

- map 这个容器本身的并发访问安全

### 它不能保证什么

它不能保证：

- 某个 value 对象内部字段读写的并发安全

而 `modelHealth` 里面有这些共享字段：

- `consecutiveFailures`
- `openUntil`
- `halfOpenInFlight`
- `state`

这些状态必须一起转移，否则就会出现竞态条件。

所以这里是两层并发控制：

- 外层保护“查哪个对象”
- 内层保护“改对象内部状态”

这是一种非常标准、也非常值得拿来解释并发设计分层的例子。

## `RoutingLLmService` 扮演什么角色

`RoutingLLmService` 是业务层真正看到的 chat 服务。

它主要做两件事：

### 1. 非流式 chat 路由

`ChatWithRequest(...)` 会：

- 让 selector 选候选
- 调 `ExecuteWithFallback(...)`
- 传入 `resolveClient(...)`
- 最终调用 `client.Chat(...)`

### 2. 流式 chat 路由

`StreamChatWithRequest(...)` 更复杂一些，因为它除了候选切换，还要解决“流到底有没有真正开始”的问题。

它会：

- 选候选
- 过滤熔断模型
- 调每个 client 的 `StreamChat(...)`
- 配合 `ProbeStreamBridge` 做首包探测
- 成功后才 `MarkSuccess(...)`
- 失败则 `MarkFailure(...)` 并切下一个模型

这说明在这个系统里，流式路由不是非流式逻辑的简单复制，而是带了专门的可用性保护。

## 值得注意的设计细节

### 1. 选择和执行是拆开的

这体现了很清晰的职责分离：

- `Selector` 决定“试谁”
- `Executor` 决定“怎么试”

### 2. 选择结果已经包含健康状态过滤

`buildModelTarget(...)` 里就会把不可用模型过滤掉，所以业务拿到的候选链已经是“当前时刻尽量可用”的集合。

### 3. 熔断状态不绑在 provider 上，而绑在 model id 上

这让系统可以更细粒度地控制具体模型，而不是粗暴地把整个 provider 全部视为不可用。

### 4. 同一套路由框架服务多个能力域

这说明 infra-ai 层是真正做成了基础设施，而不是聊天功能里的局部工具类。

### 5. 流式路由把“首包探测”纳入健康判断

这很成熟，因为流式模型最常见的失败不只是“请求报错”，还包括“建连成功但根本不出首包”。

## 预测面试题

1. `ModelSelector` 和 `ModelRoutingExecutor` 为什么拆开？
2. 模型候选链是怎么生成的？
3. 深度思考请求为什么要额外考虑 `supports-thinking`？
4. fallback 是在哪一层实现的？
5. 三态熔断器的 `Closed / Open / HalfOpen` 分别表示什么？
6. `HalfOpen` 为什么只允许一个探测请求进入？
7. 为什么 `sync.Map` 外面还要给单个模型状态再加 `Mutex`？
8. chat、embedding、rerank 为什么能复用同一套路由执行框架？
9. `RoutingLLmService` 在整个架构里扮演什么角色？
10. 流式调用为什么要把首包探测也纳入健康判断？

