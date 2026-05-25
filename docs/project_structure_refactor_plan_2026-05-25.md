# 项目结构整改实施计划

日期：2026-05-25

关联文档：

- [项目结构纪律评估](./project_structure_module_review_2026-05-25.md)
- [Project Progress Context](./project_progress_context.md)
- [Tool 包健康度评估与优化计划](./tool_package_health_plan.md)

---

## 一、目标

本文档不是再次判断“哪里有问题”，而是把结构评估落成一份可以执行的整改计划。

这份计划的目标有三层：

1. 先打掉已经明显过载的大文件和聚合型 handler
2. 再把“同包文件分组”升级成真正的包边界
3. 在不改变对外语义的前提下，让后续功能继续增长时不再回到 god file / 大杂烩包模式

本轮结构整改明确遵循以下原则：

- 不引入“先堆进去，后面再拆”的策略
- 不通过新建 `util` / `helper` 包逃避责任归属
- 能落到已有合理层级，就优先贴合现有层级
- 兼容层只允许变薄，不允许继续吸收新语义

---

## 二、执行范围

本轮纳入整改的模块：

1. `internal/app/knowledge/service`
2. `internal/adapter/http/rag`
3. `internal/adapter/http/knowledge`
4. `internal/app/ingestion/service`
5. `internal/app/rag/tool`

观察但暂不优先整改的模块：

6. `internal/adapter/vectorstore/pgvector`

本轮不作为整改重点的模块：

- `internal/app/rag/service/longtermmemory`
- `internal/app/rag/service/rag_chat_*`
- `internal/app/knowledge/schedule`

---

## 三、执行顺序

建议分 4 个阶段推进。

### Phase 1：先拆大文件和聚合 handler

目标模块：

1. `internal/app/knowledge/service`
2. `internal/adapter/http/rag`
3. `internal/adapter/http/knowledge`

目标：

- 优先降低单文件复杂度
- 先把最容易继续膨胀的位置止血
- 尽量不改变 import path 和对外行为

### Phase 2：建立清晰包边界

目标模块：

4. `internal/app/ingestion/service`
5. `internal/app/rag/tool`

目标：

- 从“同包文件分组”升级成“分子包责任归属”
- 明确依赖方向
- 让后续新增能力有稳定落点

### Phase 3：补结构性测试和装配验证

目标：

- 让拆分后的边界有直接测试覆盖
- 防止整理完目录后行为回退

### Phase 4：处理观察名单

目标模块：

6. `internal/adapter/vectorstore/pgvector`

目标：

- 只在前面 5 组完成后再评估是否值得跟进

---

## 四、模块级实施方案

### 4.1 `internal/app/knowledge/service`

这是第一优先级。

#### 当前问题

- `knowledge_document_service.go` 过大且职责混合
- `knowledge_chunk_service.go` 同时承载 CRUD 和 vector sync
- `document_process_service.go` 同时承载 pipeline 主流程和细节步骤

#### 目标结构

建议保留 `internal/app/knowledge/service` 目录，但把服务按责任拆成以下文件：

```text
internal/app/knowledge/service/
├── knowledge_document_command_service.go
├── knowledge_document_query_service.go
├── knowledge_document_ingestion_service.go
├── knowledge_document_input.go
├── knowledge_chunk_command_service.go
├── knowledge_chunk_vector_sync.go
├── document_chunk_pipeline_service.go
├── document_chunk_persist_service.go
├── document_chunk_log_service.go
├── service_types.go
└── service_wire.go
```

#### 责任分配

- `knowledge_document_command_service.go`
  - `Upload`
  - `Update`
  - `Enable`
  - `Delete`
- `knowledge_document_query_service.go`
  - `Get`
  - `Page`
  - `Search`
  - `PageChunkLogs`
  - `PageScheduleExecs`
- `knowledge_document_ingestion_service.go`
  - `SetIngestionTaskCreator`
  - `SetIngestionTaskReader`
  - `SetIngestionReconcileRecorder`
  - `OnIngestionTaskCompleted`
  - 文档与 pipeline chunk log 回写
- `knowledge_document_input.go`
  - sourceType / processMode / chunkStrategy normalize
  - fileName / fileType / storage key / config validate
- `knowledge_chunk_command_service.go`
  - chunk create / update / delete / enable / batch toggle
- `knowledge_chunk_vector_sync.go`
  - rebuild / upsert / delete / sync all
- `document_chunk_pipeline_service.go`
  - `ExecuteChunk`
  - `ProcessRefreshedDocument`
  - parse -> chunk -> embed -> persist 主流程编排
- `document_chunk_persist_service.go`
  - 持久化 steps
  - chunk/vector 构造
  - document chunk count update
- `document_chunk_log_service.go`
  - running / success / failed chunk log lifecycle

#### 落地顺序

1. 先抽 `knowledge_document_input.go`
2. 再拆 `knowledge_document_query_service.go`
3. 再拆 `knowledge_document_ingestion_service.go`
4. 最后拆 command 部分
5. chunk service 和 process service 单独一轮处理，不和 document service 混在一次 commit 中

#### 迁移策略

- 第一轮只做文件拆分，不引入新 package
- `KnowledgeDocumentService` / `KnowledgeChunkService` / `DocumentProcessService` 结构体名先保持不变
- 先稳定外部调用和测试，再视情况评估是否继续拆成更细 service 类型

#### 验收标准

- `knowledge_document_service.go` 不再存在或降到仅保留轻量装配
- 文档类单文件尽量控制在 `300-400` 行以内
- vector sync 与 chunk CRUD 不再同文件
- process 主流程与 chunk log / persist 细节不再同文件

---

### 4.2 `internal/adapter/http/rag`

这是第一阶段第二优先级。

#### 当前问题

- 一个 `handlers.go` 同时承载 conversation / chat / feedback / memory 多类接口
- request DTO、VO、路由注册都堆在一起

#### 目标结构

```text
internal/adapter/http/rag/
├── routes.go
├── conversation_handler.go
├── chat_handler.go
├── memory_handler.go
├── message_feedback_handler.go
├── conversation_views.go
├── message_views.go
├── memory_views.go
├── requests.go
├── trace_handlers.go
├── memory_cache_metrics_handler.go
└── test/
```

#### 责任分配

- `routes.go`
  - `RegisterRoutes`
  - admin route 组合
- `conversation_handler.go`
  - `ListConversations`
  - `RenameConversation`
  - `DeleteConversation`
  - `ListMessages`
- `chat_handler.go`
  - `Chat`
  - `StopChat`
- `memory_handler.go`
  - `ListMemories`
  - `Remember`
  - `ExpireMemory`
- `message_feedback_handler.go`
  - `SubmitFeedback`
- `*_views.go`
  - 各自子域的 VO 映射
- `requests.go`
  - rename / feedback / remember request

#### 落地顺序

1. 先抽 `routes.go`
2. 再抽 `chat_handler.go`
3. 再抽 `memory_handler.go`
4. 再抽 conversation / feedback
5. 最后清 DTO 归属

#### 迁移策略

- 不改路由 path
- 不改 handler struct 对外构造方式
- 保留 `Handler` 作为轻量 façade 也可以，但不能继续承载所有方法实现

#### 验收标准

- `handlers.go` 被拆空或删除
- conversation / chat / memory 不再同文件
- DTO 不再混在一个大文件里

---

### 4.3 `internal/adapter/http/knowledge`

这是第一阶段第三优先级。

#### 当前问题

- `knowledge_document_handler.go` 同时承载文档写接口、查询接口、运行日志接口
- 文档 VO、ingestion VO、schedule exec VO 全部混放

#### 目标结构

```text
internal/adapter/http/knowledge/
├── routes.go
├── knowledge_document_command_handler.go
├── knowledge_document_query_handler.go
├── knowledge_document_runtime_handler.go
├── knowledge_document_views.go
├── knowledge_document_runtime_views.go
├── knowledge_document_requests.go
├── knowledge_base_handler.go
├── knowledge_chunk_handler.go
└── test/
```

#### 责任分配

- `knowledge_document_command_handler.go`
  - upload / chunk / update / enable / delete
- `knowledge_document_query_handler.go`
  - get / page / search
- `knowledge_document_runtime_handler.go`
  - chunk logs / schedule execs
- `knowledge_document_views.go`
  - 文档主 VO / search VO
- `knowledge_document_runtime_views.go`
  - chunk log / ingestion task / ingestion node / schedule exec VO
- `knowledge_document_requests.go`
  - update request 及通用请求解析

#### 落地顺序

1. 先拆 runtime 相关接口与 VO
2. 再拆 command / query
3. 最后整理 routes

#### 验收标准

- 文档运行态查询与文档基础 CRUD 不再同文件
- ingestion task/node VO 与文档主 VO 分离
- `knowledge_document_handler.go` 不再是聚合桶

---

### 4.4 `internal/app/ingestion/service`

这是第二阶段核心任务，重点在包边界，而不是只切文件。

#### 当前问题

- `service` 包内同时放了 service / executor / workflow / runner / observer
- 依赖关系只能靠约定阅读，包边界不显式

#### 目标结构

```text
internal/app/ingestion/
├── domain/
├── port/
├── service/
│   ├── pipeline_service.go
│   ├── task_service.go
│   └── service_types.go
├── workflow/
│   ├── builder_eino.go
│   ├── condition.go
│   ├── execution_state.go
│   └── node_runner_registry.go
├── executor/
│   ├── workflow_executor.go
│   ├── eino_graph_executor.go
│   └── options.go
├── runner/
│   ├── fetcher_runner.go
│   ├── parser_runner.go
│   ├── chunker_runner.go
│   ├── enricher_runner.go
│   ├── enhancer_runner.go
│   ├── indexer_runner.go
│   ├── enrichment_helpers.go
│   └── shared_helpers.go
└── observer/
    ├── repository_observer.go
    ├── metrics_observer.go
    ├── multi_observer.go
    └── observer_types.go
```

#### 依赖方向

应明确为：

```text
service -> executor -> workflow -> runner
service -> observer
runner -> port / domain
observer -> port / domain
```

不应出现：

- runner 反向依赖 service
- observer 反向依赖 executor 实现细节
- workflow 反向依赖具体 handler 或 adapter

#### 落地顺序

1. 先抽 `observer/`
2. 再抽 `runner/`
3. 再抽 `workflow/`
4. 最后抽 `executor/`
5. `service/` 留到最后，只保留 pipeline/task 应用服务

#### 迁移策略

- 第一步只做 package 移动和 import 收敛，不同时改业务逻辑
- 每次只迁一类责任，避免一次大范围搬迁导致回归难定位
- `doc.go` 可保留，但要同步改成新的目录说明，而不是旧的“同包分组说明”

#### 验收标准

- `internal/app/ingestion/service` 不再同时存在 runner / observer / workflow / executor 文件
- ingestion 责任分层从文件名前缀升级为子包边界
- package import direction 清晰可读

---

### 4.5 `internal/app/rag/tool`

这是第二阶段另一个重点，目标是继续瘦身顶层 compat 层。

#### 当前问题

- 顶层 `tool` 仍有较多 alias / wrapper / behavior forwarding / view forwarding
- 新人阅读时不容易判断 canonical implementation 所在

#### 目标结构

保留现有子目录主框架：

```text
internal/app/rag/tool/
├── assembly/
├── core/
├── planner/
├── runtime/
├── invokers/
├── modules/
└── tool.go / workflow.go / module.go
```

但要求顶层只保留极薄入口，不再扩张：

- `tool.go`
  - 核心导出类型 alias
- `workflow.go`
  - workflow 相关 alias
- `module.go`
  - module / behavior alias

其余顶层文件作为整改目标：

- `behaviors.go`
- `views.go`
- `modular_wrappers.go`
- `compat_helpers.go`

#### 实施策略

1. 先给顶层 `tool` 定规则
   - 新语义不得落入根包
2. 再清理 wrapper
   - 能直接由调用方引用 `core` / `runtime` / `modules` 的，逐步改直连
3. 再清理 compat
   - 删除仅剩单层转发、无兼容价值的 wrapper
4. 必要时增加 `doc.go`
   - 明确根包只是稳定入口，不是主实现层

#### 可接受状态

- 顶层仍可以保留少量 alias，方便兼容
- 但不允许继续承担新的运行时逻辑、行为逻辑或 view 组合逻辑

#### 验收标准

- 顶层 `tool` 根包文件数明显下降或显著瘦身
- `behaviors.go` / `views.go` 若保留，必须说明其仅为兼容导出，不承载新逻辑
- 新增功能不再落到根包

---

### 4.6 `internal/adapter/vectorstore/pgvector`

这是观察名单。

#### 目标结构

如果后续继续增长，建议拆为：

```text
internal/adapter/vectorstore/pgvector/
├── write_store.go
├── vector_search.go
├── keyword_search.go
├── metadata_search.go
├── row_mapper.go
├── lexical.go
└── vector_store.go
```

#### 当前策略

- 本轮不主动处理
- 仅在前面五项完成后，若该文件继续超过当前规模，再单独立项

---

## 五、实施节奏建议

建议按以下节奏推进，避免一次性大搬家。

### 批次 A：Knowledge 文件治理

- `knowledge_document_*`
- `knowledge_chunk_*`
- `document_process_*`

预期收益：

- 立刻降低当前最大的大文件风险

### 批次 B：HTTP handler 文件治理

- `adapter/http/rag`
- `adapter/http/knowledge`

预期收益：

- 控制接口层继续膨胀
- 为后续接口扩展留出清晰入口

### 批次 C：Ingestion 包边界治理

- `observer`
- `runner`
- `workflow`
- `executor`
- `service`

预期收益：

- 从“能看懂”变成“能长期演化”

### 批次 D：Tool 顶层 compat 瘦身

- 根包 alias / wrapper / forwarding 清理

预期收益：

- 防止 `tool` 再次回到历史上的顶层混装模式

---

## 六、每批次的执行约束

每一批次都建议遵守同样的收口规则：

1. 一次只处理一类结构问题
   - 不把“拆文件”和“改业务行为”混在一个任务里

2. 先抽纯 helper，再抽主流程
   - 先抽输入规范化、VO、view mapper、纯函数
   - 再抽编排逻辑

3. 先保留 import path 稳定，再考虑进一步升级
   - 第一轮优先降低风险
   - 第二轮再考虑是否重命名类型或调整公开入口

4. 每完成一批都跑对应测试
   - 不等全部拆完再统一验证

---

## 七、验证建议

虽然本次文档不直接改代码，但后续实施时建议按模块跑验证：

### Knowledge

```powershell
go test ./internal/app/knowledge/service ./internal/adapter/http/knowledge ./internal/bootstrap/knowledge -count=1
```

### Ingestion

```powershell
go test ./internal/app/ingestion/... ./internal/adapter/http/ingestion ./internal/bootstrap/ingestion -count=1
```

### RAG Tool

```powershell
go test ./internal/app/rag/tool/... ./internal/bootstrap/rag -count=1
```

### RAG HTTP

```powershell
go test ./internal/adapter/http/rag/... ./internal/app/rag/service ./internal/bootstrap/rag -count=1
```

---

## 八、完成定义

当以下条件满足时，可以认为这一轮结构整改完成：

1. `knowledge/service` 不再存在 500+ 行且多职责混合的主 service 文件
2. `adapter/http/rag` 与 `adapter/http/knowledge` 不再由单一聚合 handler 文件承载多个接口族
3. `ingestion` 的 runner / observer / workflow / executor 已形成真实子包边界
4. `rag/tool` 顶层 compat 层停止继续承载新语义，并完成一轮瘦身
5. 对应模块测试保持通过
6. 新增功能需求出现时，能够直接回答“应该落到哪个包、哪个文件”，而不是再回到大文件追加

---

## 九、建议的下一步

如果从执行收益和风险平衡看，建议下一步按下面顺序直接开工：

1. `internal/app/knowledge/service`
   - 先拆 `knowledge_document_service.go`
2. `internal/adapter/http/rag`
   - 先拆 `handlers.go`
3. `internal/adapter/http/knowledge`
   - 先拆 `knowledge_document_handler.go`
4. `internal/app/ingestion/service`
   - 先迁 `observer/`
5. `internal/app/rag/tool`
   - 先定义“根包禁增新语义”的边界并清 wrapper

也就是说，最推荐的首个落地任务是：

> 先把 `knowledge_document_service.go` 拆成 command / query / ingestion / input 四组文件。

这是当前结构收益最高、同时又最不需要大范围迁移 package 的切入点。
