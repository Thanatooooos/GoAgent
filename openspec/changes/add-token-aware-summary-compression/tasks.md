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

- [x] 定义共享 `TokenEstimator` 接口和默认实现。
- [x] 定义 message overhead、safety factor 及其应用位置。
- [x] 定义 chat stage budget 配置结构。
- [x] 定义显式 `summary-trigger-tokens` 与自动 history budget 的优先级。
- [x] 定义非法配置和 estimator 失败时的 fallback 行为。

验证：

- estimator 合同测试
- budget 计算边界测试
- 安全系数不会重复应用

## 2. Unify Existing Estimation Call Sites

- [x] 将 summary trigger 切换到共享 estimator。
- [x] 将 chat prompt budget 切换到共享 estimator。
- [x] 将 summary strategy token accounting 切换到共享 estimator。
- [x] 清理或适配重复的 production estimator 实现。

验证：

- 相同文本在 summary 和 chat 路径得到相同估算
- 现有 chat context budget 测试继续通过

## 3. Implement Incremental Coverage Loading

- [x] 基于最新 summary 的 `CoveredToMessageID` 查询未覆盖消息。
- [x] 无 summary 时加载全部可压缩消息。
- [x] 触发统计、summary 输入和保存覆盖边界使用同一消息集合。
- [x] 移除“全部历史判断 + 固定最近消息压缩”的不一致行为。
- [x] 支持未覆盖尾部超大时的分批增量压缩。

验证：

- 已覆盖消息不再重复计入
- 新 summary 正确继承 previous structured summary
- 覆盖范围连续且不倒退

## 4. Wire the Real Chat Trigger

- [x] 定义 `SummaryTrigger` 或等价调度接口。
- [x] 在 assistant 消息成功落库后调度 summary check。
- [x] 保证 user 与 assistant 消息都已可见后再执行 worker。
- [x] 调度和压缩失败保持 fail-open。
- [x] `summary-start-turns` 不再作为硬触发门槛。

验证：

- 从真实 `RagChatService` 成功路径可观察到 summary check
- chat finish 不等待 summary 生成
- helper 单测之外存在生产链路集成测试

## 5. Add Concurrency and Idempotency Guards

- [x] 增加 conversation-scoped in-flight 去重。
- [x] 任务携带 enqueue 时的目标消息边界。
- [x] worker 执行前重新检查 summary coverage。
- [x] 保存前阻止相同或更旧覆盖边界的重复 summary。
- [x] 明确单实例与多实例下的最终一致性保护。

验证：

- 重复 enqueue 只产生一个有效 summary
- 新er coverage 已存在时旧任务跳过
- 并发测试无覆盖范围回退

## 6. Add Retrieve Token Budget

- [x] 为 retrieve context 增加配置化 token budget。
- [x] 按 chunk 排序与 chunk 边界组装 context。
- [x] 单个超大 chunk 支持 token 截断。
- [x] 保留来源定位信息和截断标记。
- [x] 输出 before/after token 与 retained chunk 数据。

验证：

- context 不超过 retrieve budget
- 高排序 chunk 优先保留
- 截断后仍可定位来源

## 7. Add Tool Token Budget

- [x] 为 tool context 增加配置化 token budget。
- [x] 保留字符 hard cap 作为异常保护。
- [x] 按结果 section 优先级组装和截断。
- [x] 优先保留结论、关键证据与来源。
- [x] 将 renderer 的 retained/dropped section 统计透传到生产 trace。

验证：

- 中英文、代码和 JSON 输出均受 token 上限控制
- 截断不会优先丢弃关键结论
- 超大异常输出仍受字符 hard cap 保护

## 8. Recalculate Actual Prompt Budget

- [x] 在 retrieve/tool 完成后构建真实 prompt messages。
- [x] 使用共享 estimator 计算完整 prompt。
- [x] 保留动态借用未使用阶段预算的能力。
- [x] 按结构化优先级执行降级。
- [x] 保证最终 prompt 不超过 `max-prompt-tokens`。

验证：

- 无 tool 时未使用预算可被其他 context 使用
- 超限时 degradation steps 可解释
- summary 和当前问题不会被普通裁剪移除

## 9. Add Observability

- [x] 记录 summary trigger token breakdown。
- [x] 记录 estimator 名称/版本与 safety factor。
- [x] 记录 stage token before/after。
- [x] 记录 summary、retrieve、tool 的触发、跳过、截断与失败原因。
- [x] 复用现有 trace / logs / metrics，不新增观测基础设施。

验证：

- 可从一次 trace 重建 prompt token 分配
- 可按数据计算各阶段 P90/P95

## 10. Verification and Rollout

- [x] 运行 history、chat、bootstrap、evaluation 聚焦测试。
- [x] 增加真实 chat -> async summary 的集成测试。
- [x] 将 summary strategy eval 的候选阈值从轮数口径升级为 token budget 口径并执行比较。
- [ ] 在开发环境验证下一轮请求早于 summary 完成的行为。
- [ ] 根据真实 trace 数据校准初始 stage budget。
- [x] 更新项目进展文档与 OpenSpec task 状态。

完成标准：

- 模块、集成、生产入口三个层次均可证明 token trigger 已交付
- 默认配置可运行
- 不依赖手工调用 summary helper

## Verification Notes

### 2026-06-23

- 聚焦测试通过：
  - `internal/app/rag/core/tokenbudget`
  - `internal/app/rag/core/history`
  - `internal/app/rag/core/retrieve`
  - `internal/app/rag/tool/...`
  - `internal/app/rag/service/sessionrecall`
  - `internal/app/rag/service/chat`
  - `internal/adapter/repository/postgres/rag`
  - `internal/bootstrap/rag`
  - `internal/app/rag/evaluation`
- 生产链路集成测试已验证：
  - assistant 消息落库后异步调度 summary
  - summary 覆盖边界推进到 assistant message id
  - 重复目标不会创建第二条有效 summary
- 最终审查补充验证：
  - chat prompt token 估算与 summary 触发统一计入 message overhead
  - 同一会话的异步任务串行执行，并合并到最新目标覆盖边界
  - retrieve 截断后保留 document/chunk 来源标识
  - tool retained/dropped section 统计已透传到 chat trace
  - summary token check 已记录 estimator 名称/版本
  - worker 并发回归用例连续运行 100 次通过
  - `openspec validate add-token-aware-summary-compression --strict` 通过
- 尚未完成的 rollout 校准工作：
  - 尚无开发环境“下一轮早于 summary 完成”的实测记录
  - 尚无真实 trace 的 P90/P95 数据用于调整初始 stage budget
- 当前 Windows Go 环境未启用 CGO，`go test -race` 无法执行。
- 2026-06-23 已执行 token threshold strategy sweep：
  - 命令阈值：`800 / 1200 / 1600`
  - 结果文件：`tmp/summary_strategy_token_thresholds_20260623.json`
  - 三档 `threshold_unit` 均为 `tokens`，pass rate 和 downstream equivalence 均为 `1.0`
  - 当前 strategy 样本在检查点的累计基线约为 `230 tokens`，三档 `summary_call_count` 均为 `0`，因此 token saved ratio 均为 `0`
  - 本次结果证明 token 阈值入口、单位输出和真实模型评估链路可运行，但样本长度不足以区分 `800 / 1200 / 1600` 的压缩效果；后续预算校准应补充更长样本或使用更低候选阈值
- 2026-06-24 已补充真实长对话 strategy 样本：
  - 问题脚本：`testdata/evals/summary/long_dialogue_questions.json`
  - 生成产物：`testdata/evals/summary/generated/software_project_state_transitions_v1.json`
  - reviewed 样本：`testdata/evals/summary/strategy_long_samples.json`
  - 生成方式：每轮调用外部模型时携带 system prompt、历史 user/assistant 问答和当前问题，最终得到 `24` 轮、`48` 条 source messages、约 `18k` dialogue tokens 的长对话
  - focused fixture 验证通过：`go test ./internal/app/rag/evaluation -run 'Test(LongSummaryStrategyFixture|GeneratedLongSummaryDialogArtifactFixture|ParseSummarySamples|SummaryDialog)' -count=1`
- 2026-06-24 长对话 token threshold sweep 证据：
  - `1600` token 阈值结果文件：`tmp/summary_strategy_long_threshold_1600_20260624.json`
    - `summary_call_count=11`
    - `token_saved_ratio=0.975`
    - `avg_downstream_equivalence=0.55`
    - `critical_failure_count=4`
    - `dangerous_drift_count=3`
    - 结果：未通过
  - `4000` token 阈值结果文件：`tmp/summary_strategy_long_threshold_4000_20260624.json`
    - `summary_call_count=4`
    - `token_saved_ratio=0.844`
    - `avg_downstream_equivalence=0.388`
    - `critical_failure_count=4`
    - `dangerous_drift_count=4`
    - 结果：未通过
  - `8000` token 阈值结果文件：`tmp/summary_strategy_long_threshold_8000_20260624.json`
    - `summary_call_count=2`
    - `token_saved_ratio=0.75`
    - `avg_downstream_equivalence=0.65`
    - `critical_failure_count=4`
    - `dangerous_drift_count=2`
    - 结果：未通过
  - 初步结论：
    - `800 / 1200 / 1600` 更适合作为 stress thresholds，用于制造频繁压缩并观察 drift，不宜直接作为生产候选阈值。
    - `4000 / 8000` 更接近 realistic candidate 区间；其中 `8000` 将 24 轮长对话压缩次数降到 `2`，仍保留约 `75%` token savings。
    - 质量失败并非单纯由阈值过小造成；即使 `8000` 仍出现 `critical_entities_missing`，说明当前 summary 生成 prompt / structured summary schema / critical contract 对齐仍需加强对 state override、critical entities 和 forbidden claims 的保留。
    - 本次证据不能替代真实 trace 的 P90/P95 校准；`production safety factor` 与初始 stage budget 仍保持开放。
- 仓库级 `go test ./...` 的本次相关包全部通过，但整体仍被工作区既有问题阻塞：
  - 根目录多个 `tmp_qwenmax_*.go` 重复声明 `main` 和辅助类型
  - `scripts` 目录多个独立脚本位于同一 package，重复声明 `main`
  - `config` 测试仍断言 `qwen-plus`，当前工作区配置为 `qwen-max-test`
- `go vet ./...` 另有既有 `context.WithTimeout` cancel 未调用警告。

### 2026-06-25

- 已补充 summary diagnostic mini-suite 的 5 个 controlled scenario，用于诊断单一样本以外的 summary 质量问题：
  - `database_decision_override`
  - `implementation_boundary`
  - `threshold_candidate_vs_production`
  - `retrieve_tool_budget_uncertainty`
  - `open_question_resolution`
- v3 样本生成状态：
  - 原始问答产物位于 `tmp/summary_diagnostic_generated_v3/*.json`
  - `database_decision_override`：18 轮，约 `13168` tokens
  - `implementation_boundary`：18 轮，约 `11347` tokens
  - `threshold_candidate_vs_production`：18 轮，约 `9284` tokens
  - `retrieve_tool_budget_uncertainty`：18 轮，约 `11492` tokens
  - `open_question_resolution`：19 轮，约 `8487` tokens；该场景因 18 轮接近但未稳定超过 8000 tokens，补充第 19 轮
  - UTF-8 校验通过，重建后的 `tmp/summary_diagnostic_samples_v3.json` 不含 `????` 残留
- 已执行 8000 token 阈值的 diagnostic mini-suite strategy eval：
  - `database_decision_override` 结果文件：`tmp/summary_diagnostic_8000_database_decision_override_20260625.json`
    - `summary_call_count=1`
    - `token_saved_ratio=0.616`
    - `summary_fidelity_score=0`
    - `summary_usefulness_score=0.907`
    - `downstream_equivalence_score=0.967`
    - `critical_failure_count=3`
    - `dangerous_drift_count=0`
    - 结果：未通过
  - `implementation_boundary` 结果文件：`tmp/summary_diagnostic_8000_implementation_boundary_20260625.json`
    - `summary_call_count=1`
    - `token_saved_ratio=0.372`
    - `summary_fidelity_score=0`
    - `summary_usefulness_score=0.793`
    - `downstream_equivalence_score=0.933`
    - `critical_failure_count=3`
    - `dangerous_drift_count=1`
    - 结果：未通过
  - `open_question_resolution` 结果文件：`tmp/summary_diagnostic_8000_open_question_resolution_20260625.json`
    - `summary_call_count=1`
    - `token_saved_ratio=0.536`
    - `summary_fidelity_score=0.233`
    - `summary_usefulness_score=0.833`
    - `downstream_equivalence_score=0.867`
    - `critical_failure_count=2`
    - `dangerous_drift_count=1`
    - 结果：未通过
  - `retrieve_tool_budget_uncertainty` 结果文件：`tmp/summary_diagnostic_8000_retrieve_tool_budget_uncertainty_20260625.json`
    - `summary_call_count=1`
    - `token_saved_ratio=0.386`
    - `summary_fidelity_score=0`
    - `summary_usefulness_score=0.887`
    - `downstream_equivalence_score=0.733`
    - `critical_failure_count=3`
    - `dangerous_drift_count=2`
    - 结果：未通过
  - `threshold_candidate_vs_production` 结果文件：`tmp/summary_diagnostic_8000_threshold_candidate_vs_production_20260625.json`
    - `summary_call_count=1`
    - `token_saved_ratio=0.431`
    - `summary_fidelity_score=0`
    - `summary_usefulness_score=0.903`
    - `downstream_equivalence_score=0.967`
    - `critical_failure_count=5`
    - `dangerous_drift_count=0`
    - 结果：未通过
  - 聚合结果：
    - pass count：`0/5`
    - 平均 `token_saved_ratio=0.468`
    - 平均 `summary_fidelity_score=0.047`
    - 平均 `summary_usefulness_score=0.865`
    - 平均 `downstream_equivalence_score=0.893`
    - 合计 `critical_failure_count=16`
    - 合计 `dangerous_drift_count=4`
- 诊断结论：
  - `8000` token 阈值在 mini-suite 上可将每个 scenario 的压缩次数控制到 `1` 次，但不能证明当前 summary 质量可用。
  - 下游等价分整体不低，说明模型在最终问答层面仍能恢复部分语义；但 structured summary fidelity 很低，主要问题集中在 `established_facts` 对关键边界、状态覆盖和禁止断言的显式保留不足。
  - 失败不再主要是“压缩过频”问题，而是 summary prompt / structured schema / critical contract 映射没有强制把 state override、production boundary、candidate-vs-production 状态、retrieve/tool uncertainty 和 open-question resolution 写入稳定字段。
  - 后续优化应先针对 summary 内容保真与关键事实落槽，而不是继续只调整 token 阈值。
