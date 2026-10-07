# 文档索引与使用范围

更新：2026-10-07。

## 从哪里开始

先读 [项目进展上下文](project_progress_context.md)，了解当前架构、已验证行为、待办及本地测试环境。设计文档描述目标，计划描述施工步骤，报告描述验证证据；不能把历史计划中的未勾选步骤直接作为当前待办，也不能把设计目标当作已经交付的能力。

## 当前工作与近期证据

- 普通 Chat Agent 执行过程：[视觉优化与验收](superpowers/reports/2026-10-07-agent-execution-visual.md)。多段思考、工具与结果时间线、长执行折叠及滚动跟随已接入，明暗与窄屏浏览器验收、8 项前端回归通过。

- Work 长期协作主题：[产品规格](superpowers/specs/2026-10-01-work-topics-product-design.md)、[交互设计](superpowers/specs/2026-10-01-work-topics-interaction-design.md)、[技术设计](superpowers/specs/2026-10-01-work-topics-technical-design.md)、[实施计划](superpowers/plans/2026-10-01-work-topics.md)、[交付核对](superpowers/reports/2026-10-02-work-delivery-audit.md)。Work 第一版六阶段及相关原入口回归完成，业务承诺 aligned；全量类型基线、原有数据事件与既有任务后续范围分别记录。
- Work 共同编辑文档：[方案与验收](superpowers/specs/2026-10-01-work-document-collaboration-design.md)。同一文档、人/AI 按轮次保存、富文本/版本/恢复/CAS/冲突草稿保护已实现；真实模型和故障验证见交付核对。
- Work 界面与视觉：[已确认设计](superpowers/specs/2026-10-01-work-visual-design.md)。正式主题首页、合并导航、文档编辑、深色与窄屏交互已实现并验证；[场景推演](superpowers/reports/2026-10-01-work-topic-scenario-walkthrough.md) 仍是历史设计依据。
- Work 预览误启动：[影响与修正](superpowers/reports/2026-10-01-work-preview-config-incident.md)。原有库受影响记录未回滚；后续预览必须使用专用库和受保护启动脚本。
- 通用定时任务：[规格](superpowers/specs/2026-09-30-scheduled-agent-tasks-design.md)、[计划](superpowers/plans/2026-09-30-scheduled-agent-tasks.md)、[首轮交付审计](superpowers/reports/2026-10-01-scheduled-task-delivery-audit.md)。最新详情布局和显式状态按钮见项目进展上下文。
- DailyBrief 通用任务迁移：[首次代码审计](superpowers/reports/2026-10-03-dailybrief-scheduled-task-migration.md)、[迁移完成与运行验收](superpowers/reports/2026-10-06-dailybrief-migration-followup.md)。原库 20 条订阅已绑定，9090 的 scheduled 服务已停用旧链路；真实模型/HTTP/浏览器/实库验收通过。保留唯一今日失败并明确为 missed，明天按原时刻运行；体育试点仍开放。
- 对话运行时：[收敛规格](conversation-agent-runtime-spec.md)。`internal/app/runtime` 已存在，本文有分阶段实施历史，继续开发应对照实际装配，不重新引入旧 agent 引擎。
- Chat / Work 正文一致性：[修复与验证](superpowers/reports/2026-10-07-chat-visible-body-consistency.md)。各模型轮次的可见正文统一发布，长回答历史展示原文，完成事件校准流式结果；相关回归通过，未部署或历史回填。
- 普通 Chat 深度思考：[开关接入与验证](superpowers/reports/2026-10-07-chat-thinking-switch.md)。请求开关控制各模型轮次 thinking，开启时独立 SSE 展示，关闭默认显式生效；未部署，历史 thinking 展示及 Work 开关仍属后续范围。
- 文件解析与图片证据：[DocReader 接入](docreader_integration.md)、[图片证据规格](superpowers/specs/2026-09-24-image-evidence-ingestion-design.md)、[计划](superpowers/plans/2026-09-24-image-evidence-ingestion.md)、[验收记录](image_evidence_delivery_report.md)。文件名中的 ingestion 表示入库业务，不代表已移除的 ingestion 模块仍存在。
- 检索实验：[预算检验](retrieval_budget_probe_report.md)、[跨文档证据检验](cross_document_relation_probe_report.md)、[图检索检验](graph_retrieval_probe_report.md)。样本 JSON 与报告配套保留，不能把实验结果直接外推为线上可靠性。
- 检索评估与改写：[评估计划](retrieve_eval_plan.md)、[报告模板](retrieve_eval_report_template.md)、[术语治理](rewrite_governance.md)。具体命令和样本数量执行前以代码核对。

## 保留的历史设计如何使用

- 六月摘要、记忆写回及 token 预算设计保留：相关结构、评估器或机制仍有代码对应，较早日期不代表失效。它们是设计来源，不是当前进度总表。
- DailyBrief 设计保留：issue 与历史页面保留，checked-in 默认 mixed 保护其他未交接环境；本机原库已完成显式交接并使用 scheduled，关闭旧采集/调度/重试。后续重启、备份及证据边界以 2026-10-06 完成报告为准。
- Wiki、llmgen、并发限流、SSE、引用协议及 UI 设计保留：仍有独立模块或使用价值。旧 ingestion 节点和旧 agent capability 接入部分不再适用，相关文档已增加范围说明。
- 优先使用项目进展上下文和对应交付报告判断现状；历史文档出现旧目录、模型配置、测试端口时，不照搬恢复旧实现。

## 2026-10-01 清理记录

已删除 27 个文档。删除前核对它们均为已跟踪且没有本轮前未提交修改的文件；没有删除当前未跟踪的规格、实验报告或样本。

清理依据：旧 agent 模块及接入链路已经不存在；旧 ingestion/DAG/Redis 队列与增强节点已被统一 chunk 处理替代；五月、六月评估和综合待办是过期快照；一次性临时输入实验与早期评估进度不能代表当前结果；旧联调修复计划绑定过期本地配置。保留核心摘要设计，删除的只是一次性实验文档。删除文件的已提交版本可从 Git 历史恢复。

删除清单：

- `agent_capability_followup.md`
- `agent_capability_onboarding.md`
- `agent_runtime_scope_20260611.md`
- `agent_tool_parity_matrix.md`
- `agent_tool_parity_test_plan.md`
- `archive/agent_module_evaluation_20260610.md`
- `archive/codebase_evaluation_20260608.md`
- `archive/eino_graph_agent_loop_design.md`
- `archive/rag_service_split_plan.md`
- `functional_improvement_todo_20260609.md`
- `structural_improvement_plan.md`
- `memory_improvement_plan.md`
- `ingestion/target_direction.md`
- `superpowers/plans/2026-06-25-evolve-agent-runtime-engine.md`
- `superpowers/specs/2026-06-25-evolve-agent-runtime-engine-design.md`
- `superpowers/plans/2026-07-11-durable-ingestion-queue.md`
- `superpowers/plans/2026-07-11-document-enrichment.md`
- `superpowers/specs/2026-07-11-document-enrichment-design.md`
- `superpowers/plans/2026-06-21-summary-eval-progress.md`
- `superpowers/plans/2026-06-22-summary-processing-from-prompt-history-v2.md`
- `superpowers/specs/2026-06-22-summary-processing-from-prompt-history-v2-design.md`
- `superpowers/plans/2026-08-09-wiki-p0.md`
- `superpowers/plans/2026-08-09-wiki-p1.md`
- `superpowers/plans/2026-08-09-wiki-p3.md`
- `superpowers/plans/2026-08-09-llmgen-guardrails.md`
- `superpowers/plans/2026-08-10-integration-repair-plan.md`
- `superpowers/specs/2026-08-10-integration-repair-design.md`
