# 07 Tool 与 Agent Loop 面试题整理

## 适用场景

适用于介绍下面这段经历时的技术面试准备：

> 搭建智能体工具运行框架，抽象查询、诊断、网页检索等工具的统一注册、执行与结果消费流程，并基于多轮 Plan-Act-Observe 决策机制完成问题定位与外部证据补充，增强复杂场景下的自主处理能力。

这份文档的目标不是背标准答案，而是帮助你把这段经历讲成一套完整、可信、可追问的工程故事。

---

## 一句话版本

建议先把这段经历压缩成一句总述，方便开场：

> 我负责把查询、诊断、网页检索等能力收敛成统一的 tool runtime，把工具的定义、注册、执行、结果语义消费串成一条完整链路；在此基础上用多轮 Plan-Act-Observe 机制让系统能持续下钻定位问题，或者在知识不足时自动补充外部证据。

---

## 面试官最可能问的 12 个核心问题

### 1. 你们为什么要做一套统一的 tool 框架？

**考察点**

- 你能不能讲清楚业务问题，而不是只讲技术抽象
- 你是否知道“工具多了以后不统一会发生什么”

**建议回答**

- 早期工具通常是散的，调用方式、参数格式、结果结构、后续消费方式都不一致。
- 一旦想做多轮 agent，就不只是“能调工具”这么简单，而是要统一：
  - 如何注册工具
  - 如何执行工具
  - 如何理解结果
  - 如何决定下一步
  - 如何把结果组织进最终回答
- 所以我们抽了统一 runtime，让查询类、诊断类、网页类工具都能走同一套生命周期。

**一句强化**

> 本质上是在把“工具调用”升级成“可被 Agent 编排和消费的运行时能力”。

### 2. 你们的最小抽象是什么？

**考察点**

- 你是否真的理解核心对象
- 你会不会把概念讲混

**建议回答**

- 执行层最小单元是 `Tool`，负责定义和调用。
- 当前主抽象是 `ToolModule`，它把三层能力绑在一起：
  - `Invoker`：怎么执行
  - `Spec`：能力域、风险、证据来源等元数据
  - `Behavior`：结果如何被 agent 消费
- 运行时通过 `Registry` 管理这些 module，再由 `Executor` 和 `AgentLoop` 共同消费。

**回答关键词**

- `Tool`
- `ToolModule`
- `Registry`
- `Executor`
- `Behavior`
- `AgentLoop`

### 3. 为什么需要 `Behavior`，不能只保留 tool 本身吗？

**考察点**

- 这是高频追问
- 你能不能讲出“执行”和“语义消费”的边界

**建议回答**

- 只保留 tool，系统只能做到“把工具跑起来”。
- 但 Agent 真正需要的是：
  - 结果够不够回答
  - 下一步是否继续
  - 下一步调用哪个工具
  - 结果怎么进入 prompt
  - 最终回答时如何使用这份证据
- 这些都不是 invoker 的职责，所以要用 `Behavior` 把“工具结果”翻译成“agent 能理解的控制信号和回答信号”。

**一句强化**

> `Invoker` 解决“怎么查”，`Behavior` 解决“查完意味着什么”。

### 4. 一个 tool 从定义到最终回答，完整链路是什么？

**考察点**

- 你是否真的做过，而不是只会讲概念
- 你能不能把调用链说完整

**建议回答**

用 `document_ingestion_diagnose` 或 `external_evidence_workflow` 举例，按顺序讲：

1. 定义 tool 的参数和执行逻辑
2. 给它配置 `Behavior`
3. 在 assembly 层注册成 `ToolModule`
4. `Registry` 保存 module
5. `Planner` 或 hint 选择这个 tool
6. `Executor` 执行 tool
7. `Observer` 结合 behavior 产出 `AgentState`
8. `AgentLoop` 决定继续还是停止
9. `RenderContext` 生成 tool context
10. `BuildGuidance` 生成 answer guidance
11. 最终进入 prompt，生成回答

**回答建议**

面试时最好只挑一个 tool，顺着讲到最终回答，不要列太多工具名字。

### 5. 你们的 Plan-Act-Observe 怎么运转？

**考察点**

- 你有没有真正设计过 agent runtime
- 状态是怎么流动的

**建议回答**

- `Plan`：根据问题、已有证据和当前状态决定本轮调用哪些 tool
- `Act`：执行工具，得到结构化 `Result`
- `Observe`：消费本轮结果，判断：
  - 是否收敛
  - 当前 hypothesis 是什么
  - 是否需要继续
  - 下一步 hint 是什么
- 每一轮的输出会进入 `AgentState`，下一轮继续使用

**一句强化**

> 它不是单轮 function calling，而是带状态的多轮证据收集与决策闭环。

### 6. 状态为什么重要？你们保存了什么状态？

**考察点**

- 你对多轮系统的理解深不深

**建议回答**

- 多轮系统里最怕每一轮都把上下文当成一张白纸重新推理。
- 所以我们保留 `AgentState`，里面会有：
  - 当前阶段 `phase`
  - 当前假设 `hypothesis`
  - 置信度 `confidence`
  - 已检查工具
  - 未解问题
  - `NextHintCalls`
- 这样下一轮 planning 不需要完全重算，而是能沿着上一轮观察结果继续下钻。

### 7. 为什么不是把所有决策都交给 LLM？

**考察点**

- 你是否理解 LLM 的边界
- 你有没有做成本和稳定性权衡

**建议回答**

- 全交给 LLM 的问题有三个：
  - 成本高
  - 不稳定
  - 容易幻觉
- 所以我们做的是混合决策：
  - 首轮或复杂开放问题可以交给 LLM planner / observer
  - 明确的结构化续推，优先走 rules 或 behavior 产出的 hint
  - 不合法 hint 或异常路径回退到规则观察器

**一句强化**

> LLM 负责高层推理，规则和 behavior 负责稳定续推与语义兜底。

### 8. 你们怎么减少多轮里的冗余 LLM 调用？

**考察点**

- 你是否做过真实优化，而不是只会抽象

**建议回答**

- 我们后来做了一次关键优化：
  - Round 1 仍允许 LLM 参与规划
  - Round 2 以后如果 observer 已产出 `NextHintCalls`
  - 就直接按 hint 继续，不再重复进入 `LLMPlanner`
- 这样把“首轮判断”和“后续机械下钻”分开，显著减少了多轮诊断链里的重复规划成本。

### 9. 如果 tool 执行成功了，但最终回答还是很差，你先查哪里？

**考察点**

- 排障思路
- 你是不是知道软故障比硬故障更难查

**建议回答**

优先排这四层：

1. `Result.Data` 是否缺关键字段
2. `Behavior.Observe / Next` 是否正确产出后续 hint
3. `RenderContext` 是否把关键信息写进 tool context
4. `BuildGuidance` 或最终 prompt 是否对答案施加了正确约束

**一句强化**

> 这类问题很多不是“tool 没跑”，而是“tool 跑了但结果没被 runtime 正确消费”。

### 10. 你们怎么做外部证据补充？

**考察点**

- 能不能把 RAG 和 tool agent 结合讲清楚

**建议回答**

- 当知识库证据不足，或者结构化查询无法直接收敛时，tool runtime 可以触发 `external_evidence_workflow`。
- 这个 workflow 内部会执行：
  - `web_search`
  - `web_fetch`
  - 结果评估
- 最终把来源、可用性、质量信息回灌到 tool result，再进入 prompt。

**一句强化**

> 外部搜索不是独立旁路，而是被纳入统一 tool workflow 的一部分。

### 11. 你们怎么做可观测性？

**考察点**

- 工程落地能力
- 你是否知道 agent 系统最难 debug

**建议回答**

- 每轮要能看见：
  - planning source
  - tool calls
  - round observation
  - state 演化
  - 是否跳过 LLM planner
- 最终还要记录：
  - round count
  - tool call count
  - degrade reason
  - 是否生成了 tool context 和 answer guidance

**回答建议**

如果面试官追问，直接举 `planningSource / llmPlannerSkipped / nextHintCallCount` 这类指标，会显得很落地。

### 12. 如果现在重做一遍，你最想改什么？

**考察点**

- 复盘能力
- 是否有架构判断力

**建议回答**

- 我不会马上推翻现有架构，而是会继续做收口：
  - 让首轮更多利用结构化路由，而不是默认强依赖 LLM
  - 补齐 `BuildGuidance` 在生产链路里的使用
  - 继续压缩集中式规则，让更多语义回到 module behavior

**一句强化**

> 当前架构主问题不是“不能用”，而是“还有一些入口太重、语义还没有完全模块化”。

---

## 面试官高频追问与回答策略

### 追问 1：统一注册到底统一了什么？

不要只答“统一了接口”，建议回答四层统一：

1. 工具定义统一
2. 执行入口统一
3. 结果结构统一
4. 结果消费方式统一

这样比只说“有 registry”更像真正做过架构。

### 追问 2：为什么不用纯函数调用链，而要做 agent loop？

建议回答：

- 纯函数调用链适合固定流程
- 但诊断和证据补充是条件分支很强、需要边查边判断的场景
- agent loop 的价值是“证据驱动的动态续推”

### 追问 3：为什么不把所有续推逻辑都写在 observer 里？

建议回答：

- observer 是运行时编排组件，不应该承担所有业务语义
- 如果所有续推都写在 observer 里，很快会变成一个中心化大 `switch`
- 把语义下沉到 `Behavior`，新 tool 才能按模块扩展

### 追问 4：你们怎么防止状态乱掉？

建议回答：

- 明确 `AgentState` 是多轮唯一状态载体
- observer 统一返回 `State`
- loop 只消费归一化后的 state
- 尽量避免多个平行字段互相同步

### 追问 5：你在这里做过的最关键优化是什么？

建议回答：

- 不用泛泛而谈“提升了稳定性”
- 直接说：
  - 我们把多轮中的 planner 冗余调用砍掉了
  - 后续轮次优先消费 `NextHintCalls`
  - 结果是复杂诊断链里减少了重复规划，且行为不回退

---

## 建议你重点准备的 3 个例子

### 例子 1：`doc_fail_01 为什么导入失败？`

**适合讲**

- 结构化诊断链
- 一跳收敛
- 诊断类 tool 如何直接产出结论

**回答重点**

- planner 选中诊断 tool
- tool 返回结构化诊断结果
- behavior 渲染出结论、事实和建议
- 最终 prompt 用这些结果生成答案

### 例子 2：`doc_run_01 现在跑到哪个节点了？`

**适合讲**

- 多轮 Plan-Act-Observe
- `NextHintCalls`
- planner 短路优化

**回答重点**

- Round 1 先做浅层查询
- observer 判断证据不够，产出下一步 hint
- Round 2、3 直接按 hint 下钻
- 最终收敛到 task/node 级状态

### 例子 3：`Go 泛型怎么用？`

**适合讲**

- 知识库不足时如何触发外部证据工作流
- 外部证据如何纳入统一 tool 体系

**回答重点**

- 首轮路由到 `external_evidence_workflow`
- 内部走 `web_search -> web_fetch -> assess`
- 结果经过 context render 后进入最终 prompt
- 回答不仅给知识点，还能带来源和证据质量

---

## 一段建议背熟的完整回答

下面这段适合在面试中作为 2 分钟版本回答：

> 我做的这块不是单纯加几个 function calling，而是搭了一套完整的 tool runtime。核心思路是把工具分成执行、元数据和结果语义三层：执行层负责真实取数，元数据层描述能力域和风险，行为层负责把结果翻译成 agent 能理解的状态、下一步 hint、上下文和回答指导。  
> 在运行时上，我们用统一的 registry 和 executor 管理工具，再在外层包一层 Plan-Act-Observe 的 agent loop。这样系统面对文档诊断、任务追踪、网页检索这类问题时，不是只调一次工具，而是能根据上一轮证据决定下一轮继续查什么。  
> 我们后来还做了一次关键优化，把多轮里的重复 planner 调用压掉了：如果 observer 已经产出结构化 `NextHintCalls`，后续轮次就直接续推，不再重复调用 LLM planner。这样既保留了首轮推理能力，也控制住了复杂场景下的成本和不稳定性。  
> 最后工具结果不会停留在 runtime 内部，而是会通过 `RenderContext` 和 `AnswerGuidance` 进入 prompt，让最终回答真正建立在诊断证据和外部来源之上。

---

## 面试时最容易讲差的地方

### 1. 只讲概念，不讲真实链路

不要只说：

- 我们有 planner
- 我们有 observer
- 我们有 tool registry

要尽量落到一条具体链路，比如：

`doc_run_01 -> document_query -> document_ingestion_diagnose -> ingestion_task_query`

### 2. 只讲调用，不讲结果消费

很多人会把重点全放在“如何调工具”，但真正高级的点在：

- tool 结果如何进入 state
- 如何触发下一轮
- 如何进入 prompt
- 如何约束最终回答

### 3. 只讲 LLM，很少讲规则和稳定性

如果你把所有功劳都说成“模型自己做的”，面试官通常会觉得你没有工程控制力。

更好的讲法是：

- 高层推理交给 LLM
- 稳定续推和兜底交给规则与 behavior
- 两者共同组成可控的 agent runtime

### 4. 把框架说成万能智能体

不要夸大成“它可以自主解决所有复杂问题”。更可信的说法是：

- 它擅长结构化诊断、多轮下钻和证据补充
- 边界外的问题仍要依赖 fallback 和人工设计

---

## 最后 5 个必练题

如果时间有限，至少把下面 5 题练熟：

1. 为什么需要 `Behavior`？
2. 一个 tool 从注册到最终回答的完整链路是什么？
3. Plan-Act-Observe 里状态是怎么流动的？
4. 你们为什么不用全 LLM 决策？
5. 你做过的最关键优化是什么？

这 5 题一旦答顺了，这个模块基本就能撑住一轮深入面试。

