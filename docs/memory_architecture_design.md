# Memory Architecture Design

讨论日期：2026-05-17
重写日期：2026-05-25

这份文档基于当前 `goagent` 已落地实现、项目进度文档，以及已确认的长期记忆治理原则，对 memory 设计进行重写。它的目标不是继续发散讨论“要不要做 memory”，而是明确：

1. 当前已经确认的设计事实
2. 当前已经落地的实现边界
3. 长期记忆治理的核心模型
4. 下一阶段应继续推进的动作

一句话定义：

> 长期记忆不等于历史记录存档。  
> 长期记忆是可检索、可更新、可失效的结构化知识。

---

## 1. 本文档的核心结论

当前阶段，`goagent` 的 memory 设计已经从“V1 可实施方案”推进到“V1 已有基础闭环，接下来要补治理与检索投影”的阶段。

当前共识如下：

1. 长期记忆不是存原始聊天记录，而是存结构化事实。
2. 长期记忆写入前必须经过 `Memory Gate`。
3. 长期记忆必须支持 `Create / Update / Merge / Delete(Expire)`。
4. 同一单值事实只能有一个 `Active` 版本。
5. memory 需要同时按“时间层次”和“用途”分层。
6. 不是所有长期记忆都应该走统一 recall 总线。
7. 规则型记忆与证据型记忆必须分治。
8. 下一阶段重点不是继续讨论概念，而是补齐治理、冲突处理和事实型记忆检索投影。

---

## 2. 已确认的治理原则

### 2.1 长期记忆不是聊天记录，而是结构化事实

长期记忆保存的是未来仍会影响回答和行为的稳定信息，而不是原始对话本身。

不应直接进入长期记忆的内容：

- 原始对话全文
- 一次性请求
- 临时上下文
- 原始日志 / 原始堆栈 / 原始长代码
- 未被用户确认的 assistant 推断

适合进入长期记忆的内容：

- 用户偏好
- 行为约束
- 项目长期状态
- 历史技术决策
- 用户确认过的固定工作方式
- 稳定的项目事实与边界条件

### 2.2 写入前必须经过 `Memory Gate`

长期记忆不是“抽出来就写”，而是“抽出来以后再经过治理判断”。

`Memory Gate` 至少要回答以下问题：

1. 这条信息是否具有长期有效性？
2. 它未来是否会影响回答、推理或工具使用？
3. 它是否只是当前会话里的临时上下文？
4. 它是否与已有记忆冲突？
5. 它应该落到哪个作用域？
6. 它是规则型记忆还是事实型记忆？

### 2.3 长期记忆必须支持 CRUD 与合并

长期记忆不是 append-only 日志。

必须支持：

- `Create`：新增记忆
- `Update`：修正已有记忆
- `Merge`：合并同类记忆
- `Delete / Expire`：删除或失效

对长期运行系统来说，真正重要的不是“能存”，而是“能持续维护为真”。

### 2.4 同一单值事实只能有一个 `Active` 版本

对同一个单值键，任一时刻只能有一个生效版本。

示例：

```text
project.messaging.main_bus = RocketMQ        (superseded)
project.messaging.main_bus = Removed         (active)
```

这条原则只适用于单值键，不机械适用于所有记忆。

### 2.5 记忆需要分层

memory 至少要按两条维度分层：

1. 按时间层次分层
2. 按用途分层

时间层次：

- `STM`：短期记忆，最近 N 轮对话
- `MTM`：中期记忆，会话压缩与任务摘要
- `LTM`：长期记忆，跨会话复用的结构化知识

用途分层：

- 规则型记忆：偏好、行为约束、稳定前提
- 事实型记忆：项目状态、技术决策、稳定项目事实
- 证据型记忆：长日志、长配置、长代码、长文档节选

### 2.6 检索时不要把所有记忆直接塞进 Prompt

长期记忆读取必须经过选择、排序和压缩，而不是把历史记忆简单拼接。

推荐链路：

```text
User Query
  -> Memory Recall
  -> Memory Rerank
  -> TopK
  -> Memory Summary
  -> Prompt
```

但要注意：

- 规则型记忆不一定要先经过 recall/rerank
- 某些稳定规则应直接进入 `MemoryContext`

### 2.7 每条长期记忆都应带治理元数据

最低限度建议带如下元数据：

```json
{
  "id": "mem_xxx",
  "namespace": "user/global or kb/project",
  "type": "preference | knowledge | feedback",
  "key": "response.language",
  "value": "zh-CN",
  "confidence": "high",
  "importance": 100,
  "status": "active",
  "created_at": "2026-05-22T10:00:00Z",
  "updated_at": "2026-05-22T10:00:00Z",
  "last_used_at": null
}
```

在 `goagent` 语境下，还建议包含：

- `user_id`
- `scope_type`
- `scope_id`
- `source_message_id`
- `supersedes_id`
- `expires_at`
- `extraction_method`

### 2.8 长期记忆治理的重点

长期记忆治理不是只做“写入能力”，而是围绕四件事闭环：

1. 写什么：`Memory Gate`
2. 怎么更新：`Merge / Conflict`
3. 怎么失效：`Lifecycle`
4. 怎么取回：`Recall / Rerank / Summary`

---

## 3. `goagent` 当前实现事实与约束

以下内容是当前设计必须尊重的现实约束。

### 3.1 短期记忆已经存在

当前已有短期记忆基础：

- `conversation_message`
- `conversation_summary`
- `LoadHistory`
- `CompressIfNeeded`

因此 memory 设计不是从零开始，而是在现有短期记忆和 RAG 主链路之上增强。

### 3.2 会话并不天然等于长期记忆作用域

当前 `conversation` 绑定的是：

- `conversation_id`
- `user_id`

而 `knowledgeBaseId` 是 chat 请求时动态传入的。

因此：

- 不能把 `conversation` 当成长期记忆默认作用域
- 长期记忆必须显式建模 `scope_type / scope_id`

### 3.3 `ThinkingContent` 不能承载长消息原文

`ThinkingContent` 已经用于 assistant thinking / 推理补充内容。

因此：

- 用户长消息原文不能复用 `ThinkingContent`
- 长消息原文与长期记忆都需要独立语义边界

### 3.4 `knowledge_document` 不适合作为长期记忆主模型

当前 `knowledge_document` 更偏文档与 ingestion 模型：

- 文件
- URL
- 抓取内容
- pipeline / chunk / index

它不适合作为长期记忆业务真相层，原因包括：

1. 缺少记忆自己的生命周期语义
2. 缺少作用域语义
3. 缺少冲突、覆盖、确认、过期语义
4. 当前模型过于偏文档输入

### 3.5 当前 retrieve 主过滤维度仍以 `KnowledgeBaseIDs` 为主

当前的主检索入口仍然是 `KnowledgeBaseIDs`。

这意味着：

- 事实型长期记忆如果要自然加入检索，需要做专门投影设计
- 不宜把所有长期记忆都强行伪装成普通 document chunk

---

## 4. `goagent` 的 memory 分层模型

### 4.1 时间层次分层

### `STM`：短期记忆

用于承载最近 N 轮对话的直接上下文。

当前对应实现：

- `conversation_message`
- `LoadHistory`

### `MTM`：中期记忆

用于承载“当前会话内或任务内可复用，但不一定跨会话沉淀”的压缩内容和证据内容。

当前对应实现：

- `conversation_summary`
- `SessionChunk`
- `SessionRecall`

未来可继续承载：

- 任务摘要
- 会话级决策摘要
- 工作流阶段摘要

### `LTM`：长期记忆

用于承载跨会话复用的结构化知识。

当前对应实现：

- `memory_item`
- `MemoryService`
- `MemoryContext`

### 4.2 用途分层

#### 规则型记忆

面向用户偏好、行为约束和稳定工作方式。

示例：

- “以后都用中文回答”
- “先看代码再给建议”
- “不要先讲理论，先定位代码”

特点：

- 更像稳定前置约束
- 更适合直接进入 `MemoryContext`
- 不应和普通 chunk 竞争统一检索排序

#### 事实型记忆

面向稳定项目事实、技术决策和系统边界。

示例：

- “这个服务部署在内网，不能联网”
- “RocketMQ 已移除”
- “项目使用自定义 chunker”

特点：

- 更适合被结构化存储
- 后续可参与 recall/rerank
- 是 Phase 3 检索投影的主要对象

#### 证据型记忆

面向用户在当前会话提供的长日志、长代码、长配置、长文档。

特点：

- 目标是补回原文细节
- 默认属于会话检索层
- 不默认升级为长期记忆

当前主要由：

- `SessionChunk`
- `SessionRecall`
- `SessionContext`

承载。

---

## 5. 当前已落地实现

### 5.1 长消息处理已完成

已完成内容：

- `conversation_message` 新增 `RawContent / ContentSummary / IsSummarized`
- 长消息按三档阈值处理
- `AddMessage` 已具备处理器化接入点
- 写时摘要已替代“读时才压缩”的思路

结论：

- 长消息不再直接污染短期上下文主文本
- 原文与摘要已经分离

### 5.2 会话检索层已完成 V1 闭环

已完成内容：

- `SessionChunk` / `SessionChunkEmbedding`
- `SessionRecallService`
- `SessionContext`
- `session_recall` trace
- 中等长消息与超长消息都可进入会话检索层

结论：

- 证据型记忆的当前主路径已经跑通
- 会话内可以补回长原文关键细节

### 5.3 显式长期记忆已完成 Phase 2.1 治理闭环

已完成内容：

- `domain.MemoryItem`
- `MemoryItemRepository`
- `MemoryService`
- `internal/app/rag/service/longtermmemory`
- `POST /rag/v3/remember`
- `POST /rag/v3/memories`
- `GET /rag/v3/memories`
- `POST /rag/v3/memories/:memoryId/expire`
- `MemoryContext`
- `prepareChat(...)` 中的长期记忆 stage

当前代码组织已进一步收口为：

- `STM`
  - `internal/app/rag/core/history`
  - 负责 history / summary load 与会话压缩
- `MTM / 证据型记忆`
  - `LongMessageContentProcessor`
  - `SessionChunk`
  - `SessionRecallService`
- `LTM / 长期记忆`
  - `internal/app/rag/service/longtermmemory`
  - root package 保持稳定公开入口
  - `MemoryService` 负责显式保存、过期与对外装配
  - 公开输入/输出/option 类型继续从根包导出，避免外部 import path 抖动
  - `governance/`
    - 负责 normalize / gate / schema / conflict / save / lifecycle
  - `recall/`
    - 负责 recall / ranking / tokens / projection / context rendering / cache support
  - `types/`
    - 作为小型叶子包承载共享 public DTO / options，专门用于避免 Go 包循环
- `cross-layer ports`
  - `internal/app/rag/port`
  - `MemoryRecallCache`
  - `MemoryMutationTransaction`

这意味着当前长期记忆的结构边界已经从“单包承载全部职责”收敛为：

```text
longtermmemory (stable public entry)
  -> governance (write-path rules)
  -> recall (read-path recall/cache/projection)

adapter -> port <- service
```

同时，`RagChatService` 当前只依赖长期记忆 recall interface，不再直接依赖长期记忆 CRUD service。

当前结论：

- “显式保存 -> 后续对话可用”已经跑通
- 长期记忆在代码结构上已经有独立应用层边界
- `Phase 2.1` 的显式治理链路也已经落地

本轮进一步完成的治理能力包括：

- `memory_item` 已补齐结构化治理字段：
  - `namespace`
  - `category`
  - `canonical_key`
  - `value_type`
  - `value_json`
  - `display_value`
  - `importance`
  - `last_used_at`
  - `supersedes_id`
  - `extraction_method`
- 已新增第一版 `canonical_key` 白名单与 key 规格
- `SaveExplicitMemory(...)` 已切换为：
  - normalize
  - `Memory Gate`
  - `Conflict Detector`
  - transactional persist
- 单值键 / 多值键治理规则已落地
- `superseded` 已成为正式覆盖状态，不再混用 `expired`
- HTTP `remember / memories` 接口已兼容式支持治理字段输入输出

### 5.4 当前仍未完成的部分

以下能力仍然缺失或不完整：

1. 事实型长期记忆检索投影尚未开始
2. `last_used_at` 的运行时更新仍未接入
3. 生命周期清理策略与后台治理任务尚未落地
4. 自动识别与异步抽取尚未开始
5. 规则型 / 事实型记忆的 recall 路径仍未彻底分层产品化

---

## 6. 长期记忆的数据模型

### 6.1 主模型：`memory_item`

`memory_item` 继续作为长期记忆的业务真相层。

当前已具备的核心字段：

- `id`
- `user_id`
- `scope_type`
- `scope_id`
- `namespace`
- `memory_type`
- `category`
- `canonical_key`
- `value_type`
- `value_json`
- `display_value`
- `source_message_id`
- `content`
- `summary`
- `confidence`
- `importance`
- `status`
- `last_confirmed_at`
- `last_used_at`
- `expires_at`
- `supersedes_id`
- `extraction_method`
- `created_by`
- `updated_by`

### 6.2 已补齐的治理字段

为了支持规则型/事实型区分、冲突治理和后续检索投影，当前已经补齐：

- `namespace`
- `category`
- `canonical_key`
- `value_type`
- `value_json`
- `display_value`
- `importance`
- `last_used_at`
- `supersedes_id`
- `extraction_method`

当前逻辑结构可概括为：

```json
{
  "id": "mem_xxx",
  "user_id": "u_xxx",
  "scope_type": "global | kb",
  "scope_id": "kb_xxx | null",
  "memory_type": "preference | knowledge | feedback",
  "category": "response | workflow | behavior | project",
  "canonical_key": "response.language",
  "value_type": "text | enum | boolean | json",
  "value_json": "zh-CN",
  "display_value": "中文",
  "content": "以后都用中文回答",
  "summary": "用户偏好使用中文回答",
  "confidence": "high",
  "importance": 100,
  "status": "active",
  "source_message_id": "msg_xxx",
  "supersedes_id": null,
  "last_confirmed_at": "2026-05-22T10:00:00Z",
  "last_used_at": null,
  "expires_at": null,
  "extraction_method": "manual | explicit_rule | explicit_llm"
}
```

### 6.3 `single-valued` 与 `multi-valued` 键

长期记忆不能只按 `memory_type` 管理，还要按键本身的基数管理。

#### 单值键

同一个作用域内只能存在一个 `active` 版本。

示例：

- `response.language`
- `workflow.first_step`
- `project.constraint.network`
- `project.messaging.main_bus`

治理规则：

- 新值生效时，旧值必须 `superseded` 或 `expired`

#### 多值键

同一个作用域内允许多个值同时存在，但需要去重和合并。

示例：

- `behavior.avoid`
- `project.fact.dependencies`
- `project.integrations`

治理规则：

- 命中同值则去重
- 命中相近值则合并
- 不应简单无限 append

---

## 7. 写入链路设计

推荐写入流程：

```text
User Message
  -> Answer
  -> Memory Extractor
  -> Memory Gate
  -> Conflict Detector
  -> Merge / Update / Ignore
  -> PostgreSQL
```

### 7.1 `Memory Extractor`

负责从消息中抽取“候选长期记忆”。

V1 / 当前推荐原则：

- 优先只处理 `user` 消息
- 优先处理显式高信号表达
- 宁可漏，不要错

高信号候选包括：

- 用户显式偏好
- 用户显式纠偏
- 用户提供的稳定项目事实
- 用户确认过的工作方式

### 7.2 `Memory Gate`

`Memory Gate` 是长期记忆治理核心，不应省略。

输出不只是 `yes/no`，而是以下决策之一：

- `create`
- `update`
- `merge`
- `ignore`
- `pending`

推荐判定维度：

1. 是否长期有效
2. 是否未来会影响回答
3. 是否只是临时上下文
4. 是否属于规则型记忆
5. 是否属于事实型记忆
6. 是否已存在冲突事实
7. 是否需要覆盖旧版本

当前已落地的最小治理边界：

- 无 `userID` / 无 `content` / 非法 scope / 非法 memory type 直接拒绝
- `canonical_key` 非空时，必须命中白名单并与 spec 一致
- `feedback` 默认不参与覆盖旧事实
- `canonical_key` 为空的旧式记忆允许继续保存，但按 generic structured memory 处理
- 显式 remember 路径默认落 `active`，不产出自动抽取型 `pending`

### 7.3 冲突检测与合并

`Conflict Detector` 的职责不是只查重，而是决定如何维护“真相”。

对单值键：

- 查当前作用域同 key 的 `active` 版本
- 若值变化，则创建新版本并让旧版本失效

对多值键：

- 先判等值去重
- 当前仅做 exact-match merge / refresh
- 不做 LLM 近义合并

当前已落地的明确规则：

- 单值键同值重复保存：更新现有记录，不新建
- 单值键新值覆盖旧值：旧记录 `superseded`，新记录 `active`
- 多值键同值重复保存：merge / refresh
- 多值键不同值：允许并存多个 `active`

### 7.4 生命周期状态

当前实现已有：

- `pending`
- `active`
- `rejected`
- `expired`
- `superseded`

推荐后续明确扩展或显式表达：

- `deleted`

即使物理上不单独加状态，也应在逻辑上表达：

- 为什么失效
- 被谁覆盖
- 何时过期

---

## 8. 读取链路设计

推荐读取流程：

```text
User Query
  -> Memory Recall
  -> Memory Rerank
  -> TopK
  -> Memory Summary
  -> RAG Retrieve
  -> Agent Planner
  -> Final Answer
```

但在 `goagent` 中，实际应拆成三条不同子路径，而不是一个统一 memory 池。

### 8.1 规则型记忆：直接注入 `MemoryContext`

规则型记忆的读取目标是稳定影响回答方式与行为前提。

适合直接注入：

- `User Preferences`
- `Behavior Rules`
- `Stable Working Style`

不建议：

- 先把这类内容打散成检索候选
- 再和 document chunk 一起竞争排序

### 8.2 事实型记忆：走 recall / rerank / summary

事实型记忆更接近“可检索稳定事实”。

推荐读取链路：

```text
Fact Memory Recall
  -> Rerank
  -> TopK
  -> Summary
  -> 注入 Prompt
```

这类记忆是后续 Phase 3 检索投影的主要对象。

### 8.3 证据型记忆：继续走 `SessionRecall`

证据型记忆的目标不是长期沉淀，而是会话内补回原文细节。

因此继续保持：

- 长消息写时摘要
- 原文 chunk 存入 `SessionChunk`
- follow-up query 时通过 `SessionRecall` 召回 excerpt
- 独立注入 `SessionContext`

### 8.4 不要把三种 memory 混成一个 recall 池

如果把规则型、事实型、证据型都混进同一个 recall 池，代价会非常明显：

- prompt 稳定性下降
- 检索可解释性下降
- 生命周期治理变复杂
- 事实覆盖与证据补全文本互相干扰

因此推荐保持：

```text
规则型记忆   -> MemoryContext
事实型记忆   -> FactMemory Recall Path
证据型记忆   -> SessionRecall / SessionContext
```

---

## 9. 与当前 RAG 主链路的关系

### 9.1 当前 `prepareChat(...)` 已有的 memory 相关阶段

当前已接入：

- `runMemoryStage`
- `runLongTermMemoryStage`
- `runSessionRecallStage`
- `runRetrieveStage`

这说明 memory 已经进入 chat 主链路，而不是独立旁路。

按当前实现，主链路中的分工是：

- `runMemoryStage`
  - 读取 `STM`
  - 对应 `core/history`
- `runLongTermMemoryStage`
  - 读取 `LTM`
  - 对应 `service/longtermmemory`
- `runSessionRecallStage`
  - 读取 `MTM / evidence memory`
  - 对应 `SessionRecallService`

### 9.2 当前阶段的推荐演进方向

下一阶段不建议做“大一统 memory router”，而建议继续沿着当前结构演进：

1. 先收口规则型记忆的数据模型和治理
2. 再补事实型记忆的检索投影
3. 保持 `SessionRecall` 独立
4. 保持 `service/longtermmemory` 与 `core/history` 的语义边界，不再回退到泛化 `memory` 命名

### 9.3 与 retrieve 的边界

当前结论：

- 规则型记忆不要求先进入 retrieve
- 事实型记忆后续可作为独立 memory channel 参与检索融合
- 证据型记忆继续独立于长期记忆检索

---

## 10. 下一阶段动作

### 10.1 Phase 2.1：治理收口

当前状态：已完成。

本轮已完成：

1. 明确 `memory_item` 的结构化字段
2. 定义 `canonical_key` 白名单
3. 定义单值键 / 多值键
4. 实现 `Memory Gate`
5. 实现 `Conflict Detector`
6. 实现 `create / update / merge / expire` 策略

当前结果：

- 长期记忆已经从“可保存”升级为“可治理”

### 10.2 Phase 3：事实型记忆检索投影

这一步是长期记忆真正参与检索的开始。

工作项：

1. 只选择事实型记忆进入检索投影
2. 设计投影 metadata
3. 建立 recall / rerank / topK / summary 链路
4. 与普通 knowledge retrieve 做融合策略
5. 增加 trace 与 metrics

目标：

- 让长期事实能自然参与后续问答
- 不把偏好型记忆和证据型记忆塞进统一总线

### 10.3 Phase 4：生命周期治理

工作项：

1. `last_used_at` 的运行时更新
2. 清理策略
3. `deleted` / hard cleanup 边界
4. 后台治理任务
5. 生命周期 trace / metrics
6. 治理策略的管理面可观测性

目标：

- 让记忆库不会随时间失真和膨胀

### 10.4 Phase 5：异步自动识别

工作项：

1. Cheap model 分类
2. 置信度阈值
3. 灰度开关
4. 自动抽取只进入 `pending` 或高置信 `active`

目标：

- 降低用户手动保存成本
- 不牺牲记忆质量

---

## 11. 当前阶段的最终判断

截至 2026-05-22，`goagent` 的 memory 设计与实现可以概括为：

1. 长消息处理已经落地，写时摘要成立。
2. 证据型记忆的会话召回链路已经可用。
3. 显式长期记忆已经具备 `Phase 2.1` 治理闭环。
4. `STM / MTM / LTM` 的代码结构边界已经收清，长期记忆拥有独立应用层入口。
5. 当前最大缺口已经从“怎么治理”进一步转向“怎么检索与怎么做生命周期治理”。

因此，当前最重要的不是继续扩展 memory 概念，而是把以下三件事做好：

1. 规则型记忆与事实型记忆在读取路径上进一步明确分治
2. 事实型记忆进入 `Phase 3` 检索投影
3. 生命周期治理补齐 `last_used_at / cleanup / maintenance`

最终推荐主路线：

```text
STM
  -> core/history
  -> history / summary / compression

结构化长期记忆
  -> service/longtermmemory
  -> memory_item
  -> Memory Gate
  -> Conflict / Merge / Lifecycle
  -> MemoryContext or FactMemory Recall

会话内证据记忆
  -> LongMessageContentProcessor
  -> SessionChunk
  -> SessionRecall
  -> SessionContext
```

这条路线的价值在于：

- 它与当前已落地实现一致
- 它避免把长期记忆退化成聊天归档
- 它能在不打乱现有 RAG 主链路的前提下继续演进

---

## 12. 健康状态评估（2026-05-22 审计）

本次审计基于对 `longtermmemory/`、`domain/memory_item.go`、Postgres repo 层、HTTP 接入层、chat prepare 集成点的完整代码审阅。

### 12.1 总体判断

Memory 系统处于 **架构健康、治理到位、但规模化前需要补齐三项基础设施** 的状态。Phase 2.1 的治理闭环是当前质量最高的部分；召回链路概念正确但缺少性能护栏；运维与生命周期治理完全是空白。

| 维度 | 评分 | 说明 |
|---|---|---|
| 架构分层 | 8/10 | STM/MTM/LTM 边界清晰，依赖方向正确 |
| 写入治理 | 8/10 | Gate + Conflict + Transactional persist 到位 |
| 召回检索 | 6/10 | 双信号融合设计正确，但全量加载不可持续 |
| 运维可观测性 | 5/10 | trace 元数据丰富，但无 metrics / alert / reconciliation |
| 可扩展性 | 5/10 | 核心扩展点被硬编码阻断，读路径未按类型分治 |

### 12.2 架构分层（健康）

**做得好的：**

- `RagChatService` 只依赖 `RecallService` 接口（`rag_chat_prepare.go:256`），不直接感知 CRUD 实现
- 长期记忆失败采用 **fails-open** — 召回失败不阻断 chat 主链路（`rag_chat_prepare.go:44-46`）
- `MemoryService` 是管理面聚合根，`RecallService` 是 chat 面入口，职责不混淆
- `runMemoryStage → runLongTermMemoryStage → runSessionRecallStage → runRetrieveStage` 的 pipeline 顺序体现了 STM/MTM/LTM 的分层语义

**设计偏差：**

设计文档第 8.1 节明确要求规则型记忆"直接注入 MemoryContext，不走 recall/rerank"，但当前 `recallService.RecallMemories()` 把 preference / knowledge / feedback 三种类型全部塞进同一个 keyword+vector 融合排序管道。`memoryTypePriority()` 给了 preference 300 分加权，但这只是排序加成，不是路由隔离。如果用户保存了 30 条行为偏好，它们会挤掉事实型记忆的 TopK 名额。

### 12.3 写入治理（当前最扎实，少数盲区）

**做得好的：**

- **`Memory Gate`**（`gate.go`）完整校验链：userID 非空 → content 非空 → scope type 白名单 → memory type 白名单 → value type 白名单 → extraction method 白名单 → canonical key spec 一致性。每一步都有明确的 client exception。
- **单值键覆盖语义**（`conflict_detector.go:60-84`）事务内完成：旧 `superseded` + 新 `active` + `supersedes_id` 链，逻辑正确。
- **规范化层**（`normalization.go`）厚实：未传治理字段时从 key spec 补默认值，对旧 API 调用方完全兼容。
- **事务包装**（`service.go:261-272`）：`runMemoryMutation` 在注入 `mutationTx` 时走事务，否则退化到裸 repo。`NewMemoryItemTransaction` 通过 GORM transaction 回调正确隔离。

**发现的问题：**

**A. JSON 等值判断存在盲区。** `memoryItemsEquivalent()`（`conflict_detector.go:110-118`）对 `value_type=json` 只做 `collapseInnerWhitespace`（多空格归一），不做 JSON 结构等价比较。`{"a":1,"b":2}` 和 `{"b":2,"a":1}` 会被判定为"不同值"，在单值键场景下触发不必要的 supersede 链。

**B. Embedding 持久化是"发射后不管"。** `persistMemoryEmbedding()`（`service.go:220-238`）静默吞掉所有错误 — embedding 服务挂了，memory 照常创建成功，但该记录从此对向量搜索不可见。没有 reconciliation 机制补建丢失的 embedding，也没有告警。

**C. canonical key 注册表硬编码。** `canonicalMemoryKeyRegistry`（`schema_registry.go:9-73`）是 Go map，目前只有 7 个 key。新增 key = 改代码 + 部署。长期记忆的 key 白名单应该是可动态管理的。

### 12.4 召回检索（概念正确，缺规模化护栏）

**做得好的：**

- 双信号融合（keyword + vector）设计合理，每条命中的记忆标注了 `hitSources`（keyword / vector / hybrid）和 `contributionKind`
- CJK 友好：`scoreMemoryText` 有中日韩 bigram 匹配（`recall_service.go:287-294`）
- 融合分数加权策略合理：keyword 精确匹配 120 分，token 匹配 20 分/token，CJK bigram 8 分/bigram，向量匹配 `score*100 + 30(有 keyword) 或 80(无 keyword)`
- scope 优先级：KB 1000 > global 500，体现了"贴近当前知识库的记忆更相关"
- Recall 结果有丰富的 trace 元数据：`scopeCounts / sourceCounts / contributionCounts / typeCounts / selectedMemoryIDs`

**发现的结构性问题：**

**A. O(n) 全量加载是最大规模化风险。** `RecallMemories` 每次 chat 执行两次无筛选的 `List`（kb + global），各取最多 `MaxCandidatesPerScope` 条（默认 24）。虽然 Limit 参数限制了返回量，但 `List` 在 repo 层扫描所有 active 记录然后排序。`MemoryItemListFilter` 没有 `Keywords` 或 `SearchText` 字段来做数据库层预过滤。如果一个用户有 500 条 active memory，每次 chat 都要先全量加载再做应用层排序。

**B. 规则型/事实型未分治。** 设计文档第 8.1/8.2 节要求的分路径在实际代码中并未实现。preference / knowledge / feedback 三种类型在同一个融合管道中以上述优先级权重竞争 TopK。这违反了设计文档的核心约束："规则型记忆不应和普通 chunk 竞争统一检索排序。"

**C. 无缓存层。** 默认配置下，同一 conversation 的连续多轮对话，每一轮都重新执行完整的 List + Vector Search。没有 conversation 级或 request 级的 recall 缓存。

**D. raw SQL 向量搜索与 GORM 查询并存。** `SearchByVector`（`memory_item_embedding_repo.go:80-150`）用原始 SQL + 手动 Scan 25+ 字段。如果 domain 模型加字段，这个 SQL 必须同步更新。而同一 repo 文件的 `List` 方法用 GORM 查询构建器，字段变更不会遗漏。两种并存风格增加了维护成本。

### 12.5 运维与生命周期（几乎空白）

设计文档 Phase 4（第 10.3 节）列出的内容当前**全部未落地**：

| 能力 | 状态 |
|---|---|
| `last_used_at` 运行时更新 | 字段存在，无任何代码写入 |
| 过期/失效记忆清理 | 无后台 job，无 hard delete |
| 记忆库膨胀监控 | 无 metrics |
| 后台治理任务 | 不存在 |
| embedding reconciliation | 不存在 |

`ExpireMemory` 只是把 status 改成 `expired`（软失效），这些记录和 `superseded` 的记录一样永久留在数据库里。当前没有按 `status + update_time` 的索引优化的批量清理路径。

### 12.6 可扩展性

**当前容易扩展的：**
- 新增 canonical key（改 `schema_registry.go` 一行 map entry，但需部署）
- 切换 embedding 后端（`EmbeddingService` 已接口化）
- 新增 memory type（需同步改 gate / normalization / recall scoring 三处）

**当前难以扩展的：**
- **事实型记忆检索投影（Phase 3）**：如果要把 knowledge 型记忆接入主 retrieve 管道作为独立 channel，`RecallMemories` 的单一文本输出不够。需要 `MemoryChannel` 实现 `RetrieveChannel` 接口，返回 `[]RetrieveChunk` 而不是拼接后的纯文本。
- **异步抽取（Phase 5）**：当前架构没有消息队列或事件驱动的抽取触发点。`SaveExplicitMemory` 是同步的，自动抽取管线需从零搭建。
- **多用户/多租户 scope**：如果加 `scope_type=team`，需改 gate / normalization / recall 三处。

### 12.7 优先修复建议

按影响程度排序：

**P0 — 应立即做（影响规模化正确性）：**

1. **召回链路增加 DB 层关键词预过滤。** 在 `MemoryItemListFilter` 增加 `SearchText` 字段，repo 层对 `summary` / `content` / `display_value` 做 `ILIKE` 或 tsvector 预过滤，避免每次全量加载 active memories。
2. **实现规则型/事实型的读路径分治。** 在 `RecallMemories` 中，将 `memory_type=preference` 的高重要性记忆（如 importance >= 80）直接纳入 "always-include" 集合，不参与 keyword/vector rank 竞争。或者更彻底：在 `runLongTermMemoryStage` 中做两次调用 — 一次直取偏好注入 context，一次走 recall rank。

**P1 — 应尽快做（防止数据腐化）：**

3. **`last_used_at` 运行时更新。** 在 `RecallMemories` 中，为选中的记忆异步或同步写回 `last_used_at`。
4. **embedding 持久化失败告警。** 至少打一条 `log.Warnf`，并在 trace 中标记该 memory 的 embedding 状态。
5. **JSON 等值判断改用结构化比较。** `memoryItemsEquivalent` 对 `value_type=json` 时应 unmarshal 后做 `reflect.DeepEqual` 或排序后字符串比较。

**P2 — 为 Phase 3/4 铺路：**

6. canonical key 注册表从硬编码迁移到配置驱动（至少 yaml），理想情况迁移到 `t_memory_key_spec` 数据库表 + 管理 API。
7. 过期/superseded 记录的定期清理 job（按 `status IN ('expired','superseded') AND update_time < NOW() - INTERVAL '90 days'`）。
8. conversation 级 memory recall 结果缓存（同会话内同一 query 不重复召回）。
9. 暴露 memory 相关 Prometheus metrics：`memory_items_total{status}`, `memory_recall_candidates`, `memory_recall_selected`, `memory_embedding_failures`。

## 13. 2026-05-23 Additional Update: Memory V1 读路径分治与召回收口

### 13.1 状态摘要

- `memory V1` 已从“Phase 2.1 治理闭环完成，但读路径仍未按设计分治”推进到“长期记忆读取路径首次与设计结论对齐”。
- 这轮工作的重点不是开启 `Phase 3` 检索投影，而是先把 `Phase 2.x` 中最关键的读侧结构问题收掉：
  - 规则型记忆不再和事实型记忆竞争统一 TopK
  - 事实型召回不再无脑全量扫 active memories
  - `last_used_at` 首次进入运行时闭环
  - 两个已知的数据腐化点已经补上

### 13.2 已完成内容

#### 1. 长期记忆读取路径已按规则型 / 事实型分治

- `RecallMemories(...)` 当前已拆成两条内部路径：
  - `memory_type=preference`：作为规则型记忆直读，直接进入 `MemoryContext`
  - `memory_type=knowledge`：作为事实型记忆，继续走 recall / rerank / topK
- `memory_type=feedback` 当前明确排除出 chat recall：
  - 仍可保存 / 查询 / expire
  - 但不会再注入聊天上下文

这意味着：

- 规则型记忆不再挤占事实型记忆的 TopK 名额
- 长期记忆读路径终于开始符合第 8 章的设计边界

#### 2. `MemoryContext` 已改成分段渲染

- 当前长期记忆 prompt 注入不再是“一个统一 memory 列表”
- `ContextRenderer` 现在会输出两个显式 section：
  - `Rule Memories`
  - `Fact Memories`
- 每段内部继续保持：
  - `KB-Scoped Memories`
  - `Global Memories`

这使长期记忆在 prompt 中的语义边界变得更清晰，也降低了规则与事实互相干扰的概率。

#### 3. 事实型 recall 新增 DB 层预过滤

- `MemoryItemListFilter` 已新增 `SearchText`
- Postgres `MemoryItemRepository.List(...)` 已支持对以下字段做 `ILIKE` 预过滤：
  - `summary`
  - `content`
  - `display_value`
  - `canonical_key`
- 事实型 recall 当前会先用 `SearchText` 缩小候选，再做应用层 keyword/vector 融合排序
- 规则型直读路径不使用 `SearchText`

这轮之后，之前“每次 recall 都扫描一批 active memory 再做应用层排序”的风险已经被部分收敛，虽然还没有做到真正的全文索引或缓存层。

#### 4. `last_used_at` 已首次接入运行时更新

- `MemoryItemRepository` 已新增：
  - `TouchLastUsed(ctx, userID, ids, at)`
- 长期记忆 recall 选中结果后，会对被实际消费的 memories 批量 best-effort 更新 `last_used_at`
- 更新失败只记 warning，不阻断 chat 主链路

这意味着 `last_used_at` 不再只是 schema 装饰字段，而是开始承载后续 lifecycle 治理所需的运行时信号。

#### 5. 两个已知数据腐化点已补上

- `value_type=json` 的等值判断已从字符串比较升级为结构化比较：
  - JSON 字段顺序不同不再误判为不同值
  - 单值键不再因 JSON key 顺序差异制造无意义的 `superseded` 链
- `persistMemoryEmbedding(...)` 已不再静默吞错：
  - embedding 生成失败会 `Warn`
  - embedding upsert 失败也会 `Warn`
  - 仍保持 fails-open，不影响显式保存成功

### 13.3 当前阶段判断更新

截至 `2026-05-23`，此前第 12 章审计里最关键的几项 P0 / P1 问题已有以下变化：

- “规则型 / 事实型未分治”：
  - 已从设计偏差变成已落地能力
- “DB 层无关键词预过滤”：
  - 已完成第一版 `ILIKE` 预过滤
- “`last_used_at` 无任何代码写入”：
  - 已完成 best-effort 运行时更新
- “embedding 持久化静默失败”：
  - 已补日志告警
- “JSON 等值判断有盲区”：
  - 已改为结构化比较

当前仍未完成的重点则进一步收敛为：

1. 事实型长期记忆仍未进入真正的 `Phase 3` retrieval projection
2. recall 还没有 conversation 级缓存
3. canonical key 仍是硬编码 registry
4. lifecycle cleanup / maintenance / metrics 仍未落地

### 13.4 验证

已验证：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service/longtermmemory -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/adapter/repository/postgres/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/adapter/http/rag/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/bootstrap/rag -count=1
```

结果：

- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/repository/postgres/rag` PASS
- `internal/adapter/http/rag/test` PASS
- `internal/bootstrap/rag` PASS

### 13.5 下一步建议

1. 进入 `Phase 3` 前，先决定事实型 memory 是继续停留在 `MemoryContext`，还是开始抽象为独立 recall channel。
2. 如果下一轮继续做 P1/P2，优先补：
   - conversation 级 recall cache
   - memory metrics
   - cleanup / maintenance job
3. 如果主线切到 `Phase 3`，当前最合适的起点是：
   - 只让 `knowledge` 型长期记忆进入 retrieval projection
   - 不把 `preference` 和 `feedback` 混入统一 retrieve 总线

## 14. 2026-05-23 Additional Update: Phase 3 Retrieval Projection Landed and Phase 4 Cache Design Started

### 14.1 Status Update

截至 `2026-05-23`，此前第 13 章里“事实型长期记忆仍未进入真正的 `Phase 3` retrieval projection”这一点已经发生变化：

- `knowledge` 型长期记忆已作为独立 `memory_fact` channel 接入主 retrieve 流水线
- `preference` 继续只停留在 `MemoryContext`
- `feedback` 继续不进入 chat retrieve

这意味着当前设计状态已从：

- `Phase 2.1` 治理闭环完成
- `Phase 2.x` 读路径按规则型 / 事实型分治

推进到：

- `Phase 3` 最小 retrieval projection 已落地
- `Phase 4` 的缓存基础设施已开始建设

### 14.2 Phase 3 实际落地形态

这次不是把长期记忆“伪装成 document chunk”，而是按设计文档第 8 章和第 10.2 节的方向，做成独立 retrieval channel：

- `internal/app/rag/core/retrieve/search_types.go`
  - 新增 `ChannelMemoryFact = "memory_fact"`
- `internal/app/rag/core/retrieve/channels.go`
  - 新增 fact-memory channel
- `internal/app/rag/service/longtermmemory/retrieve_projection.go`
  - `knowledge` 型 memory 投影成标准 `RetrievedChunk`
- `internal/bootstrap/rag/runtime.go`
  - `explicitMemoryService.FactRetriever()` 已接入 retrieve engine

当前 Phase 3 的边界被明确收敛为：

1. 只让 `memory_type=knowledge` 进入 retrieval projection
2. `preference` 继续只走 `MemoryContext`
3. `feedback` 继续排除在 chat retrieve 之外
4. `memory_fact` 进入现有 `fusion / dedup / rerank` 流水线，但默认权重低于 `vector_global`

### 14.3 Phase 3 第一版收益闭环

当前代码与测试已经验证以下场景：

- `doc miss, memory hit`
  - 文档检索全空时，事实型长期记忆可以救回 retrieve
- `doc hit + memory hit`
  - 文档仍优先，memory 作为补充
- `preference isolation`
  - 规则型记忆不混入 retrieve 结果
- 项目事实类场景
  - `main_bus removed`
  - `dependency constraint`
- scope 优先级
  - `kb fact > global fact`

这说明当前的 `memory_fact` 已经不是“prompt 拼接文本”，而是主检索链路中的第一版正式能力。

### 14.4 对第 12 章审计结论的修正

此前审计中的以下结论需要更新：

- “事实型记忆检索投影尚未开始”
  - 已不再成立
  - 当前应改为：**Phase 3 最小版本已落地，但尚未进入更细的 query-aware / policy-aware 调权阶段**

- “`RecallMemories` 的单一文本输出不够，需抽象 `MemoryChannel`”
  - 该方向已开始落实
  - 当前 `MemoryChannel` 的第一版就是 `memory_fact`

- “无缓存层”
  - 这条不再是完全成立
  - 当前应改为：**Redis 导向的 recall cache skeleton 已建立，但 request-scope / conversation-scope cache 仍未落地**

### 14.5 Phase 4 当前设计收敛

在进入更复杂的 retrieval tuning 前，当前已经决定 `Phase 4` 先聚焦 cache，而不是先做更复杂的排序策略。

当前确定要进缓存的三类对象：

1. `rule memories`
   - 即 `preference` 这类规则型长期记忆
   - 与 query 排序无关，适合独立缓存

2. `fact ranking result`
   - 即 `knowledge` 型 memory 的已融合排序结果
   - 是 `RecallMemories(...)` 与 `SearchFacts(...)` 共同复用的核心缓存对象

3. `query embedding`
   - 为 fact recall 与后续 session recall 提供基础复用

当前明确不缓存的对象：

- 最终 `KnowledgeContext` 大字符串
- 最终整包 `RetrievedChunk` 结果
- 整个 `prepareChat(...)` 结果

### 14.6 当前已落地的 Phase 4 cache skeleton

当前已开始的实现包括：

- `longtermmemory.RecallCache` 抽象
- Redis 适配器
- `global / kb` scope version
- `SaveExplicitMemory(...)` / `ExpireMemory(...)` 成功后的 version bump
- `RecallMemories(...)` / `SearchFacts(...)` 对：
  - `rule memories`
  - `fact rankings`
  - `query embedding`
  的缓存接缝

当前仍未完成的部分：

1. request-scope `L1` cache
2. conversation/session recall cache
3. cache hit/miss/ttl/version bump 的 metrics
4. Redis 不可用时的更细粒度降级观测

### 14.7 当前阶段的最终判断更新

截至 `2026-05-23`，`memory V1` 的核心状态已经不再是“治理完成但 retrieval 未开始”，而是：

1. 显式长期记忆已具备治理闭环
2. 规则型 / 事实型读路径已按设计分治
3. `knowledge` 型长期记忆已进入主检索链路
4. 下一阶段的真正主线已转向 `Phase 4`：
   - cache
   - lifecycle maintenance
   - metrics / diagnostics

因此，当前最重要的工程顺序应调整为：

1. 先补完 `Phase 4` cache closure
2. 再补 cleanup / maintenance / metrics
3. 最后才继续做更复杂的 `memory_fact` 融合策略细化

## 14.8 Additional Update: 2026-05-23 Phase 4 Cache Closure 已落地

本节用于修正前文中“Phase 4 仍处于 skeleton / 待实现”这一阶段性表述。

截至 `2026-05-23`，`memory V1` 的 `Phase 4` 已从设计和骨架推进到完整闭环实现。

### 14.8.1 当前缓存分层已经成型

当前 memory cache 采用三层结构：

1. `request-scope L1`
   - 单次 `prepareChat(...)` 生命周期内共享
   - 用于复用：
     - `rule memories`
     - `fact ranking result`
     - `query embedding`
     - `session recall` 请求内结果

2. `conversation-scope L1`
   - 进程内 `TTL + LRU`
   - 当前只用于 `session recall`

3. `Redis L2`
   - 继续承载长期记忆的：
     - `rule memories`
     - `fact ranking result`
     - `query embedding`
   - 通过 `global / kb` scope version 保证失效

### 14.8.2 长期记忆缓存闭环已完成

当前 `RecallMemories(...)` 与 `SearchFacts(...)` 已统一遵循：

1. 查 request-scope `L1`
2. 查 Redis `L2`
3. miss 后再做 DB / vector recompute

这意味着：

- `RecallMemories(...)` 与 `SearchFacts(...)` 已能共享同一份 fact ranking 结果
- query embedding 已成为长期记忆与 session recall 之间的共享缓存资产
- cache hit 不会破坏 `TouchLastUsed(...)` 语义

当前仍明确不缓存：

- 最终 `MemoryContext`
- 最终 `KnowledgeContext`
- 最终整包 `RetrievedChunk`
- 整个 `prepareChat(...)`

### 14.8.3 Session Recall Cache 已纳入同一轮建设

本轮新增了 `session recall` 的独立缓存闭环。

关键点：

- `SessionChunkRepository` 已支持读取 recall fingerprint
- fingerprint 由以下信号组成：
  - recallable chunk 是否存在
  - recallable chunk 数量
  - 最新更新时间
  - 最新 chunk / message 标识
- `session recall` 的最终结果已放入 conversation-scope cache
- `excludeMessageID`、rewritten query、recall fingerprint 与核心 recall 参数都会进入 cache key
- 空结果允许短 TTL 缓存，以抑制重复 miss 风暴

因此，前文中 “session recall cache 尚未开始” 的判断已不再成立，当前应修正为：

- **session recall cache 已落地，后续重点转向命中率、容量和生命周期策略优化**

### 14.8.4 可观测性已从附加项变成正式能力

Phase 4 当前已不仅是“能命中 cache”，而是具备基础诊断能力：

- 新增 `rag.memory.cache.*` 配置边界
- 新增 `GET /rag/memory/metrics`
- `long_term_memory` trace 已包含：
  - `cacheEnabled`
  - `ruleCacheLayer`
  - `factCacheLayer`
  - `embeddingCacheLayer`
  - `scopeVersions`
  - `recomputeReason`
- `session_recall` trace 已包含：
  - `cacheEnabled`
  - `cacheLayer`
  - `recallFingerprint`
  - `embeddingCacheLayer`
  - `recomputeReason`

同时，缓存异常统一采用 `fail-open`：

- Redis 不可用
- Redis 反序列化失败
- fingerprint 查询失败
- local cache evict / overflow

都只允许降级，不允许把 recall 主链路变成失败路径。

### 14.8.5 对当前阶段结论的进一步更新

截至 `2026-05-23`，本设计文档中的阶段结论应进一步更新为：

1. `Phase 1.5` 会话内长消息 recall 已完成
2. `Phase 2` 显式长期记忆保存 / 查询 / 过期 / chat 前 recall 已完成
3. `Phase 2.1` 治理闭环已完成
4. `Phase 3` `knowledge` 型长期记忆已进入主 retrieve
5. `Phase 4` cache closure 与 session recall cache 已完成

因此，memory 后续更合理的工程优先级应变为：

1. lifecycle cleanup / maintenance
2. metrics refinement / diagnostics hardening
3. `memory_fact` 融合与调权继续细化
## 14.9 Additional Update: 2026-05-24 P0 Correctness Hardening and P1 Retrieval Quality

### 14.9.1 Status Update

As of `2026-05-24`, the memory track has not entered a brand-new phase. Instead, it completed a focused hardening pass on top of the current `Phase 4+` implementation.

This round corrected three classes of problems that were already visible in the codebase:

1. single-valued memory governance could still be broken by concurrency
2. cache fallback preserved availability but could still recompute too much inside one request
3. fact-memory lexical retrieval remained too dependent on fragile full-query substring matching

So the current design status should now be read as:

- `Phase 4` cache closure: implemented
- `P0` correctness hardening: implemented
- `P1` first-pass retrieval quality tuning: partially implemented

### 14.9.2 Single-valued governance is now protected by the database

This round formalized an important design principle that previously existed mostly at the service layer:

> for a single-valued canonical key, there must be exactly one `active` version per `(user, scope, canonical_key)`

The implementation now includes:

- a database migration that creates a partial unique index for single-valued canonical keys
- `COALESCE(scope_id, '')` normalization to keep `global` and `kb` scopes consistent
- a duplicate-active precheck inside the migration so dirty historical data is surfaced explicitly before the unique constraint is created

This matters because the previous "query active -> decide create/update -> write" flow could still race under concurrent requests.

### 14.9.3 Save-path conflict handling is now convergence-oriented

The write path is now aligned with the intended governance model:

- if a single-valued write hits the unique active constraint during concurrent save:
  - reload active record
  - if semantically equal, return existing active memory
  - if a different concurrent winner already landed, return that winner
  - if multiple active rows are found, stop and surface governance corruption

In other words, the system now treats single-valued write races as a recoverable governance scenario, not only as a raw persistence error.

### 14.9.4 Conflict detection no longer depends on a small recent window

The previous implementation relied on loading only a small set of active memories and then reasoning from that subset.

That assumption is no longer part of the design:

- single-valued keys now use exact active-record loading by `(user, scope, canonical_key)`
- reusable active-conflict detection has been added at the repository layer
- service-layer governance now explicitly rejects "multiple active single-valued records" instead of silently continuing

This is an important design correction because a governance system must become more reliable as data grows, not less.

### 14.9.5 Cache fallback semantics are now closed at request scope

The design intent of `fail-open` was always:

- cache failure should not break chat
- degraded cache infrastructure should not explode repeated work inside one request

The second half is now implemented more faithfully:

- when scope version lookup is unavailable
- when Redis is disabled
- when Redis read/write falls back

the long-term-memory recall path still writes the computed result into request-scope cache.

So current semantics are now:

1. try request-scope `L1`
2. try Redis `L2`
3. recompute if needed
4. still backfill request-scope `L1` even on fallback/degraded paths

### 14.9.6 Rule memories now better match their intended semantics

The design has long treated rule memories as stable constraints rather than ordinary recall candidates.

This round improved implementation alignment by stabilizing rule-memory ordering with:

1. scope priority
2. importance
3. `last_confirmed_at`
4. `update_time`

This means rule memories are now materially closer to "governed instruction context" and less like "recently edited preference rows."

### 14.9.7 Fact-memory lexical retrieval is now token-aware

The first lexical prefilter version only used raw full-query substring matching.

The current design/implementation now supports a stronger intermediate form:

- `SearchText`: raw query fallback
- `SearchTokens`: tokenized lexical expansion

Token construction currently covers:

- ASCII word tokens
- continuous CJK bigrams
- mixed-language query cases

Repository-side SQL now applies token-based `OR` prefiltering across:

- `summary`
- `content`
- `display_value`
- `canonical_key`

This is still not a full-text index design, but it is a meaningful shift away from brittle whole-query matching.

### 14.9.8 Token denoising and lexical scoring are now aligned

An additional design-quality improvement landed in the same round:

- low-value lexical noise is now filtered from search-token construction
- the same filtered token set is also reused by in-process lexical scoring

So candidate prefiltering and application-layer ranking are now based on the same token logic rather than drifting apart.

Current denoising includes lightweight filtering for:

- common English function words such as `how`, `should`, `the`, `please`
- common Chinese filler phrases such as `请问`, `这个`, `可以`, `怎么`, `了吗`

### 14.9.9 Updated phase judgment

As of `2026-05-24`, the most accurate phase judgment is:

1. `Phase 1.5`: session-level long-message/session-recall path completed
2. `Phase 2`: explicit long-term-memory save/query/expire/chat-pre-recall completed
3. `Phase 2.1`: governance closure on the explicit-save path completed
4. `Phase 3`: `knowledge`-type long-term memory entered the main retrieve pipeline
5. `Phase 4`: cache closure completed
6. `Phase 4+`: correctness hardening and first-pass recall-quality tuning now completed for the current round

So the next most reasonable design priorities are now:

1. lifecycle cleanup / maintenance
2. metrics refinement / diagnostics hardening
3. code-structure cleanup around long-term-memory recall and cache logic
4. only after that, more aggressive `memory_fact` weighting/policy tuning

## 14.10 Additional Update: 2026-05-25 Package Boundary Refactor Landed

### 14.10.1 Status Update

As of `2026-05-25`, the previously planned structural cleanup around long-term-memory recall and cache code is no longer just a recommendation. It has landed in code.

This round stays within the same `Phase 4+` architecture scope. It does not change the product contract, the HTTP contract, or the runtime wiring order. It changes code ownership and dependency boundaries.

### 14.10.2 Root package remains stable, but internal responsibilities are now separated

The public application entry still remains:

- `internal/app/rag/service/longtermmemory`

So existing callers do not need large-scale import churn.

Internally, responsibilities are now split into:

- root package
  - owns `MemoryService`
  - keeps public input / output / option exports stable
  - remains the main application-layer facade
- `governance/`
  - owns normalize / gate / schema / conflict detection / save / lifecycle behavior
- `recall/`
  - owns recall / ranking / lexical tokens / retrieval projection / context rendering / cache support

This is the main architectural correction of the round: write-path governance and read-path recall are no longer implemented as one flat service package.

### 14.10.3 Cross-layer contracts now live in `port`

The cache and mutation transaction contracts were moved to:

- `internal/app/rag/port/memory_recall_cache.go`
- `internal/app/rag/port/memory_mutation_transaction.go`

This changes the dependency direction from:

```text
adapter -> longtermmemory internals
```

to:

```text
adapter -> port <- service
```

So Redis cache and Postgres transaction adapters no longer reverse-depend on the long-term-memory service package just to share contracts.

### 14.10.4 Why a small `types/` leaf package still exists

The original restructuring idea tried to avoid generic package names such as `core/`, and that decision remains correct.

However, a small `types/` leaf package is still intentionally kept under:

- `internal/app/rag/service/longtermmemory/types`

Its purpose is narrow:

- hold shared public DTOs and options
- let the root package and the `governance` / `recall` subpackages reuse those types
- avoid Go import cycles between parent package and child packages

So `types/` is not a new "god package" and not a semantic replacement for the old flat implementation. It is a technical leaf package used to preserve stable public API ownership without creating cyclic imports.

### 14.10.5 Recall internals are now split into narrower implementation units

Inside `recall/`, the code is further decomposed into files such as:

- `service.go`
- `ranking.go`
- `tokens.go`
- `projection.go`
- `context_renderer.go`
- `cache_support.go`
- `cache_keys.go`
- `cache_mappers.go`

This matters because the recall/cache path had already become the densest and most change-prone part of the memory module.

### 14.10.6 Integration impact

The same boundary cleanup now propagates to:

- session recall service/cache integration
- runtime wiring in `internal/bootstrap/rag/runtime.go`
- Redis cache adapter
- Postgres memory transaction adapter

The important point is that behavior stays the same, but ownership is clearer and future refactors no longer require adapters to import business-package internals.

### 14.10.7 Validation

Validated on `2026-05-25`:

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/longtermmemory -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service ./internal/bootstrap/rag ./internal/adapter/repository/postgres/rag -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/rag/service/... ./internal/adapter/cache/redis ./internal/adapter/repository/postgres/rag ./internal/bootstrap/rag -run Test^$ -count=1
```

Current result:

- `internal/app/rag/service/longtermmemory` PASS
- `internal/app/rag/service` PASS
- `internal/bootstrap/rag` PASS
- `internal/adapter/repository/postgres/rag` PASS
- `internal/adapter/cache/redis` PASS

### 14.10.8 Updated priority judgment

As of `2026-05-25`, "code-structure cleanup around long-term-memory recall and cache logic" should no longer remain on the active near-term priority list.

The more accurate next priorities are now:

1. lifecycle cleanup / maintenance jobs
2. metrics refinement / diagnostics hardening
3. direct unit-test coverage for `recall` internals
4. only after that, further `memory_fact` weighting / policy tuning
