# Summary 评估阶段进展

## 当前结论

这轮 `summary` 专项优化已经从最初基线的 `2/12` 提升到当前稳定结果 `7/12`，通过样本数增加 `5` 个，`dangerous downstream drift` 样本数从 `3` 降到 `2`。当前改动已经证明“先把当前优先级显式结构化，再把背景问题下沉”这条方向是有效的，但距离稳定收敛还差最后 `5` 个失败样本。

## 当前保留方案

- 在结构化 summary schema 中新增 `active_priorities` 与 `background_issues`。
- 在 summary prompt 中明确区分“当前优先级”和“背景问题”，要求“不是当前重点”的内容不要进入 `active_priorities`。
- 在 renderer 中把输出顺序调整为“目标 -> 当前优先级 -> 约束 -> 用户偏好 -> 已确认事实 -> 最近进展 -> 待确认问题 -> 背景问题”。
- 在 repair 中保留优先级层次，并把“不是当前重点 / 背景问题 / 暂不处理”类内容下沉到 `background_issues`。
- 在 validator 中增加两类约束：
  - 如果源对话明确存在当前 focus，但 summary 没有 `active_priorities`，直接拒绝。
  - 如果 `active_priorities` 中混入明显背景项，直接拒绝。
- 在 evaluator 搜索文本中补入 `active_priorities` 与 `background_issues`，避免评估侧忽略这些字段。
- 修复 critical entity 提取对中文标点的覆盖，避免配置 key 被截断。

## 已验证结果

- 基线结果：`testdata/evals/summary/latest_run_20260621.json`
  - `passed = 2/12`
  - `dangerous drift samples = 3`
- 当前稳定结果：`testdata/evals/summary/latest_run_priority_hierarchy.json`
  - `passed = 7/12`
  - `dangerous drift samples = 2`
- 针对“如果你现在接手这个项目，下一周你会怎么排优先级？”的单对话实验中，summary 已能把“起草 summary 样本、完成 spec/design/tasks”放到前面，并把 `CI flaky`、`ClickHouse / PostgreSQL JSONB` 这类背景项压到后面。

## 明确回退的尝试

- 曾尝试在 repair 里加入更强的 `active_priorities` 重排序启发式，把“更具体的执行项”强行排到“更宽泛的规划项”前面。
- 这次尝试会明显破坏整套评估稳定性，曾导致全量结果退化到 `3/12`，且 `dangerous drift` 升到 `5`。
- 该启发式已经回退，当前保留的是更保守的去重与补全逻辑，不再主动重写优先级顺序。

## 当前剩余失败样本

- `critical_entity_error_code`
- `fact_vs_open_question_uncertain_root_cause`
- `progress_and_open_questions_coexist`
- `critical_entity_config_key`
- `fact_vs_open_question_proposed_architecture`

这些剩余问题大体分成三类：

- 已知事实与未证实猜测的边界还不够稳，尤其是根因排查类样本。
- 已确认进展与待确认问题共存时，字段落点还不够稳定。
- 某些 critical entity 虽然抽取能力变强了，但在最终摘要落点和 judge 对齐上还未完全收口。

## 关键文件

- `internal/app/rag/core/history/summary_schema.go`
- `internal/app/rag/core/history/summary_compression.go`
- `internal/app/rag/core/history/summary_renderer.go`
- `internal/app/rag/core/history/summary_repair.go`
- `internal/app/rag/core/history/summary_validator.go`
- `internal/app/rag/evaluation/summary_rules.go`

## 配套文档与实验产物

- 方案设计：`docs/superpowers/specs/2026-06-21-summary-priority-hierarchy-design.md`
- 实施计划：`docs/superpowers/plans/2026-06-21-summary-priority-hierarchy.md`
- 单对话对比实验：
  - `tmp/summary_compare_experiment.go`
  - `tmp/summary_compare_experiment.md`
  - `tmp/summary_compare_experiment.json`

## 下一步建议

- 先集中修 `fact vs open question` 两个失败样本，继续收紧“未证实根因不得写成事实”的 repair 与 validator 规则。
- 再单独处理 `progress_and_open_questions_coexist`，让“已确认决定”和“仍待确认问题”稳定共存。
- 最后回到 critical entity 两个残余样本，核对是抽取问题、字段落点问题，还是 judge query 触发的措辞漂移。
