# Ingestion 流水线与执行架构

更新时间：2026-05-14

## 概览

`ingestion` 不是一个“上传文件然后顺手切 chunk”的小功能，而是项目里一套独立的轻量工作流系统。它负责把外部文档、URL、飞书文档、对象存储里的内容，统一加工成知识库可消费的 `knowledge chunks + vectors`，最终支撑后续的 `retrieve` 命中效果。

如果只用一句话概括它的职责，可以这样说：

- `ingestion` 以 `Pipeline` 作为流程模板，以 `Task` 作为一次执行实例，以 `TaskNode` 作为每一步执行记录，由 `ExecutorService` 调度执行，由 `NodeRunner` 插件完成具体节点逻辑，最终把结果回写到知识库和向量库。

从阅读角度，最适合按下面这条主线理解：

1. HTTP 创建 ingestion task
2. `TaskService` 先把 task 落库，再提交给 executor
3. `ExecutorService` 把 pipeline 构造成 workflow 并异步执行
4. `TaskObserver` 负责把状态和指标沉淀下来
5. `Fetcher / Parser / Chunker / Indexer` 依次处理数据
6. `Indexer` 把最终 chunk 写入 knowledge 和 vector store

## 它在系统里的位置

可以把整个项目粗分成两条主链：

- `chat / rag / agent`
  负责接住用户问题、检索、工具调用、流式回答
- `knowledge / ingestion`
  负责把原始内容加工成可检索的知识资产

如果说 `RagChatService` 是“回答问题的主编排器”，那么 `ingestion` 就是“给检索准备弹药的生产线”。

这也是为什么 `retrieve` 命不命中，不只取决于检索器本身，还强依赖 ingestion 有没有把内容正确抓取、解析、切块、写索引。

## 入口层：任务是怎么进来的

HTTP 入口主要在 [task_handler.go](D:/goagent/internal/adapter/http/ingestion/task_handler.go:89)。

最关键的接口有 4 个：

- `Create`：[task_handler.go:147](D:/goagent/internal/adapter/http/ingestion/task_handler.go:147)
- `Upload`：[task_handler.go:173](D:/goagent/internal/adapter/http/ingestion/task_handler.go:173)
- `Get`：[task_handler.go:119](D:/goagent/internal/adapter/http/ingestion/task_handler.go:119)
- `ListNodes`：[task_handler.go:133](D:/goagent/internal/adapter/http/ingestion/task_handler.go:133)

其中最重要的是前两个：

- `Create`
  适合调用方已经准备好了 `pipelineId + source`
- `Upload`
  适合用户直接上传文件，handler 会先把文件落成临时源，再统一走 task 创建

这两个入口最后都会汇聚到 [TaskService.Create](D:/goagent/internal/app/ingestion/service/service_task.go:163)。

这个收束点非常重要，因为它说明 HTTP 层不负责执行，只负责：

- 接收请求
- 翻译参数
- 创建 task
- 暴露查询接口

## 核心领域模型

ingestion 的骨架主要由 4 个对象组成。

### 1. `Pipeline`

定义在 [pipeline.go:21](D:/goagent/internal/app/ingestion/domain/pipeline.go:21)。

它表示一条流程模板，最重要的字段是：

- `Nodes []PipelineNode`

而节点定义在 [pipeline.go:33](D:/goagent/internal/app/ingestion/domain/pipeline.go:33)。

它回答的问题是：

- 这类内容要经过哪些处理步骤
- 每一步是什么类型
- 每一步带什么配置

### 2. `Task`

定义在 [task.go:28](D:/goagent/internal/app/ingestion/domain/task.go:28)。

它表示某条 pipeline 的一次真实执行实例，记录：

- 用哪条 pipeline
- 输入源是什么
- 当前状态是什么
- 最终产出了多少 chunk
- 是否失败

### 3. `TaskNode`

定义在 [task.go:47](D:/goagent/internal/app/ingestion/domain/task.go:47)。

它表示这次 task 里某个节点的执行记录，包含：

- `NodeID`
- `NodeType`
- `NodeOrder`
- `Status`
- `DurationMs`
- `ErrorMessage`
- `Output`

其中 `Output` 字段在领域层是 [task.go:58](D:/goagent/internal/app/ingestion/domain/task.go:58) 的 `map[string]any`。

### 4. `ExecutionState`

定义在 [workflow_execution_state.go:41](D:/goagent/internal/app/ingestion/service/workflow_execution_state.go:41)。

它是 ingestion 运行时最容易被低估、但实际上非常关键的对象。它不是数据库实体，而是工作流执行过程中节点之间传递的共享上下文。

它的作用是把这些中间产物串起来：

- `Source`
- `Parsed`
- `Chunks`
- `IndexResult`
- `NodeOutputs`

也就是说，ingestion 的节点之间主要不是靠“回查数据库”传数据，而是靠 `ExecutionState` 顺着 workflow 往后推进。

## 数据表和 chat 模块是不是同一套

不是。

这是 ingestion 里一个非常容易和 `chat / rag` 混淆的点。

ingestion 有自己独立的执行事实表：

- `t_ingestion_task`：[create_ingestion_tables.sql:15](D:/goagent/internal/adapter/repository/postgres/migrations/20260502120000_create_ingestion_tables.sql:15)
- `t_ingestion_task_node`：[create_ingestion_tables.sql:37](D:/goagent/internal/adapter/repository/postgres/migrations/20260502120000_create_ingestion_tables.sql:37)

`TaskNode.output` 在表里是 `JSONB`，定义在 [create_ingestion_tables.sql:48](D:/goagent/internal/adapter/repository/postgres/migrations/20260502120000_create_ingestion_tables.sql:48)。

模型层对应字段是 [task_node_model.go:21](D:/goagent/internal/adapter/repository/postgres/ingestion/models/task_node_model.go:21) 的：

```go
Output []byte `gorm:"column:output;type:jsonb"`
```

而领域层映射成 [task.go:58](D:/goagent/internal/app/ingestion/domain/task.go:58) 的：

```go
Output map[string]any
```

两者之间的转换在 mapper 里完成：

- 写入时 marshal：[mapper.go:93](D:/goagent/internal/adapter/repository/postgres/ingestion/mapper.go:93)
- 读取时 unmarshal：[mapper.go:115](D:/goagent/internal/adapter/repository/postgres/ingestion/mapper.go:115)

这套实体和 `rag trace run / trace node` 不是同一套表。更准确地说：

- ingestion 的 `task / task_node` 是业务执行事实
- rag 的 `trace_run / trace_node` 是聊天请求观测事实

名字看起来像，但语义和落表位置都不同。

## `TaskService`：为什么要先落库再执行

`TaskService` 定义在 [service_task.go:49](D:/goagent/internal/app/ingestion/service/service_task.go:49)，构造函数在 [service_task.go:58](D:/goagent/internal/app/ingestion/service/service_task.go:58)。

核心入口是 [service_task.go:163](D:/goagent/internal/app/ingestion/service/service_task.go:163) 的 `Create(...)`。

这个函数最重要的设计点，不是它做了多少校验，而是它的执行顺序：

1. 校验 `pipelineId` 和 source
2. 读取 pipeline
3. 处理 document 级并发约束
4. 构造 `domain.Task`
5. 先写 `taskRepo`
6. 再 `executor.Submit(...)`

这里一定要抓住一句话：

- ingestion 是“先有 task 记录，再异步执行”，不是“执行完再补一条记录”

这带来的价值包括：

- 任务一创建就可查
- 失败时有事实记录可排障
- 支持后台查询、重试、诊断
- 执行器和业务入口解耦

## `ExecutorService`：ingestion 的执行中控

执行器定义在 [executor_workflow.go:32](D:/goagent/internal/app/ingestion/service/executor_workflow.go:32)，构造函数在 [executor_workflow.go:53](D:/goagent/internal/app/ingestion/service/executor_workflow.go:53)。

入口是 [executor_workflow.go:93](D:/goagent/internal/app/ingestion/service/executor_workflow.go:93) 的 `Submit(...)`。

### 它负责什么

`ExecutorService` 不是业务 runner，本质上是调度器。它负责：

- 把 pipeline 构造成 workflow
- 控制 task 级并发
- 异步启动执行 goroutine
- 按顺序调度节点
- 统一处理节点重试和退避
- 统一处理 panic、关闭、收尾

### 为什么说它像轻量 workflow engine

因为它内部同时具备这些典型工作流能力：

- workflow 构建：`BuildWorkflow(...)` 在 [executor_workflow.go:120](D:/goagent/internal/app/ingestion/service/executor_workflow.go:120)
- 状态初始化：`newExecutionState(...)` 在 [executor_workflow.go:173](D:/goagent/internal/app/ingestion/service/executor_workflow.go:173)
- 异步调度：`startWorkflow(...)` 在 [executor_workflow.go:182](D:/goagent/internal/app/ingestion/service/executor_workflow.go:182)
- panic 兜底：`handleWorkflowPanic(...)` 在 [executor_workflow.go:233](D:/goagent/internal/app/ingestion/service/executor_workflow.go:233)
- 主循环：`runWorkflow(...)` 在 [executor_workflow.go:259](D:/goagent/internal/app/ingestion/service/executor_workflow.go:259)

### 它的并发控制怎么做

并发上限来自内部 `slots chan struct{}`，并通过 [executor_workflow.go:464](D:/goagent/internal/app/ingestion/service/executor_workflow.go:464) 的 `MaxConcurrent()` 暴露出去。

这说明当前并发控制粒度是：

- task 级并发

不是 node 级并发。

### 节点重试为什么放在 executor 层

`runWorkflow(...)` 里统一包裹了节点执行重试逻辑，而不是把重试散在每个 runner 内部。

这个取舍非常重要，因为它带来三个直接好处：

- 所有节点的重试策略一致
- observer 和 metrics 能统一看到 retry 事件
- runner 只需要关心“这次执行成功还是失败”

## WorkflowBuilder 和 NodeRunnerRegistry

当前 workflow builder 是线性的。

- builder 类型：[workflow_builder_linear.go:17](D:/goagent/internal/app/ingestion/service/workflow_builder_linear.go:17)
- 构造函数：[workflow_builder_linear.go:20](D:/goagent/internal/app/ingestion/service/workflow_builder_linear.go:20)
- 构建入口：[workflow_builder_linear.go:25](D:/goagent/internal/app/ingestion/service/workflow_builder_linear.go:25)

这意味着当前执行模型还是：

- `fetcher -> parser -> chunker -> indexer`

但架构上它没有把顺序直接写死在 executor 里，而是先经过 `WorkflowBuilder`，这为以后升级到更复杂的 builder 留下了边界。

节点插件分发则由 `NodeRunnerRegistry` 完成：

- 类型定义：[workflow_node_runner_registry.go:16](D:/goagent/internal/app/ingestion/service/workflow_node_runner_registry.go:16)
- 构造函数：[workflow_node_runner_registry.go:21](D:/goagent/internal/app/ingestion/service/workflow_node_runner_registry.go:21)
- 注册入口：[workflow_node_runner_registry.go:32](D:/goagent/internal/app/ingestion/service/workflow_node_runner_registry.go:32)

这个 registry 的价值在于：

- executor 不需要知道 fetcher、parser、chunker、indexer 的细节
- 只要按 `nodeType` 找对应 runner 即可

所以 ingestion 的扩展方式很清晰：

1. 定义新 `nodeType`
2. 实现新 `NodeRunner`
3. 注册进 registry

## Observer：为什么执行和落库要拆开

observer 的核心接口在 [observer_task_repository.go:28](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:28)。

它定义了 5 个生命周期事件：

- `OnTaskStarted`
- `OnTaskCompleted`
- `OnNodeStarted`
- `OnNodeRetry`
- `OnNodeCompleted`

这层设计的核心价值是：

- executor 负责“跑”
- observer 负责“记”

也就是说，执行主循环不会把：

- task 状态更新
- task node 状态更新
- metrics 聚合

硬编码在一起，而是在关键时刻发事件，让 observer 去消费。

### `RepositoryTaskObserver`

定义在 [observer_task_repository.go:37](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:37)。

几个关键回调分别在：

- task started：[observer_task_repository.go:56](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:56)
- task completed：[observer_task_repository.go:69](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:69)
- node started：[observer_task_repository.go:89](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:89)
- node retry：[observer_task_repository.go:128](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:128)
- node completed：[observer_task_repository.go:162](D:/goagent/internal/app/ingestion/service/observer_task_repository.go:162)

它负责把执行事实写回：

- `t_ingestion_task`
- `t_ingestion_task_node`

这里最值得注意的设计点是：

- 一个 `taskId + nodeId` 只对应一条 `task_node` 聚合记录

数据库唯一约束就在 [create_ingestion_tables.sql:52](D:/goagent/internal/adapter/repository/postgres/migrations/20260502120000_create_ingestion_tables.sql:52)。

这说明系统保存的是：

- 节点级聚合结果

而不是：

- 每次 attempt 一条独立明细

### `MultiTaskObserver`

定义在 [observer_task_multi.go:12](D:/goagent/internal/app/ingestion/service/observer_task_multi.go:12)，构造函数在 [observer_task_multi.go:17](D:/goagent/internal/app/ingestion/service/observer_task_multi.go:17)。

它的作用很简单但很重要：

- 把多个 observer 组合成一个统一 observer

当前 runtime 里，repository observer 和 metrics observer 就是一起挂进去的。

### `MetricsObserver`

指标服务定义在 [observer_metrics.go:91](D:/goagent/internal/app/ingestion/service/observer_metrics.go:91)，快照出口在 [observer_metrics.go:130](D:/goagent/internal/app/ingestion/service/observer_metrics.go:130)。

observer 自身定义在 [observer_metrics.go:209](D:/goagent/internal/app/ingestion/service/observer_metrics.go:209)。

它记录的不只是成功失败，还包括：

- running tasks
- submitted / started / succeeded / failed / canceled
- retries
- node type 级 runs / successes / failures / avgDuration / maxDuration

这说明 ingestion 的可观测性不是“只有日志”，而是已经具备运行态指标面板的雏形。

## `TaskNode.output` 到底是什么

`TaskNode.output` 是 ingestion 里非常值得注意的一个设计。

它的本质不是固定 schema 的业务 DTO，而是：

- runner 的业务输出
- observer 补进去的执行元数据

的混合结构。

常见通用字段包括：

- `retryCount`
- `attemptCount`
- `durationMs`
- `success`
- `lastError`
- `errorCategory`

这个字段的主要消费者不是最终用户，而是：

- repository observer 持久化
- ingestion 查询接口和后台页面
- RAG / Agent 的 ingestion 诊断工具
- 人工排障

它更像“节点执行快照”，而不是 workflow 内部的数据总线。真正的节点间数据传递主线，仍然是 `ExecutionState`。

## `ExecutionState`：为什么它是 ingestion 的数据总线

类型定义在 [workflow_execution_state.go:41](D:/goagent/internal/app/ingestion/service/workflow_execution_state.go:41)。

其中几个关键子结构是：

- `SourcePayload`：[workflow_execution_state.go:10](D:/goagent/internal/app/ingestion/service/workflow_execution_state.go:10)
- `IndexResult`：[workflow_execution_state.go:34](D:/goagent/internal/app/ingestion/service/workflow_execution_state.go:34)

它解决的是一个非常实际的问题：

- fetcher 产出的源内容，parser 怎么拿
- parser 产出的文本，chunker 怎么拿
- chunker 产出的 chunk，indexer 怎么拿

系统的答案不是“每个节点都重新去查数据库”，而是：

- 上一个节点更新 `ExecutionState`
- executor 把新的 state 传给下一个节点

所以你可以把它理解成：

- ingestion workflow 里的共享运行时上下文

## 四个核心 runner

当前 runtime 默认注册 4 个 runner：

- `FetcherNodeRunner`
- `ParserNodeRunner`
- `ChunkerNodeRunner`
- `IndexerNodeRunner`

装配代码在 [runtime.go:95-106](D:/goagent/internal/bootstrap/ingestion/runtime.go:95)。

### 1. `FetcherNodeRunner`

- 类型：[runner_fetcher.go:21](D:/goagent/internal/app/ingestion/service/runner_fetcher.go:21)
- 入口：[runner_fetcher.go:52](D:/goagent/internal/app/ingestion/service/runner_fetcher.go:52)

它的核心职责不是“读文件”，而是：

- 把不同来源统一归一化成 `SourcePayload`

当前支持的来源包括：

- file
- url
- feishu
- s3 / storage-backed source

此外它还支持飞书客户端注入，入口在 [runner_fetcher.go:39](D:/goagent/internal/app/ingestion/service/runner_fetcher.go:39)。

### 2. `ParserNodeRunner`

- 类型和入口：[runner_parser.go:31](D:/goagent/internal/app/ingestion/service/runner_parser.go:31)

它消费 `state.Source`，产出 `state.Parsed`，并返回结构化 output。

当前 output 里最常见的字段包括：

- `parserType`
- `contentLength`
- `title`
- `placeholder`

对应构造位置在 [runner_parser.go:89](D:/goagent/internal/app/ingestion/service/runner_parser.go:89)。

### 3. `ChunkerNodeRunner`

- 类型和入口：[runner_chunker.go:31](D:/goagent/internal/app/ingestion/service/runner_chunker.go:31)

它消费 `state.Parsed`，产出 `state.Chunks`。

切块策略从 [runner_chunker.go:41](D:/goagent/internal/app/ingestion/service/runner_chunker.go:41) 开始读取，默认是 fixed size。

output 里最常见的字段在 [runner_chunker.go:67](D:/goagent/internal/app/ingestion/service/runner_chunker.go:67)：

- `strategy`
- `chunkCount`
- `placeholder`

### 4. `IndexerNodeRunner`

- 类型：[runner_indexer.go:29](D:/goagent/internal/app/ingestion/service/runner_indexer.go:29)
- 入口：[runner_indexer.go:59](D:/goagent/internal/app/ingestion/service/runner_indexer.go:59)

这是 ingestion 里最像生产逻辑的一个 runner，也是整个链路最值得深挖的地方。

## `IndexerNodeRunner`：最值得深挖的节点

这个 runner 的职责可以概括成一句话：

- 它把 `state.Chunks` 变成真正的 `knowledge chunks + vectors`，并通过内容复用、整文档替换和失败补偿来保证索引写入的一致性和可恢复性。

### 它依赖什么

结构体字段集中在 [runner_indexer.go:29](D:/goagent/internal/app/ingestion/service/runner_indexer.go:29) 附近，核心依赖是：

- `baseRepo`
- `chunkRepo`
- `vectorStore`
- `embedding`

这已经说明它不只是“写向量”，而是同时连接了：

- 知识库配置
- chunk 持久化
- 向量写入
- embedding 服务

### `Run(...)` 的主线

入口在 [runner_indexer.go:59](D:/goagent/internal/app/ingestion/service/runner_indexer.go:59)。

它的大致执行顺序是：

1. 校验必须有 `state.Chunks`
2. 解析 `target`
3. 解析 `knowledgeBaseId`
4. 解析 `documentId / documentName`
5. 解析 `embeddingModel`
6. 做 embedding
7. 计算 `contentFingerprint`
8. 准备补偿逻辑
9. 构建 knowledge chunks 和 vector chunks
10. 写 knowledge / vector store
11. 回写 `IndexResult`

其中几个关键字段解析位置分别在：

- `target`：[runner_indexer.go:70](D:/goagent/internal/app/ingestion/service/runner_indexer.go:70)
- `knowledgeBaseID`：[runner_indexer.go:78](D:/goagent/internal/app/ingestion/service/runner_indexer.go:78)
- `documentID`：[runner_indexer.go:87](D:/goagent/internal/app/ingestion/service/runner_indexer.go:87)
- `documentName`：[runner_indexer.go:92](D:/goagent/internal/app/ingestion/service/runner_indexer.go:92)
- `embeddingModel`：[runner_indexer.go:99](D:/goagent/internal/app/ingestion/service/runner_indexer.go:99)

### 为什么 embedding 放在 indexer 而不是 chunker

embedding 逻辑在 [runner_indexer.go:227](D:/goagent/internal/app/ingestion/service/runner_indexer.go:227)。

这个边界很合理，因为：

- chunker 负责把文本结构化切开
- indexer 负责为索引写入做准备

embedding 更接近索引写入前处理，而不是文本切分本身。

### `contentFingerprint` 是干什么的

计算在 [runner_indexer.go:125](D:/goagent/internal/app/ingestion/service/runner_indexer.go:125)，hash helper 在 [runner_indexer.go:357](D:/goagent/internal/app/ingestion/service/runner_indexer.go:357)。

它的作用不是直接替代 chunk 对比，而是提供一个：

- 能快速描述“这次 chunk 集合内容版本”的摘要

这对排障和幂等对比都很有帮助。

### 失败补偿为什么很重要

补偿逻辑集中在 [runner_indexer.go:128-163](D:/goagent/internal/app/ingestion/service/runner_indexer.go:128)。

它会在失败时根据实际副作用清理：

- 已写 knowledge chunks，就删 chunks
- 已写 vectors，就删 vectors

这说明它不是“失败了就报错”，而是显式考虑了：

- chunk 写了一半
- vector 写了一半
- 下次重试怎么尽量从干净状态恢复

这是这个模块明显偏生产化而不是 demo 化的地方。

### vector chunks 是怎么构造的

真正的构造点在 [runner_indexer.go:165](D:/goagent/internal/app/ingestion/service/runner_indexer.go:165)，调用的是 [runner_indexer.go:291](D:/goagent/internal/app/ingestion/service/runner_indexer.go:291) 的 `buildVectorChunks(...)`。

每个向量单元至少包含：

- `ChunkID`
- `KnowledgeBaseID`
- `DocumentID`
- `DocumentName`
- `ChunkIndex`
- `Content`
- `Embedding`
- `Metadata`

其中 metadata 的构造逻辑在 [runner_indexer.go:316](D:/goagent/internal/app/ingestion/service/runner_indexer.go:316) 的 `buildIndexMetadata(...)`。

默认会写入：

- `task_id`
- `document_id`
- `document_name`
- `knowledge_base_id`
- `source_type`
- `source_location`
- `source_file_name`
- `source_content_type`
- `chunk_index`

此外还会合并 chunk metadata 和 task metadata 里显式指定的字段。

### knowledge chunks 为什么支持 `reuse`

knowledge chunks 的构造在 [runner_indexer.go:244](D:/goagent/internal/app/ingestion/service/runner_indexer.go:244)。

复用判断在 [runner_indexer.go:267](D:/goagent/internal/app/ingestion/service/runner_indexer.go:267) 的 `knowledgeChunksMatch(...)`。

它会比较：

- 数量
- `ID`
- `DocumentID`
- `KnowledgeBaseID`
- `ChunkIndex`
- `ContentHash`
- `Content`

真正单个 chunk 的相等判断在 [runner_indexer.go:369](D:/goagent/internal/app/ingestion/service/runner_indexer.go:369)。

这说明当前复用策略是：

- 只有完全一致才 `reuse`
- 否则整体 `replace`

这种判断保守，但安全，也更容易解释。

### 为什么 vector 当前统一 `replace`

knowledge 分支和向量分支在 `Run(...)` 后半段展开：

- knowledge chunk 分支：[runner_indexer.go:166-188](D:/goagent/internal/app/ingestion/service/runner_indexer.go:166)
- vector 写入分支：[runner_indexer.go:189-196](D:/goagent/internal/app/ingestion/service/runner_indexer.go:189)

这里最重要的取舍是：

- knowledge chunk 支持 `reuse / replace`
- vector 当前始终 `replace`

原因并不复杂：

- vector 做整文档删除再 upsert，更容易保证一致性
- 脏向量残留的风险更低
- 当前阶段实现成本更可控

所以这是一个非常典型的工程折中：

- chunk 层尽量复用，减少无意义重写
- vector 层优先保证状态简单和一致

### 最后怎么把索引结果带回 workflow

回写发生在 [runner_indexer.go:198-209](D:/goagent/internal/app/ingestion/service/runner_indexer.go:198)。

最终会设置 `next.IndexResult`，至少包含：

- `Target`
- `ChunkCount`
- `knowledgeBaseId`
- `documentId`
- `documentName`
- `embeddingModel`
- `chunkWriteMode`

所以 indexer 的结果有两份沉淀：

- 一份进入 `task_node.output`
- 一份进入 `ExecutionState.IndexResult`

这也再次体现出：

- `output` 更偏排障快照
- `ExecutionState` 更偏 workflow 运行态数据

## Runtime 装配：为什么它能说明整体架构

如果只想看 ingestion 最完整的装配图，最推荐直接看 [runtime.go:42](D:/goagent/internal/bootstrap/ingestion/runtime.go:42) 的 `NewRuntime(...)`。

这里把整个模块如何拼起来写得很清楚：

1. 建库和表检查
2. 创建 pipeline/task/taskNode repo
3. 创建 knowledge base / chunk repo
4. 创建 metrics service
5. 创建 repository observer + metrics observer
6. 创建 fetcher / parser / chunker / indexer
7. 创建 registry
8. 创建 executor
9. 创建 `PipelineService` 和 `TaskService`

几个最关键的装配点分别在：

- metrics service：[runtime.go:75](D:/goagent/internal/bootstrap/ingestion/runtime.go:75)
- observer 组合：[runtime.go:76-78](D:/goagent/internal/bootstrap/ingestion/runtime.go:76)
- fetcher：[runtime.go:95](D:/goagent/internal/bootstrap/ingestion/runtime.go:95)
- parser / chunker / indexer 注册：[runtime.go:102-104](D:/goagent/internal/bootstrap/ingestion/runtime.go:102)
- executor：[runtime.go:106](D:/goagent/internal/bootstrap/ingestion/runtime.go:106)

如果你需要一句面试表达，可以直接说：

- `runtime.go` 展示了 ingestion 的完整装配方式，它不是单个 service，而是一整套由 repo、observer、runner、executor 和 service 共同组成的运行时。

## 值得特别注意的设计细节

这里是这个模块最值得记住、也最容易被追问的点。

### 1. 它本质是轻量 workflow 系统

不是导入脚本，不是单个 service，不是“上传后同步处理”的 handler。

### 2. `Pipeline / Task / TaskNode` 三层模型很关键

- `Pipeline` 是模板
- `Task` 是实例
- `TaskNode` 是步骤执行记录

这三层分清，整个 ingestion 架构就不会乱。

### 3. `ExecutionState` 是节点间数据流动中心

这决定了 ingestion 不是通过数据库回查串联节点，而是通过运行态上下文向前推进。

### 4. 重试统一在 executor 层

这是一种可观测性更好、行为更一致的实现方式。

### 5. `TaskNode.output` 是排障快照，不是强类型 API 契约

它牺牲了一部分类型严格性，换来了极强的灵活性和排障价值。

### 6. `IndexerNodeRunner` 已经有明显生产化意识

尤其体现在：

- content fingerprint
- knowledge chunk reuse
- vector replace
- failure compensation

### 7. ingestion 和 rag/chat 用的是两套不同的 task/node 实体

名字相似，但表、职责、使用场景都不同。

## 预测面试题

下面这些问题非常值得提前准备。

### 1. ingestion 整体架构是什么

建议回答：

- ingestion 本质上是一套轻量工作流系统。`Pipeline` 定义流程模板，`Task` 表示一次执行实例，`TaskNode` 记录每一步执行结果。`TaskService` 负责创建任务并提交，`ExecutorService` 负责 workflow 调度和重试，`NodeRunner` 负责具体业务节点，`TaskObserver` 负责状态持久化和指标统计，最终形成从 source 到 knowledge/vector 的完整导入闭环。

### 2. 为什么 task 要先落库再执行

建议回答：

- 因为 ingestion 是异步任务系统。先落库能保证任务一创建就可查，也更适合状态追踪、失败排障、重试恢复和后台运营，而不是把执行结果当成临时副产品。

### 3. ingestion 的 task/node 和 chat 里的 task/node 是一回事吗

建议回答：

- 不是。ingestion 的 `task / task_node` 是导入执行事实，落在 `t_ingestion_task / t_ingestion_task_node`；chat/rag 里的 trace run/node 是请求观测事实，是另一套实体和表。

### 4. 为什么 `ExecutionState` 很重要

建议回答：

- 因为它是 workflow 节点之间的数据总线。fetcher、parser、chunker、indexer 不是靠数据库反查串联，而是通过共享 state 顺序推进，这让执行链更清晰，也更容易统一收尾。

### 5. 为什么 retry 放在 executor，不放在 runner

建议回答：

- 统一重试可以让所有节点遵循一致的重试、退避和取消语义，也更方便 observer 和 metrics 统一记录。如果把重试散在各 runner 里，会让可观测性和行为一致性变差。

### 6. `task_node.output` 为什么要做成半结构化

建议回答：

- 因为不同节点的业务输出天生不一样，fetcher、parser、chunker、indexer 关注点完全不同。把它设计成 `map[string]any + JSONB` 可以让 runner 自由扩展业务结果，同时又允许 observer 统一补 retry、duration、error 这些执行元数据，特别适合后台排障。

### 7. indexer 为什么 chunk 可以复用，vector 却统一 replace

建议回答：

- chunk 复用能减少无意义重写，但 vector 层更重视一致性和脏数据控制。整文档 delete + upsert 更容易保证状态简单可靠，是当前阶段更稳妥的取舍。

## 一句话总结

如果需要把整个模块压缩成一句话，可以这样说：

- ingestion 是项目里的知识导入工作流系统，它从多种 source 抓取内容，经过 parser 和 chunker 生成结构化 chunk，再由 indexer 写入 knowledge 和 vector store，同时通过 task/task_node、observer 和 metrics 形成可追踪、可诊断、可恢复的执行闭环。
