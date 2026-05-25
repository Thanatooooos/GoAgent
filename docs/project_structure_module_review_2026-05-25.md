# 项目结构纪律评估

日期：2026-05-25

---

## 一、目的

本文档基于 [`.agents/skills/project-structure-discipline/SKILL.md`](../.agents/skills/project-structure-discipline/SKILL.md) 的结构纪律原则，对当前项目中哪些模块的文件结构已经偏离预期做一次聚焦评估。

关注点不是“代码能不能跑”，而是：

- 责任是否清晰归属到正确层级
- 是否出现单文件多职责
- 是否出现包内混放多类责任、仅靠文件名前缀分组
- 是否继续放大已经过载的文件或兼容层
- 是否存在明显的后续维护风险

---

## 二、评估标准

本次评估主要按以下标准判断：

1. 单文件是否只有一个主要变更原因
2. handler / service / repository / adapter 的职责是否混淆
3. 包边界是否清楚，而不是“同包内按文件名前缀假分层”
4. 文件大小是否超过健康阈值
5. 兼容层是否过厚，导致真实职责入口不清晰

参考阈值：

- `<300` 行：理想
- `300-500` 行：可接受，但需要关注
- `>500` 行：需要 review
- `>800` 行：应优先拆分

---

## 三、总体结论

当前最明显不符合结构纪律的模块有 5 组：

1. `internal/app/knowledge/service`
2. `internal/adapter/http/rag`
3. `internal/adapter/http/knowledge`
4. `internal/app/ingestion/service`
5. `internal/app/rag/tool` 顶层兼容层

其中：

- `knowledge/service` 是当前最严重的结构热点，已经同时出现“大文件 + 多职责 + service 内部混合流程编排与细节实现”。
- `ingestion/service` 的问题不是单个 god file，而是“一个包里放了 service / executor / runner / observer / workflow 五类职责”，包边界不清。
- `rag/tool` 已经明显往模块化收敛，但顶层 `tool` 包仍保留了较厚的 alias / wrapper / compat 层，真实职责入口不够直接。
- `adapter/http/rag` 和 `adapter/http/knowledge` 主要是 handler 文件过载，一个文件承载了多个子域接口族。

相对更符合 skill 原则的区域：

- `internal/app/rag/service/longtermmemory`
- `internal/app/rag/service` 中按 chat stage 拆开的文件
- `internal/app/knowledge/schedule`

---

## 四、优先级总表

| 模块 | 主要问题 | 严重度 | 判断 |
|---|---|---:|---|
| `internal/app/knowledge/service` | 单文件多职责、文件超大、service 过载 | P0 | 优先治理 |
| `internal/app/ingestion/service` | 同包混放多类责任，包边界不清 | P1 | 应拆子包 |
| `internal/app/rag/tool` 顶层 | compat / wrapper 过厚，入口语义不清 | P1 | 继续瘦身 |
| `internal/adapter/http/rag` | 单 handler 文件承载多个子域接口 | P1 | 应按接口族拆分 |
| `internal/adapter/http/knowledge` | 文档 handler 过载，DTO/VO/路由混堆 | P1 | 应按能力拆分 |
| `internal/adapter/vectorstore/pgvector` | 适配器文件偏大，但职责仍相对单一 | P2 | 观察名单 |

---

## 五、详细评估

### 5.1 `internal/app/knowledge/service`

这是当前最明显偏离结构纪律的模块。

#### 主要信号

- [`knowledge_document_service.go`](../internal/app/knowledge/service/knowledge_document_service.go)
  - 1143 行
- [`knowledge_chunk_service.go`](../internal/app/knowledge/service/knowledge_chunk_service.go)
  - 643 行
- [`document_process_service.go`](../internal/app/knowledge/service/document_process_service.go)
  - 556 行

#### 问题判断

1. `knowledge_document_service.go` 不是单一职责文件。
   它同时承载了：
   - upload / update / get / page / search / enable / delete
   - chunk log 分页
   - schedule exec 分页
   - ingestion 完成回写
   - sourceType / processMode / chunkStrategy 规范化与校验
   - storage key、file type、file name 处理

2. `knowledge_chunk_service.go` 把两类责任混在一起：
   - chunk CRUD
   - vector 同步 / 删除 / 重建 / embedding 相关流程

3. `document_process_service.go` 也过载：
   - 文件读取
   - 文本解析
   - chunk 切分
   - embedding
   - chunk/vector 持久化
   - chunk log
   - document 状态流转

#### 为什么不符合 skill

- 单文件已经超过 review 阈值甚至拆分阈值
- service 文件中既有应用流程编排，也有较多底层细节和数据构造
- “知识文档管理”和“文档处理执行”虽然都属于 knowledge，但不应继续堆在同一批大 service 文件里

#### 建议方向

建议按责任拆成更窄的 service 文件，至少做到：

- `knowledge_document_command_service.go`
  - upload / update / enable / delete
- `knowledge_document_query_service.go`
  - get / page / search / chunkLogs / scheduleExecs
- `knowledge_document_ingestion_service.go`
  - ingestion 完成回写、pipeline 关联逻辑
- `knowledge_document_input.go`
  - normalize / validate / file meta helper

对 chunk 和 process 再继续拆：

- `knowledge_chunk_command_service.go`
  - create / update / delete / enable / batch toggle
- `knowledge_chunk_vector_sync.go`
  - vector rebuild / upsert / delete
- `document_chunk_pipeline_service.go`
  - parse -> chunk -> embed -> persist 主流程
- `document_chunk_log_service.go`
  - chunk log lifecycle

#### 结论

`knowledge/service` 是当前结构治理的第一优先级。

---

### 5.2 `internal/adapter/http/rag`

这个模块主要问题在于 handler 文件过载，而不是层级错误。

#### 主要信号

- [`handlers.go`](../internal/adapter/http/rag/handlers.go)
  - 516 行
  - 同时包含：
    - conversation routes
    - message routes
    - feedback routes
    - chat routes
    - memory routes
    - request/response DTO

#### 问题判断

一个 handler 文件中同时承载多个子域：

- conversation 管理
- chat 执行
- feedback
- long-term memory 管理

这几个能力虽然都在 RAG 域内，但变更原因并不相同。后续任何一个子域升级，都会持续把同一个 handler 文件继续放大。

#### 为什么不符合 skill

- handler 层职责虽然仍是 HTTP，但文件职责不再单一
- request/response DTO、路由注册、业务入口全部堆在一个文件里

#### 建议方向

建议按接口族拆分：

- `conversation_handler.go`
- `message_feedback_handler.go`
- `chat_handler.go`
- `memory_handler.go`
- `routes.go`
- `view_types.go` 或按子域分 DTO 文件

#### 结论

这是一个明确的 P1 结构问题，适合在不动业务语义的前提下先做文件级拆分。

---

### 5.3 `internal/adapter/http/knowledge`

这个模块的问题与 `adapter/http/rag` 类似，但在文档 handler 上更集中。

#### 主要信号

- [`knowledge_document_handler.go`](../internal/adapter/http/knowledge/knowledge_document_handler.go)
  - 509 行
  - 同时承载：
    - upload / chunk / delete / get / update / page / search / enable
    - chunk logs / schedule execs
    - 文档、ingestion task/node、schedule exec 的 VO/DTO

#### 问题判断

当前 `knowledge document` handler 文件同时扮演了：

- 路由装配入口
- 文档写接口
- 文档查接口
- 运行日志查询接口
- schedule 执行记录查询接口
- 大量展示层 DTO 容器

#### 为什么不符合 skill

- 一个文件已经覆盖多个接口族
- DTO 和 handler method 一起膨胀
- 后续 schedule 或 ingestion 展示字段变化，会继续推高这个文件复杂度

#### 建议方向

建议至少拆成：

- `knowledge_document_command_handler.go`
- `knowledge_document_query_handler.go`
- `knowledge_document_runtime_handler.go`
  - chunk logs / schedule execs
- `knowledge_document_views.go`
- `routes.go`

#### 结论

这是 P1 问题，属于“职责还在 handler 层，但文件变成聚合桶”。

---

### 5.4 `internal/app/ingestion/service`

这个模块的文件名已经在表达分组，但包结构仍不符合 skill 期望。

#### 主要信号

[`doc.go`](../internal/app/ingestion/service/doc.go) 已明确说明当前同一个 `service` 包内混放了：

- `service_*`
- `executor_*`
- `workflow_*`
- `runner_*`
- `observer_*`

对应文件例如：

- [`service_pipeline.go`](../internal/app/ingestion/service/service_pipeline.go)
- [`executor_workflow.go`](../internal/app/ingestion/service/executor_workflow.go)
- [`workflow_builder_eino.go`](../internal/app/ingestion/service/workflow_builder_eino.go)
- [`runner_fetcher.go`](../internal/app/ingestion/service/runner_fetcher.go)
- [`observer_task_repository.go`](../internal/app/ingestion/service/observer_task_repository.go)

#### 问题判断

当前 ingestion 已经具备独立子系统形态，但包仍然是单一 `service`：

- pipeline/task 应用服务
- workflow 构建
- executor 执行
- node runner
- task observer / metrics

这些职责虽然都属于 ingestion，但已经不是“一个包一个 concern”。

#### 为什么不符合 skill

- skill 鼓励清晰包边界，而不是同包靠文件名前缀分组
- 当前目录结构让依赖方向不够显式
- 新增 runner / observer / execution feature 时，仍会持续扩大同一个包的认知负担

#### 建议方向

建议保留 `internal/app/ingestion/service` 作为应用服务入口，仅放：

- pipeline service
- task service

其余拆成子包：

- `internal/app/ingestion/workflow`
  - builder / condition / execution state
- `internal/app/ingestion/executor`
  - executor service / graph runtime
- `internal/app/ingestion/runner`
  - fetcher / parser / chunker / enricher / enhancer / indexer
- `internal/app/ingestion/observer`
  - repository observer / metrics observer / multi observer

#### 结论

这是一个典型的“文件看起来分组了，但包边界还没真正建立”的模块，优先级 P1。

---

### 5.5 `internal/app/rag/tool` 顶层兼容层

这个模块已经在模块化上做了大量收敛，但顶层 `tool` 包还没有彻底瘦下来。

#### 主要信号

顶层存在较多 alias / wrapper / compat 文件：

- [`tool.go`](../internal/app/rag/tool/tool.go)
- [`workflow.go`](../internal/app/rag/tool/workflow.go)
- [`module.go`](../internal/app/rag/tool/module.go)
- [`behaviors.go`](../internal/app/rag/tool/behaviors.go)
- [`views.go`](../internal/app/rag/tool/views.go)
- [`modular_wrappers.go`](../internal/app/rag/tool/modular_wrappers.go)
- [`compat_helpers.go`](../internal/app/rag/tool/compat_helpers.go)

而真实职责已分别进入：

- `core/`
- `runtime/`
- `planner/`
- `modules/`
- `invokers/`
- `assembly/`

#### 问题判断

现在的顶层 `tool` 包同时承担：

- 对外兼容入口
- type alias 容器
- module behavior 转发
- result view 转发
- runtime wrapper

这比此前已经好多了，但仍会造成两个问题：

1. 读代码的人不容易判断“真正实现在哪”
2. 顶层 compat 层容易再次吸收新语义，回到以前的过载状态

#### 为什么不符合 skill

- skill 明确要求已有合理架构下应继续收敛，而不是让 compat 层继续承载新职责
- 这里最大的风险不是当前代码错，而是结构回退风险高

#### 建议方向

继续坚持“module-first + compat 只保留薄导出层”：

- 顶层 `tool` 只保留稳定导出类型与极少量组装入口
- `behaviors.go` / `views.go` 评估是否继续下沉到模块调用方
- `modular_wrappers.go` 和 `compat_helpers.go` 继续清理，只保留无法立刻移除的兼容桥
- 新语义禁止再进入顶层 `tool` 根包

#### 结论

`rag/tool` 不是当前最差模块，但它是“已经进入正确方向、仍需防止反弹”的重点观察区，优先级 P1。

---

### 5.6 `internal/adapter/vectorstore/pgvector`

这是观察名单，不是当前主整改对象。

#### 主要信号

- [`vector_store.go`](../internal/adapter/vectorstore/pgvector/vector_store.go)
  - 536 行

#### 判断

这个文件偏大，但职责仍主要集中在 `pgvector` 适配器本身：

- chunk upsert
- keyword / metadata / vector search
- SQL scan / metadata 转换

它有拆分价值，但目前不像 `knowledge/service` 那样已经明显跨了多种变更原因。

#### 建议方向

若后续继续增长，可按能力拆：

- `write_store.go`
- `search_vector.go`
- `search_keyword.go`
- `search_metadata.go`
- `row_mapper.go`

#### 结论

列入 P2 观察，不建议抢在前面几项之前处理。

---

## 六、建议整改顺序

### 第一阶段：先打掉最明显的结构热点

1. `internal/app/knowledge/service`
2. `internal/adapter/http/rag`
3. `internal/adapter/http/knowledge`

目标：

- 先解决大文件和多职责文件
- 降低后续业务继续堆积的风险

### 第二阶段：把“文件分组”升级成“包边界”

4. `internal/app/ingestion/service`
5. `internal/app/rag/tool` 顶层 compat 层

目标：

- 让 ingestion 和 tool 两个正在持续演进的模块真正形成稳定边界

### 第三阶段：处理观察名单

6. `internal/adapter/vectorstore/pgvector`

---

## 七、不建议现在动的区域

以下区域目前不建议作为本轮结构治理重点：

- `internal/app/rag/service/longtermmemory`
  - 最近已经完成按 governance / recall / types 的责任拆分
- `internal/app/rag/service/rag_chat_*`
  - 已经按 prepare / execute / observability 等阶段拆开
- `internal/app/knowledge/schedule`
  - 虽然仍有优化空间，但整体职责分层已明显优于 `knowledge/service`

---

## 八、最终结论

如果只看“最需要尽快治理的结构问题”，当前项目的排序应是：

1. `internal/app/knowledge/service`
2. `internal/app/ingestion/service`
3. `internal/adapter/http/rag`
4. `internal/adapter/http/knowledge`
5. `internal/app/rag/tool` 顶层兼容层

其中最关键的一点是：

当前项目不是“没有分层”，而是部分核心模块已经进入第二阶段问题：

- 分层存在
- 但文件和包边界没有及时跟着能力增长一起收口

后续治理的重点不应只是“把大文件切小”，而应同步回答：

- 这个职责属于哪个层
- 这个层里应该落到哪个包
- 这个包是否已经承载了过多不同变更原因

只有按这个标准继续收口，项目后面的 Agent、Memory、Knowledge、Ingestion 主线才不会重新滑回大包大文件模式。
