# Agentic Chat 联调问题总结 - 2026-05-10

## 背景

今天围绕 `Agentic Chat` 的联调样例，重点排查了 `doc_fail_01` 在多轮诊断链路中的稳定性和可解释性问题。

联调目标不是“让回答看起来像 Agent”，而是让标准失败样例稳定完成：

1. `document -> task -> node` 的多轮下钻
2. 前端可见的 `agent_think`
3. 最终回答优先采用更深一层的节点级证据
4. trace 可正常落库，便于复盘每轮行为

---

## 发现的问题

### 1. `doc_fail_01` 首轮过早结束，没有继续下钻到 task/node

现象：

- 前几轮联调中，`doc_fail_01 为什么导入失败了？` 的回答停留在：
  - 文档失败
  - 最新 task 失败
  - chunk log 提示 `indexer failed after retries`
- 但回答一直说“未捕获到具体失败节点”，没有稳定下钻到 `indexer` 节点详情。

根因：

- `observeDocumentDiagnosis(...)` 在拿到 `latestLogError` 后就直接 `Done=true`
- 也就是说，系统把 task/chunk log 级别的错误摘要当成了“已经足够具体的错误”
- 对 `doc_fail_01` 这种标准样例来说，这个终止条件过早，因为样例里其实还存在更深一层的节点级证据

影响：

- Agent 多轮链路不稳定
- 标准失败样例无法稳定打透到节点级根因
- 回答容易停留在 `medium` 置信度

### 2. 相同问题多次提问，最终回答不稳定

现象：

- 连续三次问 `doc_fail_01 为什么导入失败了？`
- 前两次回答相同，仍说“未捕获到具体失败节点”
- 第三次开始说“最可能的原因是索引阶段”

根因：

- 回答阶段优先消费了前一轮较弱的 diagnose 结论
- 即使后续 tool 调用已经拿到了更深一层的 `task / node` 证据，最终 guidance 仍可能被前一轮的 diagnose 结果覆盖

影响：

- 同一标准样例存在回答漂移
- “事实”和“推断”的边界不稳定

### 3. Agent trace 节点落库失败

现象：

- PostgreSQL 日志报错：`value too long for type character varying(16)`
- 出错节点包括：
  - `node_type='agent_observation'`
  - `node_id='agent_observation_01'`

根因：

- `t_rag_trace_node.node_type` 只有 `varchar(16)`
- 新增的 Agent trace 节点命名超过字段长度

影响：

- trace 观测链路残缺
- 联调期间难以准确复盘每轮 `observe` 是如何决策的

### 4. `BuildAnswerGuidance(...)` 没有优先采用更深一层证据

现象：

- 在 `document_ingestion_diagnose` 之后，虽然已经拿到了 `ingestion_task_node_query`
- 最终回答仍沿用前面 diagnose 里的较弱结论，例如：
  - “未捕获到具体失败节点”
- 同时证据区却已经展示了明确的节点失败信息，造成结论和证据冲突

根因：

- `BuildAnswerGuidance(...)` 之前是“从前往后遇到第一条 diagnose 就直接返回”
- 这会让早一轮的诊断结论抢占最终回答控制权

影响：

- 最终回答内部不自洽
- 节点级证据虽然查到了，但没有真正体现在最终回答里

---

## 解决方式

### 1. 收紧 `document diagnose` 的终止条件

策略调整：

- 只有拿到真正的节点级错误 `latestNodeError` 时，才允许直接结束
- 如果只有 `latestTaskId + latestLogError`，继续下钻到 `ingestion_task_query(includeNodes=true)`
- 如果 `ingestion_task_query` 已经暴露出 `failed/running node`，继续下钻到 `ingestion_task_node_query`

结果：

- `doc_fail_01` 不再因为一句 `indexer failed after retries` 就过早结束
- 多轮链路变成稳定的：
  - `document_ingestion_diagnose`
  - `ingestion_task_query`
  - `ingestion_task_node_query`

### 2. 让结构化 hint 保留 `includeNodes=true`

策略调整：

- 修正 `buildNextHint(...)`
- 让布尔参数 `includeNodes=true` 可以稳定序列化进：
  - `tool:ingestion_task_query|taskId=xxx|includeNodes=true`

结果：

- 下一轮 planner / rule fallback 获取到的上下文更完整
- 减少依赖隐式默认行为

### 3. 压缩 Agent trace 节点命名，避免数据库字段越界

策略调整：

- 将 trace 节点命名改为数据库安全长度：
  - `agent_round` -> `agt_round`
  - `agent_observation` -> `agt_obs`
  - `agent_round_01` -> `agt_round_01`
  - `agent_observation_01` -> `agt_obs_01`

结果：

- `t_rag_trace_node` 不再因为 `varchar(16)` 限制落库失败
- 联调时可以正常观察 round / observation 节点

### 4. 让最终回答优先采用“最新、最深”的节点级证据

策略调整：

- `BuildAnswerGuidance(...)` 改为：
  - 先选取最新 diagnose 结果
  - 再扫描后续更深一层的 `ingestion_task_node_query`
  - 如果拿到同一 `taskId` 的节点级结果，则用节点级证据升级：
    - `conclusion`
    - `confidence`
    - `facts`
    - `inferences`

结果：

- 最终回答不再停留在“未捕获到具体失败节点”
- `doc_fail_01` 能稳定收敛到：
  - 失败节点是 `indexer`
  - 具体错误是 `connection refused: vector store unavailable`
  - 置信度为 `high`

---

## 具体改动位置

### 后端规则与规划

- `internal/app/rag/tool/observer_rule.go`
  - 调整 `observeDocumentDiagnosis(...)` 的提前终止逻辑
  - 调整 `observeTaskQuery(...)` 的下钻逻辑
  - 补 `readHintArg(...)`，保证布尔参数可进入结构化 hint

- `internal/app/rag/tool/agent_loop.go`
  - 补 `planCallsFromResults(...)` 的更多回退链路
  - 增加从 `taskNodeSummary` 识别 failed/running node 的逻辑

- `internal/app/rag/tool/planner/planner.go`
  - 增加 few-shot 风格的增量规划约束
  - 强化“优先使用 hint / 不重复调用 / 证据足够时返回空 tools”

### 回答引导与诊断输出

- `internal/app/rag/tool/answer_guidance.go`
  - 重写 `BuildAnswerGuidance(...)`
  - 引入“更深一层证据优先”的回答引导策略

- `internal/app/rag/tool/builtin/diagnose_helpers.go`
  - 优化 `inferences / riskHints`
  - 让输出更接近用户表达，而非英文或工程内部术语

### trace / 可观测性

- `internal/app/rag/service/chat_tracer.go`
  - 压缩 `agent_round / agent_observation` 的 `node_id / node_type`
  - 避免数据库字段长度超限

### 测试

- `internal/app/rag/tool/agent_loop_test.go`
  - 增加：
    - `document_query -> diagnose` 回退
    - `chunk_log -> task_query` 回退
    - `task_query -> node_query` 回退
    - “只有 log 级错误时继续下钻”
    - “已有 node 级错误时允许结束”

- `internal/app/rag/tool/planner/planner_test.go`
  - 增加 few-shot 与 structured hint 相关测试

- `internal/app/rag/tool/tool_test.go`
  - 增加“更深节点级证据优先”测试

- `internal/app/rag/service/rag_chat_service_test.go`
  - 增加 Agent trace 节点命名安全测试

---

## 联调结果

经过上述修复后，`doc_fail_01` 已达到预期目标：

- 前端可见 2 条 `Agent 推理`
- 多轮链路稳定走到 node detail
- 最终回答已经能够稳定落到：
  - `indexer` 节点失败
  - `connection refused: vector store unavailable`
  - `high` 置信度

当前判断：

- `doc_fail_01` 联调通过
- 说明 `document -> task -> node` 这条高频排障链路已经具备可用性
- 下一步更适合转向：
  - `doc_run_01`
  - `task_run_01`
  - `trace_bad_01`
  继续验证运行中和 retrieval 诊断场景

---

## 新一轮回归问题

### 5. 切到 `LLMObserver` 后，`doc_fail_01` 又出现了错误下钻

现象：

- 前端回答重新退化为：
  - 文档导入失败
  - task 失败
  - 但“未能捕获具体失败节点”
  - 置信度停留在 `medium`
- 日志里出现了错误查询：
  - `task_id = 'task_fail_01' AND node_id = 'node_0'`
- 但样例数据里真实失败节点明明是：
  - `indexer`
  - `connection refused: vector store unavailable`

根因：

1. `LLMObserver` / `LLMPlanner` prompt 里只看到了 task 的弱摘要信息，例如 `nodes=4`
2. `ingestion_task_query` 虽然把真实节点列表放进了 `taskNodeSummary`，但没有把它显式展开给 LLM
3. LLM 在不知道真实节点名的情况下，幻觉出了并不存在的 `node_0`
4. 系统当时只校验 `nextHint` 格式是否合法，没有校验 `nodeId` 是否真的来自已有证据
5. 结果错误的 `node_0` 被真正执行，`ingestion_task_node_query` 查询失败
6. 一旦 node detail 没拿到，最终回答就只能退回到 `document/task` 级弱证据，因此停在 `medium`

影响：

- `doc_fail_01` 从“已稳定通过”退回到“回答不合格”
- `LLMObserver` 主路径出现参数幻觉风险
- 多轮链路虽然继续执行了，但执行到了错误的节点目标

---

## 新一轮修复

### 5. 给 `LLMObserver / LLMPlanner` 补真实节点证据

策略调整：

- 新增统一的 LLM 结果摘要逻辑
- 不再只把 `result.Summary` 喂给模型
- 额外显式展开：
  - `latestTaskId`
  - `latestNodeId`
  - `latestNodeError`
  - `taskNodeSummary`
  - `nodes`

结果：

- LLM 在 `task_query` 场景下可以直接看到：
  - 哪个节点是 `failed/running`
  - 真实节点名是否为 `indexer`
- 不再需要依赖模型“猜”节点名

### 6. 强校验 `nextHint` 与 planner call 的证据一致性

策略调整：

- 对 `LLMObserver` 产出的 `nextHint` 做证据校验：
  - `documentId / taskId / nodeId / traceId`
  - 必须来自：
    - 用户问题
    - 上一轮结构化 hint
    - 已有 tool result data
- 对 `LLMPlanner` 真正规划出的 tool call 也做同样校验
- 如果像 `node_0` 这种参数不在已有证据里：
  - 直接拒绝
  - 自动退回 `RuleObserver / 规则回退链路`

结果：

- 即使 LLM 仍然偶发幻觉参数，也不会真正执行到数据库查询层
- `node_0` 这类错误节点名会在落地前被拦住
- 系统会自动回退到规则路径，重新从 `taskNodeSummary` 中识别出真实 `indexer`

### 7. 强化 `ingestion_task_query` 的摘要表达

策略调整：

- `ingestion_task_query` 的 summary 不再只有：
  - `nodes=4`
- 现在还会补：
  - `interestingNodes=[indexer(status=failed,type=indexer)]`
  - 或运行中节点摘要

结果：

- 就算只读 summary，模型也能拿到关键节点名
- `task -> node` 下钻的可解释性和稳定性都更高

---

## 最终结果补充

经过这一轮修复后，`doc_fail_01` 重新恢复到预期状态：

- `LLMObserver` 成为主 observer，但不会再把幻觉参数直接落地执行
- `RuleObserver` 保留为兜底 observer
- `doc_fail_01` 可以再次稳定收敛到：
  - 失败节点是 `indexer`
  - 具体错误是 `connection refused: vector store unavailable`
  - `high` 置信度

这轮修复也说明了一个重要结论：

- `LLMObserver` 上线后，真正的风险不只是“会不会继续下钻”
- 更关键的是“LLM 产出的下一步参数是否真的来自已有证据”
- 因此 observer/planner 的提示词约束必须和执行前证据校验一起存在，缺一不可

---

## 验证命令

```powershell
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool/... -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/service ./internal/adapter/http/rag ./internal/bootstrap/rag -count=1
$env:GOCACHE='D:\goagent\.gocache-agent'; go test ./internal/app/rag/tool ./internal/app/rag/tool/planner ./internal/app/rag/tool/builtin -count=1
```

验证结果：

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/builtin` PASS
- `internal/app/rag/tool/planner` PASS
- `internal/app/rag/service` PASS
- `internal/adapter/http/rag` 可正常编译
- `internal/bootstrap/rag` 可正常编译
