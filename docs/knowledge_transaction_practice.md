# Knowledge Transaction Practice

更新时间：2026-04-29

这份文档总结 `goagent` 当前 knowledge 模块里已经落地的事务设计、适用边界和这次新增的删除链路/文档状态机收口方案。

重点覆盖：

- 文档切块持久化事务
- 文档 schedule 删除事务
- chunk 增删改启停事务
- 文档删除事务
- 文档状态机

目标不是把所有步骤都塞进数据库事务，而是明确：

- 哪些步骤必须原子化
- 哪些步骤只适合放在事务外
- 哪些跨系统动作必须靠补偿和重试解决

## 1. 当前事务设计总览

项目当前采用的是：

> service 层声明事务函数类型，postgres 适配层提供 tx 版 repository / vector store 实现。

已经落地的事务链路有：

- `DocumentProcessService`
  - 代码：`internal/app/knowledge/service/document_process_service.go`
  - 事务实现：`internal/adapter/repository/postgres/knowledge/document_process_transaction.go`
- `KnowledgeDocumentScheduleService`
  - 代码：`internal/app/knowledge/service/knowledge_document_schedule_service.go`
  - 事务实现：`internal/adapter/repository/postgres/knowledge/knowledge_document_schedule_transaction.go`
- `KnowledgeChunkService`
  - 代码：`internal/app/knowledge/service/knowledge_chunk_service.go`
  - 事务实现：`internal/adapter/repository/postgres/knowledge/knowledge_chunk_transaction.go`
- `KnowledgeDocumentService.Delete`
  - 代码：`internal/app/knowledge/service/knowledge_document_service.go`
  - 事务实现：`internal/adapter/repository/postgres/knowledge/knowledge_document_delete_transaction.go`

共同特点：

- service 层不直接依赖 `gorm.DB.Transaction`
- 事务只覆盖“必须一起提交的库内状态”
- 对象存储、MQ、远程 HTTP 这类跨系统步骤不强行塞进数据库事务

## 2. 为什么要这样设计

knowledge 模块的典型风险不是单表 CRUD，而是跨对象一致性：

- `t_knowledge_document`
- `t_knowledge_chunk`
- `t_knowledge_chunk_vector`
- `t_knowledge_document_schedule`
- `t_knowledge_document_schedule_exec`

如果这些动作拆开提交，就会出现半成功状态，比如：

- chunk 已删，但 vector 还在
- vector 已更新，但 chunk 内容还是旧的
- document 已删，但 schedule / exec 残留
- 文档已从数据库删除，但对象存储文件没删掉

所以当前采用的原则是：

- 同一数据库内必须一起变化的对象，优先做短事务
- 跨系统动作只在事务提交后执行
- 事务外失败要靠补偿思路解决，不靠“伪分布式事务”

## 3. 文档切块事务

`DocumentProcessService` 的事务入口定义在：

- `DocumentChunkPersistenceTransaction`
  - 文件：`internal/app/knowledge/service/document_process_service.go`

postgres 实现：

- `NewDocumentProcessTransaction`
  - 文件：`internal/adapter/repository/postgres/knowledge/document_process_transaction.go`

事务内步骤在 `persistDocumentChunksWithDeps(...)`：

1. 删除旧 chunk
2. 删除旧 vector
3. 批量写入新 chunk
4. 批量写入新 vector
5. 更新 `document.chunk_count`

这里的关键边界是：

- 文本解析、切块、embedding 都在事务外
- 只有最终持久化写入在事务内

这样可以同时兼顾一致性和事务长度。

## 4. schedule 删除事务

`KnowledgeDocumentScheduleService.DeleteByDocID(...)` 使用独立事务清理：

1. 删除 `schedule_exec`
2. 删除 `schedule`

对应代码：

- service：`internal/app/knowledge/service/knowledge_document_schedule_service.go`
- postgres：`internal/adapter/repository/postgres/knowledge/knowledge_document_schedule_transaction.go`

这里的经验是：

- 父子关系明确的库内清理，事务通常比补偿更直接
- service 层显式要求事务依赖存在，比静默降级更安全

## 5. chunk 变更事务

`KnowledgeChunkService` 的事务化已经覆盖：

- `Create`
- `Update`
- `Delete`
- `Enable`
- `BatchToggleEnabled`

事务入口：

- `KnowledgeChunkMutationTransaction`
  - 文件：`internal/app/knowledge/service/knowledge_chunk_service.go`

postgres 实现：

- `NewKnowledgeChunkTransaction`
  - 文件：`internal/adapter/repository/postgres/knowledge/knowledge_chunk_transaction.go`

这部分的原则仍然一致：

- chunk 明细
- vector 明细
- `document.chunk_count`

必须在一个事务里一起收口。

## 6. 这次新增：文档状态机收口

这次对 `KnowledgeDocument` 增加了显式状态迁移约束，核心代码在：

- `internal/app/knowledge/domain/knowledge_document.go`

### 6.1 当前状态集合

当前文档状态为：

- `pending`
- `running`
- `success`
- `failed`
- `deleting`

其中：

- `deleting` 是这次新增的中间态
- 它的用途不是长期展示，而是作为“删除事务占用资格”的显式状态

### 6.2 允许的状态迁移

目前明确允许的迁移有：

- `pending -> running`
- `failed -> running`
- `success -> running`
- `running -> success`
- `running -> failed`
- `pending -> deleting`
- `failed -> deleting`
- `success -> deleting`

明确不允许的迁移有：

- `running -> deleting`
- `deleting -> running`
- `deleting -> success`
- `deleting -> failed`

### 6.3 这次做了哪几步

这次状态机收口主要做了 3 步：

1. 在 domain 层补 `KnowledgeDocumentStatusDeleting`
2. 在 domain 层补 `CanKnowledgeDocumentTransition(...)`、`CanDelete()`
3. 把 `StartChunk` / schedule 占用运行状态 / delete 前置判断都统一收口到有限状态集合，而不是继续用“不是 running 就行”这种宽松条件

### 6.4 得出的约束

最终约束是：

- `running` 表示文档正在被真实处理，不能被删除
- `deleting` 表示文档已经被删除事务占用，不能重新进入处理态
- 允许进入 `running` 的只有稳定态：`pending / failed / success`
- 允许进入 `deleting` 的也只有稳定态：`pending / failed / success`

这让“开始处理”和“开始删除”两类互斥动作都变成了显式状态迁移，而不是散落在各 service 里的条件判断。

## 7. 这次新增：文档删除事务收口

这是这次最重要的调整。

### 7.1 调整前的问题

旧的 `KnowledgeDocumentService.Delete` 是顺序删除：

1. 删 schedule
2. 删 vector
3. 删 chunk
4. 删 document
5. 删对象存储文件

问题在于：

- 这些步骤没有统一事务
- 中途失败会留下半成功状态
- 删除资格没有显式占用，和 `StartChunk` 存在并发竞争窗口

### 7.2 调整后的步骤

新的 `KnowledgeDocumentService.Delete` 改成了：

#### 第一步：事务内占用删除资格

先用条件更新把文档状态从：

- `pending`
- `failed`
- `success`

原子地改成：

- `deleting`

如果更新不到任何行，直接认为：

- 文档正在运行
- 文档已删除
- 或状态已不允许删除

这样删除资格就被事务内原子占用。

#### 第二步：事务内清库内关联数据

在同一个事务里继续完成：

1. 删除 `schedule_exec`
2. 删除 `schedule`
3. 删除 document 对应 vector
4. 删除 document 对应 chunk
5. 软删 document

这里数据库与 pgvector 仍然在同一个 Postgres 事务里提交。

#### 第三步：事务提交后删除对象存储文件

只有当事务成功提交后，才执行：

1. 删除 `document.file_url` 对应对象存储文件

这个步骤明确放在事务外。

### 7.3 新边界的意义

这样重构后，删除链路被拆成两层：

#### 事务内

保证以下对象一起成功或一起失败：

- `document.status=deleting` 占用资格
- `schedule_exec`
- `schedule`
- `chunk`
- `vector`
- `document.deleted`

#### 事务外

处理外部对象存储文件清理：

- `storage.Delete(document.file_url)`

### 7.4 这次重构得出的结论

这次 delete 重构最终确认了一条很重要的实践：

> 文档删除的“主状态一致性”属于数据库事务问题；文件删除属于事务外补偿问题。

也就是说：

- 数据库里是否还存在 document / chunk / vector / schedule，必须强一致
- 对象存储文件是否立即删掉，不适合硬塞进数据库事务

### 7.5 当前仍保留的现实边界

这次虽然已经把删除主链路收紧了，但仍然保留一个明确边界：

- 如果数据库事务已经提交成功，但对象存储删除失败，当前接口会返回错误
- 此时数据库主状态已经收口，对象存储残留文件需要靠后续补偿/重试治理

这正是后续应该继续补的地方，但主一致性已经从“跨系统顺序 best effort”提升成了“库内强一致 + 库外待补偿”。

## 8. 这套事务设计的前提

当前事务能够覆盖 `vector`，前提是：

- 当前向量实现还是 `pgvector`
- `t_knowledge_chunk_vector` 与业务表共用同一个 Postgres 事务连接

如果未来切到外部向量库，例如：

- Milvus
- Qdrant
- ES 向量索引

那么就必须切换成：

- 本地事务只保证业务表一致
- 外部向量写入改为异步修复、补偿和重试

这个前提在架构层必须始终记住。

## 9. 当前推荐的事务操作规范

结合现有源码，后续建议继续统一遵循下面这些规则。

### 9.1 service 定义事务接口，adapter 提供实现

推荐做法：

- 在 service 层定义函数类型
- 在 postgres 层实现 `NewXxxTransaction(db)`
- 在 runtime 装配到 service

不推荐：

- service 直接拿 `gorm.DB`
- service 自己写 `Begin/Commit/Rollback`

### 9.2 长耗时步骤不进事务

例如：

- 文件读取
- 文本解析
- chunk 切分
- embedding
- 远程文件抓取
- 对象存储删除

这些步骤应该放在事务外。

### 9.3 事务内依赖必须显式替换成 tx 版本

不要在事务回调里继续偷用 service 原本持有的 repo/store。

正确方式是：

- 事务函数把 tx 版 repo/store 作为参数传入
- service 内部调用 `xxxWithRepo / xxxWithStore` 版本或直接使用回调参数

### 9.4 状态字段要和核心数据一起提交

像这些字段都属于一致性的一部分：

- `document.status`
- `document.chunk_count`
- `schedule.next_run_time`
- `schedule.enabled`

不能把它们当成“无关紧要的附加字段”延后更新。

## 10. 什么适合事务，什么适合补偿

### 10.1 适合事务

- 同一数据库内的多表写入
- pgvector 与业务表联动
- 父子表同步删除
- 计数与明细必须一致
- 删除资格占用这种状态抢占

### 10.2 更适合补偿或异步修复

- 调用 MQ
- 调用对象存储
- 调用第三方 HTTP
- 未来如果接入外部向量库

原因很简单：

- 这些外部系统通常不在同一个数据库事务里
- 强行做“伪事务”只会制造更难排障的半成功状态

## 11. 现阶段的结论

可以把当前项目里的事务实践归纳成一句话：

> 用短事务保证数据库内一致性，用事务外补偿处理跨系统一致性。

这次 delete 与状态机重构，进一步把这条原则从“经验”落实成了代码边界：

- `running / deleting` 变成显式的互斥状态
- `Delete` 变成显式的“事务内收主状态，事务外收外部资源”

后续 knowledge 模块继续扩展时，建议优先沿用这条原则。
