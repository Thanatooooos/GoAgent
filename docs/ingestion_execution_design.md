# Ingestion Execution Design

更新时间：2026-05-02

本文档承接 `docs/ingestion_module_goal.md`，聚焦第一阶段 ingestion 的实施设计。
目标不是一次定义完整终态，而是明确现在就要落地的职责拆分、用户交互、模块关系与骨架边界。

## 1. 功能定位

`ingestion` 是独立的数据进入模块，不是 `knowledge` 上传能力的别名。

第一阶段它负责四件事：

- 定义可配置的 pipeline
- 基于 pipeline 创建异步 task
- 执行最小处理链路
- 记录 task 与 task node 运行结果

它不直接承担知识库业务语义，不直接提供问答能力，也不负责替代 `knowledge` 和 `rag`。

## 2. 用户交互方式

第一阶段用户交互分成两条主线。

### 2.1 Pipeline 管理

管理员在后台维护 pipeline：

- 查看 pipeline 列表
- 新建 pipeline
- 编辑 pipeline
- 删除 pipeline
- 查看节点定义

当前 pipeline 本质上是一个“节点序列定义”：

- 节点通过 `nextNodeId` 串联
- 可保留 `condition` 字段，但第一阶段不执行复杂分支
- 节点配置由后端做基础校验

### 2.2 Task 执行

管理员基于 pipeline 发起 task：

- 选择 pipeline
- 选择 source type
- 填 source location，或上传文件
- 提交 task
- 查看 task 状态与 task node 日志

这意味着 ingestion 的产品模型是：

`配置流程 -> 发起异步任务 -> 查看执行结果`

而不是同步接口风格。

## 3. 与其他模块的关系

### 3.1 与 Knowledge 的关系

`knowledge` 负责知识库、文档、chunk、向量与检索能力。

`ingestion` 负责：

- 数据如何进入系统
- 进入后按什么流程处理
- 处理执行过程如何观测

第一阶段不直接把 ingestion 做成 knowledge 的子 service，而是保持独立模块。

后续合理的集成路径是：

`KnowledgeDocument -> IngestionTask -> Pipeline 执行 -> 产出 chunks / index result -> 回写或关联到 knowledge`

这样可以避免在 `knowledge` 内部重复复制 fetch、parse、chunk、index 流程。

### 3.2 与 RAG / Trace 的关系

`rag` 当前已经具备最小 trace 查询闭环。

ingestion 第一阶段不强行复用 `rag_trace_run / rag_trace_node` 表模型，而是先在自己的 `task / task_node` 维度记录执行状态。
但在观测设计上应复用同一套思路：

- run 级状态
- node 级状态
- duration
- error message
- 输出摘要

后续如果需要统一观测入口，再评估是否抽象通用 trace 模型。

### 3.3 与 MQ / Worker 的关系

第一阶段不强依赖 MQ。

推荐先落成：

`Create Task -> 落库 -> 同进程异步执行 -> 持续写 task_node -> 回写 task`

等最小闭环稳定后，再决定是否演进到 MQ / worker。

## 4. 第一阶段的最小执行链路

第一阶段只要求强落地四类节点：

- `fetcher`
- `parser`
- `chunker`
- `indexer`

建议职责如下：

- `fetcher`：根据 source type 统一拿到原始内容或文件引用
- `parser`：把原始内容转成标准文本载荷
- `chunker`：把文本切成 chunk 结果
- `indexer`：把 chunk 结果写入下游存储或索引目标

以下节点先保留为扩展位，不要求第一阶段完成真实执行：

- `enhancer`
- `enricher`

## 5. 推荐分层

第一阶段建议保持与现有项目一致的结构：

```text
internal/app/ingestion/
  domain/
  port/
  service/

internal/adapter/http/ingestion/
internal/bootstrap/ingestion/
```

如果后续开始落库，再补：

```text
internal/adapter/repository/postgres/ingestion/
```

### 5.1 Domain

第一阶段核心实体：

- `Pipeline`
- `PipelineNode`
- `Task`
- `TaskNode`

其中：

- `PipelineNode` 先作为 `Pipeline` 内部聚合对象存在
- 不强制单独拆表
- `TaskNode` 用于记录执行阶段日志

### 5.2 Port

第一阶段推荐先抽象这些端口：

- `PipelineRepository`
- `TaskRepository`
- `TaskNodeRepository`
- `TaskExecutor`

这样可以先把业务骨架站稳，再决定具体落 PostgreSQL、内存实现还是别的执行方式。

### 5.3 Service

第一阶段推荐服务分层：

- `PipelineService`
  - CRUD
  - pipeline 节点基础校验
- `TaskService`
  - 创建 task
  - 查询 task / task nodes
  - 串联 pipeline 校验与 executor 提交
- `ExecutorService`
  - 作为后续真实执行编排入口
  - 当前先提供边界，不在本轮强落完整逻辑

## 6. 当前骨架要解决的问题

本轮代码骨架的目标是：

- 让 ingestion 的领域对象和服务边界先明确下来
- 让 HTTP 契约和前端已有接口定义对齐
- 为后续 repository / runtime / executor 真正落地预留稳定入口

本轮不追求：

- 真正写库
- 真正执行 pipeline
- 把 ingestion 路由接入主程序并对外开放

原因是当前还没有完整 repo 与执行实现，先接入主程序只会暴露“返回 500 的假能力”。

## 7. 下一步落地顺序

建议在当前骨架之上继续推进：

1. 落 PostgreSQL 表结构与 repository 实现
2. 让 `PipelineService` 和 `TaskService` 真正可落库
3. 落最小 `ExecutorService`
4. 接入主程序路由
5. 再把 `knowledge processMode = pipeline` 接回 ingestion
