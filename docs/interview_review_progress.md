# Interview Review Progress

更新时间：2026-05-14

这份文档用于沉淀当前 `goagent / Go-RAG` 项目的面试复习进度，帮助后续持续补充“已经复习过什么、核心结论是什么、下一步建议看什么”。

## 当前复习范围

截至目前，我们已经重点复习了以下模块：

1. `RAG / Agent` 总入口与主链路认知
2. `trace / SSE` 如何暴露检索与工具过程
3. `retrieve merge` 合并逻辑
4. `infra-ai` 模型选择、路由执行、熔断降级
5. 流式聊天 callback / cancel handle / SSE sender
6. `ingestion` 流水线与执行架构
7. `knowledge -> ingestion -> retrieve` 闭环里的 ingestion 写回链路
8. `tool workflow` 总入口与装配骨架

尚未系统展开的模块：

- 多通道检索底层细节的进一步口语化整理
- 会话记忆模块的面试表达整理
- `tool workflow` 里的 `AgentLoop` 完整收敛过程
- planner / observer / tool module 行为层的进一步细化

---

## 1. 总入口与主链路

已掌握的关键点：

- 聊天入口是 `GET /rag/v3/chat`
- HTTP 层主要负责鉴权、参数解析和 SSE 建联，不承载核心业务决策
- 主业务入口是 `RagChatService.Chat(...)`
- 主链路顺序是：
  - conversation / memory
  - rewrite
  - retrieve
  - tool workflow（可选）
  - prompt
  - stream chat

面试表达重点：

- `RagChatService` 是总编排层，不是一进来就直接调大模型
- 系统会先补齐上下文、检索知识、必要时跑工具，再进入最终回答
- 这是一个“RAG 优先，Agent 增强”的链路，而不是“裸 Agent”

---

## 2. Trace / SSE 可观测性

已掌握的关键点：

- `trace` 和 `node` 在数据库层是拆开的
- `trace run` 存一次请求的全局摘要
- `trace node` 存请求内部各阶段明细
- 两者通过 `TraceID` 关联
- `node` 再通过 `ParentNodeID + Depth` 组织成树

### 2.1 为什么拆成两个表

已总结的结论：

- `run` 和 `node` 不是同一粒度的数据
- 一条请求对应多条节点，天然是一对多
- 列表页只查 `run` 更高效
- 详情页再按 `TraceID` 查 `node`
- `run` 更像主记录，`node` 更像过程明细日志

### 2.2 ChatTracer 三个核心函数

已掌握：

- `startTraceRun(...)`
  - 在请求开始时创建一条 trace run
  - 记录 `traceId / conversationId / taskId / userId / startTime / status=running`
- `recordTraceNodeAt(...)`
  - 为具体阶段写 trace node
  - 记录 `nodeId / parentNodeId / depth / status / duration / extraData`
- `finishTraceRun(...)`
  - 在请求结束时回写最终状态、总耗时和错误信息

面试表达重点：

- 这三个函数共同实现了“请求级 + 节点级”的可观测性闭环
- 既能看一次请求整体成功/失败，也能逐层下钻定位问题

### 2.3 检索过程如何暴露

已掌握：

- 初始 `rewrite / retrieve` 主要通过 `trace node` 暴露
- `retrieve` 会在 `extraData` 里记录：
  - `searchMode`
  - `chunkCount`
  - `topScore`
  - `searchChannels`
  - `channelStats`
- SSE 对初始检索过程只做轻量反馈：
  - `meta.searchMode`
  - 低置信度 `fallback`
- 更细粒度的实时事件主要发生在 tool workflow 阶段

面试表达重点：

- SSE 偏用户实时体验
- trace 偏研发排障和复盘
- 两者职责不同，不是所有底层细节都要实时推给前端

---

## 3. Retrieve Merge

已掌握的关键点：

- `runRetrieveStage(...)` 会对每个 `subQuestion` 单独执行一次 retrieve
- 多个子问题结果最终通过 `ragretrieve.MergeResults(...)` 合并

### 3.1 Merge 的行为

已确认：

- 会按 `chunk.ID` 去重
- 如果多个子问题命中同一个 `chunk.ID`
  - 只保留一份
  - 保留分数更高的那条
- 如果内容类似但 `ID` 不同
  - 当前 merge 不会去重

### 3.2 对这套设计的判断

当前结论：

- 作为第一版设计是合理的
- 优点：
  - 防止重复 chunk 膨胀上下文
  - 实现简单、稳定、低误伤
- 局限：
  - 只能处理同 ID 重复
  - 会丢失“同一 chunk 被多个子问题命中”的信号
  - 跨子问题 score 是否完全可比有一定边界

面试表达重点：

- 这是一个合理的工程折中
- 如果继续优化，可以在去重后保留 `hitCount` 等聚合特征，而不是只保留最高分

---

## 4. 模型选择与路由执行

涉及模块：

- `ModelSelector`
- `ModelRoutingExecutor`
- `ModelHealthStore`
- `RoutingLLmService`

### 4.1 ModelSelector

已掌握：

- 职责是把配置中的候选模型，转成当前请求可执行的有序候选链
- chat 选择时会考虑：
  - `default-model`
  - `deep-thinking-model`
  - `supports-thinking`
  - `priority`
  - 当前健康状态

已总结的原因：

- 不是所有请求都适合所有模型
- 需要主备顺序
- 需要在选择阶段提前过滤不适合当前请求或当前不健康的候选

面试表达重点：

- `Selector` 决定“试谁”
- 输出的是 `[]ModelTarget`，不是最终结果

### 4.2 ModelRoutingExecutor

已掌握：

- 通用入口是 `ExecuteWithFallback(...)`
- 它负责：
  - 顺序遍历候选
  - 解析 provider client
  - 调用具体模型
  - 失败后切下一个
  - 成功/失败回写健康状态

面试表达重点：

- `Executor` 决定“怎么试”
- chat / embedding / rerank 都复用同一套路由执行框架

---

## 5. 三态熔断器

核心模块：

- `ModelHealthStore`

### 5.1 状态机

已掌握：

- `Closed`
  - 正常可调用
- `Open`
  - 熔断打开，在 `openUntil` 之前直接拒绝调用
- `HalfOpen`
  - 熔断冷却后允许一个探测请求进入

### 5.2 关键状态转移

已掌握：

- 正常失败累计到阈值后：`Closed -> Open`
- 冷却时间结束后第一次放行：`Open -> HalfOpen`
- 半开探测成功：`HalfOpen -> Closed`
- 半开探测失败：`HalfOpen -> Open`

### 5.3 并发设计

已掌握：

- `healthByID` 用 `sync.Map`
  - 管理 `modelID -> *modelHealth` 的并发安全映射
- `modelHealth` 内部再有一个 `Mutex`
  - 保证单个模型状态机转移原子化

已总结的结论：

- `sync.Map` 只保证表级访问安全
- 不保证 value 内部字段的并发安全
- 所以需要两层并发控制：
  - 外层 map 安全
  - 内层单模型状态安全

面试表达重点：

- 这是“对象索引并发安全”和“对象内部状态并发安全”的分层设计

---

## 6. 流式聊天、首包探测与取消

涉及模块：

- `RoutingLLmService.StreamChatWithRequest(...)`
- `ProbeStreamBridge`
- `ragChatStreamCallback`
- `TaskRegistry`

### 6.1 为什么要做首包探测

已掌握：

- 流式请求中，`StreamChat(...)` 返回 handle 成功，不代表模型真的开始输出 token
- 可能出现：
  - 建连成功但一直没首包
  - 很快报错
  - 直接 complete 但没有内容

所以当前设计要求：

- 只有收到 `OnContent(...)` 或 `OnThinking(...)` 这类首个有效事件，才算这个候选模型真正可用

### 6.2 ProbeStreamBridge 在干嘛

已掌握：

- provider callback 先进入 `ProbeStreamBridge`
- 在首包成功前，事件先缓存在 `buffer`
- `AwaitFirstPacket(...)` 等首包结果
- 成功后 `commit()`，再把缓存事件真正转发给下游 callback
- 失败/超时/空流则取消当前流并切换到下一个候选

面试表达重点：

- 首包探测解决的是“流式启动可用性”
- 避免把假成功流直接暴露给业务层和前端

### 6.3 callback / handle / cancel 是怎么串起来的

已掌握：

- 业务层创建 `ragChatTask`
  - 包含 `cancelCh / doneCh / handle`
- `ragChatStreamCallback`
  - 一边累积完整 `content / thinking`
  - 一边通过 `sink` 把增量推给前端
- 底层 provider client 返回 `StreamCancellationHandle`
  - 本质是一个可取消 HTTP request context 的 handle
- `TaskRegistry.Cancel(taskID)`
  - 会同时：
    - 关闭 `cancelCh`
    - 调底层 `handle.Cancel()`

这实现了两层取消：

- 业务层主流程尽快返回
- 底层模型 HTTP 流也真正停掉

---

## 7. SSE Sender

核心模块：

- `internal/framework/web/sse_emitter_sender.go`

### 7.1 设计目标

已掌握：

- 它是通用的 SSE 写出器
- 负责把业务事件写成标准 `text/event-stream` 帧
- 负责 flush、关闭、并发安全、写超时

### 7.2 它做了什么

已掌握：

- 初始化时设置 SSE 头：
  - `Content-Type: text/event-stream`
  - `Cache-Control: no-cache`
  - `Connection: keep-alive`
  - `X-Accel-Buffering: no`
- 监听 `Request.Context().Done()` 感知客户端断连
- `SendEvent(...)`
  - 串行化写入
  - 构造 `event/data` payload
  - 设置 write deadline
  - `Flush()`
- `Complete()`
  - 幂等关闭

### 7.3 并发设计

已掌握：

- `atomic.Bool`
  - 管连接是否关闭
- `Mutex`
  - 保证同一连接上的 SSE 写操作串行执行

已总结的结论：

- `atomic.Bool` 解决“还该不该写”
- `Mutex` 解决“写的时候会不会互相打断”
- 两者不是重复，而是解决不同问题

面试表达重点：

- 这是一个通用基础设施层 sender
- `sseChatSink` 是聊天业务适配层
- sender 是真正把后端事件帧写到前端 EventSource 的最后一跳

---

## 8. Ingestion 流水线与执行架构

已掌握的关键点：

- `ingestion` 不是简单导入函数，而是一套轻量工作流系统
- 核心三层模型是：
  - `Pipeline`
  - `Task`
  - `TaskNode`
- HTTP 入口最终都会汇聚到 `TaskService.Create(...)`
- `TaskService` 采用“先落 task，再异步提交 executor”的模式
- `ExecutorService` 负责 workflow 构建、并发控制、重试、生命周期管理
- `ExecutionState` 是节点之间的数据总线
- `NodeRunnerRegistry` 负责按 `nodeType` 分发具体 runner
- `TaskObserver` 把执行与落库 / metrics 观测解耦

### 8.1 ingestion 和 chat 里的 task / node 不是一回事

已掌握：

- ingestion 的运行实体是：
  - `t_ingestion_task`
  - `t_ingestion_task_node`
- rag/chat 那边的请求观测实体是 trace run / trace node
- 两边名字相似，但职责、字段和落表位置不同

面试表达重点：

- ingestion 的 `task / task_node` 是业务执行事实
- rag 的 `trace_run / trace_node` 是请求观测事实

### 8.2 observer 的作用

已掌握：

- `TaskObserver` 统一抽象了 5 个生命周期事件：
  - `OnTaskStarted`
  - `OnTaskCompleted`
  - `OnNodeStarted`
  - `OnNodeRetry`
  - `OnNodeCompleted`
- `RepositoryTaskObserver` 负责事实落库
- `MetricsObserver` 负责运行指标聚合
- `MultiTaskObserver` 负责广播组合

已总结的结论：

- executor 负责“跑”
- observer 负责“记”
- 执行、持久化、metrics 三者是解耦的

### 8.3 `task_node.output`

已掌握：

- 表字段类型是 `JSONB`
- model 层是 `[]byte`
- domain 层是 `map[string]any`
- 它是 runner 业务输出和 observer 执行元数据的混合结构

已总结的结论：

- 这个字段更像“节点执行快照”
- 主要给后台排障、查询接口、Agent 诊断工具消费
- 它不是 workflow 内部节点传递数据的主通道，主通道仍然是 `ExecutionState`

### 8.4 四个核心 runner

已掌握：

- `FetcherNodeRunner`
  - 负责把 file / url / feishu / storage source 归一化成统一 `SourcePayload`
- `ParserNodeRunner`
  - 负责把 source 转成 parsed 内容
- `ChunkerNodeRunner`
  - 负责把 parsed 内容切成 chunks
- `IndexerNodeRunner`
  - 负责把 chunks 写成 knowledge chunks + vectors

面试表达重点：

- 当前 ingestion 已经不是“同步导入函数”，而是完整的线性 workflow runtime

---

## 9. knowledge -> ingestion -> retrieve 闭环里的写回链路

已掌握的关键点：

- 真正的 knowledge 写回核心在 `IndexerNodeRunner.Run(...)`
- 它会先确定：
  - `knowledgeBaseId`
  - `documentId`
  - `documentName`
- 然后把 `ExecutionState.Chunks` 转成两套下游对象：
  - `KnowledgeChunk`
  - `ChunkVector`

### 9.1 knowledge chunk 写回

已掌握：

- `buildKnowledgeChunks(...)` 会把 chunk 转成 knowledge 域对象
- knowledge chunk id 采用稳定生成：
  - `documentID-index`
- `knowledgeChunksMatch(...)` 会判断新旧 chunk 是否完全一致
- 如果一致：
  - `chunkWriteMode = reuse`
- 如果不一致：
  - 删旧 chunk
  - 批量创建新 chunk
  - `chunkWriteMode = replace`

已总结的结论：

- knowledge chunk 层支持“完全一致时复用”
- 这样可以减少无意义重写，保留一定幂等优化能力

### 9.2 vector 写回

已掌握：

- `buildVectorChunks(...)` 会把 embedded chunks 转成向量写入对象
- 每个 vector chunk 会附带：
  - `document_id`
  - `document_name`
  - `knowledge_base_id`
  - `source_*`
  - `chunk_index`
- vector 写入采用：
  - `DeleteByDocumentID(...)`
  - `UpsertDocumentChunks(...)`

已总结的结论：

- knowledge chunk 层尽量复用
- vector 层统一 `replace`
- 这是一个“复用 chunk、重建向量”的工程折中

### 9.3 failure compensation

已掌握：

- indexer 在失败时会按实际已落地副作用做补偿清理：
  - 写过 chunk 就删 chunk
  - 写过 vector 就删 vector

面试表达重点：

- 这说明 ingestion 已经明显有生产化意识，不只是 demo 级导入链路

---

## 10. Tool Workflow 总入口与骨架

已掌握的关键点：

- `tool workflow` 是 `RagChatService` 主链里的一个独立阶段
- 它位于 `rewrite / retrieve` 之后
- 不是默认必跑，而是可插拔增强层

### 10.1 在聊天主链里的位置

已掌握：

- `RagChatService.Chat(...)` 在 retrieve 之后调用 `runToolWorkflowStage(...)`
- tool workflow 的输入不是裸问题，而是整轮上下文：
  - `Question`
  - `History`
  - `RewriteResult`
  - `RetrieveResult`
  - `KnowledgeBaseIDs`
  - `TraceID`
  - `EventSink`

已总结的结论：

- 系统策略是 `RAG-first`
- 本地知识不够时，再升级到 tool / agent 能力

### 10.2 workflow 是怎么装起来的

已掌握：

- runtime 会调用 `BuildLocalWorkflow(...)`
- 装配顺序大致是：
  - 建 `registry`
  - 注册 meta / web / system / trace tools
  - 创建 executor
  - 注册 graph tools
  - 最后创建 `AgentLoop`

### 10.3 `AgentLoop` 当前已看到的骨架

已掌握：

- `AgentLoop` 是一个多轮 `Plan -> Act -> Observe` 循环执行器
- 它持有：
  - `executor`
  - `planner`
  - `observer`
  - `maxIterations`
  - 并行 tool call 配置
- 默认 observer 是规则观察器
- 若 runtime 注入 `chatService`，则会启用：
  - `LLMPlanner`
  - `LLMObserver`

### 10.4 planner 的作用

已掌握：

- planner 负责决定“下一轮该调哪些 tool”
- 优先尝试 `LLMPlanner`
- 若 planner 无结果或失败，则回退到规则规划

### 10.5 当前阶段的结论

已总结：

- tool workflow 不是“模型自由调用工具”
- 它是一个有 registry、executor、planner、observer、event sink 的受控 agent runtime
- 当前只完成了总入口和骨架对齐，下一步最应该补的是 `AgentLoop` 的完整一轮收敛过程

---

## 当前适合重点背诵的高频问答

1. 为什么 trace 要拆成 `run + node` 两张表？
2. `startTraceRun / recordTraceNodeAt / finishTraceRun` 分别做什么？
3. 多子问题 merge 时是否会跨子问题去重？去重依据是什么？
4. `ModelSelector` 和 `ModelRoutingExecutor` 为什么要拆开？
5. 三态熔断器的 `Closed / Open / HalfOpen` 分别表示什么？
6. 为什么 `sync.Map` 之外，单个 `modelHealth` 里还要再放一个 `Mutex`？
7. 为什么流式调用不能只看 `StreamChat(...)` 返回成功，还要做首包探测？
8. callback、cancel handle、SSE sink 三者在流式聊天里是怎么配合的？
9. 为什么 `SseEmitterSender` 要同时用 `atomic.Bool + Mutex`？
10. 为什么说 ingestion 本质上是一套轻量工作流系统？
11. `Pipeline / Task / TaskNode / ExecutionState` 在 ingestion 里分别是什么角色？
12. 为什么 `task_node.output` 要设计成半结构化字段？
13. indexer 为什么 knowledge chunk 可以 `reuse`，vector 却统一 `replace`？
14. tool workflow 为什么放在 retrieve 之后，而不是一开始就跑？
15. `AgentLoop` 为什么说是受控 agent runtime，而不是“模型自由调工具”？

---

## 下一步建议

优先推荐继续复习：

1. `AgentLoop` 概览
   - 先把一轮 `Plan -> Act -> Observe` 的完整收敛过程讲顺
   - 看 planner、observer、event sink、round summary 是怎么配合的
   - 对齐“什么时候继续，什么时候停止，什么时候 degraded”

2. `tool workflow` 细化
   - `tool module / tool behavior / graph tool`
   - `web_search / web_fetch / external_evidence_workflow`
   - planner 与规则回退边界

3. 把已复习模块继续压缩成更适合面试口语表达的 30 秒 / 1 分钟版本

---

## 2026-05-15 Additional Review Update: AgentLoop Deep Dive

### 今天新增掌握

1. `AgentLoop` 的真实职责边界
   - 它不是“模型自由调工具”，而是受控的多轮 `Plan -> Act -> Observe` runtime
   - `plan` 负责选下一步动作，不直接产出最终诊断结论
   - `observe` 负责判断当前证据是否足够，以及是否继续下钻/验证

2. `AgentLoop` 关键数据结构已经完成一轮字段级梳理
   - `WorkflowInput`
   - `PlanInput / PlanResult`
   - `ObserveInput / ObserveResult`
   - `AgentState`
   - `HintCall`
   - `CallSummary`
   - `RoundSummary`
   - `WorkflowResult`

3. `AgentState` 的角色已经明确
   - 它是多轮 agent 的跨轮状态，而不是普通日志字段
   - 其中最关键的是：
     - `Phase`
     - `Hypothesis`
     - `OpenQuestions`
     - `CheckedTools`
     - `NextHintCalls`
   - `NextHintCalls` 是 observe 与下一轮 plan 之间的结构化接力点

4. `phase` 当前的工程意义已经厘清
   - 现在它更像“弱状态字段”而不是强状态机
   - 主要作用是表达当前工作模式，例如：
     - `initial_diagnosis`
     - `deep_dive`
     - `verification`
     - `external_search`
     - `fetching`
     - `complete`
   - 在更广泛场景里，它适合作为 planner / observer / trace / UI 的统一阶段标签

5. `plan` 阶段的输入、约束和护栏已经看清
   - `planWithLLM(...)` 会看到：
     - 用户问题
     - rewrite 摘要
     - retrieve 摘要
     - 当前 `AgentState`
     - 历史 tool 结果摘要
     - 当前知识库范围
   - planner prompt 已明确约束：
     - 不编造 ID
     - 优先跟随 `nextHintCalls`
     - 不重复调已执行 call
     - 同一实体一轮不做多层 drill-down
     - 只在独立场景下并行规划多个 tool

6. planner 不是“说了就算”，运行时还有二次校验
   - `validateCallAgainstEvidence(...)` 会校验 call 中的 `documentId / taskId / nodeId / traceId`
   - 参数只能来自：
     - 用户问题中的结构化 ID
     - 上一轮 `NextHintCalls`
     - 历史结果中已暴露的 ID
   - 这意味着 planner 即使出现参数幻觉，runtime 也会拒绝执行

### 用两个典型样例建立了收敛直觉

1. `doc_fail_01`
   - 核心路径是：`document_ingestion_diagnose -> ingestion_task_query -> ingestion_task_node_query`
   - 重点理解的是“多轮下钻直到拿到 node-level error”

2. `doc_run_01`
   - 核心路径仍可能下钻到 task/node，但语义是 `verification`
   - 重点理解的是“运行中场景优先确认真实运行态，避免过早宣判失败”

### 当前更适合面试表达的结论

- `AgentLoop` 不是裸 agent，而是一个带 `planner / executor / observer / event sink / round summary` 的受控 runtime
- `plan` 的本质是“下一步调什么工具”，不是“直接给最终答案”
- 多轮收敛依赖的不是自由推理，而是 `AgentState + NextHintCalls + 历史结果 + 参数验真`
- running 场景和 failed 场景的关键差别，不在 tool 名称，而在 `phase / hypothesis / openQuestions` 的演化方向不同

### 下一步建议

1. 继续把 `observe` 阶段单独拆开
   - 看 `RuleObserver` 和 `LLMObserver` 各自怎么判断 `done / continue`
   - 看它们如何产出 `NextHintCalls`

2. 补齐 `RoundSummary / WorkflowResult` 的口语化表达
   - 重点回答“一轮如何收敛”“为什么停止”“degraded 是什么含义”

3. 进一步压缩 `AgentLoop` 面试表述
   - 30 秒版本：解释它是什么
   - 1 分钟版本：解释一轮 `Plan -> Act -> Observe` 如何工作
