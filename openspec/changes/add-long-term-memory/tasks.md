# add-long-term-memory Tasks

## Phase 1 Scope

本期只实现：

- `global` scope
- `memory_type = preference`
- 回答完成后的异步候选抽取
- `pending -> active / rejected`
- `active` 偏好召回

本期不实现：

- `knowledge` 自动写入
- `kb` scope 长期记忆
- 完整记忆中心

## Contract Stage

### [x] Task C1: Freeze Phase-1 Candidate and Key Contract

- 明确本期只支持 `global + preference`
- 明确 canonical key allowlist：
  - `response.language`
  - `workflow.troubleshooting.first_step`
  - `behavior.avoid`
- 定义统一候选字段：
  - `scope_type`
  - `memory_type`
  - `canonical_key`
  - `summary`
  - `content`
  - `source_message_id`
  - `extraction_method`
  - `confidence`
  - `status`
- 明确 Phase 1 不单独持久化 `requires_confirmation`，由 `pending` 状态承载确认语义
- future `knowledge` 只保留合同兼容性，不进入本期实现范围

独立测试：

- 合同测试能验证候选字段完整性
- 合同测试能验证 `requires_confirmation` 不是 Phase 1 必需持久化字段
- 非 allowlist key 会被拒绝
- `workflow.first_step` 不再被接受，统一使用 `workflow.troubleshooting.first_step`

### [x] Task C2: Define Phase-1 Confirmation API Contract

- 明确 Phase 1 只交付后端 API transport
- 定义最小后端能力：
  - 列出待确认 candidates
  - 确认 candidate
  - 拒绝 candidate
- 不引入完整 UI 或记忆中心

独立测试：

- API contract tests 能覆盖 list / confirm / reject 的输入输出
- 缺失用户身份、候选不存在、状态非法等请求能得到稳定错误

## Extraction Stage

### [x] Task E1: Implement Preference Trigger Detection and Gated Pre-Filter

- 先识别是否存在长期偏好触发信号
- 只有在没有长期偏好触发信号时，才允许一次性输入被前置过滤直接跳过
- 一次性输入类别包括：
  - 纯算法题代码
  - 一次性报错
  - 临时命令
  - 寒暄
  - 翻译
  - 计算
  - 短上下文追问

独立测试：

- 没有偏好触发信号的算法题、报错、翻译等输入会被跳过
- 带有偏好触发信号的同类输入不会被前置过滤直接跳过

### [x] Task E2: Implement LLM Structured Preference Extraction

- 对通过前置过滤的输入执行 LLM 结构化抽取
- 只允许输出 `preference` 类型候选
- “遇到问题先……”类表达统一规范化到 `workflow.troubleshooting.first_step`
- LLM 输出至少包含：
  - `memory_type`
  - `canonical_key`
  - `summary`
  - `content`
  - `confidence`

独立测试：

- 有效偏好表达会产出结构化候选
- 宽泛 workflow key 不会出现在输出中
- 非 `preference` 类型输出会被判定为非法结构

### [x] Task E3: Implement Rule Post-Filter and Normalization

- 对 LLM 输出执行后验过滤与规范化
- 后验规则至少包括：
  - `content` 长度限制
  - 明显敏感信息拦截
  - `confidence` 阈值，默认 `0.8`
  - `memory_type` 白名单校验
  - `canonical_key` 规范化校验
  - 临时词拦截
  - `workflow.troubleshooting.first_step` 的 `content` 必须落成明确动作，不接受泛化表达
- `confidence` 阈值集中定义为配置项或常量，而不是散落在调用逻辑中
- 默认拦存以下临时词相关候选：
  - 今天
  - 刚刚
  - 现在
  - 这次
  - 明天

独立测试：

- 超长值、敏感值、低置信度值会被拒绝
- 包含临时词的候选不会进入持久化阶段
- 低于 `0.8` 的候选会被稳定拒绝
- 合法候选会被规范化为 allowlist key
- `workflow.troubleshooting.first_step` 的泛化表达不会被当作合格候选

## Lifecycle Stage

### [x] Task L1: Persist Pending Preference Candidates

- 将通过三段式抽取管道的 preference candidate 保存为 `pending`
- 复用现有 `memory_item` 主模型
- 正确写入：
  - `source_message_id`
  - `extraction_method`
  - `created_by / updated_by`
- 异步保存失败不得影响主回答链路

独立测试：

- 合法候选会以 `pending` 状态保存
- 保存失败时主回答链路仍成功返回
- 审计字段会随候选一起落库

### [x] Task L2: Implement Confirm / Reject State Transitions

- 支持：
  - `pending -> active`
  - `pending -> rejected`
- 单值 preference 确认时支持 supersede 旧值
- 多值 preference 确认时支持并存
- `behavior.avoid` 在 `active` 状态下默认最多 `10` 条
- 当 `behavior.avoid` 已达到上限时，确认新项返回 `quota exceeded` 或等价错误
- 超限时不自动删除旧项
- 非 `pending` 候选不得重复确认或重复拒绝

独立测试：

- `pending` 候选可被成功确认或拒绝
- 重复确认、重复拒绝或非法状态转换会被拦截
- `behavior.avoid` 超过 `10` 条时新确认会被拒绝，且旧项保持不变
- 单值 / 多值 preference 的治理语义符合预期

## Integration Stage

### [x] Task I1: Reuse Active Preference Recall in Chat and Agent

- 继续复用现有长期记忆召回主路径
- 只允许 `active preference` 进入召回
- 确保 chat 侧可消费 active 偏好
- 确保 agent 侧可消费 active 偏好
- 确保当前轮显式输入优先于历史偏好

独立测试：

- `active` 偏好会在 chat 和 agent 中生效
- `pending` 和 `rejected` 候选不会进入召回
- 当前轮新指令可以覆盖历史偏好

### [x] Task I2: Reuse Existing Logs and Metrics Hooks

- 仅复用现有日志和 metrics 入口补充最小必要字段
- 不新增观测基础设施、collector、dashboard、存储或独立事件管道
- 记录最小必要事件：
  - pre-filter skipped
  - extraction attempted
  - post-filter rejected
  - pending persisted
  - candidate confirmed
  - candidate rejected
  - preference recalled
  - recall overridden by current-turn input

独立测试：

- 关键路径事件会出现在现有日志或现有 metrics 输出中
- 不需要引入新的观测组件即可完成验证

## Verification Stage

### [x] Task V1: Add Focused Unit and Contract Tests

- 覆盖合同校验
- 覆盖触发信号与前置过滤
- 覆盖 LLM 结构化抽取结果校验
- 覆盖后验过滤与规范化
- 覆盖状态流转规则

独立测试：

- 相关测试可在不依赖端到端链路的情况下单独运行并通过

### [x] Task V2: Add End-to-End Lifecycle Validation

- 验证完整闭环：
  - 用户显式表达偏好
  - 候选生成
  - 候选保存为 `pending`
  - 用户确认
  - 后续对话召回生效
- 验证拒绝后不生效
- 验证一次性输入在无触发信号时不会进入长期记忆
- 验证带触发信号的一次性输入仍可进入抽取管道

独立测试：

- 端到端集成测试可单独运行并验证完整生命周期
