# Memory Architecture Design

讨论日期：2026-05-17  
重写日期：2026-05-22

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
  - `MemoryService` 负责显式保存与管理
  - `RecallService` 负责聊天前 recall
  - `ContextRenderer` 负责生成 `MemoryContext`

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
