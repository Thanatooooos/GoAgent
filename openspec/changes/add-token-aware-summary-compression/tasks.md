# add-token-aware-summary-compression Tasks

## Scope

本变更交付：

- 统一 token estimator
- token-aware summary trigger
- 基于 summary coverage 的增量压缩
- assistant 落库后的异步生产接入
- retrieve / tool token budget
- 当前轮完整 prompt 实测与兜底
- 并发幂等与观测

本变更不交付：

- 模型级精确 tokenizer 全量适配
- 自适应或用户级压缩策略
- 新的分布式任务队列
- summary schema 重设计

## 1. Freeze Token Budget Contracts

- [ ] 定义共享 `TokenEstimator` 接口和默认实现。
- [ ] 定义 message overhead、safety factor 及其应用位置。
- [ ] 定义 chat stage budget 配置结构。
- [ ] 定义显式 `summary-trigger-tokens` 与自动 history budget 的优先级。
- [ ] 定义非法配置和 estimator 失败时的 fallback 行为。

验证：

- estimator 合同测试
- budget 计算边界测试
- 安全系数不会重复应用

## 2. Unify Existing Estimation Call Sites

- [ ] 将 summary trigger 切换到共享 estimator。
- [ ] 将 chat prompt budget 切换到共享 estimator。
- [ ] 将 summary strategy token accounting 切换到共享 estimator。
- [ ] 清理或适配重复的 production estimator 实现。

验证：

- 相同文本在 summary 和 chat 路径得到相同估算
- 现有 chat context budget 测试继续通过

## 3. Implement Incremental Coverage Loading

- [ ] 基于最新 summary 的 `CoveredToMessageID` 查询未覆盖消息。
- [ ] 无 summary 时加载全部可压缩消息。
- [ ] 触发统计、summary 输入和保存覆盖边界使用同一消息集合。
- [ ] 移除“全部历史判断 + 固定最近消息压缩”的不一致行为。
- [ ] 支持未覆盖尾部超大时的分批增量压缩。

验证：

- 已覆盖消息不再重复计入
- 新 summary 正确继承 previous structured summary
- 覆盖范围连续且不倒退

## 4. Wire the Real Chat Trigger

- [ ] 定义 `SummaryTrigger` 或等价调度接口。
- [ ] 在 assistant 消息成功落库后调度 summary check。
- [ ] 保证 user 与 assistant 消息都已可见后再执行 worker。
- [ ] 调度和压缩失败保持 fail-open。
- [ ] `summary-start-turns` 不再作为硬触发门槛。

验证：

- 从真实 `RagChatService` 成功路径可观察到 summary check
- chat finish 不等待 summary 生成
- helper 单测之外存在生产链路集成测试

## 5. Add Concurrency and Idempotency Guards

- [ ] 增加 conversation-scoped in-flight 去重。
- [ ] 任务携带 enqueue 时的目标消息边界。
- [ ] worker 执行前重新检查 summary coverage。
- [ ] 保存前阻止相同或更旧覆盖边界的重复 summary。
- [ ] 明确单实例与多实例下的最终一致性保护。

验证：

- 重复 enqueue 只产生一个有效 summary
- 新er coverage 已存在时旧任务跳过
- 并发测试无覆盖范围回退

## 6. Add Retrieve Token Budget

- [ ] 为 retrieve context 增加配置化 token budget。
- [ ] 按 chunk 排序与 chunk 边界组装 context。
- [ ] 单个超大 chunk 支持 token 截断。
- [ ] 保留来源定位信息和截断标记。
- [ ] 输出 before/after token 与 retained chunk 数据。

验证：

- context 不超过 retrieve budget
- 高排序 chunk 优先保留
- 截断后仍可定位来源

## 7. Add Tool Token Budget

- [ ] 为 tool context 增加配置化 token budget。
- [ ] 保留字符 hard cap 作为异常保护。
- [ ] 按结果 section 优先级组装和截断。
- [ ] 优先保留结论、关键证据与来源。
- [ ] 输出 before/after token 与 dropped section 数据。

验证：

- 中英文、代码和 JSON 输出均受 token 上限控制
- 截断不会优先丢弃关键结论
- 超大异常输出仍受字符 hard cap 保护

## 8. Recalculate Actual Prompt Budget

- [ ] 在 retrieve/tool 完成后构建真实 prompt messages。
- [ ] 使用共享 estimator 计算完整 prompt。
- [ ] 保留动态借用未使用阶段预算的能力。
- [ ] 按结构化优先级执行降级。
- [ ] 保证最终 prompt 不超过 `max-prompt-tokens`。

验证：

- 无 tool 时未使用预算可被其他 context 使用
- 超限时 degradation steps 可解释
- summary 和当前问题不会被普通裁剪移除

## 9. Add Observability

- [ ] 记录 summary trigger token breakdown。
- [ ] 记录 estimator 名称/版本与 safety factor。
- [ ] 记录 stage token before/after。
- [ ] 记录 summary、retrieve、tool 的触发、跳过、截断与失败原因。
- [ ] 复用现有 trace / logs / metrics，不新增观测基础设施。

验证：

- 可从一次 trace 重建 prompt token 分配
- 可按数据计算各阶段 P90/P95

## 10. Verification and Rollout

- [ ] 运行 history、chat、bootstrap、evaluation 聚焦测试。
- [ ] 增加真实 chat -> async summary 的集成测试。
- [ ] 使用 summary strategy eval 比较候选阈值。
- [ ] 在开发环境验证下一轮请求早于 summary 完成的行为。
- [ ] 根据 trace 数据校准初始 stage budget。
- [ ] 更新项目进展文档与 OpenSpec task 状态。

完成标准：

- 模块、集成、生产入口三个层次均可证明 token trigger 已交付
- 默认配置可运行
- 不依赖手工调用 summary helper
