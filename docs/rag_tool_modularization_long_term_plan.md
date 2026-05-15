# RAG Tool 模块化长期重构路线图

## Summary

目标是把当前 `tool / agent` 架构从“中央按 tool 名字硬编码语义”，重构为“模块自描述、注册驱动、行为可插拔”的可扩展模型。完成后，新增一个外部 tool 只需要新增一个模块并注册，不再修改中央调度、观察、trace 归因、结果解码或回答引导逻辑。

这条路线是扩展性优先，不以最小改动为目标。允许阶段性兼容层，但最终目标是让中央框架不再持有任何具体 tool 的私有知识。

## Long-Term Architecture

### 1. 统一扩展单元：`ToolModule`

不再把 `Tool` 视为完整扩展边界，新增统一模块模型：

- `ToolInvoker`
  只负责执行一次调用。
- `ToolSpec`
  负责静态语义：
  - `Definition`
  - `Capability`
  - `EvidenceSources`
  - `RiskLevel`
  - `ApprovalRequirement`
  - `ReadOnly`
  - `Family`
- `ToolBehavior`
  负责动态语义：
  - `Decode`
  - `Next`
  - `Observe`
  - `RenderContext`
  - `BuildGuidance`
- `ToolModule`
  作为唯一注册对象，包含 `Invoker + Spec + Behavior`

实现要求：

- 新增任何 tool 时，必须注册为 `ToolModule`
- 不允许新增“只实现 Tool，不声明 Spec/Behavior”的新形态
- 老 tool 迁移期间可通过 `LegacyToolAdapter` 兼容，但 adapter 不是长期接口

### 2. 结果对象升级：`Result.Meta`

扩展 `Result`，让执行语义随结果流动，而不是由后置逻辑推断：

- `Capability`
- `EvidenceSources`
- `RiskLevel`
- `ApprovalRequirement`
- `ReadOnly`
- `Family`
- `Terminal`
- `Retryable`

规则：

- `Executor` 在执行完成后统一写入 `Result.Meta`
- `WorkflowControl`、trace、observer、renderer、guidance 一律消费 `Result.Meta`
- 禁止继续在中央代码里通过 `result.Name` 推断 capability/evidence

### 3. Registry 升级为 `ModuleRegistry`

新增模块注册中心，替换当前只存 `Tool` 的 registry：

- `Register(module ToolModule) error`
- `MustRegister(module ToolModule)`
- `GetModule(name string) (ToolModule, bool)`
- `ListDefinitions() []Definition`
- `GetBehavior(name string) (ToolBehavior, bool)`
- `GetSpec(name string) (ToolSpec, bool)`

规则：

- planner 仍通过 `Definition` 工作，不直接依赖 behavior
- executor、observer、renderer、trace 聚合器都从 registry 取模块
- 不再以 `registry.Get(name) -> Tool` 作为主路径

### 4. 去中央硬编码

以下中央文件降级为通用 fallback，不再承载具体 tool 语义：

- `internal/app/rag/tool/next_action.go`
- `internal/app/rag/tool/observer_rule.go`
- `internal/app/rag/tool/workflow_control.go`

主路径改为：

- `Next`：`module.Behavior.Next(...)`
- `Observe`：`module.Behavior.Observe(...)`
- capability/evidence 聚合：`Result.Meta`
- context/guidance：`module.Behavior.RenderContext(...)` / `BuildGuidance(...)`

fallback 仅处理：

- 模块缺失
- 未实现某行为
- 空结果
- 达到最大轮次
- 异常降级

### 5. typed result view 模块内聚

不再继续在 core 层堆 `ViewXxxResult()` 的名字分发。复杂结果由各模块自行提供 decoder 和 concrete view。

规则：

- 每个复杂 tool 必须实现 `Decode`
- `renderer / observer / guidance / graph tool` 优先使用 decode 结果
- `map[string]any` 只作为底层载体，不再作为上层长期消费接口

### 6. 工具家族化

重构后目录按“模块家族”组织，而不是全部平铺：

- `tool/core`
  放 `Result / ToolSpec / ToolBehavior / ToolModule / ModuleRegistry`
- `tool/runtime`
  放 `Executor / Observer / TraceAggregator / PlannerBridge`
- `tool/modules/web`
  放 `web_search / web_fetch / external_evidence_workflow`
- `tool/modules/system`
  放 `document_query / task_query / list / diagnose`
- `tool/modules/github`
  预留 GitHub readonly 家族
- `tool/modules/api`
  预留 JSON API readonly 家族
- `tool/modules/db`
  预留领域化 DB readonly 家族

目标是让“新增一类外部 tool”变成新增一个模块家族，而不是继续膨胀 core。

## Staged Execution Plan

### Phase 1. Foundation

先建立新 core，不迁具体业务语义。

实现内容：

- 新增 `ToolModule / ToolSpec / ToolBehavior / ResultMeta / ModuleRegistry`
- 重写 `Executor` 支持 module 执行并注入 `Result.Meta`
- 增加 `LegacyToolAdapter`
- 保持 `WorkflowInput / WorkflowResult / ToolCallEvent` 的外部形状尽量稳定
- `buildLocalToolWorkflow(...)` 改为注册 module，而不是直接注册 tool

验收标准：

- 旧 tool 通过 adapter 正常运行
- 现有 planner 与 agent loop 不退化
- 当前 `tool` 基础测试通过

### Phase 2. 迁移 web 工具链做样板

优先迁移这三个模块：

- `web_search`
- `web_fetch`
- `external_evidence_workflow`

实现内容：

- 各自提供完整 `ToolModule`
- 各自实现 `Decode / Next / Observe / RenderContext / BuildGuidance`
- 将 web 相关 typed view 从 core 层迁到模块层
- 中央 observer/next-action 不再特殊识别 web 工具名

验收标准：

- `web_search -> web_fetch` 串联由模块 behavior 驱动
- `external_evidence_workflow` 自身完成 terminal 收口
- 删除中央 web 名字分支后，链路仍工作

### Phase 3. 中央编排瘦身

把核心框架改成只做“通用编排”。

实现内容：

- `AgentLoop` 的下一步决策改为调用 behavior
- `RuleObserver` 改为编排器，不再维护具体 tool 分支
- `workflow_control` 改为聚合 `Result.Meta`
- `renderer / answer_guidance` 主路径改为调用 behavior

验收标准：

- 中央文件不再新增具体 tool 分支
- capability/evidence trace 全部来自 `Result.Meta`
- fallback 仅覆盖未迁移或异常场景

### Phase 4. 迁移系统内工具家族

按家族迁移，不按文件散打。

顺序固定为：

1. `document_query / document_chunk_log_query / document_list`
2. `ingestion_task_query / ingestion_task_node_query / task_list`
3. `document_ingestion_diagnose / task_ingestion_diagnose / trace_retrieval_diagnose / trace_node_query`
4. `think`
5. graph 工具：`document_root_cause_diagnosis / document_diagnose_with_search`

规则：

- 每个家族迁完后，对应中央名字分支必须删掉
- graph 工具通过 module executor 调用底层模块，不直接依赖 raw map 结构

### Phase 5. 接入新外部工具家族

在模块化底座稳定后，再接新外部 tool。

首批家族固定为：

- `api/*`
  先做 readonly JSON API 工具
- `github/*`
  先做 repo / issue / release / file readonly 工具
- `db/*`
  只做领域化 readonly 查询，不开放通用 SQL

新增外部 tool 的强约束：

- 必须注册为 `ToolModule`
- 必须显式声明 `ToolSpec`
- 必须实现 `Decode`
- 存在后续链路时必须实现 `Next`
- 复杂结果必须实现 `RenderContext` 和 `BuildGuidance`
- 禁止通过修改中央 `switch` 来支持新工具

## Important Interface Changes

### 新增核心类型

- `ToolModule`
- `ToolSpec`
- `ToolBehavior`
- `ToolInvoker`
- `ModuleRegistry`
- `ResultMeta`
- `NextDecision`
- `GuidanceInput`
- `GuidanceNote`

### 兼容层

- 新增 `LegacyToolAdapter`
- 迁移期允许旧 `Tool` 通过 adapter 注册为 module
- adapter 删除条件：
  - 所有现有 tool 已迁移为原生 module
  - 中央 fallback 不再依赖旧 tool 行为

### 外部可见行为约束

- planner 继续通过 `Definition` 感知工具
- SSE / trace 外部结构尽量维持兼容，但内部来源改为 `Result.Meta`
- `WorkflowResult.Control / TraceMeta` 的语义来源从“推断”改为“执行结果聚合”

## Test Plan

### Core tests

- `ModuleRegistry` 的注册、查找、去重、Definition 列表
- `Executor` 将 `ToolSpec` 正确注入 `Result.Meta`
- `LegacyToolAdapter` 兼容旧 tool 行为
- 未实现 behavior 的模块走 fallback，不 panic

### Behavior tests

- `web_search` 的 `Decode / Next / Observe`
- `web_fetch` 的 terminal 行为
- `external_evidence_workflow` 的 readiness/quality 收口
- 新增一个模拟模块，在不改中央代码前提下完成注册和运行

### Workflow tests

- `AgentLoop` 在 module 模型下仍支持并行 tool calls
- `RuleObserver` 不依赖名字分支也能驱动 web 工具链
- trace 中 capability/evidence 来源于 `Result.Meta`
- graph 工具通过 module executor 调用下层模块后，结果仍可被上层消费

### Acceptance cases

- 新增一个 `api_fetch` 模块，只改模块包和注册入口即可运行
- 新增一个 `github_issue_search` 模块，只改模块包和注册入口即可运行
- 删除某具体 tool 的中央名字分支后，原功能不退化

## First Execution Tranche

落地顺序固定如下，后续每次开新对话默认从这里继续：

1. 先做 `Phase 1 Foundation`
   目标是不改业务语义，只建立 `ToolModule` 和 `Result.Meta` 底座。
2. 接着立刻做 `Phase 2 web 工具链迁移`
   因为 web 家族最接近后续外部 tool 扩展目标，最能验证模块化价值。
3. 完成后再进入 `Phase 3 中央编排瘦身`
   这一阶段不再追求兼容旧习惯，而是开始真正删除中心化名字分支。

默认执行策略：

- 每次只完整收口一个 phase，不跨 phase 混做
- 每个 phase 结束必须补齐测试，并删掉对应旧分支
- 不允许“新旧两套主路径长期并存”

## Assumptions

- 该路线以可扩展性优先，不以最小风险为首要目标。
- 近期不会引入真正的交互式审批系统，`ApprovalRequirement` 先作为强语义元数据保留。
- 新外部 DB 工具不开放自由 SQL，只做领域化 readonly。
- planner 暂不升级为直接理解 `Behavior`，保持其与 `Definition` 对接即可。
- 文档创建完成后，未来新对话默认以本路线图为上位约束，不再回到“按 tool 名字中央硬编码”的方向。

## Execution Status Update

### Updated On

- 2026-05-13

### Current Status By Phase

#### Phase 1 Foundation

- Status: completed
- Landed:
  - `ToolModule / ToolSpec / ToolBehavior / ToolInvoker / ResultMeta / ModuleRegistry`
  - module-based `Executor`
  - module-based runtime registration
  - legacy `Tool` compatibility through `LegacyToolAdapter`

#### Phase 2 Web 工具链迁移

- Status: completed
- Landed:
  - `web_search`
  - `web_fetch`
  - `external_evidence_workflow`
- These tools already own:
  - `Decode`
  - `Next`
  - `Observe`
  - `RenderContext`
  - `BuildGuidance`

#### Phase 3 中央编排瘦身

- Status: completed
- Landed:
  - `AgentLoop` next-step planning is registry-aware
  - `RuleObserver` is module-first
  - `RenderContext` and `AnswerGuidance` are module-first
  - `workflow_control` prefers `Result.Meta`
  - legacy compatibility for `next_action.go` now routes through inferred behavior instead of central name branching
  - workflow control fallback now resolves capability / evidence / execution metadata through legacy module spec inference

#### Phase 4 系统内工具家族迁移

- Status: completed
- Migrated families:
  - `document_query / document_chunk_log_query / document_list`
  - `ingestion_task_query / ingestion_task_node_query / task_list`
  - `document_ingestion_diagnose / task_ingestion_diagnose / trace_retrieval_diagnose / trace_node_query`
  - `think`
  - `document_root_cause_diagnosis / document_diagnose_with_search`
- Additional compatibility landed:
  - legacy `MustRegister(tool)` can now auto-infer known behavior for migrated tool families
  - `RuleObserver` now falls back to inferred legacy behavior before generic completion
  - `planCallsFromResults(...)` no longer keeps a separate `web_search` special case and instead reuses inferred behavior fallback
  - `deriveWorkflowControl(...)` and `buildWorkflowTraceMeta(...)` no longer keep tool-name-specific capability / evidence branching in the central file

#### Phase 5 接入新外部工具家族

- Status: not started
- Recommended start order remains:
  - `api/*`
  - `github/*`
  - `db/*`

### What Changed The Most

1. The architecture has crossed the “foundation only” stage.
2. The main path is already module-first for web, system diagnosis/query, trace, think, and graph families.
3. Central hardcoded branches are no longer the expansion path; they are now compatibility fallback.

### Recommended Next Work

1. Start the first truly new external family under the module model, preferably `api/*`.
2. Add one acceptance-grade external family (`api/*` or `github/*`) without touching central orchestration code.
3. Revisit physical folder reorganization only if module count makes the current package layout hard to navigate.
