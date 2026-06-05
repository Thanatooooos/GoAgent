# Agent Pattern Design

Date: `2026-06-04`

## 目的

这份文档用于对齐当前 agent runtime 中 `pattern`、`capability`、`runtime`
三层的职责边界，并作为下一步实现 `plan-execute` 与 `reactive` 收口时的
工作基线。

当前结论只有一条：

- `pattern` 应该定义执行骨架
- `capability` 应该定义可调用能力
- `runtime` 应该定义生命周期与对外契约

这三者需要协作，但不应该互相吞并职责。

## 设计原则

### 1. Pattern 不是 capability 列表

Pattern 不应该预先写死“本次要调用哪些 capability”。

Pattern 应该只知道：

- 当前目标是什么
- 当前 runtime 上下文是什么
- 当前可用 capability 集合是什么
- 计划、执行、评估、终止的骨架是什么

然后由 pattern 内部节点根据目标和 capability contract 决定后续步骤。

### 2. Capability 不是 pattern 私有实现

Capability 不应该为了适配某个 pattern，退化成一组只能被固定节点调用的私有函数。

Capability 至少需要稳定表达：

- `name`
- `kind`
- `family`
- `role`
- `input schema`
- `requires approval`
- `supports resume`
- `produces evidence`

Pattern 可以依赖这些稳定元信息，但不应该依赖 capability 的具体实现类。

### 3. Runtime 才是统一底座

以下能力应属于 runtime，而不是分散到各 pattern 自己实现：

- approval
- checkpoint
- resume
- runtime event
- reducer merge
- service error
- degraded / completed / awaiting approval 对外状态

Pattern 可以触发这些机制，但不应各写一套外部契约。

## Reactive 的理想形态

`reactive` 不是万能 pattern。

它的职责应该非常明确：

- 围绕目标持续收集证据
- 根据证据充分性决定继续、回答、审批或降级

它适合的问题类型：

- 需要搜索外部信息
- 需要拉取正文
- 需要多轮补证据
- 需要基于证据是否充分来决定下一步

它的理想骨架应该是：

`prepare -> collect evidence -> observe -> continue | answer | approval | degrade`

这里的 `collect evidence` 可以由不同 capability 实现：

- `search + fetch`
- `external_evidence_collect`
- 未来其他高层 evidence capability

因此，reactive 应该保留领域假设，但不应写死某个具体能力实现名。

### Reactive 应保留的耦合

- 面向证据收集场景
- 面向 evidence sufficiency 的分支决策
- 可以保留固定 loop 结构

### Reactive 不该保留的耦合

- 不该写死 capability 实现类
- 不该把 capability 名称作为主要行为判断条件
- 不该自己维护 approval / checkpoint / replay 契约

## Plan-Execute 的理想形态

`plan-execute` 的目标不是“另一条 search/fetch workflow”，而是：

- 先根据目标和 capability 集合形成显式计划
- 再逐步执行计划
- 再根据步骤结果决定继续、重规划、审批或结束

最小骨架建议统一为：

`plan -> resolve -> execute -> assess -> replan | end`

其中：

- `plan`
  - 输入：目标、上下文、capability catalog
  - 输出：显式 plan steps
- `resolve`
  - 输入：当前 step
  - 输出：具体 capability handle 与 normalized input
- `execute`
  - 执行当前 step
- `assess`
  - 判断是否完成、是否继续、是否需要 replan、是否需要 approval

### 核心要求

`plan` 节点必须是 capability-aware 的，但不是 capability-hardcoded 的。

也就是说：

- 它应该看到当前有哪些 capability
- 再对照当前目标决定计划中放哪些 step
- 而不是默认永远产出固定步骤模板

### PlanStep 最小稳定字段

`PlanStep` 至少应包含：

- `step_id`
- `title`
- `capability_name`
- `capability_kind`
- `capability_family`
- `capability_role`
- `capability_input`
- `depends_on`
- `requires_approval`
- `expected_evidence`

其中：

- `capability_name` 用于最终执行定位
- `kind / family / role` 用于 plan 与 assess 保持语义稳定
- `input` 是 step 到 capability 的执行边界

### Plan-Execute 应保留的耦合

- 固定的计划驱动执行骨架
- 需要显式 step state
- 需要 assess/replan 机制

### Plan-Execute 不该保留的耦合

- 不该默认长期绑定 `search -> fetch`
- 不该主要依赖 `CapabilityName == xxx` 决定评估逻辑
- 不该把某一类 capability 当成唯一主路径

## Pattern 与 Capability 的关系

推荐采用以下关系模型：

- pattern 负责“怎么组织执行”
- capability 负责“能执行什么”
- selector / planner 负责“为了目标应该选什么能力”
- resolver 负责“把 step 映射到具体 capability”

也就是说：

- pattern 不是 capability registry
- capability 不是 graph node
- resolver 不是 planner

三者要明确分层。

## Pattern Routing 建议

当前不建议直接让 LLM 自由选择 pattern。

建议路线：

### 阶段一：配置或规则指定

由系统规则选择 pattern，例如：

- 证据补充型问题 -> `reactive`
- 显式任务拆解型问题 -> `plan-execute`

### 阶段二：系统自动路由

将规则路由沉到独立 router 层，统一决定 pattern。

### 阶段三：LLM 辅助路由

允许 LLM 提供 pattern 建议，但仍由系统规则做约束和兜底。

不建议一开始就完全让 LLM 决定 pattern，因为 pattern 选错的代价高于
capability 选错。

## 对当前实现的收口目标

### Reactive

- 保留证据循环语义
- 减少对具体 capability name 的依赖
- 允许高层 evidence capability 成为正式主路径

### Plan-Execute

- 让 selector-driven plan 成为主路径
- 让 `assess` 更多依赖 step result 与 capability contract
- 将模板化 `search -> fetch` plan 降为 fallback

### Runtime

- 继续统一 approval / resume / checkpoint / event / service error
- 让前端和上层服务不需要感知底层具体 pattern

## 当前推荐的工程方向

下一阶段优先做三件事：

1. 收敛 `plan-execute`，让 `plan` 真正基于目标和 capability 集合生成步骤
2. 收敛 `plan-execute assess`，减少对具体 capability name 的特判
3. 收敛 `reactive observe`，让其主要依赖 evidence contract 而不是能力名

## 结论

理想状态不是让所有 pattern 都变成万能框架。

理想状态是：

- `reactive` 做好证据循环
- `plan-execute` 做好任务分解与步骤执行
- `capability` 做好统一执行单元
- `runtime` 做好统一生命周期底座

最终目标不是“抽象得最漂亮”，而是：

- pattern 专用
- capability 稳定
- runtime 统一
- 上层容易接入和演进
