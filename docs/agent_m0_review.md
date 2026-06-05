# Agent 模块当前状态审查报告

日期：`2026-05-30`
版本：v4（全量重审，覆盖全部 20 个 package）

审查范围：`internal/app/agent/` 下 17,554 行 Go 代码，22 个包（含 spike）。

测试结论：`go test ./internal/app/agent/... -count=1` — **19 PASS, 3 no test files，零回归。**

---

## 一、代码全景

```
internal/app/agent/                  17554 lines total
├── state/                 220+96+92  # 7 域 snapshot + delta + 14 event 常量 + reducer + clone + register
├── runtime/                          # session + projection + replay + session_store + session_clone
├── kernel/                           # Node/Builder/Runner/journal/checkpoint（含 spike/）
├── capability/                       # Handle + Spec + Registry + RoleBindings + Validation + Options
│   ├── catalog/                      # Card + RegistryBuilder（LLM-facing catalog）
│   ├── resolve/                      # Resolver → Match/Resolve（selector→handle）
│   └── select/                       # LLMSelector（LLM-driven capability selection）
├── planner/                          # Planner 接口 + LLMPlanner + BuildSummary
├── pattern/                          # 共享 AssemblyContext + RuntimeConfig
│   ├── reactive/                     # prepare→(search→fetch|external_evidence)→observe→branch(5路)
│   ├── reactive/ + continue node     # ✅ 已支持 continue 循环
│   └── planexecute/                  # build_plan→select_step→[execute|approval|finalize]
├── handoff/                          # Build(Result{EvidenceBundle, DecisionSummary, Replay})
├── fetch/                            # fetch service（含 Capability 包装）
├── search/                           # search service（含 Capability 包装）+ provider/
├── websearch/                        # Eino tool wrapper（非主路径）
├── webfetch/                         # Eino tool wrapper（非主路径）
├── document_investigation/           # ✅ 新 capability family
├── external_evidence/                # ✅ 新 composite capability (search→fetch)
├── service.go (126)                  # Service struct + NewService
├── service_run.go (244)              # Run/RunDetailed/RunHandoff + approval 检测
├── service_resume.go (148)           # ResumeAfterApproval + finalizeRejectedApproval
├── service_approval.go               # approval 数据结构
├── service_approval_resume.go        # approval 恢复决议
├── service_pattern.go (228)          # 模式选择 + 装配
├── service_error.go                  # 统一错误体系
├── pattern_validation_test.go        # 跨模式验证测试
├── service_test.go                   # 顶层 E2E 测试
└── request_response.go               # 顶层 Request/Response DTO
```

---

## 二、里程碑对照

### M0（设计冻结）— 全部达成

所有 7 个核心抽象均已落地并被消费。

### M1（Kernel + State System Skeleton）— 全部达成 + 大量超额

| 产出 | 状态 |
|---|---|
| graph compile skeleton | ✅ `kernel/builder.go` |
| reducer skeleton | ✅ `builder.invokeNode` |
| in-memory journal | ✅ `kernel/journal.go` |
| snapshot projection | ✅ `runtime/projection.go` |
| basic replay harness | ✅ `runtime/replay.go` |
| checkpoint / resume | ✅ `kernel/runner.go` + `kernel_test.go` |
| branch routing | ✅ `branch_selected` 事件 |
| interrupt metadata | ✅ `interrupt` + `resume_completed` 事件 |

### 当前实际已达成能力（超出 M1）

| 能力 | 状态 | 位置 |
|---|---|---|
| **LLM Planner** | ✅ | `planner/llm_planner.go` — 含结构化 Summary、decision 校验、stop signals |
| **Continue 循环** | ✅ | `pattern/reactive/nodes_continue.go` + 5 路 branch |
| **Plan-Execute Pattern** | ✅ | `pattern/planexecute/` — 含 build_plan/select_step/execute/assess/replan |
| **Approval + Resume** | ✅ | `service_run.go` + `service_resume.go` — 完整中断→等待→批准/拒绝→恢复流程 |
| **Handoff 投影** | ✅ | `handoff/` — EvidenceBundle + DecisionSummary + ReplayView |
| **Unified Capability Handle** | ✅ | `capability/invocation.go` — `Handle.Invoke(InvocationRequest) → InvocationResult` |
| **Role-Based Bindings** | ✅ | `capability/bindings.go` — Pattern 按 role 解析 capability，支持显式绑定和自动推导 |
| **Capability Catalog + Selector + Resolver** | ✅ | `catalog/` + `select/` + `resolve/` — LLM-driven 能力选择链路 |
| **Document Investigation Capability** | ✅ | `document_investigation/` — Kind: workflow, Family: document_investigation |
| **External Evidence Composite** | ✅ | `external_evidence/` — 组合 search + fetch 为一个 workflow capability |
| **Input Validation** | ✅ | `capability/validation.go` — Precondition + reflect-based 字段校验 |
| **Session Store** | ✅ | `runtime/session_store.go` + `session_clone.go` — approval 场景 session 持久化 |

---

## 三、本轮新增重大结构改动

### 3.1 Capability 从 typed interface 升级为 Unified Handle

**旧设计（已废弃）：**
```go
type SearchCapability interface {
    Spec() Spec
    Invoke(ctx, SearchInput) (SearchOutput, error)
}
type FetchCapability interface { ... }
```

**新设计：**
```go
type Handle interface {
    Spec() Spec
    Invoke(ctx context.Context, req InvocationRequest) (InvocationResult, error)
}
```

`InvocationRequest` 携带 `SessionID + Snapshot + Input(any)`，`InvocationResult` 返回 `Output(any) + Delta + EvidenceRefs + Action + Observation + Status`。

**评价：这是一个关键的架构升级。** Capability 现在可以直接产出 `StateDelta` 和 `EvidenceRefs`，而不需要调用方手动构建。`InvocationResult.Action` 和 `Observation` 为 action-observation 可追溯性提供了结构化记录。统一的 `Handle` 接口让 Registry 从分类型 map 简化为单一 `map[string]Handle`。

### 3.2 RoleBindings — Pattern 与 Capability 的解耦

```go
type RoleBindings map[string]string // e.g. {"search": "web_search", "fetch": "web_fetch"}
```

Pattern 不再硬编码 capability 名称，而是通过 role 名称（如 `RoleSearch`、`RoleFetch`）间接引用。`ResolveBinding(registry, bindings, role)` 支持三种模式：
1. 显式绑定（bindings 中已指定）
2. 自动推导（registry 中唯一匹配该 role 的 capability）
3. 多候选报错（要求显式绑定消除歧义）

**评价：这是 pattern-capability 解耦的正确方向。** 同一个 reactive pattern 可以配置不同的 search/fetch 实现，只需要改变注册表中的 binding。

### 3.3 Selector → Resolver → Invoke 三级调用链

```
LLM Selector（选 capability）
  → Catalog（从 Registry 生成 LLM-facing Card 列表）
    → LLM 返回 CapabilitySelection{Name, Family, Role, Input}
      → Resolver.Match（name/family/role → 唯一 capability）
        → Resolver.Resolve（Match + NormalizeInput + ValidateInput）
          → Handle.Invoke（执行）
```

这条链路完整实现了"LLM 决定选哪个 capability → 运行时校验输入 → 执行"的闭环。`CapabilitySelection` 支持按 name/family/role/kind 四种维度选择，LLM 不需要精确知道 capability 名称。

### 3.4 Reactive Pattern 升级为真循环

**旧拓扑（单 pass）：**
```
START → prepare → search → fetch → observe → branch(answer|degrade)
```

**新拓扑（支持循环 + handoff + approval）：**
```
START → prepare → (search→fetch | external_evidence) → observe → branch
                                                                    ├─ answer → END
                                                                    ├─ handoff → END
                                                                    ├─ continue → (search→fetch | external_evidence) ↩
                                                                    ├─ approval → [branch(execute|degrade)] → END
                                                                    └─ degrade → END
```

**关键变化：**

1. **Continue 节点** 使 loop 成为事实 — 不再是单 pass
2. **Handoff 分支** 将结果投影为 handoff.Result 而非直接返回 Response
3. **Approval 分支** 在 capability.RequiresApproval=true 时触发中断-等待-恢复流程
4. **External Evidence Workflow** 作为 search→fetch 的替代执行路径（通过 `PreferExternalEvidenceWorkflow` 配置切换）

### 3.5 Plan-Execute Pattern 落地

```
START → build_plan → select_step → branch
                                     ├─ execute_step → assess_step → branch
                                     │                                ├─ select_step ↩
                                     │                                ├─ build_plan ↩ (replan)
                                     │                                └─ finalize → END
                                     ├─ approval → branch(execute|finalize) → END
                                     └─ finalize → END
```

这是设计文档 Pattern B（Plan-Execute）的完整实现。`build_plan` 节点使用 LLM Selector 生成显式 plan（`PlanState{Steps}`），`select_step` 按序选择下一步，`execute_step` 通过 `Resolver.Resolve` → `Handle.Invoke` 执行，`assess_step` 做结果评估并决定继续/重规划/结束。

### 3.6 State 扩展至 7 域

```go
type StateSnapshot struct {
    Request   RequestState      // 不可变请求上下文 + RuntimeOptions(含 OutputMode)
    Context   ContextState      // 执行中间产物 + SeenURLs + PreferredURLs + AvoidURLs + ErrorClass
    Plan      PlanState         // build_plan 产出的 PlanSteps + 执行进度
    Evidence  EvidenceState     // 证据累积 + Sufficient + NewItemsThisRound
    Execution ExecutionState    // 控制流进度 + ContinueCount + NewURLCount + ProgressKind
    Approval  ApprovalState     // 审批门禁状态
    Answer    AnswerState       // 答案生成结果
}
```

**关键新增字段：**

| 域 | 新字段 | 用途 |
|---|---|---|
| Context | `SeenURLs`, `PreferredURLs`, `AvoidURLs` | 跨轮 URL 去重、偏好排序 |
| Context | `SearchErrorClass`, `FetchErrorClass` | 错误分类（供 Planner 判断是否可重试） |
| Plan | `PlanState{Steps, CurrentStep}` | Plan-Execute 模式专属 |
| Evidence | `NewItemsThisRound` | 本轮新增证据计数 |
| Execution | `ContinueCount`, `LastNewURLCount`, `LastNewEvidenceCount`, `ConsecutiveNoProgressRounds` | 循环进度追踪，供 Planner 判断 stop signals |
| Approval | `ApprovalState{Status, Node, Capability, CheckpointID}` | 完整审批状态机 |
| RuntimeOptions | `OutputMode` | 控制输出模式（final_answer vs handoff） |

---

## 四、当前仍存在的问题

### P1 — 结构性问题

**1. Continue 逻辑仅包含 Notes 写入**

`nodes_continue.go` 的 delta 只设置了一个 note + execution delta。它不修改 search query、不清除 fetch results、不重置任何状态——只是标记"继续循环"。这意味着第二轮 search 使用完全相同的 query 会得到相同结果。真正的"re-search with refined query"需要 Planner 在 observe 阶段产出新的 `SearchQuery`。

**2. `pattern/reactive` 和 `pattern/planexecute` 共享 AssemblyContext/RuntimeConfig 但实现方式不同**

reactive 通过 `RoleBindings` 获取 capability handle，planexecute 通过 `Selector→Resolver→Handle`。两者的装配代码存在重复（如 `mergeInterruptBeforeNodes`、`resolveBindings`）。建议提取到 `pattern/` 共享层。

**3. `external_evidence/capability.go` 的 `mergeContextDelta` 是自己实现的浅 merge**

它先复制 left 全部字段，再覆盖 right 的部分字段。这个逻辑与 `state/reducer.go` 的 Reducer 是重复的语义。如果 capability 产出的 delta 需要 merge，应该由 kernel 的 Reducer 统一承担，而不是每个 composite capability 自己写 merge 逻辑。

**4. `finalizeRejectedApproval` 直接写 `session.Snapshot`**

`service_resume.go:110-133` 在 approval 被拒绝时直接修改 `session.Snapshot.Execution.CurrentNode`、`session.Snapshot.Answer.Final` 等多处——绕过了 Reducer。这在第一版审查中已标记为问题，当前场景扩展到了 service 层。

### P2 — 远期关注

**5. `fetch/capability.go` 和 `search/capability.go` 与旧 service 并存**

`search/` 下同时存在 `service.go`（旧 service 模式）和 `capability.go`（新 Handle 包装）。`fetch/` 同理。两套路径都在使用，增加理解成本。

**6. `websearch/` / `webfetch/` 仍然是非主路径的 Eino tool wrapper**

不在任何 capability registry 中注册，也不被任何 pattern 使用。保留它们可能是因为有其他调用方（如旧 RAG tool 体系），但已不属于 agent 模块的核心资产。

**7. 缺少 `pattern/routing` 和 `pattern/chain`**

Anthropic 6 模式中，Routing 和 Prompt Chaining 是独立模式。当前 reactive 和 planexecute 覆盖了 Parallelization + Evaluator-Optimizer + Autonomous Agent 的核心语义，但简单的"意图分类→路由到不同处理链"还没有独立的 pattern 支持。

**8. Plan-Execute Pattern 的 Planner 集成尚未完成**

`planexecute/builder.go:38-46` 创建 `buildPlanNode` 时传入了 `CapabilityCatalogBuilder`、`CapabilitySelector`、`CapabilityResolver` —— 但当前 `buildPlanNode` 的实际实现需进一步确认是否已接入 LLMPlanner。如果 `build_plan` 目前仍是规则驱动，则发挥不出 Plan-Execute 的完整能力。

---

## 五、总结评估

### 整体评分

| 维度 | 评分 | 说明 |
|---|---|---|
| State 系统 | 🟢 优秀 | 7 域覆盖了 reactive + planexecute 的全部需求 |
| Capability 体系 | 🟢 优秀 | Unified Handle + RoleBindings + Selector/Resolver 三级链路，架构超前 |
| Kernel | 🟢 良好 | Builder/Runner/Journal/Checkpoint 稳定，错误分支绕过 Reducer 待修 |
| Reactive Pattern | 🟢 良好 | 已支持 continue 循环 + handoff + approval 5 路 branch |
| Plan-Execute Pattern | 🟡 可用 | 图拓扑完整，Planner 集成深度待验证 |
| Service 层 | 🟢 良好 | 错误体系 + Approval/Resume 流程完善，两层 Request 仍存 |
| Handoff 投影 | 🟢 良好 | EvidenceBundle + DecisionSummary + Replay 三件套完备 |
| 测试覆盖 | 🟢 良好 | 19 PASS，核心链路均有测试 |

### 当前架构的核心优势

1. **Pattern 与 Capability 完全解耦** — 同一个 reactive pattern 可以接入不同的 search/fetch 实现
2. **LLM-driven 能力选择** — Selector → Resolver → Invoke 三级链路，LLM 按 name/family/role/kind 选择
3. **Approval → Resume 完整闭环** — 中断→序列化→等待→恢复，经过测试验证
4. **两种执行范式并存** — Reactive（循环式）和 Plan-Execute（计划式），选择权在 RuntimeConfig
5. **Event Journal 完备** — 14 种 EventType 常量覆盖了全部运行时生命周期

### 当前最值得改进的三件事

1. **消除绕过 Reducer 的直接 snapshot 写入**（kernel/builder.go 错误分支 + service_resume.go rejection 路径）
2. **合并 reactive 和 planexecute 的共享代码**（`mergeInterruptBeforeNodes`、`resolveBindings`、`executionDelta` 构造）
3. **评估 websearch/webfetch 的去留** — 已完全不在 agent 主路径上
