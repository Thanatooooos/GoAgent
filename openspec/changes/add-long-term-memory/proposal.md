# add-long-term-memory Proposal

## Summary

本变更旨在将当前项目中已经存在的长期记忆底座产品化，形成一个“保守、结构化、可治理”的长期记忆闭环。

第一阶段不引入新的记忆主模型，不尝试“记住一切”，而是在现有 `memory_item + governance + recall` 基础上，补齐自动抽取、受控写入、稳定召回、冲突治理和用户控制能力，用于沉淀以下高价值信息：

- 用户长期偏好
- 可复用的行为约束
- 项目固定事实
- KB 级上下文约束

## Why

当前系统已经具备以下能力：

- 会话级短期记忆：`conversation`、`conversation_message`、`conversation_summary`
- 会话内召回：`session_recall`
- 长期记忆存储与治理：`memory_item`、embedding、canonical key、冲突处理、过期维护
- 长期记忆召回：`rag_chat` 中的 `long_term_memory` stage、retrieve 的 `memory_fact` channel、agent 的 `memory_recall` capability

但当前长期记忆仍缺少端到端闭环：

- 可以召回，但缺少稳定的自动写入路径
- 可以手工写入，但缺少保守的系统化抽取策略
- 可以存储，但缺少清晰的写入边界和产品语义
- 可以治理，但缺少“哪些信息值得长期保留”的统一规则

这导致跨会话连续性仍然依赖：

- 会话历史
- 用户重复输入
- 人工 remember 接口

本 proposal 的目标是把长期记忆从“底层能力存在”推进到“上层体验可用”。

## Goals

本变更的目标是：

1. 让系统能够从对话中识别少量高价值、可治理的信息，并写入结构化长期记忆。
2. 让长期记忆在后续 chat 和 agent 执行中被稳定召回，并作为约束或事实补充使用。
3. 确保长期记忆具备可解释、可列出、可过期、可纠正的治理能力。
4. 在不影响主回答链路稳定性的前提下完成第一版落地。
5. 为未来升级到“双层混合记忆”保留明确演进边界。

## Non-Goals

本变更第一阶段不包括：

- 不做“记住所有对话内容”的全量长期归档
- 不做跨会话原始消息档案 recall
- 不做双层混合记忆
- 不做复杂的用户画像或隐式人格推断
- 不做高风险、完全自治的 agent 自主记忆写入
- 不做新的长期记忆主存储模型
- 不让长期记忆覆盖用户当前轮的显式新指令

## Proposed Changes

### 1. Product Positioning

长期记忆第一阶段被定义为：

“系统以结构化方式保留的、跨会话仍有价值的偏好、约束和事实”

它不是：

- 原始会话日志
- 对话全文档案
- 任意 assistant 猜测出的用户画像

### 2. Memory Entity Strategy

第一阶段继续使用现有 `memory_item` 作为唯一长期记忆实体，不新增第二套长期记忆模型。

现有长期记忆模型继续承担以下职责：

- 结构化存储
- 范围隔离
- 类型区分
- 生命周期管理
- 冲突检测
- recall 排序与注入

### 3. MVP Memory Scope

第一阶段仅支持以下 scope：

- `global`
- `kb`

解释：

- `global` 用于用户级长期偏好和全局项目事实
- `kb` 用于特定知识库下成立的约束和事实

第一阶段不新增：

- `conversation`
- `workspace`
- `project`

如果存在项目级信息，暂时映射到 `global` 或 `kb`，并通过 `category / canonical_key` 体现其业务语义。

### 4. MVP Memory Types

第一阶段聚焦两类 memory type：

- `preference`
- `knowledge`

`feedback` 作为现有模型的一部分保留，但不作为本次自动写入 MVP 的主目标。

### 5. Canonical Key Strategy

第一阶段采用 allowlist 策略，只允许少量高价值 canonical key 自动进入长期记忆。

优先支持：

- `response.language`
- `workflow.first_step`
- `behavior.avoid`
- `project.constraint.network`
- `project.messaging.main_bus`
- `project.fact.dependencies`
- `project.integrations`

设计原则：

- 优先选择语义稳定、生命周期较长的信息
- 优先选择冲突可判断的信息
- 优先选择后续对 chat / agent 实际有帮助的信息

第一阶段不支持开放式、无限扩展的自动 key 生成。

## Write Path

### 1. Write Timing

第一阶段采用“回答完成后异步抽取”的写入时机。

原因：

- 不阻塞首包和主回答链路
- 抽取失败不影响聊天成功
- 更容易灰度和回滚
- 更适合当前 `rag_chat` 主链路结构

### 2. Write Sources

第一阶段仅从以下来源生成长期记忆候选：

- 用户显式表达的长期偏好
- 用户显式确认的工作方式约束
- 用户显式确认的项目固定事实
- 高确定性的 KB / 项目上下文事实

第一阶段不从以下来源直接写入：

- assistant 猜测
- 单轮未确认的模糊倾向
- 仅由模型推断得出的用户画像

### 3. Two-Tier Save Policy

第一阶段采用保守的两级写入策略：

1. 规则可确定的内容
   直接写入 `active`

2. LLM 抽取但仍需谨慎的内容
   先写入 `pending`

该策略复用现有 memory status 语义，避免第一版自动写入过于激进。

### 4. Save Preconditions

候选长期记忆必须同时满足：

- 内容对未来会话存在复用价值
- 信息语义足够稳定
- 能映射到受支持的 scope
- 能映射到受支持的 `memory_type`
- 满足 canonical key allowlist 或受控自由事实策略

### 5. Source Linking

每条长期记忆应尽可能保留：

- `source_message_id`
- `extraction_method`
- `created_by / updated_by`

以支持后续：

- 审计
- 纠错
- 回溯
- 决策解释

## Recall Path

### 1. Recall Reuse

第一阶段不新增新的 recall 主路径，继续复用现有能力：

- `rag_chat` 的 `long_term_memory` stage
- retrieve 的 `memory_fact` channel
- agent 的 `memory_recall` capability

### 2. Recall Role

长期记忆在第一阶段主要承担两类作用：

1. 约束补充
   例如语言偏好、流程偏好、禁忌行为、网络约束

2. 事实补充
   例如项目总线、中间件依赖、已确认的系统集成事实

### 3. Recall Priority Rules

第一阶段采用以下优先级原则：

1. 当前轮用户显式输入优先于历史长期记忆
2. `kb` scope 优先于 `global` scope
3. 结构化长期记忆作为补充上下文，不替代当前轮用户意图
4. recall 注入必须受 prompt budget 限制

### 4. Prompt Separation

第一阶段保持上下文来源分层，不将不同来源混成单一 memory 概念。

系统在逻辑上继续区分：

- 会话历史 / summary
- session recall
- structured long-term memory
- retrieve knowledge context

这条边界必须保留，为未来升级到双层混合记忆提供演进空间。

## Governance and Conflict Rules

### 1. Conflict Principles

第一阶段的核心冲突规则：

1. 本轮用户显式输入优先于旧 memory
2. 新确认的单值 memory 可以 supersede 旧值
3. `kb` 范围内的事实不应无条件污染 `global`
4. 无法稳定归类的信息不进入 `active`

### 2. Single vs Multi Cardinality

继续复用现有 canonical key 的 cardinality 语义：

- `single`
- `multi`

其中：

- 单值 key 用于稳定偏好和全局约束
- 多值 key 用于可并存事实和行为规则

### 3. Lifecycle

第一阶段继续使用现有 memory lifecycle：

- `pending`
- `active`
- `rejected`
- `expired`
- `superseded`

### 4. Expiration and Maintenance

第一阶段要求：

- 支持显式过期
- 支持维护任务清理过期与 superseded 项
- 支持保守的 recall cache 失效策略

## UX and Control Surface

第一阶段需要提供或明确以下用户能力：

- 查看已保存长期记忆
- 过期某条记忆
- 区分结构化长期记忆与普通会话历史

第一阶段建议但不强制要求立即完成：

- 针对 `pending` 项的审核入口
- 用户确认“记住这条信息”的显式交互
- 用户纠正系统记忆内容的专用流程

如果第一版没有完整审核 UI，也应在 proposal 中保留该能力为后续补齐项。

## Rollout Plan

第一阶段建议按以下顺序推进：

1. 明确允许写入的 memory 类型、scope 和 canonical key
2. 定义长期记忆候选抽取规则
3. 在 chat 成功后增加异步候选生成与受控写入
4. 复用现有 recall 路径完成端到端闭环
5. 增加观测和审计字段
6. 补用户查看、过期和后续审核能力

## Risks

本 proposal 的主要风险包括：

- LLM 抽取错误导致脏记忆写入
- 过早收紧 canonical key 影响扩展性
- scope 判定错误造成记忆污染
- recall 过量导致 prompt budget 被长期记忆挤占
- 用户无法理解“系统为什么记住了这条内容”

第一阶段的风险控制方式：

- allowlist
- `pending` 状态
- 规则优先于开放抽取
- source message 可追溯
- recall 字符预算限制
- list / expire / trace 能力

## Future Evolution

本 proposal 明确为未来双层混合记忆保留以下演进约束：

1. `memory_item` 只承载结构化、可治理的长期事实，不承载原始会话档案。
2. session recall 与 structured long-term memory 保持逻辑分层。
3. 写入路径与召回路径保持解耦，未来可以新增“档案层 -> 结构化层提升”流程。
4. 结构化长期记忆应始终是“高价值、低噪音、可解释”的事实层。

因此，第一阶段方案不是未来双层混合记忆的死胡同，而是其结构化事实层的直接前置能力。
