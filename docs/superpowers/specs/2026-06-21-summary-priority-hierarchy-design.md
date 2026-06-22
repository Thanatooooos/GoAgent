# Summary Priority Hierarchy Design

Date: `2026-06-21`

## Background

The recent summary comparison experiment showed a useful shift in failure mode.

When rendered summary length was capped, `summary`-backed answers lost tail
details such as candidate storage options and the "CI flaky is not current
focus" qualifier. After re-rendering the structured summary without a length
limit, information coverage improved materially:

- `ClickHouse` / `PostgreSQL JSONB` stayed visible as candidates
- `ERR_POOL_TIMEOUT` uncertainty stayed visible
- `CI batch3 flaky test` stayed visible as background context

However, answer quality still drifted in one important way: the summary-backed
answer reordered the next-week priorities around the most salient unresolved
items, instead of preserving the original conversation's mainline execution
focus.

In short:

- the primary problem is no longer "missing information"
- the primary problem is now "incorrect information hierarchy"

The current structured summary schema does not distinguish strongly enough
between:

- the active workstream
- unresolved but secondary issues
- background issues that should not be promoted into the next action plan

## Goals

- Preserve the original conversation's active priority ordering more reliably in
  summary-backed answers.
- Encode active priorities explicitly instead of inferring them from mixed
  fields.
- Keep unresolved technical issues visible without letting them override the
  mainline by default.
- Keep backward-compatible rendered summary text for current consumers.
- Keep the change scoped to summary generation, rendering, validation, and
  evaluation-first regression.

## Non-Goals

- Full answer-stage query-aware context routing in this phase.
- Replacing the current summary storage mechanism.
- Reworking long-term memory or chat token-budget logic.
- Solving every ranking issue through prompt wording alone.

## Problem Statement

The current schema:

- `goal`
- `constraints`
- `user_preferences`
- `established_facts`
- `recent_progress`
- `open_questions`

is sufficient for coverage, but not for priority hierarchy.

Specifically:

1. `recent_progress` mixes truly active next steps with contextual updates.
2. `open_questions` correctly stores unresolved items, but these items often
   become visually or semantically prominent in downstream answers.
3. "Not current focus" information has no first-class home, so it can remain in
   generic sections and later get promoted by the model.

This makes summary-backed planning answers vulnerable to salience drift:

- unresolved production-sounding issues rise too high
- background issues re-enter the action list
- core execution tasks lose position

## Recommended Approach

Adopt a hierarchy-first schema and renderer update.

Instead of trying to solve the drift only through stronger prose rules, make
priority class an explicit part of the stored summary structure.

Recommended v1 change:

- add `active_priorities`
- add `background_issues`

Keep `open_questions`, but narrow its meaning:

- unresolved items that may affect near-term decisions
- not a generic bucket for everything not yet decided

This yields three distinct status lanes:

1. `active_priorities`
   - what should drive the next plan by default
2. `open_questions`
   - unresolved items worth tracking, but not automatically top priority
3. `background_issues`
   - known context that should remain visible without being promoted

## Data Model Changes

### Structured Summary Schema

Recommended schema shape:

```json
{
  "schema_version": 2,
  "goal": "起草 summary 样本，并明确 must_cover 和 critical_contract 的边界",
  "active_priorities": [
    "先完成 summary 和 rewrite 的 spec、design、tasks",
    "明确 must_cover 和 critical_contract 的边界",
    "收敛样本规则、模板和金标准样本"
  ],
  "user_preferences": [
    "数据库选型为 PostgreSQL",
    "样本文本内容中文；schema key 英文"
  ],
  "constraints": [
    "本周不直接进入实现",
    "Phase 1 只做 summary 和 rewrite",
    "follow-up query 数量上限以 sample_plan 为准"
  ],
  "established_facts": [
    "MySQL 方案作废",
    "rewrite 路径出现 ERR_POOL_TIMEOUT，配置 pool.max_active=50, pool.max_idle=5"
  ],
  "recent_progress": [
    "CI batch3 flaky test 通过 retry 临时稳住"
  ],
  "open_questions": [
    "prompt template 是否引入额外的 tool call 占位符",
    "模型路由是否切到了慢节点"
  ],
  "background_issues": [
    "CI batch3 flaky test 不是当前重点",
    "评测结果存储方案未来可能采用 ClickHouse 或 PostgreSQL JSONB"
  ]
}
```

### Field Semantics

- `goal`
  - one current primary objective
- `active_priorities`
  - the concrete next-step work items that should dominate planning answers
  - ordered, highest-priority first
  - only items active in the current scope belong here
- `background_issues`
  - issues explicitly present in context but not part of the current top-line
    execution focus
  - includes "known but not current priority" items

The existing fields retain their previous meanings, with these tighter
interpretations:

- `recent_progress`
  - status changes already achieved, not the future work plan
- `open_questions`
  - unresolved questions that may affect future decisions, but are not by
    default the first things to execute

## Prompt Contract Changes

The structured summary prompt should force classification before emission.

Before placing an item, the model should determine:

1. Is this a current action priority?
2. Is this an unresolved question that affects later choices?
3. Is this contextual background that should remain visible but not promoted?

New prompt rules:

- If the conversation explicitly says an item is "not current focus",
  "background only", or similar, do not place it in `active_priorities`.
- If an item is unresolved but the conversation does not make it the active
  workstream, place it in `open_questions`, not `active_priorities`.
- If the user summarizes current focus explicitly, prefer that summary over
  earlier local salience.
- `active_priorities` should be short, ordered, and execution-oriented.
- `recent_progress` should describe what changed, not what to do next.

## Rendering Strategy

### Principle

Rendered text should reflect planning hierarchy directly.

### New Render Order

Recommended render order:

1. `goal`
2. `active_priorities`
3. `constraints`
4. `user_preferences`
5. `established_facts`
6. `recent_progress`
7. `open_questions`
8. `background_issues`

This makes the next-step workstream visually dominant while keeping uncertainty
and background context available later in the rendered text.

### Section Labels

Suggested text labels:

- `目标：`
- `当前优先级：`
- `约束：`
- `用户偏好：`
- `已确认事实：`
- `最近进展：`
- `待确认问题：`
- `背景问题：`

## Validation Changes

Validation should be extended beyond entity preservation.

New lightweight checks:

1. If the source conversation contains an explicit "current focus" summary, at
   least one aligned item must appear in `active_priorities`.
2. Items marked as non-current-focus should not appear in
   `active_priorities`.
3. `active_priorities` must not be empty when the source dialogue clearly
   contains planning or phase-boundary language.
4. `recent_progress` items that look like future action plans should be
   demoted or mirrored into `active_priorities` during repair.

These checks should remain deterministic and conservative.

## Repair Strategy

Add a repair pass that is hierarchy-aware:

- promote explicit active-scope items into `active_priorities`
- demote "not current focus" items into `background_issues`
- keep unresolved root-cause hypotheses in `open_questions`
- prevent `open_questions` from becoming the only strongly populated section
  when the conversation already includes an explicit top-line priority summary

Repair remains non-generative:

- no new facts invented
- only reclassification, deduplication, and conservative backfill

## Rollout Plan

### Phase A: Schema + Prompt + Renderer

- add new schema fields
- update prompt instructions
- update rendering order and labels
- keep evaluation-compatible output artifacts

### Phase B: Repair + Validation

- implement hierarchy-aware repair
- add deterministic validation for active-priority preservation
- expand unit coverage

### Phase C: Eval Regression

- rerun the "hard planning question" comparison
- rerun the `summary` suite
- verify that priority answers stay anchored to mainline tasks without losing
  background details

## Testing Strategy

Minimum checks:

1. renderer includes `当前优先级` before `待确认问题`
2. summaries with explicit "not current focus" place such items in
   `background_issues`
3. explicit next-step items land in `active_priorities`
4. unresolved root-cause hypotheses stay in `open_questions`
5. the concrete comparison question about "下一周怎么排优先级" yields a
   summary-backed answer whose first items stay aligned with the original
   conversation

## Risks

- Overfitting to one planning-style sample if the hierarchy rules are too
  narrow.
- Making the schema too large and brittle.
- Accidentally duplicating the same item across `active_priorities`,
  `recent_progress`, and `background_issues`.

Mitigations:

- keep the added schema surface small
- favor reclassification over new generation
- add tests around conflict resolution and ordering

## Final Recommendation

Proceed with the hierarchy-first update.

The experimental evidence now says coverage is largely fixable by relaxing
budget, but answer quality still depends on how the summary encodes information
priority. The next improvement step should therefore make priority class
explicit in the structured summary and reflect that class in rendered output,
repair rules, and evaluation regression.
