# Agent Loop 架构评估与优化方向

日期：2026-05-15

---

## 一、当前架构

goagent 的 Agent Loop 采用 **Plan → Act → Observe** 三层循环（默认最多 3 轮）：

```
                 ┌──────────────┐
                 │  AgentLoop   │
                 │  .Run()      │
                 └──────┬───────┘
                        │
           ┌────────────┼────────────┐
           ▼            ▼            ▼
      Plan           Act          Observe
 ┌─────────────┐  ┌──────────┐  ┌──────────────┐
 │ LLMPlanner   │  │ Executor │  │ LLMObserver  │
 │   ↓ fallback │  │  ├timeout│  │   ↓ fallback │
 │ BaseRules    │  │  ├retry  │  │ RuleObserver │
 │   ↓ fallback │  │  └invoke │  │              │
 │ HintCalls    │  └──────────┘  └──────────────┘
 └─────────────┘                      │
                            ┌─────────┴─────────┐
                            ▼                   ▼
                      BuildAnswerGuidance   RenderContext
```

### 核心文件（行数）

| 文件 | 行数 | 职责 |
|------|------|------|
| `runtime/agent_loop.go` | 919 | 主循环：plan 调度、执行调度、observer 调用、状态合并、降级标记 |
| `runtime/observer_llm.go` | 480 | LLM 驱动的观察者：few-shot prompt + 证据校验 + 规则降级 |
| `runtime/observer_rule.go` | 347 | 规则驱动的观察者：模块行为委托 + depth 分叉 + 遗留观察函数 |
| `planner/planner.go` | 230 | LLM 驱动的规划者：few-shot prompt + 并行规划指导 |
| `core/module.go` | 202 | 模块类型定义：ToolInvoker / ToolSpec / ToolBehavior / ToolModule |
| `core/registry.go` | 112 | 模块注册中心：Register / Get / GetBehavior / GetSpec |
| `core/workflow.go` | 239 | 核心接口与数据结构：Workflow / Planner / AgentState / HintCall |
| `assembly/workflow.go` | 307 | 组装与布线：构建 Registry、注册模块、创建 AgentLoop |

### 模块行为（ToolBehavior）

每个 tool 通过五个回调封装自身语义，而非集中在中央 switch-case：

```go
type ToolBehavior struct {
    Decode        func(result Result) (any, error)           // 结果 → 类型化视图
    Next          func(result Result, input WorkflowInput) NextDecision  // 推导下一步
    Observe       func(result Result, input ObserveInput) (ObserveResult, bool)  // 判断证据是否充足
    RenderContext func(result Result) string                 // 结果 → prompt 文本
    BuildGuidance func(result Result, input GuidanceInput) []GuidanceNote  // 回答指导
}
```

---

## 二、优点

### 1. 模块抽象干净（最强资产）

ToolModule + ToolBehavior 的设计是核心亮点。五个回调将工具语义完整封装在模块内部，新增工具只需提供这些回调即可接入，无需修改核心循环。`LegacyToolAdapter` + `InferBehavior` 机制让旧式 `Tool` 注册也能自动推断行为，保证迁移平滑。

### 2. 分层降级策略合理

`LLMPlanner → BaseRules → HintCalls` 三层规划降级 + `LLMObserver → RuleObserver` 两层观察降级，保证 LLM 不可用或输出非法时系统仍能工作。降级不是简单 try-catch，而是规则层理解工具语义后做出合理决策。

### 3. 证据校验有效防止幻觉

`validateHintAgainstEvidence` 和 `validateCallAgainstEvidence` 从 question、previousHintCalls、tool results 中收集合法 entity ID 白名单，拒绝 LLM 编造的参数（联调中修复了 `node_0` 幻觉问题）。

### 4. 并行执行实现正确

信号量控制并发，`tool_start` 按序列顺序发出、`tool_result` 按序列顺序汇总。实测串行 80ms → 并行 40ms。

### 5. Graph Tool 设计聪明

Eino Graph 将确定性多跳诊断链编译为单次 tool 调用，LLM 调用从 6 次降到 0 次——对"LLM 过度参与确定性流程"的精准解法。

---

## 三、问题

### 1. Planner 与 Observer 的 LLM 双重调用

每轮 Agent Loop 可能触发两次 LLM 调用，且两者接收高度重叠的上下文。Observer 的 `nextHintCalls` 本身就是下一轮的 plan——Round 2+ 的 Planner 在做 Observer 已经做过的工作。

### 2. PlanWithBaseRules 的关键词路由在膨胀

`agent_loop.go` 中定义了 6 组关键词列表和 6 条路由规则，是硬编码的 intent 分类器。随 tool 增长会继续膨胀，且与 module behavior 的内聚原则不一致。

### 3. RuleObserver 身份模糊

自模块化迁移后主执行路径已转移到 `observeWithRegistry` 的模块行为委托。遗留的 `ObserveDocumentDiagnosis` / `ObserveTaskQuery` 等函数仍在代码中但不再是主路径——"仍被引用但非核心"的灰色地带。

### 4. AgentLoop.Run() 单一方法过长（163 行）

承担循环控制、plan 调度、执行调度、observer 调用、状态合并、降级标记、日志输出。其中的 AgentState ↔ ObserveResult 字段互填逻辑（148-183 行）有层层补丁的迹象。

### 5. NextHint 字符串与 NextHintCalls 结构化并存

整个链路同时携带 `NextHint string`（legacy，格式 `tool:name|key=value`）和 `NextHintCalls []HintCall`（结构化，当前主语义）。agent_loop.go 中有大量互填逻辑。

### 6. 并行执行的 SSE 时序语义不精确

`tool_start` 在执行前按序列发出，`tool_result` 在所有完成后按序列发出。前端看到"同时开始、同时结束"，无法展示单个 tool 的真实执行进度。

### 7. MaxIterations=3 与实际需求的差距

默认 3 轮对复杂场景可能不够。缓解手段是 graph tool 在单次调用内完成多跳——架构依赖 tool 内部复杂性弥补循环深度限制。

### 8. 可测试性

AgentLoop 的直接单元测试较少。核心循环逻辑与具体 tool 行为紧耦合，不启动完整注册链就无法测试循环决策本身。Planner 和 Observer 已是接口，Mock 测试是可行的但尚未充分利用。

---

## 四、优化方向

按收益从高到低排列。

### 方向 1：消除 Planner 的冗余 LLM 调用（P0，~10 行）

在 `planCalls()` 中加短路判断：Round 2+ 时如果 agentState 已有 nextHintCalls（上一轮 Observer 的输出），直接走规则路径，不调 LLMPlanner。

```
收益：从 2N 次 LLM 调用降到 N+1 次（N 轮时）
风险：零架构变动
```

### 方向 2：合并 Planner 与 Observer 为单一 Strategist（P1，~100 行删减）

Observer 的 `nextHintCalls` 就是下一轮的 plan，Planner 和 Observer 的输入几乎完全重叠。合并后 AgentLoop 从 Plan-Act-Observe 简化为 Assess-Act，每轮固定省 1 次 LLM 调用。

```
收益：架构简化 + LLM 调用减半 + 两个 prompt 维护成本合并
注意：Round 1 冷启动保留 BaseRules 关键词路由（不需要 LLM）
```

### 方向 3：AgentState 数据流简化（P1，~50 行删减）

ObserveResult 同时有顶层字段（Done / Confidence / NextHintCalls）和嵌套 State AgentState，不同 Observer 实现填充位置不一致导致大量互填逻辑。统一为单一数据源——只保留 State，所有消费方从 State 读取。

```
收益：删除核心循环中 ~35 行互填逻辑
```

### 方向 4：用 module behavior 彻底替代 PlanWithBaseRules 集中式路由（P2，~150 行）

给 ToolSpec 加可选的 `RouteHints`，让路由规则和 tool 定义放在一起。agent_loop.go 删除 ~110 行硬编码关键词定义。

```
收益：模块内聚，消除 agent_loop.go 中的集中式硬编码
```

### 方向 5：Graph Tool 泛化（P2，按需新增）

把 `document_root_cause_diagnosis` 的成功模式泛化到其他高频多跳链路（trace_root_cause_diagnosis、knowledge_gap_workflow），让高频路径走快车道。

```
收益：高频路径零 LLM 调用
```

---

## 五、架构评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 模块化 | ★★★★☆ | ToolModule + Behavior 设计优秀，残余集中式分支正在消退 |
| 可扩展性 | ★★★★☆ | 新增 tool = 新 module，框架层无需改动 |
| 鲁棒性 | ★★★★☆ | 多层降级 + 证据校验 + 中间件链，生产级韧性 |
| 代码清晰度 | ★★★☆☆ | 核心循环偏长，状态合并逻辑有补丁痕迹 |
| LLM 效率 | ★★★☆☆ | Planner+Observer 双调用有冗余，但 Graph Tool 弥补了确定性场景 |
| 可测试性 | ★★★☆☆ | 接口化是好的，但缺少纯循环逻辑的单测 |

**一句话总结**：这是从快速增长期进入稳定打磨期的架构——模块抽象是设计亮点，LLM 双重调用和状态合并的复杂度是当前主要的改善机会。

---

## 六、业界参考

### 1. koda — 最相似的架构，最有价值的教训

koda 的 agent loop 经历了和 goagent 几乎一样的演化：`Observe → Plan → Act → Reflect` → 6 阶段定向循环。但事后复盘（[issue #355](https://github.com/lijunzh/koda/issues/355)）删掉了 4308 行阶段机代码：

- 正式 plan 提交的 token 开销超过了节省的（~500 token/轮）
- 强模型（Claude Opus 4）自然 plan→execute，不需要显式状态机
- Review 循环让 $20 的任务多花 $5-8
- 阶段可见性对 UX 有价值，但**强制执行伤害大于帮助**
- 最终保留**按 tool 的审批模式**，而非按阶段的审批

**对 goagent 的启示**：方向 2（合并 Planner/Observer）和 koda 的教训一致——不要在循环里塞太多元推理开销。

### 2. LangGraph — 状态机范式的标杆

整个 agent loop 就是一个 `agent ↔ tools` 的二分图。Planner 和 Observer 不是独立节点——它们就是 agent 节点内部的 reasoning。预构建的 MessagesState 用 `operator.add` reducer 自动追加消息，节点只需返回增量。

**对 goagent 的启示**：AgentState 整体替换模式 → 借鉴 reducer 模式可自然消除那 35 行互填逻辑。LangGraph 1.0 的 middleware hooks（`before_model` / `after_model`）也值得参考。

### 3. Anthropic Claude Agent SDK — 循环是隐式的

SDK 内部处理循环，开发者只定义 tools 和 permissions。模型的每次响应天然包含"要不要调工具"和"调什么工具"——这两个决策是一次推理的两个输出面。goagent 拆成两次 LLM 调用是在为同一段思考付两次费。

### 4. ReAct vs Plan-and-Execute vs Hybrid — 2026 共识

| 模式 | LLM 调用 | 适用场景 | 失败模式 |
|------|----------|----------|----------|
| **ReAct** | 每步 1 次 | 探索性任务 | 无限循环、重复调用 |
| **Plan-and-Execute** | 1 次规划 + N 次轻量执行 | 结构化流水线 | plan 过时 |
| **Hybrid**（2026 主流） | 按需混合 | 通用 | 复杂度 |

2026 共识：纯 ReAct 太贵，纯 Plan-and-Execute 太脆，Hybrid 是生产级答案。

### 5. goagent 与业界的对照

```
goagent 当前:
  Plan (LLMPlanner OR BaseRules) → Act (Executor) → Observe (LLMObserver OR RuleObserver)
  每轮最多 2 次 LLM 调用

LangGraph:
  Agent (一次调用同时决定 done + next) → Tools
  每轮 1 次 LLM 调用

koda 教训:
  过度阶段化 ≈ 过度元推理 ≈ token 浪费
  强模型不需要显式状态机

Claude Agent SDK:
  循环是 SDK 内部的事，开发者不管 Plan/Observe 的分离
```

goagent 在模块化 tool behavior 和证据校验上比参考实现更细——`ToolBehavior` 五个回调在 LangGraph 和 CrewAI 中没有对等物。但在 LLM 调用效率上，Planner+Observer 双调用比 LangGraph 的单调用模式多了一倍开销。

---

## 七、推荐阅读顺序

1. **koda issue #355**（事后复盘）——最接近 goagent 的场景，避免重复踩坑
2. **LangGraph MessagesState + reducer 模式**——直接启发 AgentState merge 逻辑简化
3. **Claude Agent SDK hook 设计**（preToolUse / postToolUse）——比当前 middleware 链更灵活
