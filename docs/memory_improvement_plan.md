# Memory 模块改进计划（短期 / 中期 / 长期）

更新时间：`2026-06-10`

这份文档给出 memory 模块的分阶段改进计划，目标有两个：

1. **性能**：降低 chat 主链路上 memory 相关阶段的时延与 token 成本
2. **用户体验**：让记忆"更准、更透明、更可控"，并向"自动管理知识库的助手"这一产品目标靠拢

计划遵循 `docs/structural_improvement_plan.md` 的执行总原则（小步、测试先行、不携带无关重构），并与 `functional_improvement_todo_20260609.md` 的 P1-4 / P1-5（token budget / summary 异步化）衔接。

---

## 0. 当前 Memory 架构盘点（事实基线）

当前 memory 能力实际分布在五个层次：

### 0.1 会话历史与摘要压缩

- 代码：`internal/app/rag/core/history`
- 能力：
  - `MessageServiceStore`：最近 N 条历史加载（`history-keep-turns: 4`）
  - `summaryCompressionEngine`：消息数达到 `summary-start-turns * 2` 后触发 LLM 摘要压缩（`summary-max-chars: 200`）
  - **在途**：`InMemorySummaryJobWorker` 异步任务（`summary-async.enabled: false`，尚未默认开启）
  - **在途**：summary 生命周期字段已落库（`summary_version / covered_from|to_message_id / source_message_count / quality_status / last_rebuild_reason`，migration `20260609000000`）

### 0.2 长消息处理 + 会话内召回（Session Memory）

- 代码：`long_message_content_processor.go`、`session_recall_service.go`、`SessionChunkRepository`
- 能力：中长消息写时摘要 + 原文 chunk 入会话检索层；后续轮按向量召回，独立 `SessionContext` 注入 prompt
- 配置：`direct-context-max-tokens: 3000`、`chunk-summary-threshold-tokens: 12000`、`session-recall.max-excerpts: 3`

### 0.3 长期记忆（longtermmemory）

- 代码：`internal/app/rag/service/longtermmemory`（root + `governance` + `recall` + `types`）
- 能力：
  - 写侧：仅显式保存（`POST /rag/v3/remember`），governance 含 normalize / gate / schema / 冲突检测 / 单值唯一索引保护
  - 读侧：规则型记忆走 `MemoryContext`，事实型记忆投影进 `memory_fact` 检索通道（融合权重 0.9）
  - 缓存：request L1 / conversation L1 / Redis L2 + scope version 失效；**当前 `rag.memory.cache.enabled: false`，L2 实际未启用**
  - 运维：maintenance 后台循环 + `GET /rag/memory/metrics`
- 排序：lexical token 打分为主 + 向量分数 rerank（`recall/ranking.go`）

### 0.4 Token 预算与上下文裁剪（在途）

- 代码：`internal/app/rag/service/chat_context_budget.go`、`chat_token_usage.go`
- 能力：`max-prompt-tokens: 8000`，超限时按 `history -> tool -> knowledge -> session -> memory` 顺序降级裁剪
- 现状：估算用 `RoughTokenEstimator`（粗略字符比），真实 usage 已可从 `infra-ai/chat/token_usage.go` 拿到但尚未用于校准

### 0.5 Agent 集成

- `internal/app/agent/memory_recall`：只读 recall capability（低风险、幂等、产出 evidence）
- **没有写侧 capability**：agent 无法提议保存记忆

### 0.6 本轮盘点发现的具体缺陷（直接进短期任务）

1. `InMemorySummaryJobWorker.EnqueueConversationSummary(...)`：队列满时 `default` 分支**直接起新 goroutine 执行**，高峰期等于无界并发，且任务无重试、无指标、进程重启即丢失
2. `quality_status` 只会写 `unchecked`，没有任何校验器消费它——生命周期字段目前是"有字段、无行为"
3. `buildCompressPrompt(...)` 用 `content[:500]` 做**字节截断**，中文内容会切坏 UTF-8 字符
4. 摘要消息的识别/置顶依赖字符串前缀 `"对话摘要："`（`chat_context_budget.go` 的 `isConversationSummaryMessage` 与 `DecorateIfNeeded` 双处耦合），属于脆弱契约
5. `summary-max-chars: 200` 对长对话过于激进，摘要失真风险高

---

## 1. 短期（1~4 周）：收口在途工作，修正确性，拿到第一批性能数据

**阶段目标：summary 异步化真正可开（首包时延下降可量化）；摘要质量闭环从"有字段"变成"有行为"；token 预算从"粗估"变成"可校准"。**

### S1：异步摘要任务硬化（P0）

当前 `summary_job.go` 是最小骨架，开启前必须补齐：

- 队列满时的溢出策略改为**丢弃 + 计数**（或同步降级执行一次），删除"溢出起 goroutine"路径
- 每个 job 增加独立超时 context（复用 `rag.memory.maintenance.run-timeout-ms` 的工程模式）
- `Stop()` 时 drain 队列中剩余任务（带总超时），而不是直接丢弃
- 增加指标：`enqueued / processed / failed / dropped`，并入 `GET /rag/memory/metrics`
- 失败任务允许一次延迟重试；连续失败只记数，不阻塞
- 配置补 `summary-async.queue-size`

验收标准：

- 并发压入超过队列容量时，goroutine 数量有界，丢弃可观测
- 开启 `summary-async.enabled: true` 后，对比同一对话场景的首包时延（写入路径不再串行等待 LLM 摘要），数据写入本文档
- 进程重启丢任务作为 v1 已知边界明确记录（补偿：下一轮 `CompressIfNeeded` 天然重触发，阈值逻辑已幂等）

涉及文件：`internal/app/rag/core/history/summary_job.go`、`service_store.go`、`internal/bootstrap/rag/runtime_build_conversation.go`、config

### S2：摘要质量校验闭环（P0）

让 `quality_status` 从摆设变成行为：

- 新增 `summary_quality.go`（history 包内）：最小校验器
  - 非空 / 长度边界 / 语言一致性
  - **关键实体保留校验**：从被压缩消息中提取结构化约束项（ID、错误码、数字、专有名词），校验摘要是否保留——直接复用 `core/rewrite/constraint_guard.go` 的提取思路，不重复造轮子
- 校验不通过：`quality_status=rejected`，保留旧 summary 供 prompt 使用，`last_rebuild_reason` 记录拒绝原因
- 校验通过：`quality_status=accepted`
- rejected 后下一轮触发重建（最多一次，防循环）
- trace：summary 压缩补独立 trace node（当前完全不可见），记录 `qualityStatus / rebuildReason / coveredRange`

验收标准：

- 能回答"摘要错了怎么发现、怎么止损"（对应 todo 清单第 7 项）
- 构造"摘要丢关键 ID"的测试样例，rejected 路径可复现且旧摘要不被污染

涉及文件：`internal/app/rag/core/history/`（新增 `summary_quality.go`）、`summary_compression.go`、`chat_tracer.go`

### S3：摘要正确性小修（P0，半天级）

- `buildCompressPrompt` 的 `content[:500]` 改为 rune 安全截断
- `summary-max-chars` 默认从 200 提到 400~600（配置化已具备，给出推荐值并实测摘要质量）
- 压缩输入消息的截断长度也改为按 token 估算而非字节

### S4：摘要 pinning 契约去字符串化（P1）

- `convention.ChatMessage` 增加 `Kind`（或 metadata）标识 summary 消息，`splitPinnedConversationHistory` 改为按标识判断
- `"对话摘要："` 前缀仅保留为展示装饰，不再承担识别语义
- 同步修改 `chat_context_budget.go` 与 `service_store.go::DecorateIfNeeded`

### S5：Token 估算校准与观测（P1）

- 用 `chat_token_usage.go` 已拿到的真实 usage 回写校准：trace 中记录 `estimatedPromptTokens` vs `actualPromptTokens` 偏差
- `RoughTokenEstimator` 按偏差数据调整系数（中文/英文分系数即可，不引入 tokenizer 依赖）
- `max-prompt-tokens` 按目标模型上下文窗口给出推荐配置说明

验收标准：估算偏差中位数 < 15%；偏差可在 trace 中观测。

### S6：开启并验证 L2 缓存（P1）

- 当前 `rag.memory.cache.enabled: false`，Phase 4 建设的 Redis L2 实际闲置
- 任务：在带 Redis 的环境开启，跑 recall 基准（冷/热），记录 `rule / fact / embedding` 三类缓存命中率与时延差
- 根据数据决定默认值；若保持关闭，在配置注释中写明原因

### S7：recall 内部单测补齐 + 指标接 Prometheus（P2）

- `recall` 包（`ranking / tokens / projection / cache_support`）补直接单测——这是进度文档 2026-05-25 遗留的明确待办
- memory 指标随 P1-7（Prometheus 接入）一并暴露，优先低基数 counter：缓存命中、maintenance、摘要任务、fail-open 计数

---

## 2. 中期（1~3 个月）：自动记忆抽取，分层摘要，召回质量可证明

**阶段目标：记忆写入从"纯手动"升级为"自动候选 + 用户确认"；摘要从"单条滚动"升级为"分层可重建"；memory 对检索/回答的贡献可量化。**

### M1：自动长期记忆抽取——三层漏斗的第 2 层（P0）

设计依据：原 memory 架构设计的三层漏斗（显式标记 → 异步 LLM 分类 → 会话结束聚合），当前只有第 1 层。

- 新增 `longtermmemory/extraction` 子包（遵循现有 governance/recall 的分包纪律）：
  - 消息入库后异步触发（复用 S1 硬化后的 job worker 模式，独立队列）
  - LLM 分类输出候选记忆：`memory_type(preference|knowledge|feedback) + content + confidence`
  - 候选一律 `status=pending_confirmation`，**不直接 active**
  - 复用 governance 的 normalize / gate / 冲突检测，pending 记录不参与 recall
- 成本控制（必须有，否则每条消息一次 LLM 调用不可接受）：
  - 仅 user 消息、长度阈值过滤、规则预筛（含偏好/事实信号词才送 LLM）
  - 每会话每小时抽取上限
- 确认链路：
  - API：`POST /rag/v3/memories/:id/confirm`、`/reject`
  - chat SSE meta 增加"检测到可保存的记忆"提示事件，前端可做轻量确认卡片

验收标准：

- 构造 20 条含偏好/事实表达的对话样例，抽取召回率与误报率有基线数字
- pending 记忆不影响现有 recall 行为（回归保障）
- LLM 调用量有硬上限且可观测

### M2：会话结束聚合——三层漏斗的第 3 层（P1）

- 触发：会话 idle 超时或显式关闭（复用 maintenance 循环扫描，不新增调度设施）
- 聚合本会话的 pending 候选 + 高频实体，生成"会话级稳定记忆"建议（去重后合并置信度）
- 与 M1 共用确认链路

### M3：Agent `memory_save` capability（P1）

- 新增 `internal/app/agent/memory_save`：
  - `Kind=Tool`、`RiskLevelMedium`、**需要 approval**（approval/resume 机制已闭环，直接复用）
  - 输入走与 M1 相同的 governance + pending 路径，agent 提议、用户批准
- 这是 agent 第一个"写操作"能力，也是知识库助手"自动沉淀知识"的种子：先在 memory 上验证写操作 + 审批的完整模式，再推广到 document 写操作

验收标准：plan-execute / reactive 两个 pattern 下，`memory_save -> approval -> resume` 全链路服务级回归通过（参照 `service_pattern_planexecute_test.go` 的覆盖方式）。

### M4：分层摘要与可重建（P1）

S2 的生命周期字段是地基，这一步给它真正语义：

- 摘要分两层：
  - **段摘要（segment summary）**：每达到阈值生成一段，`covered_from/to` 精确覆盖，不可变
  - **主摘要（master summary）**：由段摘要滚动合成，进 prompt
- 重建能力：主摘要被 rejected 或漂移时，从段摘要重新合成，而不是从原始消息全量重算
- 摘要链查询 API（调试用）：按 conversation 列出段摘要 + 当前主摘要 + 质量状态

验收标准：构造 40+ 轮长会话样例，验证主摘要重建后关键实体保留；重建成本（LLM 调用数）相比全量重算下降可量化。

### M5：memory 召回质量评估闭环（P0，与 M1 并行）

对应 todo 清单"补 retrieve / answer / agent 三层评估闭环"在 memory 维度的落地：

- 扩充 `testdata/memory_fact_phase3_samples.json`：从 6 类样例扩到 30+ 条，覆盖口语化偏好、跨轮指代、kb/global 冲突、过期记忆
- `cmd/retrieve-eval` 输出按通道归因：`memory_fact` 通道的独立 `Hit@K / MRR` 与对融合结果的增量贡献
- 数据驱动调整 `memory_fact` 权重 0.9（当前是先验值）与 `explicit-recall.max-items: 6`
- 增加"负样本"评估：不该召回记忆的 query 是否被记忆污染

验收标准：能回答"长期记忆让哪类问题变好了多少、有没有让别的问题变差"。

### M6：记忆管理与透明度 UX（P1）

- API 补齐：记忆更新/修正（当前只有 list / expire / confirm）、按 `memory_type / scope / status` 过滤检索、provenance（`source_message_id` 关联跳转）
- chat 透明度：回答使用了哪些记忆，通过 SSE meta 下发 `memoryRefs`（trace 已有数据，缺前端下发口）
- "为什么助手知道这个"：recall 结果带 provenance，前端可展示来源消息

### M7：遗忘与衰减策略（P2）

- 置信度时间衰减：基于 `last_confirmed_at`，recall 命中自动续期（`touchLastUsed` 已有，补衰减读侧逻辑）
- 按 `memory_type` 区分过期策略：preference 长存、knowledge 衰减、feedback 短期
- maintenance 扩展：低置信度且长期未命中的记忆自动转 `archived`（不删除，可恢复）

### M8：memory 阶段并行化（P2，性能）

- 现状：`prepareChat` 串行执行 `runLongTermMemoryStage -> runSessionRecallStage -> runRetrieveStage`
- LTM recall 与 session recall 互不依赖，且都只依赖 rewrite 输出——两者可并行，再与 retrieve 重叠部分预算
- 前置：P1-1（检索通道并行化）落地后做，复用其顺序稳定性测试模式
- 验收：prepare 阶段 P95 时延下降可量化；trace 顺序与结果顺序保持稳定

---

## 3. 长期（3~6 个月+）：记忆与知识库统一，个性化，平台化

**阶段目标：memory 不再是 chat 的附属上下文，而是"用户知识资产"的一部分，与知识库助手的产品目标合流。**

### L1：Memory → Knowledge 晋升通道

原架构设计中"长期记忆复用 ingestion pipeline、落为 knowledge document（sourceType=memory）"的方向在此阶段落地：

- 高置信、多次确认的 knowledge 型记忆，可一键（或经 agent 提议 + 审批）晋升为知识库文档
- 复用现有 ingestion pipeline 与 chunk/vector 链路，不另建存储
- 晋升后原 memory 记录标记 `promoted`，retrieve 去重避免双通道重复命中
- 与知识库治理共用冲突检测（governance 的 conflict_detector 泛化）

价值：用户对话中沉淀的知识自动进入知识库——这是"自动管理知识库的助手"的核心闭环之一。

### L2：用户画像与个性化注入

- 从 preference 型记忆聚合出结构化用户 profile（语言偏好、回答风格、技术栈、关注主题）
- 注入点：
  - rewrite：术语偏好参与归一化
  - retrieve：关注主题影响通道权重/KB 路由
  - agent planner：默认偏好（如"回答用中文"）进入 planning 上下文而非每轮 recall
- profile 是派生数据，可随时从记忆重算，不引入新的真源

### L3：Agent 情景记忆（Episodic Memory）

- 把 agent 任务执行结果（诊断结论、检索策略效果、失败原因）沉淀为可召回的情景记忆
- 新 runtime 的 state snapshot / checkpoint 是现成的数据来源
- 用途：同类任务（如同一文档的再次诊断）优先召回历史结论，减少重复下钻轮次
- 这一项把"memory 服务于用户"扩展为"memory 服务于 agent 自身"

### L4：记忆质量飞轮

- `message_feedback` 与 memory 贡献关联：负反馈回答所引用的记忆降置信度，正反馈续期
- Answer 层评估（P1-6）接入 memory 归因：记忆参与的回答 vs 未参与的回答质量对比
- 数据闭环驱动 `memory_fact` 权重、recall 数量、衰减速率的自动调参

### L5：规模与合规

- `memory_item` 向量检索升级：参照 `20260608000000_add_chunk_vector_hnsw_index.sql` 为记忆 embedding 补 HNSW 索引（量级达到阈值再做）
- 每用户记忆配额与清理策略
- 隐私合规：记忆全量导出、一键清除（GDPR 式删除）、敏感内容入库前过滤
- 多租户/团队场景的记忆隔离与共享边界（与知识库权限模型对齐）

---

## 4. 阶段产出与衡量指标汇总

| 阶段 | 核心产出 | 关键指标 |
|------|----------|----------|
| 短期 | 异步摘要可默认开启；质量校验闭环；token 估算可校准；L2 缓存有数据结论 | 首包时延下降 %；估算偏差中位数 <15%；缓存命中率基线 |
| 中期 | 自动抽取 + 确认链路；agent memory_save；分层摘要；memory 评估集 | 抽取召回/误报基线；memory_fact 通道独立 Hit@K/MRR；prepare P95 下降 % |
| 长期 | 记忆晋升知识库；用户 profile；agent 情景记忆；质量飞轮 | 晋升知识的检索命中率；记忆参与回答的质量增量；重复任务轮次下降 |

## 5. 明确不做（本计划范围外）

- 不引入图数据库/知识图谱式记忆（当前规模下 ROI 不足，L2 的结构化 profile 已覆盖主要收益）
- 不把 memory 迁出 PostgreSQL（pgvector + HNSW 足够支撑当前量级）
- 不在短期引入精确 tokenizer 依赖（校准后的粗估已满足裁剪场景）
- 不做跨用户记忆共享（合规边界未定义前禁止）

## 6. 执行顺序建议

1. **第 1-2 周**：S1 + S2 + S3（摘要链路收口，这是当前在途工作，避免半成品挂起）
2. **第 2-3 周**：S4 + S5 + S6（契约与观测）
3. **第 3-4 周**：S7 + M5 评估集先行（先有尺子，再做 M1）
4. **第 2 个月**：M1 + M3（自动抽取 + agent 写能力，注意 M3 依赖双 runtime 收口完成）
5. **第 3 个月**：M2 + M4 + M6 + M7 + M8
6. **长期项**：L1 依赖知识库写操作与审批推广完成后启动；L2-L5 按产品节奏排期

一个依赖关系提醒：M3（agent memory_save）应排在 `structural_improvement_plan.md` P0-4（双 Agent Runtime 收口）之后，避免新写能力又要面对"接哪套 runtime"的问题。
