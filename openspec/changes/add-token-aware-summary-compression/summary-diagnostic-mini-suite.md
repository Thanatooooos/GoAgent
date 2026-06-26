# Summary Diagnostic Mini-Suite Draft

## Purpose

本草案用于在优化 summary prompt / validation / schema 前，先扩展诊断样本覆盖面，避免只针对单个长对话样本过拟合。

目标不是立刻提高分数，而是定位当前 summary 策略在哪类状态模式上稳定失败。

## Scope

第一版 mini-suite 包含 5 个受控 scenario，主体为每个 18 轮用户问题；`open_question_resolution` 由于 18 轮生成结果贴近但略低于 `8000` estimated dialogue tokens，额外补 1 轮最终状态复述，因此为 19 轮。后续用当前默认 chat 模型生成真实 assistant 回答，并在 review 后转成 `SummarySample`。

每个 scenario 的长度目标是明显超过 `8000` estimated dialogue tokens，使 `summary-token-thresholds=8000` 至少触发一次 summary。12 轮草案曾作为初版生成验证，但多数样本不足 `8000` tokens，不适合作为 8000 阈值压缩诊断样本。

本阶段只产出：

- 受控问题脚本
- checkpoint gold contract 草案
- failure taxonomy 标签

本阶段不产出：

- 外部模型生成结果
- 已 review 的最终 gold sample
- summary prompt / schema / validation 实现改动

## Scenarios

### 1. `database_decision_override`

测试模式：旧数据库方案先暂定，后续被正式作废，新数据库方案成为当前有效决定。

主要风险：

- stale fact retained：MySQL 被保留成当前有效方案。
- state override failure：PostgreSQL 生效、MySQL 作废没有同时保留。
- forbidden claim：迁移尚未执行却被写成已完成。

### 2. `implementation_boundary`

测试模式：多次讨论实现方案，但用户明确当前只允许 spec/design/tasks，不允许进入生产实现。

主要风险：

- proposal promoted：assistant 的实现建议被摘要成已完成进展。
- forbidden claim：摘要声称已经开始编码、迁移或上线。
- constraint missing：不能进入生产实现的硬约束被丢掉。

### 3. `threshold_candidate_vs_production`

测试模式：stress thresholds、realistic candidates、production threshold 三种状态并存。

主要风险：

- numeric threshold drift：800/1200/1600 被误写成生产候选或最终阈值。
- open question promoted：production threshold / safety factor 被写成已确定。
- current goal drift：当前目标从“实现”漂到“校准阈值”之外。

### 4. `retrieve_tool_budget_uncertainty`

测试模式：retrieve/tool 动态注入只能估算，真实 trace 尚未校准，最终 prompt 实测仍是硬保护。

主要风险：

- uncertainty collapse：估算、观测、真实 trace 校准被写成已经解决。
- budget contract missing：retrieve/tool stage budget 与 final prompt budget 的关系被丢失。
- forbidden claim：声称已经能精确预测下一轮 retrieve/tool 内容。

### 5. `open_question_resolution`

测试模式：多个开放问题中，一部分被解决，一部分仍开放。

主要风险：

- open question promoted：仍开放的问题被写成已解决。
- resolved fact missing：已解决的问题没有进入 established facts / recent progress。
- mixed state confusion：所有问题被一锅端成“都已解决”或“都未解决”。

## Generation Rules

后续生成真实问答时，每轮请求应携带：

- system prompt
- 当前 scenario 内此前所有 user/assistant 历史
- 当前用户问题

不得发送：

- 项目源码
- API key / 请求头
- 与 scenario 无关的仓库内容

## Review Rules

生成后需要人工 review：

- assistant 是否把尚未执行的计划说成已完成；
- 是否出现 provider、鉴权、配置私密信息；
- 是否存在明显跑题或不自然的空泛回答；
- gold contract 是否只根据用户确认和对话真实内容填写，而不是根据设计意图凭空填写。

## First Evaluation Recommendation

第一轮只跑一个 realistic threshold：

- `summary-token-thresholds=8000`

前提是 5 个 scenario 的 reviewed 样本均超过 `8000` estimated dialogue tokens。若生成后的样本仍不足，应继续扩展问题脚本或降低诊断阈值，并明确该阈值是 diagnostic/stress 运行参数，不是生产结论。

如果 5 个 scenario 中失败模式集中，再针对集中失败类型优化 summary；如果失败模式分散，优先改诊断输出或 schema，而不是盲目调 prompt。
