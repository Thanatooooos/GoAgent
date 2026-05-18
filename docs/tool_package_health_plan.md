# Tool 包健康度评估与优化计划

日期：2026-05-17

---

## 一、评估总览

Tool 包（`internal/app/rag/tool/`）已从"硬编码 switch"成功迁移到"模块化行为驱动"架构。`ToolModule + ToolBehavior + Registry` 的抽象层次清晰，新增工具只需提供 `Invoker` + `Behavior` 即可接入，无需修改核心循环。这是当前代码库中架构质量最高的包之一。

但经过 5 月高强度的功能迭代（AgentLoop V1、LLMObserver、15 个 tool、并行执行、外部证据工作流、模块化改造），也积累了一些需要收口的结构性问题。本文档以具体文件和行号为粒度，给出一份可执行的优化计划。

### 核心数据

| 维度 | 数值 |
|---|---|
| 源文件数 | 69 个（非测试） |
| 测试文件数 | 14 个 |
| 总源行数（非测试） | ~8,500 行 |
| 总测试行数 | ~5,000 行 |
| 测试/源比 | ~0.59 |

### 本次计划 vs 已有计划的关系

- [agent_loop_architecture_review.md](agent_loop_architecture_review.md) — 架构评估，确认优点和方向。本计划的"上游输入"。
- [agent_loop_optimization_plan.md](agent_loop_optimization_plan.md) — Planner 短路、状态流收口、RuleObserver 清理。**已部分落地**（RuleObserver 从 347 行缩至 102 行）。
- **本文档** — 聚焦尚未覆盖的结构问题：大文件拆分、代码重复消除、测试分层、可扩展性瓶颈。

---

## 二、当前架构回顾

### 2.1 分层结构

```
internal/app/rag/tool/
├── core/          (13 files, ~1,827 行) — 类型定义 + 接口 + Registry
│   ├── tool.go             — Tool/Call/Result/Definition
│   ├── module.go           — ToolModule/ToolBehavior/ToolSpec/LegacyToolAdapter
│   ├── registry.go         — Registry (map[string]ToolModule)
│   ├── workflow.go         — WorkflowInput/Result, AgentState, HintCall, Planner 接口
│   ├── workflow_control.go — 执行模式/风险等级/审批要求 枚举
│   ├── middleware.go       — ToolHandler/ToolMiddleware 链
│   ├── summary.go          — LLM 摘要生成
│   └── ...
├── runtime/       (13 files, ~3,737 行) — 所有具体实现
│   ├── agent_loop.go       (1,015 行) — 主循环编排 ⚠️
│   ├── observer_llm.go     (476 行)   — LLM 驱动观察者 ⚠️
│   ├── observer_rule.go    (102 行)   — 规则观察者 ✅
│   ├── answer_guidance.go  (596 行)   — 回答指导生成 ⚠️
│   ├── next_action.go      (167 行)   — 旧式下一动作 ⚠️ (重复代码)
│   ├── executor.go         — 单次 tool 执行
│   ├── renderer.go         — 上下文渲染
│   └── ...
├── planner/       (1 file, ~230 行)   — LLM Planner
├── invokers/      (~20 files)        — 工具执行实现 (按 family 分组)
│   ├── system/  — document/task 诊断与查询 (10 files)
│   ├── web/     — web_search/web_fetch/Tavily
│   ├── trace/   — trace 诊断与查询
│   ├── graph/   — 3 个 Eino Graph (external_evidence 达 898 行)
│   └── meta/    — think 元工具
├── modules/       (~8 files)         — 每个 family 的 ToolBehavior
│   ├── system/behavior.go  (496 行)  — 6+ 工具的全部行为 ⚠️
│   ├── web/     — web 行为 + result_views
│   ├── trace/   — trace 行为
│   ├── graph/   — graph 行为
│   └── meta/    — think 行为
├── assembly/     (1 file)            — 依赖注入 & 组装入口
└── (root)        (13 files, ~1,200 行) — 桥接层 + 兼容适配器 + 测试
```

### 2.2 核心执行流程

```
AgentLoop.Run()
  │
  └─ for round 1..maxIterations:
       ├─ Plan  → HintCalls > LLMPlanner > planCallsFromResults > PlanWithBaseRules
       ├─ Act   → Executor.Execute() (with timeout/retry middleware)
       └─ Observe → LLMObserver (with fallback to RuleObserver)
                      │
                      ├─ Done=true  → exit loop
                      └─ Done=false → next round with NextHintCalls
```

### 2.3 已验证的架构优点

详见 [agent_loop_architecture_review.md](agent_loop_architecture_review.md)，这里不再展开。核心优势：

1. **模块抽象干净** — ToolBehavior 五个回调完整封装工具语义
2. **分层降级合理** — Planner/Observer 各两层降级
3. **证据校验有效** — validateHintAgainstEvidence 防止 LLM 幻觉 ID
4. **并行执行正确** — 信号量控制，结果顺序稳定

---

## 三、问题清单（按严重程度排列）

### P0 — 代码重复（双重维护风险）

#### 3.1 `nextAction*` 函数在两处重复定义

两个文件中的以下函数**逻辑完全相同**（仅包引用前缀不同）：

| 函数 | runtime/next_action.go | modules/system/behavior.go |
|---|---|---|
| `nextActionDocumentQuery` | L40-48 | L189-197 |
| `nextActionChunkLogQuery` | L50-69 | L199-217 |
| `nextActionDocumentDiagnosis` | L71-91 | L219-238 |
| `nextActionTaskQuery` | L93-106 | L240-253 |
| `nextActionTaskDiagnosis` | L150-166 | L255-270 |

`runtime/next_action.go` 还额外定义了 `nextActionWebSearch`（L108-132），而 `modules/web/behavior.go` 中另有独立版本。

这两个文件在包引用上的差异：
- `runtime/next_action.go` 使用 `. "core"`（点导入），直接写 `Result`、`HintCall`
- `modules/system/behavior.go` 使用 `ragcore.` 前缀

**风险**：修改 tool 链式调用逻辑时必须改两处，已在 agent_loop_optimization_plan.md 的计划中做过一轮"单一决策源"改造，但运行时旧路径清理不彻底，导致同一份逻辑仍在两处独立演化。

**解决方向**：删除 `runtime/next_action.go` 中的 system 系列 `nextAction*` 函数，让调用方通过 `Registry.GetBehavior().Next()` 走模块路径。`nextActionWebSearch` 移到 `modules/web/behavior.go`（如果尚未完全迁移）。

#### 3.2 `planCallsFromResultsWithRegistry` 在两处定义

`runtime/module_runtime.go` 和 `modular_wrappers.go` 中均存在同名函数，功能相近但签名略有差异。需要确认是否可以收敛为一处。

---

### P1 — 大文件拆分

#### 3.3 `runtime/agent_loop.go` — 1,015 行

函数分布（行号）：

| 行号范围 | 函数 | 行数 | 职责 |
|---|---|---|---|
| L49-55 | `SetPlanner` | 6 | 配置 |
| L56-62 | `SetObserver` | 6 | 配置 |
| L63-73 | `SetMaxIterations` | 10 | 配置 |
| L74-85 | `SetParallelToolCalls` | 11 | 配置 |
| L86-321 | `Run` | **235** | 主循环 |
| L322-343 | `planCalls` | 21 | 规划调度 |
| L344-376 | `planWithLLM` | 32 | LLM 规划 |
| L377-385 | `planWithRules` | 9 | 规则规划 |
| **L387-902** | **`PlanWithBaseRules`** | **515** | **关键词路由（超过文件一半！）** |
| L903-962 | `executeRoundCalls` | 59 | 执行调度 |
| L963-985 | `executeCallsInParallel` | 22 | 并行执行 |
| L986-1006 | `executeSingleCall` | 20 | 单次执行 |
| L1007-1015 | `roundExecutionMode` | 8 | 辅助 |

**问题**：`PlanWithBaseRules` 占据文件一半以上，包含大量中英文关键词正则匹配和硬编码路由规则。这不是"编排逻辑"，是"路由数据"。应该从 AgentLoop 中抽离。

**建议拆分**：抽离为 `runtime/base_rules.go`（路由数据）+ `agent_loop.go` 仅保留编排。

#### 3.4 `modules/system/behavior.go` — 496 行

包含 8 个 `*Behavior()` 构造函数 + 6 个 `nextAction*` 函数 + 5 个 `observe*` 函数 + 3 个 `render*` 函数 + 辅助函数。所有 system 家族工具的行为混在一个文件里。

**建议拆分**：
- `modules/system/document_behavior.go` — DocumentQuery + DocumentChunkLogQuery + DocumentList + DocumentIngestionDiagnose
- `modules/system/task_behavior.go` — TaskQuery + TaskNodeQuery + TaskList + TaskDiagnose
- 保留 `modules/system/behavior.go` 为 re-export 或 `doc.go`

#### 3.5 `runtime/answer_guidance.go` — 596 行

函数过多（~20 个），覆盖 diagnosis、web_search、external_evidence 三种不同场景的 guidance 构建。当前全部混在一个文件。

**建议拆分**：
- `runtime/guidance_diagnosis.go` — diagnosis 相关
- `runtime/guidance_web.go` — web_search/web_fetch/external_evidence 相关
- `runtime/answer_guidance.go` — 入口函数 `BuildAnswerGuidance` + 公共辅助

#### 3.6 `invokers/graph/external_evidence_workflow_graph.go` — 898 行

单个 Eino Graph 定义文件接近 900 行。Graph 节点定义、select/assess 逻辑、结果序列化全部耦合。

**建议**：将 select/assess 的纯逻辑抽离为独立辅助文件，保留 Graph 构建在主文件。

---

### P2 — 测试结构优化

#### 3.7 测试金字塔倒挂

| 测试文件 | 行数 | 类型 |
|---|---|---|
| `agent_loop_test.go` | 1,255 | 集成测试 |
| `tool_test.go` | 784 | 集成测试 |
| `agent_loop_graph_test.go` | 756 | 集成测试 |
| `observer_llm_test.go` | 359 | 单元+集成 |

最大 4 个测试文件都是集成层级，依赖完整的 AgentLoop 运行。优点是覆盖端到端行为，缺点是：
- 运行慢（涉及 LLM mock、多轮循环）
- 定位问题粒度粗（一个断言失败需要排查整条链路）
- 新增 tool 后需要修改 agent_loop_test 来补 case

**建议**：为以下组件补独立单元测试（不需要完整的 AgentLoop）：
- `PlanWithBaseRules` — 关键词路由正确性（纯函数，无副作用）
- 各 `nextAction*` 函数 — 给定输入结果 → 产出的下一步
- `answer_guidance.go` 中各 guidance builder
- `validateHintAgainstEvidence` / `validateCallAgainstEvidence`

---

### P3 — 可扩展性瓶颈

#### 3.8 Base rules 关键词硬编码

`AgentLoop.PlanWithBaseRules`（agent_loop.go L387-902）包含：
- 硬编码的中文关键词列表（"失败"、"报错"、"为什么"…）
- 硬编码的英文关键词列表（"failed"、"error"、"diagnose"…）
- 硬编码的 ID 提取正则 + 路由规则

新增"当用户问 X 时路由到 Y 工具"的场景必须修改 agent_loop.go。

**建议**：将 base rules 定义为数据结构（`[]RouteRule`），从 `ToolSpec` 或独立配置注入。`PlanWithBaseRules` 退化为纯规则匹配引擎。

#### 3.9 LLMObserver few-shot 示例硬编码

`observer_llm.go` 中的 `observerExamples` 常量（约 100 行）包含 8+ 个 few-shot 示例，覆盖 document/task/trace/web 各家族场景。新增 tool family 时需要修改 observer_llm.go 才能让 LLM 理解何时调用该工具。

**建议**：在 `ToolBehavior` 中新增可选的 `ObserverExamples []string` 字段，Observer 构建 prompt 时从 Registry 收集所有模块的示例。

#### 3.10 Planner/Observer 合并探索

当前 Planner 和 Observer 各调用一次 LLM（串行两次）。在简单场景下（如 diagnose 一步到位），Observer 的 LLM 调用是冗余的——RuleObserver 已经能处理。在复杂场景下，两次调用可能产生不一致的决策。

[agent_loop_architecture_review.md](agent_loop_architecture_review.md) 中已提到这一点。建议在完成 P0/P1 收口后，评估将 Plan+Observe 合并为单次 LLM 调用的可行性和收益。

---

## 四、执行顺序

### 第一阶段（P0 收口，预计 ~2h）

1. **消除 `nextAction*` 重复**
   - 确认 `modules/system/behavior.go` 的 `nextAction*` 是唯一调用路径
   - 将 `runtime/next_action.go` 中的 system 系列函数标记为 deprecated 或直接删除
   - 将 `nextActionWebSearch` 逻辑确认已在 `modules/web/behavior.go` 中覆盖
   - 清理 `modular_wrappers.go` 与 `runtime/module_runtime.go` 之间的重复

2. **确认无回归**
   - 运行 `go test ./internal/app/rag/tool/... -count=1`
   - 关键场景回归：`doc_fail_01` / `doc_run_01` / `task_run_01` / `trace_bad_01` / web search

### 第二阶段（P1 大文件拆分，预计 ~3h）

3. **拆分 `agent_loop.go`**
   - 抽离 `PlanWithBaseRules` → `runtime/base_rules.go`
   - agent_loop.go 保留 Run / planCalls / executeRoundCalls 等编排逻辑
   - 预期：agent_loop.go ~500 行，base_rules.go ~500 行

4. **拆分 `modules/system/behavior.go`**
   - `document_behavior.go` — 4 个 document tool 的 Behavior
   - `task_behavior.go` — 4 个 task tool 的 Behavior
   - 保留原文件做兼容 re-export

5. **拆分 `answer_guidance.go`**
   - `guidance_diagnosis.go` — diagnosis 场景
   - `guidance_web.go` — web/external_evidence 场景
   - 原文件保留入口函数

6. **拆分 `external_evidence_workflow_graph.go`**
   - 抽离 select/assess 纯逻辑到独立文件

### 第三阶段（P2 测试补强，预计 ~2h）

7. **补单元测试**
   - `base_rules_test.go` — `PlanWithBaseRules` 路由正确性
   - `next_action_test.go` — 各 nextAction 函数
   - `answer_guidance_test.go` — guidance builder 独立测试

### 第四阶段（P3 扩展性改善，预计按需执行）

8. Base rules 数据结构化（从 `[]RouteRule` 注入）
9. Observer few-shot 示例模块化（`ToolBehavior.ObserverExamples`）
10. Planner/Observer 合并探索

---

## 五、风险与约束

1. **不改变对外行为** — 本轮只做结构收口，HTTP 接口、SSE 事件、trace 格式保持不变
2. **不新增工具** — 15 个 tool 的 `Invoker` 实现不动
3. **关键场景不回退** — `doc_fail_01` / `doc_run_01` / `external_evidence_workflow` 等联调样例行为不变
4. **拆分即验证** — 每个阶段完成后跑全量 `go test ./internal/app/rag/tool/...` 确认零回归
5. **慎重的函数重命名** — 跨包引用的导出函数（如 `PlanWithBaseRules`）如要改名需同步更新所有调用方

---

## 六、验收标准

- [ ] `runtime/next_action.go` 不再包含与 `modules/system/behavior.go` 重复的 `nextAction*` 函数
- [ ] `agent_loop.go` 从 1,015 行降至 ~500 行（编排逻辑）
- [ ] `modules/system/behavior.go` 从 496 行拆为 <300 行/document + <250 行/task
- [ ] `answer_guidance.go` 从 596 行拆为多个 <300 行的文件
- [ ] `go test ./internal/app/rag/tool/... -count=1` 全量 PASS
- [ ] `go build ./...` PASS
- [ ] 关键场景手动回归无回退
