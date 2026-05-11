# Development Notes - 2026-05-10

## 今日主题

今天的工作围绕 `Agentic Chat` 联调支持展开，主线不是继续扩功能，而是把昨天落地的 `AgentLoop V1` 真正推进到“可联调、可观察、可解释”的状态。

今天的目标可以分成五块：

1. 阅读项目上下文文档，重新对齐当前阶段与目标
2. 按 `docs/agentic_chat_dev_plan.md` 第八节，逐项审查代码中已识别的问题是否仍存在
3. 先处理两项 P0 阻塞：
   - `agent_think` 前端可见
   - Observer hint 结构化传递
4. 为前后端联调准备一套可直接插入 PostgreSQL 的样例数据
5. 根据联调反馈，优化诊断回答中的“证据”表达，避免直接罗列数据库字段

## 本次改动

### 1. 重新对齐了项目上下文与 Agentic Chat 当前目标

阅读并总结了两份关键文档：

- `docs/project_progress_context.md`
- `docs/agentic_chat_dev_plan.md`

得到的当前判断：

- 项目阶段已经从“基础能力搭建”进入“主链路闭环、联调和质量收口”
- `ingestion` 已阶段性收口，短期不再是主攻方向
- 近期主线转为：
  - `RAG retrieve` 稳定性与解释性
  - `diagnose` 质量
  - `tool / trace / fallback` 消费闭环
- `AgentLoop V1` 已落地，但仍属于“结构化诊断 agent”，并非完整自主 Agent

这一步的价值主要是把“今天做什么”重新收敛回文档中定义的 V2 目标，而不是继续泛化扩展。

### 2. 按第八节风险清单做了一轮代码审查

围绕 `docs/agentic_chat_dev_plan.md` 第八节“风险与当前判断”，逐项回查代码，确认 5 个问题在代码库里的实际状态。

审查结论：

1. `agent_think` 事件仍在前端被丢弃
2. Observer hint 传递仍是自然语言 + 字符串反解析，机制脆弱
3. `LLMPlanner` 的增量规划提示仍缺少 few-shot
4. `LocalWorkflow` 仍作为 dead code 留在仓库里
5. `planCallsFromResults` 的回退覆盖仍有盲区

这轮审查的输出不是代码改动，而是把“文档里的问题仍然成立”确认成了可执行的修复项，方便后续集中处理。

### 3. 处理了两项 P0：前端消费 `agent_think` 与 hint 结构化

#### A. 前端接通 `agent_think`

改动点：

- `frontend/src/stores/chatStore.ts`
  - 注册 `onAgentThink`
  - 新增 `appendAgentThink(...)`
  - 将推理文本挂到当前 assistant 消息

- `frontend/src/types/index.ts`
  - `Message` 新增 `agentThinks?: string[]`

- `frontend/src/components/chat/MessageItem.tsx`
  - 新增 `Agent 推理` 卡片
  - 对当前 assistant 消息中的推理文本做可见展示

结果：

- Agent 多轮下钻时，用户现在可以看到“为什么继续调用下一轮 tool”
- 这一步补上了 `AgentLoop V1` 可解释性闭环里最明显的缺口

#### B. Observer hint 改为结构化传递

改动点：

- `internal/app/rag/tool/observer_rule.go`
  - 新增 `buildNextHint(...)`
  - `RuleObserver` 不再输出自然语言 `NextHint`
  - 改为输出机器可读格式：
    - `tool:<toolName>|param=value|param=value`

- `internal/app/rag/tool/agent_loop.go`
  - 移除原先依赖 `strings.Contains + extractArgValue` 的自然语言反解析逻辑
  - 新增 `parseNextHint(...)`
  - 精确解析结构化 hint 并恢复为 tool call

- `internal/app/rag/tool/planner/planner.go`
  - planner prompt 中将上一轮 hint 明确标注为“结构化建议”

结果：

- hint 传递从“人读文本 + 机器猜测”收敛为稳定的结构化协议
- 多轮规划链路的脆弱点明显下降

#### C. 补齐测试

新增/更新：

- `internal/app/rag/tool/agent_loop_test.go`
  - 验证第二轮 planner 收到结构化 `AgentState`
  - 验证 `planCallsFromHint(...)` 能正确解析结构化 hint

验证命令：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
```

结果：

- `internal/app/rag/tool` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）
- `internal/bootstrap/rag` PASS（无测试文件，包可正常编译）

### 4. 为前后端联调准备了样例问题与数据库测试数据

今天后半段进入前后端联调支持，先准备了一批针对当前能力边界的联调问题，覆盖：

- `doc_fail_01 / doc_run_01 / doc_ok_01`
- `task_fail_01 / task_run_01 / task_ok_01`
- `trace_bad_01 / trace_tool_01`

随后又准备了一份可直接插入 PostgreSQL 的样例 SQL：

- `test.sql`

样例覆盖场景：

1. 文档失败并可继续下钻到失败节点
2. 任务运行中并可继续下钻到运行节点
3. 文档成功且不应误下钻
4. trace 检索差
5. tool workflow degraded

准备样例过程中还顺手确认了几件 schema/联动细节：

- `chunk_log.id` 与 `ingestion_task.id` 要保持 task-scoped 对齐
- `t_rag_trace_run` / `t_rag_trace_node` 需要显式写入 `create_time / update_time`

最终也直接帮忙修正了 `test.sql`，使其可稳定导入当前 PostgreSQL。

### 5. 根据联调反馈，优化了诊断回答中的“证据”表达

联调反馈里暴露出的一个问题是：

- `Agent 推理` 卡片已经可见
- 但最终回答中的“证据”部分仍然过于接近数据库字段 dump
- 用户看到的是：
  - `task.status=failed`
  - `failedNode=indexer`
  - `taskNodes.failed=1`
  - 这类工程内部表达，不够友好

为此做了一轮后端 payload 优化。

改动点：

- `internal/app/rag/tool/builtin/diagnose_helpers.go`
  - `buildDiagnosisPayload(...)` 不再直接把原始 `evidence` 同时塞进 `facts`
  - 保留：
    - `evidence`
    - `rawEvidence`
  - 新增：
    - `humanizeDiagnosisFacts(...)`
  - 根据 `diagnosisScope` 将原始字段式 evidence 改写成更用户化的 `facts`

示例效果：

- 原始：
  - `task.status=failed`
  - `failedNode=indexer`
  - `failedNode.error=connection refused: vector store unavailable`

- 用户化后：
  - `任务当前状态为失败。`
  - `失败节点是 indexer，节点报错为 "connection refused: vector store unavailable"。`
  - `共有 4 个节点，3 个成功，1 个失败，说明流程已经推进到 indexer 阶段。`

改动结果：

- 机器和调试仍能读到 `rawEvidence`
- 模型回答时优先使用更自然的 `facts`
- 不需要前端做额外改造

#### 对应测试

更新：

- `internal/app/rag/tool/builtin/query_tools_test.go`
  - 断言 `facts` 已经变成用户化表达
  - 断言 `rawEvidence` 仍保留原始字段

- `internal/app/rag/tool/tool_test.go`
  - 同步 `BuildAnswerGuidance(...)` 的测试输入

验证命令：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/builtin -count=1
```

结果：

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/builtin` PASS

## 联调观察

今天实际联调里已经能看到两个比较明确的变化：

1. 有些问题已经可以稳定出现 `Agent 推理` 卡片
2. 回答中的“证据”比之前更接近面向用户的解释

同时也确认了一个当前边界：

- 需要多轮 Agent 推理的问题并不多

这不是新 bug，更接近 `AgentLoop V1` 的能力边界：

- 当前主要适用于：
  - `documentId` 已知的 ingestion 排障
  - `taskId` 已知的 task / node 排障
  - `traceId` 已知的 retrieval 诊断
- 对一轮就够的信息查询，系统不会为了“看起来像 Agent”而强行空转

所以接下来真正要提升的，不是“让所有问题都显示 Agent 卡片”，而是：

- 扩大多轮下钻的适用面
- 提升 planner / observer 对“还差什么证据”的判断能力

## 今日验证

今天新增改动已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/builtin -count=1
```

结果：

- `internal/app/rag/tool` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` PASS（无测试文件，包可正常编译）
- `internal/bootstrap/rag` PASS（无测试文件，包可正常编译）
- `internal/app/rag/tool/builtin` PASS

## 晚间联调补充

### 1. `doc_fail_01` 暴露出 observer 提前终止问题

晚间继续联调 `doc_fail_01` 时，发现一个关键问题：

- `Agent 推理` 已经出现
- 但标准失败样例仍经常停留在：
  - 文档失败
  - task 失败
  - chunk log 报 `indexer failed after retries`
- 最终回答却说“未捕获到具体失败节点”

结合运行日志定位到根因：

- `observeDocumentDiagnosis(...)` 在拿到 `latestLogError` 后就直接 `Done=true`
- 系统把 task/chunk log 级摘要错误误当成了“已经足够具体的错误”
- 因而没有继续下钻到 `ingestion_task_query / ingestion_task_node_query`

这说明白天的联调虽然已经打通了多轮框架，但在标准失败样例上，终止条件仍偏保守，导致 `document -> task -> node` 没有稳定走透。

### 2. 修复了 `document diagnose` 的提前终止逻辑

改动点：

- `internal/app/rag/tool/observer_rule.go`
  - 只有拿到真正的 `latestNodeError` 才允许直接结束
  - 如果只有 `latestTaskId + latestLogError`，继续下钻到 `ingestion_task_query(includeNodes=true)`
  - `ingestion_task_query` 若已暴露 failed/running node，则继续走 `ingestion_task_node_query`

结果：

- `doc_fail_01` 开始稳定走通：
  - `document_ingestion_diagnose`
  - `ingestion_task_query`
  - `ingestion_task_node_query`
- `Agent 推理` 卡片也和这条链路一致：
  - 先说明只有 task/chunk log 级摘要
  - 再说明 task query 已经暴露出 failed node，需要继续查 node detail

### 3. 修复了 Agent trace 节点落库失败

联调日志同时暴露了一个独立问题：

- PostgreSQL 报错：`value too long for type character varying(16)`
- 出错字段来自：
  - `node_type='agent_observation'`
  - `node_id='agent_observation_01'`

改动点：

- `internal/app/rag/service/chat_tracer.go`
  - 将 trace 节点命名压缩为数据库安全长度：
    - `agent_round` -> `agt_round`
    - `agent_observation` -> `agt_obs`
    - `agent_round_01` -> `agt_round_01`
    - `agent_observation_01` -> `agt_obs_01`

结果：

- `agent_round / agent_observation` 节点现在可以正常落库
- 联调时可以继续依赖 trace 复盘 observe 行为

### 4. 修复了“后面已拿到 node detail，但最终回答仍沿用较弱 diagnose 结论”的问题

在 `doc_fail_01` 的后续联调里，出现了新的现象：

- `Agent 推理` 和 `工具调用` 已经显示系统确实走到了 node detail
- 回答中的证据区也已经写出：
  - 第 4 个节点 `indexer`
  - `status=failed`
  - `connection refused: vector store unavailable`
- 但结论里仍然写“未捕获到具体失败节点”

这说明问题已经不再是“有没有查到 node”，而是“最终回答有没有优先采用更深一层证据”。

改动点：

- `internal/app/rag/tool/answer_guidance.go`
  - `BuildAnswerGuidance(...)` 不再简单取前面第一条 diagnose
  - 改为优先选取最新 diagnose，再扫描后续更深一层的 `ingestion_task_node_query`
  - 若拿到同一 `taskId` 的节点级结果，则升级：
    - `conclusion`
    - `confidence`
    - `facts`
    - `inferences`

结果：

- `doc_fail_01` 最终已经可以稳定收敛到：
  - 失败节点是 `indexer`
  - 具体错误是 `connection refused: vector store unavailable`
  - 置信度为 `high`

### 5. 最终联调判断

到今天收尾时，`doc_fail_01` 已经达到当前阶段的验收预期：

- Agent 推理可见
- 多轮下钻稳定
- trace 可落库
- 最终回答与样例数据一致

这说明 `AgentLoop V1` 已经从“可以联调”推进到了“标准失败样例可稳定通过”的阶段。

接下来更合理的验证顺序会是：

1. `doc_run_01`：运行中场景
2. `task_run_01`：任务进度/卡点场景
3. `trace_bad_01`：retrieval 诊断场景

## 当前判断

今天的工作把 `AgentLoop V1` 从“后端已经落地”推进到了“可以真实联调”的阶段：

- `agent_think` 前端可见
- hint 结构化传递
- 联调样例数据齐备
- 诊断回答中的证据表达更像面向用户的说明

晚间的补充修复又进一步把它推进到：

- `doc_fail_01` 标准失败样例可稳定走通 `document -> task -> node`
- Agent trace 节点可正常落库
- 最终回答能够优先采用节点级失败证据

但也确认了 V1 的一个现实边界：

- 当前仍更像“结构化诊断 agent”
- 不是更开放场景下的完整自主 Agent

因此短期最合理的推进方向会是：

1. 把 `doc_fail_01` 的稳定模式扩展到 `doc_run_01 / task_run_01`
2. 继续打磨 planner 的增量规划质量
3. 继续提升 diagnose 输出的可读性和建议质量

## 下一步建议

### P0

- 扩大 `document_query / chunk_log_query / task_query` 的多轮下钻覆盖面
- 让更多“为什么失败 / 卡在哪 / 为什么慢”的高频问题进入多轮推理

### P1

- 在 planner prompt 中加入 few-shot，提升增量规划质量
- 补 `planCallsFromResults` 的更多回退链路
- 清理 `local_workflow.go` dead code

### P2

- 继续优化 diagnose 输出
  - 让 `建议` 更面向用户场景
  - 减少工程内部术语直接暴露
- 评估是否将部分前端“证据”展示也做成更结构化卡片

## LLMObserver 补充记录

在确认 `doc_fail_01` 的规则路径已经稳定之后，继续推进了 `LLMObserver` 落地：

- `AgentLoop` 默认 observer 切到 `LLMObserver`
- `RuleObserver` 退为 fallback / guardrail
- `AgentState / ObserveInput / ObserveResult` 全部结构化

但切换后，`doc_fail_01` 很快暴露出新的回归：

- 回答重新退回 `medium`
- 结论再次变成“未能捕获具体失败节点”
- PostgreSQL 日志里出现了错误查询：
  - `task_id = 'task_fail_01' AND node_id = 'node_0'`

这说明问题已经不再是“会不会继续下钻”，而是“LLM 产出的下一步参数是否真的来自已有证据”。

根因分析：

1. `ingestion_task_query` 当时给 LLM 的主要还是弱摘要，例如 `nodes=4`
2. `taskNodeSummary` 虽然存在于 `result.Data`，但没有显式展开给 `LLMObserver / LLMPlanner`
3. 模型不知道真实失败节点是 `indexer`，于是幻觉出了并不存在的 `node_0`
4. 系统当时只校验了 hint/call 的格式，没有校验 `nodeId` 是否来自现有证据
5. 错误参数被真正执行后，node detail 没拿到，最终回答自然退回弱结论

对应修复：

- 新增统一的 LLM 结果摘要逻辑
  - 显式暴露：
    - `latestTaskId`
    - `latestNodeId`
    - `latestNodeError`
    - `taskNodeSummary`
    - `nodes`
- `ingestion_task_query` 的 summary 不再只有 `nodes=4`
  - 还会补：
    - `interestingNodes=[indexer(status=failed,type=indexer)]`
- 对 `LLMObserver` 的 `nextHint` 做证据一致性校验
- 对 `LLMPlanner` 真正返回的 tool call 也做相同校验
- 如果参数不来自：
  - 用户问题
  - 上一轮结构化 hint
  - 已有 tool result data
  则直接拒绝，并退回规则路径

修复后的结果：

- `node_0` 这类幻觉参数不会再真正落到执行器
- `doc_fail_01` 再次稳定收敛到：
  - 失败节点是 `indexer`
  - 具体错误是 `connection refused: vector store unavailable`
  - `high` 置信度

这轮修复带来的一个重要判断是：

- `LLMObserver` 上线后，最关键的不是“让它决定继续/停止”
- 而是“保证它产出的参数必须可追溯到已有证据”
- 所以 prompt 约束和执行前 evidence-based 校验必须一起存在

补充验证命令：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/tool/builtin -count=1
```

补充验证结果：

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/app/rag/tool/builtin` PASS

## 深夜收口补充

在完成 `doc_fail_01`、`LLMObserver` 主链稳定之后，又继续做了一轮偏“工程收口”的改动，目标不是再扩能力，而是把今天暴露出来的结构性问题真正压下去，避免后面继续在脆弱边界上反复返工。

### 1. 给 `LLMPlanner` 补齐了 `rewrite / retrieve` 上下文

之前虽然 `PlanInput` 已经带了：

- `RewriteResult`
- `RetrieveResult`

但 `planner.go` 的 user prompt 里并没有真正把这些上下文喂给 Planner。

这会导致两个问题：

1. Planner 看不到检索阶段已经拿到什么证据
2. 即使 retrieve 已经足够，Planner 仍可能继续规划不必要的 diagnose/tool 下钻

这次做法是：

- 将 Observer 里已有的改写/检索摘要逻辑提升为共享函数
  - `SummarizeRewriteResultForLLM(...)`
  - `SummarizeRetrieveResultForLLM(...)`
- `LLMPlanner.buildUserPrompt(...)` 也接入同样的摘要

结果：

- Planner 和 Observer 看到的是同一套 rewrite/retrieve 上下文
- 多轮规划不再“检索盲”
- 后续若继续打磨 prompt，只需要维护一套摘要逻辑

### 2. 修掉了 `Hypothesis` 错误回退到 `Reasoning` 的语义混淆

今天审查 `observer_llm_improvements.md` 时确认，`LLMObserver` 里还有一处危险但很隐蔽的问题：

- 当 LLM 没填 `state.hypothesis` 时
- 代码会把 `reasoning` 回填进 `hypothesis`

但两者语义不同：

- `Reasoning` 是“下一步为什么这么做”
- `Hypothesis` 是“当前状态判断是什么”

这会让 AgentState 携带错误类型的信息，后续轮次的 Planner / Observer 会基于“动作说明”而不是“状态假设”继续决策。

这次修复后：

- 若 `hypothesis` 为空，只继承 `PreviousState.Hypothesis`
- 不再把 `Reasoning` 混入 `Hypothesis`

结果：

- AgentState 语义边界更清晰
- 规划链路里的状态不会被行为描述污染

### 3. 把 `result_summary` 从白名单模式改成了黑名单模式

之前 `SummarizeResultDataForLLM(...)` 的做法是：

- 只摘固定字段
- 如果 tool 新增了关键字段但没人补到白名单，LLM 就完全看不到

这次改成：

1. 先保留已有高频关键字段的优先输出顺序
2. 再自动遍历其余所有字段
3. 只排除明确噪音字段，例如：
   - `rawBody`
   - `fullText`
   - `rawText`
   - `rawContent`
   - `originalText`

结果：

- 新增 tool 字段不容易被静默丢掉
- Observer / Planner 能拿到更完整的证据
- 同时不会把大段原文、原始 payload 直接塞进 LLM

### 4. 清理了 `LocalWorkflow` dead code，并把通用 helper 抽离

原本仓库里还保留着旧的单次 `LocalWorkflow`：

- runtime 已经不再使用
- 但它下面还挂着一组 `AgentLoop` 仍在复用的 helper

这次先尝试直接删除时，马上暴露出真实耦合：

- `AgentLoop`
- `LLMObserver`

还在依赖其中的：

- `callKey(...)`
- `firstMatchedID(...)`
- `containsAny(...)`
- 相关 ID pattern

最终处理方式是：

- 删除
  - `internal/app/rag/tool/local_workflow.go`
  - `internal/app/rag/tool/local_workflow_test.go`
- 新增 `internal/app/rag/tool/workflow_helpers.go`
  - 把仍然属于“当前主链路公共依赖”的 helper 独立出来

结果：

- 旧 workflow 真正退场
- 公共逻辑不再挂在废弃实现下
- `AgentLoop` 依赖关系更干净

### 5. 完成了 `nextHint` -> `nextHintCalls` 的结构化迁移

这是今晚最大的一块收口。

之前虽然 hint 已经从自然语言变成了：

- `tool:tool_name|param=value`

这种“可机器解析”的字符串协议，但本质上仍然是扁平字符串，问题包括：

1. 参数类型丢失
2. 仍要走序列化 / 反序列化边界
3. 很难自然扩展到多个下一步 call

这次改动分两步做：

#### A. 先做兼容式升级

新增：

```go
type HintCall struct {
    Name      string
    Arguments map[string]any
}
```

并引入：

- `AgentState.NextHintCalls []HintCall`
- `ObserveResult.NextHintCalls []HintCall`
- `RoundSummary.NextHintCalls []HintCall`

同时保留旧的：

- `NextHint string`

但只作为兼容和可读性输出存在，由 `NextHintCalls` 自动派生。

#### B. 再把主逻辑切到结构化 hint

后续继续完成了几件事：

- `RuleObserver` 内部改为直接产出 `HintCall`
- `planCallsFromHintCalls(...)` 成为主消费路径
- `validateHintAgainstEvidence(...)`
- `validateCallAgainstEvidence(...)`
- `collectEvidenceIDs(...)`

都改为围绕 `[]HintCall` 工作

- `LLMObserver` prompt 示例、system rule、解析结果也切到 `nextHintCalls`
- `planner` 的 few-shot 和规则文案同步切到 `nextHintCalls`
- trace 里补充记录 `nextHintCalls`

结果：

- 内部主语义已经不再是字符串 `nextHint`
- 旧字符串现在主要只承担：
  - 兼容旧输入
  - trace/debug 可读输出

### 6. 对当前状态的判断

这一轮收口之后，`AgentLoop / LLMObserver / LLMPlanner` 的状态比白天结束时更稳：

1. Planner 不再对检索阶段“失明”
2. Observer 的状态语义更干净
3. result summary 更不容易静默丢证据
4. 旧 `LocalWorkflow` 已清理掉
5. hint 机制已经从“结构化字符串”推进到“结构化对象”

这意味着当前系统已经不只是“标准失败样例能跑通”，而是：

- 主链路的几个脆弱边界都开始系统性收口
- 后续继续扩 `doc_run_01 / task_run_01 / trace_bad_01` 时，返工概率会低很多

### 本轮补充验证

本轮新增修改已通过：

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/service ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/service ./internal/bootstrap/rag -count=1
```

结果：

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/app/rag/service` PASS
- `internal/bootstrap/rag` PASS（无测试文件，包可正常编译）

## 2026-05-10 晚间联调补充记录

### 1. AgentLoop 的执行参数已配置化

今晚把原本写死在代码里的执行参数接到了配置层：

- `rag.agent.max-iterations`
- `rag.agent.parallel-tool-calls.enabled`
- `rag.agent.parallel-tool-calls.max-concurrency`

当前默认联调配置已经切到：

- `max-iterations: 3`
- `parallel-tool-calls.enabled: true`
- `parallel-tool-calls.max-concurrency: 3`

这意味着后续联调时，不需要再改代码就可以直接比较串行 / 并发模式，也能评估不同轮次上限对多轮诊断质量的影响。

### 2. 并发执行已经补上安全边界和可观测字段

并发不是简单把 tool call 丢到 goroutine 里，而是先把顺序和观测边界收紧：

- `OnToolStart` 仍按规划顺序发出
- `OnToolResult` 仍按原顺序汇总后发出
- `RoundSummary`、`WorkflowResult.Calls`、trace 节点顺序保持稳定
- 关闭配置后可完全回退到串行逻辑

同时补了 round 级观测字段：

- `executionMode`
- `toolCallCount`
- `wallClockDurationMs`
- `totalToolDurationMs`

这些字段现在会进入 `RoundSummary`，也会落到 trace 的 `agt_round.extraData`。

### 3. 并发表现测试已经跑通

本轮专门补了并发正确性和性能测试，重点验证：

1. 并发下 `tool_start / tool_result` 事件顺序仍稳定
2. 同轮无依赖调用确实发生并发执行
3. 并发模式的真实墙钟时间低于串行模式

受控测试样例结果为：

```text
serial=80.8148ms (round wall=80ms totalTool=80ms)
parallel=40.381ms (round wall=40ms totalTool=80ms)
```

结论是：当前实现下，并发执行可以正确运行，而且用户真实等待时间约下降一半。

### 4. planner 已增加“哪些 call 不该并行”的约束

为了避免开启并发后，planner 在同一轮里把同一实体上的整条依赖链一起规划出来，今晚补了一轮明确约束：

- 只有彼此独立的 calls 才允许同轮并行
- 同一实体上的增量下钻链路必须跨轮串行推进
- 明确禁止在同一轮里混排：
  - `document_query + document_ingestion_diagnose`
  - `ingestion_task_query + ingestion_task_node_query`
  - `task_ingestion_diagnose + ingestion_task_node_query`

### 5. `doc_run_01` 联调暴露出两个工程性根因

晚间前后端联调里，`doc_run_01` 出现了“有时查不到、有时说 failed、有时又说 still running”的跳变。追日志后确认主要有两个根因：

#### A. 普通关键字被误识别成 ID

日志中出现了错误查询：

```sql
SELECT * FROM "t_knowledge_document" WHERE id = 'document'
```

这说明系统把问题里的普通词 `document` 误识别成了 `documentId`，而不是 `doc_run_01`。本轮已经收紧 ID pattern，只接受结构化 ID，例如：

- `doc_run_01`
- `task_run_01`
- `trace_bad_01`

普通的 `document / task / trace` 不再会被当成实体 ID。

#### B. 同一轮对同一实体规划过深

另一类问题来自 base rules 过于激进，会在同一轮里同时规划：

- `document_query`
- `document_ingestion_diagnose`
- 甚至更深一层的 node 级查询

这会导致浅层结果覆盖深层运行态判断，最终让回答在“查不到 / failed / still running”之间跳变。

现在已经调整为：同一实体同一轮只做最浅一层必要 lookup，运行态 / 进度 / 当前节点类问题优先走 `document_ingestion_diagnose`，下一轮再继续下钻到 `ingestion_task_query`，必要时再继续到 `ingestion_task_node_query`。

### 6. 对 `doc_run_01` 的当前判断

到今晚这一步，`doc_run_01` 暴露的问题已经不再是“主链完全跑不通”，而是更聚焦的回答优先级和冲突归一问题：

1. 模糊运行态问法仍可能被浅层 `document.status` 带偏
2. 关键字误识别为 ID 会直接把查询打歪
3. 同轮过深规划会让浅层和深层证据互相覆盖

其中第 2、3 点已经完成第一轮修复，第 1 点的入口规则也做了初步收紧。下一步需要继续在回答阶段明确：

- 当 `document.status=failed` 但 `task/node` 仍为 `running` 时，优先承认状态不一致
- 默认以更接近实时执行链路的 `task/node` 状态为主
- 显式解释“这是阶段性状态冲突，不是最终失败结论”

### 本轮补充验证

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/framework/config ./internal/app/rag/tool ./internal/app/rag/service ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool -run TestAgentLoopParallelToolCallsImproveWallClockDuration -v -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/service -count=1
```

结果：

- `internal/framework/config` PASS
- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/app/rag/service` PASS
- `internal/bootstrap/rag` PASS
- `TestAgentLoopParallelToolCallsImproveWallClockDuration` PASS
