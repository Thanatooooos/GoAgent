# add-long-term-memory Proposal

## Summary

本变更旨在将当前项目中已经存在的长期记忆底座产品化，形成一个“保守、结构化、可治理”的长期记忆闭环。

本次变更采用 `preference-first, knowledge-ready` 的阶段策略：

- `Phase 1` 聚焦用户长期偏好（`preference`）的半自动写入闭环
- 当前只实现偏好候选生成、`pending -> active` 确认流程、稳定召回与治理能力
- 同时在产品语义和设计边界上，为未来扩展到结构化长期事实（`knowledge`）预留清晰空间
- 本次不引入新的长期记忆主模型，继续复用现有 `memory_item + governance + recall` 能力

本 proposal 的目标不是“记住一切”，而是让系统以结构化方式保留少量跨会话仍然有价值、可解释、可纠正的长期信息。

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
- 可以承载多类长期信息，但当前没有明确的 `Phase 1 / Phase 2` 分层

这导致跨会话连续性仍然依赖：

- 会话历史
- 用户重复输入
- 人工 remember 接口

本 proposal 的目标是把长期记忆从“底层能力存在”推进到“上层体验可用”，同时避免在第一阶段把范围扩展得过大。

## Goals

本变更的目标是：

1. 让系统能够从对话中识别少量高价值、可治理的用户长期偏好，并以结构化方式进入长期记忆候选流程。
2. 让用户偏好在后续 chat 和 agent 执行中被稳定召回，并作为约束补充使用。
3. 确保偏好型长期记忆具备可解释、可列出、可确认、可拒绝、可过期、可纠正的治理能力。
4. 在不影响主回答链路稳定性的前提下完成第一版落地。
5. 为未来升级到 `preference + knowledge` 的长期记忆模型保留明确的产品语义和演进边界。

## Non-Goals

本变更第一阶段不包括：

- 不做“记住所有对话内容”的全量长期归档
- 不做跨会话原始消息档案 recall
- 不做复杂的用户画像或隐式人格推断
- 不做高风险、完全自治的 agent 自主记忆写入
- 不做新的长期记忆主存储模型
- 不在本期实现 `knowledge` 的自动写入闭环
- 不在本期实现 `knowledge` 的确认、审核、冲突处理产品流程
- 不让长期记忆覆盖用户当前轮的显式新指令

## Phase Boundaries

### Phase 1: Implemented in This Change

本期只实现：

- `global` scope 下的长期偏好记忆
- `memory_type = preference`
- 半自动候选生成
- 候选以 `pending` 状态进入治理流程
- 用户确认后转为 `active`
- `active` 偏好参与后续 chat / agent 召回

### Phase 2: Reserved for Future Expansion

未来可以扩展但本期不实现：

- `memory_type = knowledge`
- 更宽的 scope 策略，例如受控引入 `kb`
- 结构化长期事实的候选生成与确认流程
- 偏好与事实并存时更复杂的优先级与冲突治理
- 更完整的审核 UI 与记忆管理体验

本期 design 必须对上述 future phase 保持兼容，但 tasks 不要求落地这些能力。

## Proposed Changes

### 1. Product Positioning

长期记忆第一阶段被定义为：

“系统以结构化方式保留的、跨会话仍有价值的用户偏好约束”

它不是：

- 原始会话日志
- 对话全文档案
- 任意 assistant 猜测出的用户画像
- 未经确认的长期项目事实库

未来 `knowledge` 扩展的产品定位将是：

“系统以结构化方式保留的、跨会话仍然成立、并且经过受控治理的长期事实补充”

但该语义仅在本期 design 中预留，不在本期 proposal 范围内要求完整实现。

### 2. Memory Entity Strategy

第一阶段继续使用现有 `memory_item` 作为唯一长期记忆实体，不新增第二套长期记忆模型。

现有长期记忆模型继续承担以下职责：

- 结构化存储
- 范围隔离
- 类型区分
- 生命周期管理
- 冲突检测
- recall 排序与注入

本期实现虽然只启用 `preference`，但数据模型与候选语义应保持对未来 `knowledge` 扩展兼容。

### 3. MVP Memory Scope

第一阶段仅支持以下 scope：

- `global`

解释：

- `global` 用于用户级长期偏好
- 本期不引入 `kb` 偏好记忆
- 未来如果要支持 `knowledge`，可以再受控引入 `kb` 级长期事实

第一阶段不新增：

- `conversation`
- `workspace`
- `project`

### 4. MVP Memory Types

第一阶段只实现：

- `preference`

未来预留但本期不实现：

- `knowledge`

`feedback` 作为现有模型的一部分保留，但不作为本次自动写入 MVP 的目标。

### 5. Canonical Key Strategy

第一阶段采用 allowlist 策略，只允许少量高价值 canonical key 自动进入长期偏好候选流程。

优先支持：

- `response.language`
- `workflow.troubleshooting.first_step`
- `behavior.avoid`

设计原则：

- 优先选择语义稳定、生命周期较长的信息
- 优先选择冲突可判断的信息
- 优先选择后续对 chat / agent 实际有帮助的信息
- 优先选择用户可以明确确认或纠正的信息

未来 `knowledge` 扩展可以引入独立 allowlist，例如项目约束、集成事实、环境限制等，但不在本期实现。

## Write Path

### 1. Write Timing

第一阶段采用“回答完成后异步抽取”的写入时机。

原因：

- 不阻塞首包和主回答链路
- 抽取失败不影响聊天成功
- 更容易灰度和回滚
- 更适合当前 `rag_chat` 主链路结构

### 2. Write Sources

第一阶段仅从以下来源生成偏好型长期记忆候选：

- 用户显式表达的长期偏好
- 用户显式确认的工作方式偏好
- 用户显式表达的行为避免项

第一阶段不从以下来源直接写入：

- assistant 猜测
- 单轮未确认的模糊倾向
- 仅由模型推断得出的用户画像
- 未经确认的长期事实

### 3. Save Policy

第一阶段采用保守的候选保存策略：

- 所有自动生成的偏好候选默认写入 `pending`
- 只有经用户确认后才转为 `active`
- 被拒绝的候选进入 `rejected`
- 新确认的单值偏好可 supersede 旧值

本期不要求自动生成的偏好直接进入 `active`。

### 4. Save Preconditions

候选长期偏好必须同时满足：

- 内容对未来会话存在复用价值
- 信息语义足够稳定
- 信息来自用户显式表达，而不是 assistant 推断
- 能映射到受支持的 `canonical_key`
- 能映射到 `global + preference`
- 适合作为长期约束而不是当前轮临时要求

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
- agent 的 `memory_recall` capability

如果未来扩展到 `knowledge`，也应优先复用同一长期记忆召回骨架，而不是新建完全独立的 recall 系统。

### 2. Recall Role

长期偏好在第一阶段主要承担以下作用：

- 语言偏好补充
- 流程偏好补充
- 行为避免项补充

未来 `knowledge` 的召回角色将主要是事实补充，而不是行为约束。本期只在设计上保留这条角色区分。

### 3. Recall Priority Rules

第一阶段采用以下优先级原则：

1. 当前轮用户显式输入优先于历史长期记忆
2. `active` memory 才允许参与长期召回
3. 结构化长期偏好作为补充上下文，不替代当前轮用户意图
4. recall 注入必须受 prompt budget 限制

### 4. Prompt Separation

第一阶段保持上下文来源分层，不将不同来源混成单一 memory 概念。

系统在逻辑上继续区分：

- 会话历史 / summary
- session recall
- structured long-term memory
- retrieve knowledge context

这条边界必须保留，以便未来在不打乱结构的前提下引入 `knowledge` 型长期记忆。

## Governance and Conflict Rules

### 1. Conflict Principles

第一阶段的核心冲突规则：

1. 本轮用户显式输入优先于旧 memory
2. 新确认的单值偏好可以 supersede 旧值
3. 无法稳定归类的信息不进入 `active`
4. 未确认候选不应以 active 语义影响后续回答

### 2. Single vs Multi Cardinality

继续复用现有 canonical key 的 cardinality 语义：

- `single`
- `multi`

其中：

- `response.language`
- `workflow.troubleshooting.first_step`

适合单值语义。

- `behavior.avoid`

适合多值语义。

未来 `knowledge` 扩展也应继续复用该治理原则，而不是另起一套冲突模型。

### 3. Lifecycle

第一阶段继续使用现有 memory lifecycle：

- `pending`
- `active`
- `rejected`
- `expired`
- `superseded`

其中本期核心闭环是：

- `pending -> active`
- `pending -> rejected`
- `active -> superseded`
- `active -> expired`

### 4. Expiration and Maintenance

第一阶段要求：

- 支持显式过期
- 支持维护任务清理过期与 superseded 项
- 支持保守的 recall cache 失效策略

## UX and Control Surface

第一阶段需要提供或明确以下用户能力：

- 查看已保存长期偏好
- 区分结构化长期偏好与普通会话历史
- 确认系统建议记住的偏好候选
- 拒绝系统建议记住的偏好候选
- 过期某条已保存偏好

第一阶段不要求完整记忆中心，但必须存在最小可用的确认与拒绝语义。

第一阶段的确认/拒绝能力只要求后端 API 合同，不要求同时交付完整 UI。

未来如果扩展到 `knowledge`，应复用同一控制面模型，而不是重新定义完全不同的审核语义。

## Rollout Plan

第一阶段建议按以下顺序推进：

1. 明确允许写入的 `preference` canonical key
2. 定义长期偏好候选抽取规则
3. 在 chat 成功后增加异步候选生成与 `pending` 写入
4. 增加确认 / 拒绝流程
5. 复用现有 recall 路径完成端到端闭环
6. 复用现有日志和 metrics 补充必要审计字段
7. 补用户查看、过期和后续治理能力

## Acceptance Criteria

本变更在以下条件同时满足时才视为完成：

1. Phase 1 只会自动生成 `global + preference` 的长期记忆候选，不会自动生成 `knowledge` 候选。
2. 自动候选抽取采用“三段式管道”：规则前置过滤、LLM 结构化抽取、规则后验过滤。
3. 对于算法题、一次性报错、翻译、计算、临时命令、寒暄、短上下文追问等一次性输入，只有在未命中长期偏好触发信号时才允许被前置过滤直接跳过。
4. 所有自动生成的候选默认写入 `pending`，不会直接进入 `active`。
5. Phase 1 提供最小后端 API 合同以支持查看待确认候选、确认候选、拒绝候选；完整 UI 不属于本次验收范围。
6. `active` 偏好可被 chat 和 agent 复用召回，且当前轮用户显式输入优先于任何历史长期偏好。
7. “遇到问题时希望先做的步骤”类偏好统一收敛到 `workflow.troubleshooting.first_step`，不再使用语义过宽的 `workflow.first_step`。
8. `workflow.troubleshooting.first_step` 的候选内容必须是明确动作，不接受泛化、不可执行的表达。
9. `behavior.avoid` 在 `active` 状态下默认最多保留 `10` 条；达到上限时拒绝确认新项，不自动删除旧项。
10. 自动候选后验过滤的 `confidence` 默认阈值为 `0.8`，并以配置项或常量集中定义。
11. 观测能力只允许复用现有日志和 metrics 入口，不引入新的观测基础设施、存储或独立管道。
12. `knowledge` 的 future product semantics 和升级边界在 design 中被明确保留，但本次不实现 `knowledge` 自动写入闭环。

## Risks

本 proposal 的主要风险包括：

- LLM 抽取错误导致脏偏好候选生成
- 用户无法理解“系统为什么建议记住这条偏好”
- 旧偏好与当前轮新指令冲突
- recall 过量导致 prompt budget 被长期记忆挤占
- 为 future `knowledge` 预留空间时把本期 MVP 边界写得过宽

第一阶段的风险控制方式：

- allowlist
- `pending` 状态
- 显式确认
- source message 可追溯
- recall 字符预算限制
- list / expire / existing logs / existing metrics
- `Phase 1 / Phase 2` 边界显式写死

## Future Evolution

本 proposal 明确为未来 `preference + knowledge` 升级保留以下约束：

1. `memory_item` 继续作为结构化长期记忆的统一主实体。
2. `preference` 与 `knowledge` 可以共享候选、状态、治理和 recall 骨架，但必须保留清晰的产品语义区分。
3. `preference` 主要承担行为约束，`knowledge` 主要承担事实补充。
4. session recall 与 structured long-term memory 保持逻辑分层。
5. 当前轮用户显式输入始终高于任何历史长期记忆。
6. 未来新增 `knowledge` 时，应在现有候选与治理模型上扩展，而不是推翻本期 preference MVP 设计。

因此，第一阶段方案不是未来 `preference + knowledge` 的死胡同，而是其偏好层的直接前置能力，并为事实层扩展保留了清晰接口和产品边界。
