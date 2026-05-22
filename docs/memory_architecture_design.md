# Memory Architecture Design

讨论日期：2026-05-17  
改写日期：2026-05-19

这份文档将原有的 memory 设计讨论，收敛为一版贴近当前 `goagent` 实现状态的 **V1 可实施方案**。目标不是一次性完成“完整记忆系统”，而是在不打乱现有 RAG / Knowledge / Ingestion 主链路的前提下，先落地最有价值、最稳妥的一版。

---

## 1. 设计目标

V1 目标：

1. 解决超长单条消息对上下文窗口和摘要压缩的污染问题
2. 为“跨对话可复用”的用户偏好、领域事实、反馈模式建立长期记忆能力
3. 让长期记忆可以被后续对话自动召回，而不是只停留在数据库记录层
4. 尽量复用现有 `conversation_message`、`retrieve`、`knowledge/vector` 能力，避免另起一套大系统

V1 非目标：

1. 不追求一次性做完“全自动记忆抽取”
2. 不追求一开始就支持复杂冲突合并、版本治理、记忆评分体系
3. 不要求把长期记忆完全伪装成普通 knowledge document
4. 不要求会话结束后自动做全局总结与长期提炼

---

## 2. 当前实现约束

在当前代码基础上，memory 设计必须尊重以下事实：

### 2.1 短期记忆已存在

当前已有短期记忆能力：

- `conversation_message`
- `conversation_summary`
- `LoadHistory`
- `CompressIfNeeded`

也就是说，“当前对话窗口内的短期记忆”已经成立，V1 不是从零设计短期记忆，而是在其上补强。

### 2.2 会话不绑定知识库

当前 `conversation` 仅绑定：

- `conversation_id`
- `user_id`

而 `knowledgeBaseId` 是在每次 chat 请求中动态传入的。  
这意味着：

- 不能把“conversation = 长期记忆作用域”当成默认前提
- 长期记忆必须显式建模作用域，而不是从 conversation 隐式推断

### 2.3 `ThinkingContent` 不适合作为长消息原文存储

现有实现中，`ThinkingContent` 已用于 assistant thinking / 推理补充内容。  
因此：

- 不能把“超长用户消息原文”直接塞进 `ThinkingContent`
- 否则会混淆字段语义，并污染后续展示、调试和 memory 装载逻辑

### 2.4 `knowledge document` 当前仍偏“文档模型”

当前 `knowledge document` 的语义中心仍然是：

- 文件
- URL
- 远程抓取内容
- pipeline / chunk / index

现阶段并不天然适合直接作为“长期记忆主模型”。  
尤其是：

- `sourceType=memory` 当前未真正支持
- 文档模型缺少 memory 专属字段
- 文档权限、文档生命周期和记忆语义并不完全一致

### 2.5 检索主过滤维度仍是 `KnowledgeBaseIDs`

当前 retrieve 主过滤入口是 `KnowledgeBaseIDs`。  
这意味着：

- “全局 user_memory + 标签过滤”在概念上可行
- 但当前并没有一套完整的“任意 memory tag 路由与过滤”机制

因此 V1 设计需要兼顾未来扩展和现状可落地性。

---

## 3. 两层记忆边界

V1 保持“短期 / 长期两层记忆”设计，不做概念调整。

```text
短期记忆                          长期记忆
────────────────────────────────────────────────────
生命周期：当前对话窗口内           生命周期：跨对话持久化
存储方式：conversation_message     存储方式：memory_item + 检索投影
检索方式：直接塞上下文              检索方式：retrieve 召回
内容：本轮问题、上下文、工具结果     内容：用户偏好、领域事实、反馈模式
```

### 3.1 短期记忆

短期记忆继续由以下能力承载：

- `conversation_message`
- `conversation_summary`
- `LoadHistory`
- `CompressIfNeeded`

其中 V1 只增强“超长消息处理”，不改变短期记忆主结构。

### 3.2 长期记忆

长期记忆 V1 不直接等同于 `knowledge document`，而是分成两层：

1. **业务主模型：`memory_item`**
2. **检索投影：chunk/vector metadata**

这样做的原因是：

- `memory_item` 适合承载记忆语义、作用域、状态、冲突和过期
- 检索投影适合复用现有 retrieve / vector 能力

---

## 4. V1 核心结论

### 4.1 长消息处理：写时摘要

保持原讨论结论：**写时摘要优于读时摘要**。

原因：

- 长消息内容一旦写入，其语义通常是稳定的
- 每次读取再做摘要会增加延迟和成本
- 写时摘要更容易与短期记忆装载逻辑衔接

### 4.2 长期记忆：显式保存优先

V1 不从“全量消息异步自动分类”起步，而是优先做高精度入口：

- `/remember`
- 手动保存
- 用户明确偏好声明
- 用户明确纠正并要求遵循

结论：**高精度、低召回，优先于高召回、低稳定性**。

### 4.3 长期记忆作用域：双层作用域

V1 采用双层作用域：

- `global`
- `kb`

其中：

- `global` 适合用户偏好、长期习惯、稳定反馈
- `kb` 适合项目/知识库相关领域事实、局部纠错

这比“全局 vs 按项目二选一”更贴近当前系统结构。

### 4.4 长期记忆落地方式：主模型 + 投影

V1 采用：

```text
memory_item（主模型）
  -> 异步投影
  -> chunk/vector
  -> 后续 retrieve 自动召回
```

不建议 V1 直接把长期记忆完全伪装成普通 `knowledge document`。

---

## 5. 长消息处理设计

### 5.1 目标

避免以下内容直接进入短期上下文主文本：

- 长日志
- 长堆栈
- 长代码片段
- 大段文档引用
- 超长说明性文字

同时建立一套可扩展的长文本分层处理策略，使“上下文压缩”、“会话内可检索原文”和“长期记忆沉淀”三件事彼此解耦，而不是混成一条链路。

### 5.1.1 三层存储语义

长消息处理涉及三层不同语义，V1 必须显式分开：

1. **上下文层**
   - 当前轮直接给模型看的内容
   - 目标是控制 token 消耗，保留高密度信息

2. **会话检索层**
   - 面向当前会话的长原文 chunk 化存储
   - 目标是在后续轮次需要时按需召回原文细节

3. **长期记忆层**
   - 仅保存值得跨对话复用的偏好、领域事实、反馈模式
   - 不等于“所有长消息都入长期 memory”

关键原则：

- `chunk 入库` 默认属于 **会话检索层**
- 只有命中长期记忆规则的内容，才进入 **长期记忆层**

### 5.2 数据模型调整

建议在消息模型上新增以下字段：

- `raw_content`
- `content_summary`
- `is_summarized`

建议语义：

- `content`
  - 对模型和前端默认展示的主文本
  - 普通消息时等于原文
  - 超长消息时等于摘要
- `raw_content`
  - 原始完整消息
- `content_summary`
  - 对原始内容生成的摘要
- `is_summarized`
  - 是否发生过摘要替换

不建议复用：

- `ThinkingContent`

### 5.3 摘要生成策略

#### 规则优先

以下内容优先走规则摘要：

- 日志
- 错误堆栈
- 代码
- Markdown 结构化内容
- 明显的列表/表格类文本

目标：

- 零或低延迟
- 输出稳定
- 减少 LLM 成本

#### LLM 补充

以下内容可走 LLM 摘要：

- 通用自然语言长文本
- 混合型说明文本
- 难以规则抽取重点的复杂叙述

### 5.4 接入点

统一接入：

- `ConversationMessageService.AddMessage()`

处理顺序建议：

```text
收到消息
  -> 检测长度 / 结构特征
  -> 决定是否摘要
  -> 生成摘要
  -> 写 message
```

### 5.5 V1 阈值策略

V1 建议采用如下分层策略：

#### A. 小于 3000 token

- 直接进入上下文层
- 原文正常写入消息表
- 不做额外 chunk 化

适用场景：

- 普通问题
- 中短说明
- 代码片段较短的提问

#### B. 3000 ~ 12000 token

- 生成 `short_summary` 放入上下文层
- 原文写入 `raw_content`
- 原文切 chunk，进入会话检索层

适用场景：

- 较长日志
- 中长代码片段
- 较长文档节选
- 带较多背景信息的问题描述

注意：

- 此阶段的 `chunk 入库` 默认仅服务于**本会话后续召回**
- 不默认进入长期记忆层

#### C. 大于 12000 token

- 先切 chunk
- 对每个 chunk 生成 chunk 级摘要
- 再合并成总摘要放入上下文层
- chunk 原文进入会话检索层

适用场景：

- 超长日志包
- 大段文档
- 多文件拼接内容
- 很长的问题背景说明

补充说明：

- `chunk summary -> merged summary` 是分层摘要，不是简单截断
- 代码 / 日志 / 表格类 chunk 优先规则摘要
- 通用自然语言 chunk 再考虑 LLM 摘要

### 5.6 V1 判定规则

建议先基于简单规则判断是否触发摘要：

- 字符数超过阈值
- 行数超过阈值
- 检测到典型日志/堆栈模式
- 检测到代码块或高密度符号文本

阈值建议先配置化。

同时建议保留以下扩展能力：

- 阈值按模型上下文窗口动态调整
- 阈值按当前会话历史长度动态调整
- 阈值按内容类型动态调整

### 5.7 长消息处理的可扩展性判断

这套阈值策略的可扩展性整体较好，原因在于它天然支持：

1. **模型维度扩展**
   - `3000 / 12000` 可以从固定阈值演进为动态预算

2. **处理链路扩展**
   - 已具备直接透传、单层摘要、分层摘要三种形态
   - 后续可增加超大文本异步预处理模式

3. **内容类型扩展**
   - 日志、代码、自然语言可以逐步分流到不同摘要器

4. **召回范围扩展**
   - 先做会话内原文召回
   - 后续再升级为跨会话长期 recall

但要明确其扩展边界：

- 如果把长消息处理、会话检索、长期记忆混成一层，扩展性会迅速下降
- 如果明确三层存储语义，后续扩展会比较平滑

---

## 6. 长期记忆主模型

### 6.1 新增 `memory_item`

建议新增独立主模型：

```text
memory_item
  id
  user_id
  scope_type        # global | kb
  scope_id          # 空或具体 knowledge_base_id
  memory_type       # preference | knowledge | feedback
  source_message_id
  content
  summary
  confidence
  status            # pending | active | rejected | expired
  last_confirmed_at
  expires_at
  created_by
  updated_by
  create_time
  update_time
```

### 6.2 字段说明

- `scope_type`
  - `global`：全局用户记忆
  - `kb`：知识库作用域记忆
- `scope_id`
  - `global` 时可为空
  - `kb` 时保存具体 `knowledge_base_id`
- `memory_type`
  - `preference`：用户偏好
  - `knowledge`：用户提供的项目/领域事实
  - `feedback`：对系统行为的纠正与偏好反馈
- `source_message_id`
  - 来源消息，便于回溯和去重
- `confidence`
  - 仅对自动抽取有意义
  - V1 显式保存可直接置高
- `status`
  - 控制是否参与召回

### 6.3 为什么不直接用 `knowledge_document`

主要原因：

1. 记忆需要自己的生命周期
2. 记忆需要自己的作用域语义
3. 记忆可能会发生覆盖、确认、过期、冲突
4. 文档模型当前过于偏文件/URL 输入

所以 V1 建议：

- `memory_item` 负责业务真相
- `knowledge/vector` 负责检索消费

---

## 7. 哪些消息值得成为长期记忆

### 7.1 V1 允许入库的高信号内容

- 用户显式偏好声明  
  例如：
  - “以后都用中文回答”
  - “先看代码再给建议”

- 用户显式纠正后的行为要求  
  例如：
  - “不要先讲理论，先定位代码”

- 用户提供的项目事实 / 领域知识  
  例如：
  - “我们项目用了自定义 chunker”
  - “这个服务部署在内网，不能联网”

- 用户明确确认的成功路径  
  例如：
  - “这个排查方式可以，以后按这个来”

### 7.2 V1 明确不入库的内容

- 问候
- 一次性闲聊
- 临时请求
- 原始日志 / 原始堆栈
- 中间工具调用摘要
- 未被用户确认的 assistant 结论

### 7.3 灰区内容

以下内容 V1 默认不自动入库，可后续阶段再做：

- 隐含偏好
- 复杂场景中隐式暴露的领域上下文
- 对 assistant 多轮反复纠正后归纳出的模式

---

## 8. 识别策略

### 8.1 V1：只做第一层高精度漏斗

V1 仅实现第一层：

```text
第一层：显式保存 / 高信号规则
```

可支持：

- `/remember`
- 手动保存
- 明确句式匹配
- 用户反馈绑定保存

### 8.2 V2：异步 LLM 分类

后续再引入：

```text
消息入库
  -> 异步 LLM 分类
  -> 判断 preference / knowledge / feedback / temporary
  -> 命中才创建 memory_item
```

要求：

- 不阻塞主聊天请求
- cheap model 优先
- 必须可灰度关闭

### 8.3 V3：会话结束聚合

后续能力：

- 会话结束后总结用户偏好变化
- 提炼反复出现的领域事实
- 识别被多次纠正的错误模式

这不是 V1 必需项。

---

## 9. 长期记忆召回设计

### 9.1 V1 召回范围

V1 召回建议拆成两类：

1. `global memory`
2. `kb-scoped memory`

当请求带有 `KnowledgeBaseIDs` 时：

```text
先召回 kb-scoped memory
再召回 global memory
与普通 knowledge retrieve 结果合并
```

当请求不带 `KnowledgeBaseIDs` 时：

```text
仅召回 global memory
```

### 9.2 检索投影

建议把 `memory_item` 异步投影到可检索索引时，写入 metadata：

- `memory_item_id`
- `user_id`
- `scope_type`
- `scope_id`
- `memory_type`
- `source_message_id`

### 9.3 召回优先级

建议优先级：

1. 当前 KB 相关的 `kb-scoped memory`
2. 用户 `global memory`
3. 普通知识库 chunk
4. 外部搜索结果

这样可以保证：

- 用户长期偏好优先于普通文档
- 项目事实优先于全局偏好
- 本地知识优先于外部搜索

### 9.4 V1 不强求 retrieve 全重构

V1 不要求立即把 retrieve 改造成复杂 memory router。  
可以先做较轻量接入：

- 在 chat prepare 阶段单独召回 memory
- 再把 memory 结果并入 prompt context

这样对现有 retrieve 主链路侵入更小。

### 9.5 会话检索层与长期记忆层的边界

V1 需要明确区分：

- **会话检索层**
  - 为当前 conversation 服务
  - 主要承接长消息原文 chunk
  - 生命周期可短于长期记忆

- **长期记忆层**
  - 为跨对话复用服务
  - 只保留高价值稳定信息
  - 需要更严格的入库规则

因此：

- “原文切 chunk 入库”不等于“写入长期 memory”
- 长消息进入会话检索层，不代表它天然值得跨会话保留

---

## 10. 与现有模块的关系

### 10.1 与 `conversation_message`

关系：

- 仍是短期记忆主载体
- 承载长消息摘要后的 `content`
- 原文由新增字段承载

### 10.2 与 `conversation_summary`

关系：

- 仍用于多轮压缩
- 长消息摘要会减少 summary 压缩污染

### 10.3 与 `knowledge / vector`

关系：

- 作为长期记忆的检索消费层
- 不一定是长期记忆业务主表

### 10.4 与 `ingestion pipeline`

V1 不强制“memory 必须完整复用现有 document upload 流程”。  
更合理的复用边界是：

- 复用 chunking 思路
- 复用 embedding / vector upsert
- 复用 retrieve 能力

而不是强行套用：

- 文件上传语义
- `sourceType=file|url`
- 文档调度语义

---

## 11. V1 建议接口

### 11.1 Message 侧

在 `AddMessage` 上增加内部处理，不一定暴露新 API：

- `MaybeSummarizeLongMessage(...)`

### 11.2 Memory 侧

建议新增独立服务：

- `MemoryService`

建议最小接口：

```text
SaveExplicitMemory(ctx, input)
ListMemories(ctx, filter)
RecallMemories(ctx, input)
ExpireMemory(ctx, id)
```

### 11.3 异步处理

建议新增异步任务：

- `ProjectMemoryFromItem`

职责：

- 读取 `memory_item`
- 生成检索文本
- 写入向量索引或检索投影

---

## 12. V1 时序

### 12.1 长消息写入

```text
用户发消息
  -> AddMessage
  -> 检测是否超长
  -> 规则/LLM 摘要
  -> 写 message(content=摘要, raw_content=原文)
```

### 12.2 显式记忆保存

```text
用户发消息 / 用户手动保存
  -> MemoryService.SaveExplicitMemory
  -> 创建 memory_item(status=active)
  -> 投递异步投影任务
  -> 写入检索索引
```

### 12.3 对话召回

```text
进入 chat
  -> LoadHistory
  -> RecallMemories(global / kb)
  -> 如果需要再走普通 retrieve
  -> 合并到 prompt context
```

---

## 13. 分阶段实施建议

### Phase 1：长消息处理

目标：

- 让短期记忆不再被超长原文污染

工作项：

- 消息表新增字段
- `AddMessage` 增加长消息检测与摘要
- `LoadHistory` 保持使用 `content`
- 落地 `小于3000 / 3000~12000 / 大于12000` 三档策略

### Phase 1.5：会话检索层

目标：

- 让长消息原文可在本会话内按需召回

工作项：

- 为长消息建立 chunk 化存储
- 接入当前 conversation 作用域的原文检索
- 明确与长期记忆层分离

### Phase 2：显式长期记忆

目标：

- 支持用户主动保存长期记忆

工作项：

- 新增 `memory_item`
- 新增 `MemoryService`
- 支持 `global / kb` 双作用域
- 支持最小 recall
- 仅对显式保存或高信号规则命中的内容创建 memory_item

### Phase 3：检索投影

目标：

- 长期记忆可自动参与后续对话召回

工作项：

- memory 投影到检索索引
- 召回结果并入 prompt

### Phase 4：策略扩展

目标：

- 让长消息处理从固定规则演进为可扩展策略系统

工作项：

- 按内容类型分流摘要器
- 支持动态 token 阈值
- 支持摘要成本 / 延迟观测
- 支持会话检索层生命周期治理

### Phase 5：异步自动识别

目标：

- 降低用户手动保存成本

工作项：

- 异步 LLM 分类
- 分类置信度阈值
- 去重与覆盖策略初版

---

## 14. 待后续扩展的问题

以下问题记录为后续阶段议题，不阻塞 V1：

1. 自动分类 prompt 如何设计
2. 同类偏好如何覆盖或合并
3. 记忆是否需要人工确认
4. 记忆如何过期
5. `feedback` 类记忆如何防止短期噪音进入长期层
6. memory recall 是否应进入 trace 可观测链路
7. memory 与现有 knowledge base 权限模型如何打通
8. 会话检索层的数据保留时长与清理策略
9. 是否需要为超长消息处理增加独立 trace / metrics

---

## 15. 最终结论

V1 设计结论如下：

1. **保留两层记忆分层**
   - 短期：`conversation_message`
   - 长期：`memory_item + 检索投影`

2. **长消息处理采用写时摘要**
   - 规则优先
   - LLM 补充
   - 不复用 `ThinkingContent`

3. **长期记忆 V1 先做显式保存**
   - 高精度优先
   - 暂不追求全自动

4. **长期记忆采用双层作用域**
   - `global`
   - `kb`

5. **复用现有检索能力，但不强行把记忆等同于文档**
   - memory 有自己的业务主模型
   - knowledge/vector 负责检索消费

6. **长消息处理采用三层分离**
   - 上下文层：控制当前轮 token
   - 会话检索层：保存长原文 chunk 供本会话召回
   - 长期记忆层：仅保存跨对话高价值信息

7. **阈值策略具备良好可扩展性**
   - 小于 3000：直接进上下文
   - 3000 ~ 12000：短摘要进上下文，原文 chunk 进入会话检索层
   - 大于 12000：chunk 摘要后再合并总摘要，原文 chunk 进入会话检索层

这版方案的目标不是”设计最完整的记忆系统”，而是用最小、最稳、最符合当前代码结构的方式，让 `goagent` 先拥有真正可用的 memory V1。

---

## 15.1 2026-05-22 设计澄清：规则型与证据型 memory 分治

### 背景

结合当前 `goagent` 的真实需求，memory 不宜只按“短期 / 长期”两层理解，还应按**用途**再拆成两类对象：

1. **规则型 memory**
   - 面向用户偏好、稳定事实、行为约束、经用户确认的固定工作方式
   - 典型例子：
     - “以后都用中文回答”
     - “先看代码再给建议”
     - “这个服务部署在内网，不能联网”
     - “不要先讲理论，先定位代码”
   - 主要目标：稳定影响模型行为与回答风格

2. **证据型 memory**
   - 面向用户在当前会话中提供的超长日志、长配置、长代码片段、长文档节选
   - 主要目标：在后续追问时补回关键原文细节
   - 这类内容更像“会话内补证据”，而不是长期改变模型行为

### 推荐方案

#### 1. 结构化长期记忆：优先服务规则型 memory

对用户偏好和固定事实，推荐继续以 `memory_item` 为长期主模型，但在使用语义上把它视为**结构化长期记忆**，而不是普通检索文本。

推荐做法：

- `preference`：承载用户长期偏好
  - 例如语言偏好、回答风格、排查顺序偏好
- `knowledge`：承载用户提供并希望后续沿用的稳定事实
  - 例如项目约束、部署环境、系统边界
- `feedback`：承载对系统行为的稳定纠偏
  - 例如“不要先讲理论，先定位代码”

在 prompt 渲染时，推荐按类型整理后注入独立 `MemoryContext`，例如：

- `User Preferences`
- `Project Facts`
- `Behavior Rules`

核心原则：

- 这类 memory 的首要职责是“迎合用户偏好、补充稳定前提”
- 它们更像先验约束，而不是和普通知识库 chunk 一起竞争排序的候选证据
- 因此，在当前阶段应优先走**结构化 prompt 注入**

#### 2. 会话内长文本召回：优先服务证据型 memory

对用户提供的超长消息文本，推荐继续以 `SessionChunk + SessionRecall` 为主路径。

核心原则：

- 长日志 / 长代码 / 长配置 / 长文档节选的原文 chunk 默认属于**会话检索层**
- 这类内容的目标是“后续追问时补回关键细节”
- 不默认升级为长期记忆
- 不默认和规则型 memory 混入同一个 recall 池

因此，推荐保持：

- 写时摘要，避免污染主上下文
- 原文 chunk 落入 `SessionChunk`
- 后续轮次按 query 召回关键 excerpt
- 通过独立 `SessionContext` 注入 prompt

### 边界结论

这两类 memory 需要明确分治：

1. **规则型 memory**
   - 优先走 `MemoryContext`
   - 重点是稳定影响回答方式与前提
   - 不要求先进入统一检索主通道

2. **证据型 memory**
   - 优先走 `SessionRecall`
   - 重点是会话内补原文细节
   - 不默认沉淀为长期记忆

如果把两者混在同一个 memory 池里处理，会同时损害：

- prompt 稳定性
- 检索可解释性
- 生命周期治理

### 对 Phase 3 的修正理解

后续 `Phase 3: 检索投影` 不应理解为“把所有 memory 都统一进 retrieve”。

更准确的目标应是：

- **允许部分长期事实型 memory 更自然地参与检索**
- 而不是把“偏好型 memory”和“会话内长文本 chunk”全部塞进同一检索总线

推荐顺序：

1. `preference / feedback`
   - 继续优先走 `MemoryContext`
   - 作为规则型上下文稳定前置

2. `knowledge` 型长期 memory
   - 后续可评估是否作为独立 memory channel 参与统一检索融合
   - 这类内容更接近“稳定事实证据”

3. `SessionRecall`
   - 继续保持独立
   - 不与长期记忆检索混为一层

### 当前阶段的推荐落地

结合当前实现状态，推荐的主路线是：

```text
结构化长期记忆
  -> memory_item
  -> MemoryContext
  -> 稳定影响偏好 / 固定事实 / 行为约束

会话内长文本召回
  -> SessionChunk
  -> SessionRecall
  -> 补回当前 conversation 内的关键原文细节
```

这意味着当前阶段最重要的不是“统一所有 memory 形态”，而是：

- 让规则型 memory 更结构化、更稳定
- 让证据型 memory 在单次会话内召回更准确
- 在此基础上，再谨慎推进长期事实型 memory 的检索投影

## 16. 实施进度追踪

更新时间：2026-05-20

### Phase 1: 长消息处理 — ✅ 已完成

- `domain.ConversationMessage` 新增 `RawContent / ContentSummary / IsSummarized`
- DB migration `20260519120000_add_message_summary_fields.sql`
- `LongMessageContentProcessor`：三档阈值策略（直接透传 / 中等摘要 / 分层 chunk 摘要）
  - 规则优先（`RoughTokenEstimator` + `detectLongMessageKind`），LLM 补充
  - `splitTextByTokenBudget` 支持 overlap，保证跨 chunk 语义连续性
- `ConversationMessageService.AddMessage` 重构为 processor 模式
  - `ConversationMessageContentProcessor` 接口（策略注入点）
  - `ProcessedConversationMessageContent` 返回结构
- 配置：`RagLongMessageConfig`（8 项，均有默认值）
- 前端 API 透出：`messageVO` 新增 `rawContent / contentSummary / isSummarized`
- 测试：6 个增量测试 PASS（小消息 / 中等 / 超大 / LLM / 回退 / overlap）
- LoadHistory 无需改动（`content` 字段语义不变，对下游透明）

### Phase 1.5: 会话检索层 — ✅ 已完成（V1）

**已完成（存储侧）**：
- `domain.SessionChunk` + `domain.SessionChunkEmbedding`
- DB migration `20260519153000_create_session_chunk_tables.sql`
  - `t_session_chunk`（唯一约束 message_id + chunk_index）
  - `t_session_chunk_embedding`（pgvector）
- `port.SessionChunkRepository` + `port.SessionChunkEmbeddingRepository`
- `ConversationMessageChunkSink`：事务中同时写入 chunk + embedding
- 接入 `AddMessage` 写入链路 + `bootstrap/runtime.go` 依赖注入
- 修正了实现与设计的偏差：`3000~12000 token` 的中等长消息现在也会生成 `SessionChunks`

**已完成（召回侧）**：
- `SessionChunkRepository` 新增：
  - `ExistsRecallable(...)`
  - `SearchRecallableByVector(...)`
- 新增 `SessionRecallService`
  - 当前作用域固定为 `conversation_id`
  - 仅召回 `user` 且 `is_summarized=true` 的历史长消息
  - 排除当前轮刚写入的 `message_id`
  - 先 `ExistsRecallable(...)` 再做 query embedding，保持默认轻量尝试
- `RagChatService.prepareChat(...)` 已接入独立 `session_recall` stage
  - 顺序为 `rewrite -> session_recall -> retrieve`
  - fail-open：召回失败不阻断 chat 主链路
- `ragprompt.Context` 新增 `SessionContext`
  - prompt 中单独注入 `## 会话上下文片段`
  - 不伪装成 history，也不并入 `KnowledgeContext`
- excerpt 选择已落地：
  - chunk 足够小时直接使用完整原文
  - chunk 过大时按 token budget 二次切窗
  - 使用轻量 lexical overlap 选择最佳窗口
  - 最终输出固定为“摘要 + 原文 excerpt”
- `session_recall` trace 已具备独立可观测性
  - `used / candidateCount / excerptCount / topScore / excludedMessageId`
  - `selectedHits / skippedPerMessageLimit / truncatedBy`

**验证与护栏**：
- `LongMessageContentProcessor` 已补中等长消息产出 `SessionChunks` 的测试
- `SessionRecallService` 已补：
  - 无 recallable chunk 时跳过 embedding
  - per-message 限流
  - excerpt 选窗
  - summary + excerpt 输出
- `RagChatService` 已补近似端到端样例：
  - 长日志追问
  - 长配置追问
- 当前增量验证通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/framework/config -count=1
```

### Phase 2: 显式长期记忆 — ✅ 基础闭环已完成（2026-05-20）

**已完成（主模型与持久化）**：
- 新增 `domain.MemoryItem`
- DB migration `20260520120000_create_memory_item_table.sql`
- 新增 `MemoryItemModel`
- 新增 `port.MemoryItemRepository`
- 新增 Postgres `MemoryItemRepository`

**已完成（服务侧）**：
- 新增 `MemoryService`
  - `SaveExplicitMemory(...)`
  - `ListMemories(...)`
  - `ExpireMemory(...)`
  - `RecallMemories(...)`
- 当前显式保存语义：
  - 默认 `scope_type=global`
  - 默认 `memory_type=knowledge`
  - 自动生成 summary
  - 写入即 `status=active`

**已完成（chat 接入）**：
- `RagChatService` 新增独立 `long_term_memory` stage
- `prepareChat(...)` 已接入长期记忆 recall
- `ragprompt.Context` 新增 `MemoryContext`
  - 以独立 prompt section 注入
  - 不混入 history
  - 不并入 `KnowledgeContext`

**已完成（接口侧）**：
- `POST /rag/v3/remember`
- `POST /rag/v3/memories`
- `GET /rag/v3/memories`
- `POST /rag/v3/memories/:memoryId/expire`

**当前边界**：
- 这仍然是 Phase 2，而不是 Phase 3
- recall 目前仍是轻量实现：
  - 先按 `kb` / `global` 作用域取数
  - 再做轻量 lexical match 与优先级排序
- 尚未做：
  - 向量化长期记忆投影
  - memory 与 retrieve 的统一语义召回
  - 异步自动识别

**当前结论**：
- “显式保存 -> 后续对话可用”这条最小长期记忆链路已经跑通
- 下一阶段重点应转向 `Phase 3` 检索投影
### Phase 3: 检索投影 — ❌ 未开始

### Phase 4: 策略扩展 — ❌ 未开始

### Phase 5: 异步自动识别 — ❌ 未开始
