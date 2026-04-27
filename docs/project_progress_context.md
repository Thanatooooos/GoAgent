# Project Progress Context

更新时间：2026-04-26

## 1. 文档用途

这份文档是 `goagent` 当前阶段的长期进度文档，目标是：

- 让下次对话开始时只需要读取这一份文件，就能快速掌握项目状态
- 记录“代码已经落地了什么”，而不是只记录设想
- 明确当前主线、技术约束、未完成项和下一步任务顺序
- 避免后续推进时重复做已经定过的架构决策

后续如果继续推进 `knowledge` 相关能力，优先先读：

- `docs/project_progress_context.md`

然后再按这里给出的“下一步任务清单”继续往下做。

## 2. 当前阶段结论

项目当前已经从“基础设施准备阶段”进入“knowledge 业务骨架已落地，正在补主链路”的阶段。

更具体地说：

- `infra-ai` 已经具备可用的 `chat / embedding / rerank` 基础能力
- `internal/app/core/parser` 和 `internal/app/core/chunk` 已完成第一版
- `knowledge` 模块的 `domain / port / 部分 service / 部分 repository / migration` 已开始落地
- 业务主线已经明确从继续扩 `infra-ai`，切换到补齐 `knowledge` 主链路

当前主线不是继续扩基础设施，而是：

1. 把 `knowledge` 的业务模型、存储模型、数据库结构彻底对齐
2. 补齐文档上传、解析、切块、向量化、持久化主链路
3. 再接 RocketMQ 异步处理与 URL 定时刷新

## 3. 当前已完成项

### 3.1 核心基础能力

已经完成并可复用：

- `infra-ai`
  - `chat`
  - `embedding`
  - `rerank`
  - provider 路由与选择逻辑
- `parser`
  - `DocumentParser`
  - `MarkdownDocumentParser`
  - `TikaDocumentParser`
- `chunk`
  - `Chunk`
  - `fixed_size`
  - `markdown`
  - `Embedder`
- `server / middleware / config`
  - Gin 启动骨架
  - request-id
  - error handler
  - user context middleware
  - Viper 配置加载

已验证：

- `go test ./internal/app/core/parser ./internal/app/core/parser/test`
- `go test ./internal/app/core/chunk ./internal/app/core/chunk/test ./internal/framework/distributedid`

### 3.2 knowledge 业务层骨架

已经落地目录：

```text
internal/app/knowledge/
  domain/
  port/
  service/
```

已经落地的内容：

- `domain`
  - `KnowledgeBase`
  - `KnowledgeDocument`
  - `KnowledgeChunk`
  - `KnowledgeDocumentChunkLog`
  - `KnowledgeDocumentSchedule`
- `port`
  - repository 接口
  - storage 接口
  - task_queue 接口
  - vector_store 接口
- `service`
  - `knowledge_base_service.go` 已有可用实现
  - `knowledge_document_service.go` 目前只有空骨架，尚未实现业务逻辑

### 3.3 knowledge base service

[`internal/app/knowledge/service/knowledge_base_service.go`](d:\goagent\internal\app\knowledge\service\knowledge_base_service.go) 当前已经具备：

- 创建知识库
- 查询知识库详情
- 更新知识库
- 删除知识库
- 分页查询知识库

当前行为特点：

- 创建时会生成 `collection_name`
- 更新 `embedding_model` 时，会检查该知识库下是否已有“已分块文档”
- 删除知识库前，会检查是否仍存在文档

说明：

- 这层逻辑已经调整为和 `ragent` 的表结构方向一致
- 但部分 service 输入结构仍保留了旧字段名痕迹，例如 `Description`、`Status`
- 这些字段目前不会映射到最终 `t_knowledge_base` 表结构中，属于下一步要清理的技术债

### 3.4 PostgreSQL repository 与 migration

已经落地目录：

```text
internal/adapter/repository/postgres/
  migrations/
  models/
  sqlc/
  conn.go
  knowledge_base_repo.go
  knowledge_document_repo.go
  mapper.go
```

已经落地的内容：

- `knowledge_base_repo.go`
  - GORM 实现
  - 支持 `Create / Update / Delete / GetByID / GetByName / Count / List`
- `knowledge_document_repo.go`
  - GORM 实现简单 CRUD 与列表
  - `CountChunkedByKnowledgeBaseID` 使用 `pgx + sqlc` 风格查询
- `conn.go`
  - PostgreSQL DSN 解析
  - `gorm.DB`
  - `pgxpool.Pool`
- `mapper.go`
  - domain 与 postgres model 转换

### 3.5 数据库结构已开始对齐 ragent

已读取并参考：

- `D:\Git\ragent\resources\database\schema_pg.sql`

当前 migration 已调整为和上游 knowledge 表结构一致的方向：

- 表名采用 `t_knowledge_*`
- 时间字段采用 `create_time / update_time`
- 逻辑删除采用 `deleted SMALLINT`
- `knowledge_base` 包含：
  - `embedding_model`
  - `collection_name`
  - `created_by`
  - `updated_by`

当前 migration 文件：

- [`internal/adapter/repository/postgres/migrations/20260426212000_create_knowledge_tables.sql`](d:\goagent\internal\adapter\repository\postgres\migrations\20260426212000_create_knowledge_tables.sql)

## 4. 当前真实状态评估

如果从“是否已经能跑完整知识库主链路”这个角度评估，项目当前大约处在：

- 已完成基础设施：`80%+`
- 已完成 knowledge 骨架：`45%`
- 已完成 knowledge 主链路：`15%-20%`

原因：

- 数据结构、目录结构、接口边界已经开始成形
- `knowledge_base` 这一块已经有 service + repository + migration
- 但真正的主链路还没打通：
  - 文件上传未落地
  - `KnowledgeDocumentService` 未实现
  - `KnowledgeChunkRepository` 未实现
  - `DocumentProcessService` 未实现
  - 向量存储未实现
  - RocketMQ 任务未实现
  - HTTP handler 未实现

所以当前不是“从 0 到 1 的设计阶段”，而是“从 1 到可跑闭环的实现阶段”。

## 5. 当前存在的未对齐项

这些不是阻塞当前编译的问题，但会影响后续可维护性，应该优先清理：

### 5.1 knowledge base service 输入结构仍有旧字段

当前 `CreateKnowledgeBaseInput / UpdateKnowledgeBaseInput / PageKnowledgeBaseInput` 中仍有一些字段来自旧设计：

- `Description`
- `Status`

问题：

- 这些字段不在 `t_knowledge_base` 当前上游表结构中
- service 入参与 domain / migration / repository 已经不是完全同一语义

建议：

- 下一步清理这些不再落库的字段
- service 输入只保留真正存在于业务和表结构里的字段

### 5.2 repository 已对齐上游，service 和未来 handler 也要跟着统一

当前 repository 和 migration 已经按 `t_knowledge_*` 命名：

- `kb_id`
- `doc_name`
- `file_url`
- `deleted`

因此后续任何新写的业务层、handler、DTO、task message，都应统一使用与上游一致的语义，不要再回到：

- `knowledge_base_id`
- `storage_key`
- `mime_type`
- `deleted_at`

### 5.3 目前只有 knowledge base 基本可用

当前只有 `KnowledgeBaseService` 进入了“可用雏形”状态。

以下模块仍属于骨架或未实现：

- `KnowledgeDocumentService`
- `KnowledgeChunkService`
- `DocumentProcessService`
- `ScheduleService`
- `KnowledgeChunkRepository`
- `KnowledgeDocumentChunkLogRepository`
- `KnowledgeDocumentScheduleRepository`

## 6. 已确认的技术约束

这些决策已经比较稳定，后续默认继续沿用：

### 6.1 Web

- 继续使用 `Gin`
- 当前阶段不更换为 `Echo / Fiber / Chi`

### 6.2 数据库访问层

- `PostgreSQL`
- 简单 CRUD 用 `GORM`
- 复杂统计、扫描、批处理、调度相关查询用 `pgx + sqlc` 或手写 SQL

### 6.3 对象存储

- 使用 `S3-compatible`
- 第一阶段兼容当前 `rustfs` 配置

### 6.4 向量存储

- 第一阶段优先 `pgvector`
- 不同时推进 `pgvector + milvus`

### 6.5 消息队列

- 第一阶段统一使用 `RocketMQ`
- 不再回到 `Redis + Asynq`

### 6.6 数据库结构来源

- `knowledge` 相关表结构以 `D:\Git\ragent\resources\database\schema_pg.sql` 为准
- `goagent` 内的 migration、model、repository、domain 应尽量对齐该 schema

这是当前非常重要的约束。

## 7. 下一步任务清单

下面按优先级排序，尽量保证每一步都能形成清晰的增量成果。

### P0：先把 knowledge base 这一层彻底收口

1. 清理 `KnowledgeBaseService` 中已经不再需要的旧字段
   - 去掉或停用 `Description`
   - 去掉或停用 `Status`
2. 检查 `KnowledgeBaseRepository` 与 `t_knowledge_base` 的约束是否完整一致
   - `collection_name` 唯一
   - `deleted` 逻辑删除过滤
3. 补 `knowledge_base` 相关测试
   - service 单测
   - repository 最小行为测试

### P1：实现 KnowledgeDocumentService

这是下一阶段最重要的业务工作。

建议优先补这些能力：

1. 创建 / 上传登记文档
   - 创建 `t_knowledge_document` 记录
   - 补齐 `created_by / updated_by`
   - 设定 `process_mode / source_type / status`
2. 查询文档详情
3. 分页查询文档
4. 启用 / 禁用文档
5. 启动文档处理
   - 当前可以先只做“登记任务意图”或直接调用后续 process service

### P2：补齐 KnowledgeChunk 相关层

1. 补 `KnowledgeChunkRepository`
   - `CreateBatch`
   - `DeleteByDocumentID`
   - `GetByID`
   - `List`
   - `Update`
2. 必要时补 chunk model / mapper 的进一步细化
3. 明确 chunk 与上游 `t_knowledge_chunk` 的字段映射是否还缺：
   - `content_hash`
   - `char_count`
   - `token_count`
   - `created_by / updated_by`

### P3：打通 document 处理主链路

目标是先做同步闭环，不急着先上 MQ。

建议顺序：

1. 新建 `DocumentProcessService`
2. 串起：
   - 读取文件
   - parser
   - chunk
   - embedding
   - chunk 持久化
   - 向量写入
3. 第一阶段只支持本地上传文档
4. 暂不做 URL refresh

### P4：接对象存储

1. 落 `internal/adapter/storage/s3/`
2. 实现：
   - `Upload`
   - `Delete`
   - `Open`
3. 让 `KnowledgeDocumentService` 真的能接上传链路，而不是只创建数据库记录

### P5：接 pgvector

1. 定义统一 `vector_store` 实现
2. 优先做 `pgvector`
3. 支持：
   - 按 document 写入 chunks
   - 按 document 删除 vectors
   - 按 chunk 更新 vector

### P6：接 RocketMQ 异步任务

在同步链路跑通后再做。

1. 定义 RocketMQ producer / consumer
2. 把“文档切块 + 向量化”从同步改为异步
3. 补任务日志流转
   - `pending`
   - `running`
   - `success`
   - `failed`

### P7：补 schedule 与 URL refresh

1. 实现远程 URL 抓取
2. 实现内容变化检测
3. 接 `t_knowledge_document_schedule`
4. 接 `t_knowledge_document_schedule_exec`
5. 用轻量调度器 + RocketMQ 投递刷新任务

### P8：补 HTTP 接口

建议顺序：

1. `knowledge base`
   - create
   - get
   - update
   - delete
   - page/list
2. `knowledge document`
   - upload
   - get
   - page/list
   - enable/disable
   - start process
3. `knowledge chunk`
   - get
   - page/list
   - enable/disable

### P9：补测试与联调

至少补：

1. `knowledge base` service 测试
2. document repository / service 测试
3. 文档处理链路测试
4. 已分块文档计数 SQL 的行为测试
5. migration 执行验证

## 8. 当前不做的事情

为了保持推进节奏，当前阶段不优先展开：

- 重写 Web 框架
- 同时推进多种消息队列
- 同时推进 `pgvector` 和 `milvus`
- 提前做复杂多租户
- 提前做 MCP / 搜索 / 意图树业务接入
- 提前做 ingestion pipeline 全量实现

这些内容都排在 knowledge 主链稳定之后。

## 9. 下次对话建议工作流

下次如果继续推进，建议按这个顺序开始：

1. 先读取 `docs/project_progress_context.md`
2. 明确这次是继续哪个优先级任务
3. 优先从下面几项里选择一个切入：
   - 清理 `KnowledgeBaseService` 的旧字段
   - 开始实现 `KnowledgeDocumentService`
   - 开始实现 `KnowledgeChunkRepository`
   - 开始实现 `DocumentProcessService`

如果没有特别说明，默认下一步优先做：

- `KnowledgeDocumentService`

因为它是把“知识库定义完成”推进到“文档主链可落地”的关键桥梁。

## 10. 一句话总结

`goagent` 当前已经完成 `infra-ai + parser + chunk` 基础层，并开始落地 `knowledge` 模块；`knowledge base`、PostgreSQL repository、migration 已具备雏形，且数据库结构已开始对齐 `ragent` 的 `schema_pg.sql`；下一步的核心任务是补齐 `KnowledgeDocumentService`、`KnowledgeChunkRepository` 和文档处理主链，使项目从“骨架已成形”进入“主链可运行”阶段。
