# Ingestion Module Goal

更新时间：2026-05-01

本文档用于维护 `ingestion` 模块的目标、定位、用户交互方式、阶段边界和架构结论。  
后续只要 ingestion 方向发生变化，都应优先更新这份文档，而不是把结论散落在聊天记录里。

## 1. 模块定位

`ingestion` 不是“知识库上传页的附属功能”，而是一个独立的数据进入系统。

它主要解决的问题是：

- 数据从哪里来
- 数据如何被处理
- 处理过程如何配置
- 每次执行发生了什么
- 最终产出了什么可供下游消费的结果

因此，`ingestion` 更适合被定义为：

`可配置的数据处理流水线系统`

而不是单纯的文件上传能力。

## 2. 用户交互方式

基于当前前端页面和接口定义，用户与 ingestion 的交互分为两条主线。

### 2.1 管理 Pipeline

管理员先进入后台的“数据通道”页面，管理 pipeline：

- 查看 pipeline 列表
- 新建 pipeline
- 编辑 pipeline
- 删除 pipeline
- 查看 pipeline 节点配置

当前前端已经把 pipeline 视为一个“节点序列定义”，节点之间通过 `nextNodeId` 串联，必要时支持 `condition`。

### 2.2 发起 Task

有了 pipeline 之后，管理员再基于 pipeline 创建 task：

- 选择 pipeline
- 选择 source type
- 填来源地址，或直接上传文件
- 提交 task
- 查看 task 详情和节点执行日志

当前前端已经支持的 source type 包括：

- `file`
- `url`
- `feishu`
- `s3`

这说明 ingestion 的交互模型应当是：

`配置流程 -> 发起异步任务 -> 查看运行结果`

而不是“同步上传后立刻得到最终产物”。

## 3. 当前前端已经定义出的对象模型

根据 `frontend/src/services/ingestionService.ts`，前端已经预期存在以下核心对象：

### 3.1 Pipeline

字段重点：

- `id`
- `name`
- `description`
- `createdBy`
- `nodes`
- `createTime`
- `updateTime`

### 3.2 PipelineNode

字段重点：

- `nodeId`
- `nodeType`
- `settings`
- `condition`
- `nextNodeId`

### 3.3 Task

字段重点：

- `id`
- `pipelineId`
- `sourceType`
- `sourceLocation`
- `sourceFileName`
- `status`
- `chunkCount`
- `errorMessage`
- `metadata`
- `startedAt`
- `completedAt`
- `createdBy`

### 3.4 TaskNode

字段重点：

- `id`
- `taskId`
- `pipelineId`
- `nodeId`
- `nodeType`
- `nodeOrder`
- `status`
- `durationMs`
- `message`
- `errorMessage`
- `output`

## 4. 第一阶段目标

第一阶段不追求复杂 DAG、分布式调度或插件化工作流，而是先把 ingestion 从“页面概念”落成“真实模块”。

### 第一阶段必须完成

#### 1. Pipeline 管理
- pipeline CRUD
- 节点定义持久化
- 节点配置校验
- pipeline 详情查询

#### 2. Task 管理
- 创建 task
- 分页查看 task
- 查看 task 详情
- 查看 task node 日志

#### 3. 最小可执行 pipeline
第一版至少打通：

- `fetcher`
- `parser`
- `chunker`
- `indexer`

这是最小可执行闭环。

#### 4. 最小观测能力
- task 状态
- task node 状态
- duration
- error message
- 基础输出日志

## 5. 第一阶段明确不做

为了控制范围，第一阶段先不做：

- 复杂 DAG 分支
- 节点并行执行
- retry / dead-letter / 补偿机制
- 插件化节点市场
- 动态节点脚本执行
- 跨进程分布式调度

## 6. 与 Knowledge 的关系

当前明确结论：

`ingestion` 应作为独立模块存在，不继续塞回 `knowledge` service 内部。

合理关系应当是：

- `ingestion` 负责数据进入、处理和任务执行
- `knowledge` 负责知识库、文档、chunk、检索
- `knowledge` 可以消费 `ingestion` 的执行结果

后续当 `knowledge document processMode = pipeline` 真正闭环时，合理路径应为：

`KnowledgeDocument -> IngestionTask -> Pipeline 执行 -> 产出 chunks -> 回写或关联到 knowledge`

而不是在 `knowledge` 内部再复制一套 fetch/parse/chunk/index 逻辑。

## 7. 推荐的后端架构

建议新增独立模块目录：

```text
internal/app/ingestion/
  domain/
  port/
  service/

internal/adapter/repository/postgres/ingestion/
internal/adapter/http/ingestion/
internal/bootstrap/ingestion/
```

### 7.1 推荐的核心实体

第一版至少包含：

- `Pipeline`
- `Task`
- `TaskNode`

`PipelineNode` 第一版可以直接作为 `Pipeline.nodesJson` 的一部分存储，不强制拆表。

### 7.2 推荐的服务分层

建议拆成：

- `PipelineService`
  - CRUD
  - 节点定义校验
- `TaskService`
  - 创建 task
  - 查询 task / nodes
  - 更新 task 状态
- `ExecutorService`
  - 真正执行 pipeline
- `SourceService`
  - 统一处理 file/url/feishu/s3 输入
- `NodeRunner`
  - 按节点类型分别执行

### 7.3 推荐的执行模型

第一版建议采用：

`Create Task -> 落库 -> 同进程异步执行 -> 持续写 task_node -> 更新 task`

先做成单进程可执行闭环，后续再视需要演进到 MQ / worker。

## 8. 节点类型的第一阶段理解

当前前端已经预期存在：

- `fetcher`
- `parser`
- `enhancer`
- `chunker`
- `enricher`
- `indexer`

推荐处理方式：

### 第一阶段强落地
- `fetcher`
- `parser`
- `chunker`
- `indexer`

### 第一阶段弱落地或占位
- `enhancer`
- `enricher`

原因是前四者决定最小主链路能否闭合，后两者更偏智能增强能力。

## 9. 对 EINO 的结论

当前结论是：

`EINO 适合做 ingestion 的执行编排内核，但不适合直接承担整个 ingestion 模块。`

### 适合 EINO 的部分

- pipeline executor
- node orchestration
- callback / tracing
- 中断与恢复预留

### 不适合完全交给 EINO 的部分

- pipeline CRUD
- task 分页与详情
- 文件上传
- 权限
- 业务持久化

### 当前建议

如果要接 EINO，推荐方式是：

`TaskService -> Executor -> EINO Workflow/Graph -> Node Runners`

也就是：

- 业务外壳自己做
- 执行编排层再评估接入 EINO

第一阶段如果目标是尽快出最小闭环，可以先不强依赖 EINO；如果明确后续会快速扩展 `enhancer / enricher / 智能节点`，则可以从 executor 层开始局部引入。

## 10. 当前推荐实施顺序

### P0
- 建立 ingestion 模块目录骨架
- 落表 `pipeline / task / task_node`
- 补最小 HTTP 接口

### P0
- 打通 `fetcher -> parser -> chunker -> indexer`
- 让 task 真正可执行

### P1
- 把 `knowledge processMode = pipeline` 接到 ingestion

### P1
- 再评估 enhancer / enricher 的 AI 化增强
- 视复杂度决定是否在 executor 层引入 EINO

## 11. 维护建议

后续每次 ingestion 方向有明确结论时，优先更新以下部分：

1. `模块定位`
2. `第一阶段目标`
3. `与 Knowledge 的关系`
4. `对 EINO 的结论`
5. `当前推荐实施顺序`

如果未来第一阶段已经完成，建议把这份文档继续扩展成：

- `ingestion_module_goal.md`：目标与边界
- `ingestion_execution_design.md`：执行与技术设计

这样可以保持目标文档长期可维护，而不是把所有设计细节都堆进同一份文件。
