# Agent 自主化演进路线

版本：v1.0
日期：2026-05-10

---

## 一、当前痛点

AgentLoop V1 已经落地了 Plan → Act → Observe 循环，`doc_fail_01` 可以稳定走通 `document → task → node` 三级下钻。但当前架构的自主性被三个硬编码决策点锁死。

### 1.1 Observer 才是真正的决策者，Planner 只是它的执行工具

```
Question → [LLM Planner] → [Executor] → [RuleObserver] → 下一轮/终止
                ↑                              ↑
           失败时回退到              5 个 observeXxx() 函数
           60 行正则规则             硬编码 Done/NextHint
```

`RuleObserver` 通过硬编码规则决定了每一轮是否继续、下一轮该做什么。LLM Planner 即便做出了合理决策，也可能被 Observer 否决。对于 Observer 不认识的工具结果，默认行为是直接终止（`Done: true`）。这颠倒了自主 Agent 应有的关系——应该是 Planner 主导决策，Observer 验证安全性。

### 1.2 PlanInput 上下文扁平，Planner 看不到全局

`AgentState` 只是上一轮 `NextHint` 字符串，`PreviousResults` 只有 name + summary 的扁平列表。Planner 不知道：

- 已经查到哪个阶段（初始诊断 / 深入节点 / 验证）
- 当前假设是什么（如 "indexer 因为 connection refused 失败"）
- 还有哪些问题没回答
- 检索阶段已经拿到了什么信息（`RetrieveResult` 传入了但 AgentLoop 没使用）

### 1.3 硬编码约束分散在各处

| 约束 | 位置 | 当前值 |
|------|------|--------|
| 最大迭代次数 | `agent_loop.go` 常量 | 3 |
| 最大单次规划工具数 | `planner.go` 常量 | 3 |
| 工具集 | `runtime.go` 注册 | 恰好 8 个只读诊断工具 |
| Planner system prompt | `planner.go` 模板字符串 | 硬编码含领域示例 |
| Hint 解析 | `agent_loop.go` switch | 只识别 4 个工具名 |
| 去重逻辑 | `agent_loop.go` key() | 基于字符串拼接 |
| 诊断节点类型 | `diagnose_helpers.go` | 硬编码 4 个节点名 |
| 通道 TopK / RRF k | `retrieve/` 多处 | 硬编码 5 / 2x / 60 |

### 1.4 RuleObserver 的覆盖逻辑靠逐条手写维护

每多覆盖一个场景（`doc_run_01`、`task_run_01`、`trace_bad_01`），就需要新增或修改 `observe*` 函数。这条路的终点是规则维护成本吃掉诊断价值。

---

## 二、什么是"更自主"

在当前项目语境下，"更自主"不等于 AGI，而是指 Agent 具备以下能力：

1. **自己决定调用什么工具** — 不依赖硬编码的正则匹配和 hint 字符串解析
2. **自己判断何时停止** — 不依赖手写的 `observeXxx()` 规则函数
3. **处理未预定义的场景** — 工具结果类型超出 Observer 覆盖范围时不会直接终止
4. **维护跨轮次的结构化上下文** — 知道自己查到了什么、还缺什么、当前假设是什么

---

## 三、演进路线

分三个阶段，每阶段可独立验证，不破坏已有约束（`RagChatService` 外壳不变、Tool 契约不变、SSE 协议兼容）。

### 第一阶段：Observer 升级

**目标：** 把"是否继续、下一步查什么"的决策权从规则移到 LLM，RuleObserver 降级为安全兜底。

**具体改动：**

1. 新增 `LLMObserver` 实现 `Observer` 接口
   - 输入：`Question` + 当前轮 `RoundResults`
   - 输出：`{"done": bool, "reasoning": "...", "nextHint": "..."}`
   - 不逐条手写 `observeDocumentDiagnosis` / `observeTaskQuery` 等规则

2. 在 `runtime.go` 中将默认 Observer 从 `RuleObserver` 切换为 `LLMObserver`

3. `RuleObserver` 保留但只做两件事：
   - `ReachedMaxLoop` 检查
   - 重复调用去重

4. 移除 `agent_loop.go:170-172` 中硬编码的 Done 覆盖逻辑

**影响范围：** 约 1 个新文件 + 1 行引导代码改动。

**验证标准：** `doc_fail_01` 在 LLMObserver 下的诊断路径与 RuleObserver 一致或更优，不会无限循环。

### 第二阶段：上下文结构化

**目标：** Planner 不再只看扁平字符串，而是拥有结构化的任务状态。

**具体改动：**

1. `AgentState` 从字符串升级为结构化类型：
   ```go
   type AgentState struct {
       Phase          string   // "initial_diagnosis" | "deep_dive" | "verification" | "complete"
       Hypothesis     string   // 当前假设，如 "indexer 因连接拒绝失败"
       Confidence     float64  // 0.0 - 1.0
       OpenQuestions  []string // 尚未回答的问题
       CheckedTools   []string // 已执行过的工具
   }
   ```
   由 `LLMObserver` 产出，注入下一轮 `PlanInput`。

2. `WorkflowInput` 中已有但未使用的字段接入 AgentLoop：
   - `RetrieveResult` — Planner 知道检索阶段已拿到什么，避免工具调用重复检索
   - `RewriteResult` — Planner 知道用户意图已被如何改写
   - `KnowledgeBaseIDs` — Planner 知道当前知识库范围

3. Planner system prompt 从硬编码字符串改为模板注入，支持 `runtime.go` 装配时配置

**验证标准：** Planner 在 `doc_run_01` 场景下能自主判断"文档在运行中而非失败，不需要继续下钻到 node"。

### 第三阶段：工具集扩展 + 动态控制

**目标：** Agent 从"诊断工具调用器"变成"自主问题解决器"。

**具体改动：**

1. 在只读约束内扩展工具集：
   - `knowledge_search` — 语义搜索知识库内容
   - `conversation_history_lookup` — 查历史对话中类似问题
   - `metric_query` — 查 ingestion metrics 趋势数据

2. 迭代控制从硬编码常量改为动态判断：
   ```go
   func (w *AgentLoop) shouldContinue(round int, state AgentState) bool {
       if round >= w.maxIterations { return false }           // 硬上限仍在
       if state.Phase == "complete" { return false }
       if state.Confidence > 0.9 && len(state.OpenQuestions) == 0 { return false }
       return true
   }
   ```

3. Observer 不只输出 `Done`，还输出 `Confidence` 和 `OpenQuestions`，两者共同决定是否继续

**完成标志：** Agent 能处理未在预定义规则覆盖范围内的诊断问题，例如"最近一周有哪些文档导入失败了？原因分别是什么？"

---

## 四、当前不该做的事

| 事项 | 原因 |
|------|------|
| 引入写操作工具 | 需要确认机制、回滚能力、权限控制等完全不同的安全边界，诊断场景稳定前引入是危险的 |
| 通用 workflow 模板库 | 当前 8 个工具、1 种场景类型不足以支撑有意义的模板抽象 |
| 前端完整思考卡片 UI | `agent_think` 事件已就绪，前端维持最小消费即可。美化时机应在 Agent 决策质量稳定之后 |
| 引入第三方 Agent 框架 | 当前自研的 Workflow/Planner/Observer 接口足够灵活，引入框架会破坏约束且增加复杂度 |

---

## 五、与现有文档的关系

- [project_progress_context.md](project_progress_context.md) — 项目整体进度与验证状态
- [agentic_chat_dev_plan.md](agentic_chat_dev_plan.md) — Agentic Chat 当前实现范围与 V1/V2/V3 计划

本文档聚焦于"从诊断 Agent 向自主 Agent 演化"这一条线，是对 agentic_chat_dev_plan 中 V2/V3 的细化补充。

---

## 六、约束重申

以下约束在演进过程中保持不变：

1. 不改写 `RagChatService` 的 `Chat(ctx, input, sink)` 签名与整体流水线结构
2. 不破坏现有 SSE 协议兼容性（`meta / message / thinking / finish / done / cancel / error`）
3. Tool 接口不变（`Definition() / Invoke()` 保持现有契约）
4. 所有 tool 调用必须只读
5. 迭代上限必须存在（硬上限不取消，但可配置化）
6. 不引入新的第三方依赖
