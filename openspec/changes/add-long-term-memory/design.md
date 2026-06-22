# add-long-term-memory Design

## Overview

本设计对应 `add-long-term-memory` proposal 的 `Phase 1` 落地方案。

本期只实现：

- `global` scope
- `memory_type = preference`
- 回答完成后异步抽取偏好候选
- 候选默认进入 `pending`
- 用户确认后转 `active`
- `active` 偏好参与后续 chat / agent 召回

本设计同时要求对未来 `knowledge` 扩展保持兼容，但不在本期实现 `knowledge` 自动写入闭环。

## Goals

本设计目标：

1. 为用户长期偏好建立端到端闭环。
2. 不阻塞当前 `rag_chat` 主回答链路。
3. 复用现有 `memory_item + governance + recall` 能力。
4. 为未来 `knowledge` memory 扩展保留统一的候选、状态和治理语义。

## Non-Goals

本设计本期不包括：

- `knowledge` memory 自动写入
- `kb` scope 长期记忆实现
- 原始消息长期档案 recall
- 用户画像推断
- 高自治 agent 自主记忆写入
- 全功能记忆管理中心

## Existing System Context

当前代码库已经具备以下可复用能力：

- 会话与消息结构：
  - `conversation`
  - `conversation_message`
  - `conversation_summary`
- 短期记忆能力：
  - history summary
  - session recall
- 长期记忆能力：
  - `memory_item`
  - canonical key 治理
  - lifecycle
  - recall service
- Chat 召回接入：
  - `rag_chat` 中的 `long_term_memory` stage
- Agent 召回接入：
  - `memory_recall` capability

本期设计应尽量建立在这些现有边界之上，而不是另起新模型或新 recall 主链路。

## Design Principles

### 1. Preference-first

本期只把“用户长期偏好”作为主实现对象。

### 2. Knowledge-ready

未来 `knowledge` 扩展应能复用本期建立的候选、状态、确认、治理骨架。

### 3. Fail-open

偏好抽取或保存失败不能影响主回答链路成功。

### 4. Explicit-over-historical

当前轮用户显式输入始终高于任何历史长期记忆。

### 5. Structured-not-archival

长期记忆是结构化高价值信息，不是原始对话归档。

## Phase Model

### Phase 1: Implemented

本期实现：

- 偏好候选抽取
- `pending` 保存
- 用户确认 / 拒绝
- `active` 偏好召回
- 基本治理与审计

### Phase 2: Reserved

未来保留：

- `knowledge` memory candidate generation
- `knowledge` confirmation policy
- `kb` scope knowledge memory
- 更复杂的事实冲突治理
- 偏好与事实联合 recall 策略

## Product Semantics

### Preference Memory

`preference` 表示用户希望系统在后续对话中持续遵守的行为约束或回答偏好。

典型例子：

- 回答语言偏好
- 遇到问题时希望先做的步骤
- 希望避免的回答方式或行为

### Reserved Knowledge Memory

未来 `knowledge` 表示跨会话仍然成立、且值得结构化沉淀的长期事实。

典型例子：

- 稳定的项目约束
- 持续成立的集成事实
- 明确受范围约束的知识库事实

本期只定义这类语义，不实现其自动写入流程。

## Unified Memory Candidate Model

虽然本期只实现 `preference`，候选模型应统一支持未来扩展。

建议候选语义至少包含：

- `scope_type`
- `scope_id`
- `memory_type`
- `category`
- `canonical_key`
- `summary`
- `content`
- `source_message_id`
- `extraction_method`
- `confidence`
- `status`

本期约束：

- `scope_type = global`
- `memory_type = preference`
- `status = pending` on auto-generated candidates

Phase 1 实现上不需要单独存储 `requires_confirmation` 字段。

原因是：

- 自动生成候选默认以 `pending` 入库
- `pending` 本身已经表达“需要用户确认后才能生效”的语义
- 单独再存 `requires_confirmation` 会造成状态与字段双重表达，增加实现和治理复杂度

未来 `knowledge` 可复用同一候选语义，但会绑定不同的写入判定规则。

## Canonical Key Strategy

### Implemented in Phase 1

本期只启用以下 preference allowlist：

- `response.language`
- `workflow.troubleshooting.first_step`
- `behavior.avoid`

其中：

- `workflow.troubleshooting.first_step` 用于表达“在排障或问题定位场景下，用户希望系统优先采取的第一步”
- 它不用于表示任意工作流、任意任务或任意项目流程的通用首步骤
- 它的 `content` 必须是明确动作，例如“先看错误日志”“先给最小复现”“先检查配置差异”
- 它不接受“先分析一下”“先看看情况”“先按最佳实践处理”这类泛化、不可执行或边界不清的表达

这样可以避免 `workflow.first_step` 语义过宽、后续难以治理的问题。

### Reserved for Future Knowledge

未来可扩展 knowledge allowlist，例如：

- 项目约束类 key
- 集成类 key
- 稳定事实类 key

但 future keys 不应在本期设计里要求实现落地。

## Extraction Model

### Extraction Timing

偏好候选在回答完成后异步生成。

原因：

- 不阻塞主回答链路
- 抽取失败可 fail-open
- 更适合灰度发布与回滚

### Three-Stage Extraction Pipeline

本期采用“三段式抽取管道”：

1. 规则前置过滤
2. LLM 抽取结构化候选
3. 规则后验过滤

系统不直接把原始用户表达写入长期记忆，而是先筛掉明显不值得长期保留的输入，再让 LLM 做结构化抽取，最后再用确定性规则做一次收口。

### Extraction Constants

本期将抽取阶段涉及的关键阈值集中定义为配置项或常量，避免散落在不同调用点：

- `preference_candidate_min_confidence = 0.8`

Phase 1 默认使用 `0.8` 作为 `confidence` 阈值。低于该阈值的候选必须在后验过滤阶段被拒绝，不能进入持久化。

### Stage 1: Rule Pre-Filter

前置过滤不是“看到一次性输入就一律跳过”，而是先判断当前输入是否存在明确的长期偏好触发信号。

只有在**没有长期偏好触发信号**时，系统才允许将一次性输入直接判定为“无需进入长期记忆抽取流程”。

长期偏好触发信号的例子包括：

- “以后默认……”
- “之后都请……”
- “我更喜欢你……”
- “请一直用……”
- “遇到问题先……”
- “不要……”

如果命中了这类触发信号，即使输入同时看起来像算法题、报错、翻译或其他一次性场景，也不应被前置过滤直接跳过，而应继续进入 LLM 结构化抽取，由后续阶段决定是否形成候选。

在没有长期偏好触发信号时，前置过滤默认应排除以下情况：

- 纯算法题代码
- 一次性报错
- 临时命令
- 寒暄
- 翻译
- 计算
- 短上下文追问

这些内容通常不具有稳定的跨会话复用价值，不应进入长期偏好候选流程。

### Stage 2: LLM Structured Extraction

通过前置过滤后，系统使用 LLM 将用户表达抽取为结构化候选，而不是自由文本记忆。

LLM 输出至少应包含：

- `memory_type`
- `canonical_key`
- `summary`
- `content`
- `confidence`

本期 LLM 只允许抽取 `preference` 类型候选，不允许开放式生成任意 memory type。

LLM 的职责是：

- 判断用户表达是否具有长期偏好语义
- 将自然语言归一化为受控的 canonical key
- 生成简洁、可确认、可治理的候选内容

对于“遇到问题时希望先做什么”的偏好，LLM 只能规范化到 `workflow.troubleshooting.first_step`，不能回写为更宽泛的 workflow key。

LLM 不是最终裁决者。其输出必须进入规则后验过滤，不能直接入库。

### Stage 3: Rule Post-Filter

LLM 输出的结构化候选必须再次经过确定性规则校验，只有通过后验过滤的候选才允许保存为 `pending`。

后验过滤至少包括：

- `content` 不能太长
- 不能包含明显敏感信息
- `confidence < preference_candidate_min_confidence` 时不存
- `memory_type` 必须在白名单内
- `canonical_key` 必须规范化
- 包含临时词时默认不存

针对特定 key，本期还要求：

- `workflow.troubleshooting.first_step` 的 `content` 必须能落成明确动作
- `workflow.troubleshooting.first_step` 的 `content` 如果是泛化表达、抽象建议或不可执行描述，则拒绝写入

本期默认识别以下临时词并拒绝写入：

- 今天
- 刚刚
- 现在
- 这次
- 明天

这些词通常意味着该信息依赖当前时态或局部上下文，不适合作为长期记忆保留。

## Write Flow

### Phase 1 Implemented Flow

1. 用户发起对话
2. 系统完成正常回答
3. 回答完成后异步分析本轮用户消息
4. 若通过前置过滤，则进入 LLM 结构化抽取
5. 若通过后验过滤，则生成结构化候选
6. 候选保存为 `pending`
7. 用户确认后转 `active`
8. 用户拒绝后转 `rejected`

### Why Pending by Default

自动候选即使来自显式表达，也可能存在：

- key 映射错误
- summary 不准确
- 长期性判断偏宽

因此本期默认不允许自动候选直接进入 `active`。

### Reserved Future Knowledge Flow

未来 `knowledge` 扩展仍应复用“候选 -> 审核/确认 -> active”的基本骨架，但允许采用不同的确认策略。

## Save Preconditions

候选长期偏好必须同时满足：

- 通过规则前置过滤
- 通过 LLM 结构化抽取
- 通过规则后验过滤
- 内容对未来会话存在复用价值
- 信息语义足够稳定
- 来自用户显式表达，而不是 assistant 推断
- `memory_type` 在允许范围内
- `canonical_key` 可规范化到白名单
- 适合作为长期约束，而不是当前轮临时要求

## Confirmation Model

### Phase 1 Behavior

本期确认模型必须支持：

- 查看候选内容
- 确认保存
- 拒绝保存

确认后的结果：

- `pending -> active`

拒绝后的结果：

- `pending -> rejected`

### Contract Requirement

即使本期没有完整记忆中心，也必须存在明确的后端确认/拒绝语义，不能只把 `pending` 当作内部状态而没有完成闭环。

### Phase 1 Transport

Phase 1 的确认/拒绝 transport 明确限定为后端 API，不交付完整 UI。

本期至少需要有后端合同支持以下操作：

- 列出当前用户的待确认 preference candidates
- 确认指定候选
- 拒绝指定候选

这意味着：

- 前端可以在未来接入这些 API
- 本期不要求交付完整审核页面、记忆中心或富交互卡片
- transport 语义必须稳定，但 UI 形态不属于本次实现范围

### Future Compatibility

未来 `knowledge` 可以复用同样的确认骨架，但可以根据风险等级引入更严格规则。

## Recall Model

### Active Only

只有 `active` memory 参与长期召回。

### Preference Recall Role

本期 preference recall 主要承担：

- 语言约束补充
- 回答流程偏好补充
- 行为避免项补充

### Priority Rules

召回时遵循：

1. 当前轮用户显式输入优先
2. `active` 优先于其他状态
3. 长期偏好只作补充，不替代当前轮意图
4. 注入受 prompt budget 限制

### Chat and Agent Reuse

本期继续复用：

- `rag_chat` long-term memory stage
- `agent` memory recall capability

不新增独立 recall 主链路。

### Reserved Future Knowledge Recall

未来 `knowledge` recall 作为事实补充，而非行为约束。设计上应保留这两个角色的区别。

## Conflict and Governance

### Phase 1 Preference Rules

- 当前轮显式新偏好优先于旧偏好
- 单值偏好可 supersede 旧值
- 多值偏好允许并存
- 未确认项不能按 active 语义生效
- 无法稳定归类的候选不进入 `active`
- `workflow.troubleshooting.first_step` 只有在 `content` 为明确动作时才允许确认
- `behavior.avoid` 在 `active` 状态下默认最多保留 `10` 条
- 当 `behavior.avoid` 已达到上限时，确认新项必须返回 `quota exceeded` 或等价拒绝结果
- 超过上限时不自动删除、不自动过期、不自动替换旧项，必须由用户显式治理

### Lifecycle

本期复用现有生命周期：

- `pending`
- `active`
- `rejected`
- `expired`
- `superseded`

### Reserved Future Knowledge Governance

未来 `knowledge` 可能需要：

- 更严格的 scope 判定
- 更强的冲突治理
- 更明确的事实失效语义

这些规则在本期只预留语义，不要求实现。

## Failure Handling

本期必须满足：

- 候选抽取失败不影响主回答
- 候选保存失败不影响主回答
- 候选确认失败不影响后续普通聊天
- recall 失败按 fail-open 处理
- 所有失败应保留足够日志与观测信息

## Observability

Phase 1 只允许复用现有日志和 metrics 入口，不新增观测基础设施。

这意味着：

- 可以在现有应用日志中补充 memory candidate 相关字段
- 可以复用现有 metrics 注册与上报入口增加必要计数
- 如果当前聊天链路已存在 trace / meta 挂载点，可在原有链路上追加字段
- 不引入新的观测子系统、collector、dashboard、存储或独立事件管道

建议为以下事件保留最小观测能力：

- preference candidate generated
- preference candidate persisted as pending
- preference candidate confirmed
- preference candidate rejected
- preference recalled
- preference recall skipped due to current-turn override
- preference write / recall failure
- pre-filter skipped
- post-filter rejected

需要支持的形式仅限：

- existing logs
- existing metrics
- audit fields attached to existing records

## Security and Privacy Boundaries

本期长期记忆必须遵循：

- 只保留少量高价值长期偏好
- 不把原始会话当作长期记忆直接复制
- 不把 assistant 猜测的人格画像沉淀为长期事实
- 保持候选与来源消息可追溯
- 支持用户拒绝、纠正和过期

## Testing Strategy

本期测试至少覆盖：

- 高确定性偏好候选抽取
- 非偏好表达不应生成候选
- 前置过滤命中
- 后验过滤拒绝
- 候选默认进入 `pending`
- `pending -> active`
- `pending -> rejected`
- 单值偏好 supersede
- 多值偏好并存
- `active` 偏好参与召回
- 当前轮显式输入覆盖历史偏好
- 抽取失败 / 保存失败 fail-open

未来 `knowledge` 相关测试不在本期实现范围，但应在测试规划中预留分类位置。

## Future Upgrade Path

从本期升级到 `preference + knowledge` 时，应遵循：

1. 不替换 `memory_item` 主实体。
2. 不推翻 `pending / active / rejected / expired / superseded` 状态模型。
3. 尽量复用已有 candidate / confirmation / recall 骨架。
4. 在 recall 角色上明确区分：
   - `preference` = behavior constraint
   - `knowledge` = factual supplement
5. 如 future 引入 `kb` scope，应在不破坏 `global preference` 语义的前提下扩展。

## Open Questions

- 本期最小确认后端 API 的资源路径和分页约定是否需要进一步统一
- `workflow.troubleshooting.first_step` 是否需要进一步收敛为更受限的枚举值
- future `knowledge` 是否必须经过用户确认，还是可引入更细的来源分级策略
