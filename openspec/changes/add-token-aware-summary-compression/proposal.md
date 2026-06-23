# add-token-aware-summary-compression Proposal

## Summary

本变更将会话 summary 从“按对话轮数触发”调整为“按 token 预算触发”，并把压缩能力接入真实聊天消息写入链路。

核心策略是：

- 助手回复成功落库后异步判断是否需要压缩
- 只统计“最新有效 summary + 尚未被 summary 覆盖的消息尾部”
- 使用统一 token estimator
- 为 memory、session recall、retrieve、tool 等阶段设置独立 token 信封
- 上一轮结束时用预算信封决定是否压缩
- 当前轮各阶段完成后使用实际内容重新估算，并由最终 prompt budget 做兜底裁剪

本变更不会尝试预测下一轮具体会检索到什么、调用什么工具，而是通过明确的阶段预算上限保证 history 不会无边界占用 prompt 空间。

## Why

当前 summary 链路存在以下问题：

- 生产触发逻辑仍以 `summary-start-turns` 为主要门槛，短而密集的消息和超长消息无法得到一致处理。
- 工作区已有一版 token 触发代码，但统计的是数据库中的全部历史，而实际压缩的只是固定数量的最近消息，统计口径与压缩边界不一致。
- 已被 summary 覆盖的旧消息仍参与 token 统计，可能导致越过阈值后重复触发。
- summary 触发器和 chat prompt budget 使用不同 token estimator，估算结果不可直接比较。
- 正常聊天通过 `ConversationMessageService.AddMessage` 保存消息，而压缩调用挂在 `history.DefaultService.Append` 上；真实聊天链路没有调用该入口。
- retrieve 主要受 `topK` 控制，缺少最终 token 信封。
- tool context 当前主要使用字符长度保护，无法稳定反映中英文、代码和 JSON 的 token 成本。

因此，仅将“轮数阈值”替换成“token 阈值”不足以完成交付。需要同时统一计量口径、接通真实调用链路，并建立阶段预算。

## Goals

1. 让 summary 压缩由有效 prompt token 预算驱动，而不是由固定轮数驱动。
2. 统一 summary 与 chat prompt budgeting 使用的 token estimator。
3. 只对未被最新 summary 覆盖的新消息进行增量压缩。
4. 在助手回复成功落库后异步触发压缩，供下一轮请求使用。
5. 为 retrieve 和 tool context 建立明确的 token 上限。
6. 在当前轮 prompt 组装时按实际内容重新计量，并保持最终裁剪兜底。
7. 保证并发、重复任务和压缩失败不会破坏聊天主链路。
8. 提供足够 trace / log / metric 数据，用于后续按 P90/P95 校准预算。

## Non-Goals

本变更不包括：

- 不实现对下一轮 query、retrieve 命中或 tool 输出的精确预测。
- 不为每个模型立即接入官方精确 tokenizer。
- 不根据对话类型自动学习个性化压缩阈值。
- 不重新设计 structured summary schema、repair、validation 或 renderer。
- 不引入新的任务队列基础设施；第一阶段继续复用现有异步 summary worker。
- 不取消最终 chat context budget 的裁剪保护。

## Proposed Changes

### 1. Unified Token Estimation

summary、history、retrieve、tool 和最终 prompt budget 使用同一 `TokenEstimator` 接口及同一默认实现。

第一阶段默认使用通用 estimator，并支持：

- 每条 chat message 的结构开销
- 可配置安全系数
- 向上取整
- 后续替换为模型级 tokenizer，而不改变上层预算合同

### 2. Incremental Summary Boundary

触发计算与压缩输入都以最新有效 summary 的 `CoveredToMessageID` 为边界。

- 没有 summary：统计全部可压缩消息。
- 已有 summary：只统计 `CoveredToMessageID` 之后的新消息。
- 新 summary 合并上一版 structured summary 与未覆盖消息尾部。
- 成功保存后更新覆盖边界。

### 3. Post-Answer Async Trigger

压缩判断发生在助手回复成功落库之后。

压缩：

- 不影响当前轮响应
- 不阻塞 `finish` 事件
- 失败时 fail-open
- 生成结果供下一轮 history load 使用

### 4. Stage Budget Envelopes

`max-prompt-tokens` 被拆分为可解释预算：

- fixed prompt / current question
- long-term memory
- session recall
- retrieve context
- tool context
- safety reserve
- summary + uncovered history

上一轮结束后的压缩触发使用这些上限计算 history budget，而不是猜测下一轮实际注入内容。

### 5. Runtime Actual Recalculation

当前轮 retrieve 和 tool 完成后，系统使用实际生成内容重新计算完整 prompt token。

未使用的阶段预算可以由其他阶段利用，但完整 prompt 不得超过 `max-prompt-tokens`。现有最终裁剪逻辑继续作为最后保护。

### 6. Retrieve and Tool Token Limits

- retrieve context 在注入 prompt 前按 token budget 截断。
- tool context 使用 token budget 作为主限制，同时保留字符上限作为异常输出硬保护。
- 截断结果必须保留明确标记及观测字段。

### 7. Compatibility

`summary-start-turns` 可以在迁移期保留配置读取能力，但不再作为压缩触发的硬门槛。

## Acceptance Criteria

1. 生产聊天在助手回复成功落库后会异步执行 summary 压缩判断。
2. 压缩触发只统计最新 summary 与其后未覆盖消息，不重复统计已覆盖历史。
3. summary 与 chat prompt budgeting 使用同一 estimator。
4. token 估算包含消息结构开销和安全系数。
5. `summary-start-turns` 不再决定是否触发压缩。
6. history budget 根据 `max-prompt-tokens` 和阶段预算计算，并支持显式配置覆盖。
7. retrieve 和 tool context 都存在明确 token 上限。
8. 当前轮完成 context 注入后会对完整 prompt 重新估算。
9. 并发或重复压缩任务不会为同一覆盖边界生成重复有效 summary。
10. 压缩失败、估算失败或队列失败不会影响聊天成功。
11. trace / log / metrics 能看到各阶段估算 token、阈值、裁剪和压缩结果。
12. 测试覆盖真实 chat 写入到异步压缩的集成链路，而不仅是直接调用 summary helper。

## Risks

- 通用 estimator 与真实模型 tokenizer 存在误差。
- 阶段预算过保守会导致压缩过早。
- 阶段预算过宽会导致当前轮频繁裁剪。
- 异步 worker 可能收到重复任务。
- summary 生成耗时可能导致下一轮请求先于 summary 完成。

控制方式：

- 安全系数
- 最终 prompt 实测
- 覆盖边界幂等
- 原子或等价的重复提交保护
- 现有 prompt budget 兜底
- 记录真实使用分布并按 P90/P95 校准
