# Agent 架构复盘 & 检索模式简化 & Eino 接入评估

日期：2026-05-11

---

## 一、Agent 架构现状评估

### 1.1 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     AgentLoop (agent_loop.go)                │
│                  Plan → Act → Observe 多轮循环               │
├──────────────┬────────────────┬─────────────────────────────┤
│   Planner    │   Executor     │   Observer                  │
│  (planner/)  │ (executor.go)  │ (observer_llm.go) 主        │
│  LLMPlanner  │ + Registry     │ (observer_rule.go) 兜底     │
├──────────────┴────────────────┴─────────────────────────────┤
│              10 个 Builtin Tools (builtin/)                   │
│  诊断 ×3 | 查询 ×5 | 发现 ×2 | 外部 ×1 | 元 ×1              │
├─────────────────────────────────────────────────────────────┤
│  工具抽象层 (tool.go / registry.go / executor.go / workflow.go)│
├─────────────────────────────────────────────────────────────┤
│  输出层 (renderer.go / answer_guidance.go / result_summary.go) │
├─────────────────────────────────────────────────────────────┤
│  辅助层 (workflow_helpers.go / args.go)                       │
└─────────────────────────────────────────────────────────────┘
```

### 1.2 做得好的部分

#### 双层防护网

```
          Planner                Observer
          ───────                ────────
主路径:   LLMPlanner              LLMObserver
          │ 失败/空结果            │ 失败/校验不过
          ▼                       ▼
兜底:     planWithRules()         RuleObserver
```

LLM 输出的不可靠性被充分防御。证据白名单校验（`validateCallAgainstEvidence` / `validateHintAgainstEvidence`）是核心安全网：LLM 规划的 Call 或 Observer 输出的 Hint 中的任意 ID 参数必须在已有证据中出现过，否则拒绝。**这是 Agent 在生产环境中最关键的防线。**

#### 接口边界清晰

```go
Tool     interface { Definition(); Invoke() }       // 工具是什么
Workflow interface { Run() }                       // 怎么跑
Planner  interface { Plan() }                      // 跑什么
Observer interface { Observe() }                   // 停还是继续
```

四层接口各司其职。Chat 主链路只依赖 `Workflow` 接口，完全不知道内部是 AgentLoop。外部可替换整个 Agent 实现而不影响上游。

#### 确定性下钻链路

`planCallsFromResults()` + `RuleObserver` 的规则状态机保证了标准故障排查场景的稳定三跳：

```
document_ingestion_diagnose → ingestion_task_query → ingestion_task_node_query
```

不依赖 LLM 概率决策，保证了 `doc_fail_01` 场景下每次都能收敛到节点级根因。这对生产环境的一致性体验至关重要。

#### 并行执行 + SSE 时序保持

```
并行: goroutine 并发调用 Executor.Execute()（信号量限并发）
时序: tool_start 按规划顺序发送, tool_result 也按规划顺序发送
```

wall clock 下降可观（80ms → 40ms），且前后端不需要额外排序逻辑。

### 1.3 存在的结构性问题

#### 问题 1：通用引擎与领域编排器耦合在同一个包

```
internal/app/rag/tool/
  tool.go              ← 通用: Tool 接口
  registry.go          ← 通用: 注册表
  executor.go          ← 通用: 执行器
  agent_loop.go        ← 通用+专用混合: AgentLoop 引擎 + 诊断领域规则
  observer_rule.go     ← 专用: 诊断工具的状态机
  observer_llm.go      ← 通用+专用: LLMObserver 引擎 + 诊断领域 few-shot
  answer_guidance.go   ← 专用: 诊断结果 → 回答引导
  planner/planner.go   ← 通用: LLMPlanner
  builtin/             ← 专用: 10 个诊断/查询/发现工具
```

如果将来要构建第二个 Agent 场景（如代码审查 Agent），能复用的只有 `tool.go / registry.go / executor.go / planner.go` 四个文件。`agent_loop.go` 因嵌入了 100+ 行诊断专用关键词规则，无法直接复用。

#### 问题 2：planCallsFromResults 和 RuleObserver 的状态机逻辑重复

两边都在做 "result 类型 → next action" 的映射，判断条件几乎相同但输出格式不同（一个返回 `[]Call`，一个返回 `ObserveResult`）。具体重复面：

```
planCallsFromResults (agent_loop.go:532-592):
  document_query → document_ingestion_diagnose
  document_chunk_log_query → ingestion_task_query / document_ingestion_diagnose
  document_ingestion_diagnose → ingestion_task_node_query / ingestion_task_query
  ingestion_task_query → ingestion_task_node_query / task_ingestion_diagnose
  task_ingestion_diagnose → ingestion_task_node_query / ingestion_task_query

RuleObserver (observer_rule.go):
  observeDocumentDiagnosis   → 与上面第3条重叠
  observeTaskDiagnosis       → 与上面第5条重叠
  observeDocumentQuery       → 与上面第1条重叠
  observeChunkLogQuery       → 与上面第2条重叠
  observeTaskQuery           → 与上面第4条重叠
```

**两边共享约 317 行重复逻辑。** 改一处忘改另一处会引入静默 bug——比如修改了诊断工具的返回字段后只更新了 Observer 忘了更新 planCallsFromResults。

#### 问题 3：工具结果字段的隐式契约

`latestTaskId`, `latestNodeId`, `latestNodeError`, `latestLogError`, `taskNodeSummary`, `conclusion`, `confidence` 等字段没有类型约束——全部是 `map[string]any` 中的字符串 key。影响面统计：

```
readDataString / readDataInt 调用分布:
  agent_loop.go      → 19 处
  observer_rule.go   → 35 处
  answer_guidance.go → 21 处
  observer_llm.go    → 1 处
  result_summary.go  → 2 处
  query_tools_test.go→ 23 处 result.Data["key"].(type) 直接断言
  合计: 101 处
```

新加一个工具需要在 5 个地方同步修改：

1. 实现 `Tool.Invoke()`
2. `planWithBaseRules` 添加首轮触发规则
3. `planCallsFromResults` 添加下钻规则
4. `RuleObserver` 添加终止/继续规则
5. `answer_guidance.go` 添加结论提取逻辑

#### 问题 4：Observer 输出与 AgentState 的状态合并逻辑脆弱

`agent_loop.go:148-169` 有 20 行字段级补丁逻辑，在修补 "LLM JSON 可能返回部分字段" 的问题。根本原因是 `AgentState` 没有统一的写入路径——LLM 返回的 JSON 可能有 state block，也可能只有顶层字段，可能 partial，可能完整。

### 1.4 严重程度评估

| 问题 | 当前影响 | 未来风险 |
|---|---|---|
| 引擎/编排混合 | 低（只有一个 Agent） | 高（做第二个场景时大量重复） |
| 双层状态机重复 | 低（逻辑目前同步） | 高（改一处忘一处 = 静默 bug） |
| 隐式字段契约 | 中（新工具靠记忆） | 高（工具多了必然断裂） |
| 状态合并复杂 | 低（LLM 质量在改善） | 中（边界靠补丁累积） |

### 1.5 总体判断

> 对当前 10 个工具 + 诊断为主的使用场景来说，结构够用且防御充分。最大的技术债务不是代码写错了，而是"加一个新工具需要改 5 个文件"的隐性开发成本。

---

## 二、检索模式决策评估 & 简化方案

### 2.1 当前决策机制

`AnalyzeSearchMode()` 在 `search_mode_decision.go` 中实现了三层启发式打分：

**第一层：8 组关键词模式匹配（基础分）**

| 组 | 目标 mode | 权重 | 覆盖场景 |
|---|---|---|---|
| path_or_code_token | hybrid | 5 | 代码/路径片段 |
| error_or_runtime | hybrid | 5 | 故障排查 |
| config_or_api | hybrid | 4 | 技术组件查询 |
| event_or_protocol | hybrid | 4 | 协议/事件 |
| exact_match_intent | keyword | 4 | 精确查找意图 |
| quoted_phrase | keyword | 4 | 引号精确匹配 |
| concept_question | semantic | 4 | 概念理解 |
| architecture_flow | semantic | 3 | 架构流程 |

**第二层：6 个形态检测器（调整分）**

| 检测器 | 效果 | 权重 |
|---|---|---|
| `looksLikeFileNameLookup` | keyword +10 | 10 |
| `looksLikeSectionLookup` | keyword +5 | 5 |
| `looksLikeResourceIDLookup` | keyword +4 | 4 |
| `looksLikeCodeSymbol` | hybrid +4 | 4 |
| `looksLikeIdentifierLookup` | keyword +3 | 3 |
| `hasExactLookupShape` | keyword +2 | 2 |
| `looksLikeNaturalQuestion` | semantic +2 | 2 |

**第三层：最终裁决**

```go
if hybridScore >= 4 && hybridScore >= keywordScore && hybridScore >= semanticScore:
    → hybrid
else if keywordScore >= 4 && keywordScore > hybridScore:
    → keyword
else if semanticScore > 0:
    → semantic
else:
    → semantic (兜底)
```

### 2.2 偏差分析

**偏差 1：hybrid 结构性优势。** 4 组 hybrid 模式覆盖了大部分技术问题，基础分 4-5。semantic/keyword 要从 0 开始追到 ≥4 且严格大于 hybrid。一个"为什么配置报错"的概念性问题会被判为 hybrid——因为"配置"和"报错"各命中了一个 hybrid 模式组，得分 9:0:0。

**偏差 2：`looksLikeFileNameLookup` 权重 10 过高。** 一个检测器的分数超过所有 8 个模式组的最大可能基础分，keyword 必然胜出。

**偏差 3：普通术语查找无法触发 keyword。** "查找部署文档中关于环境变量的内容"——没有引号、没有文件扩展名、没有章节关键词、没有标识符形状 → keyword=0, hybrid=0, semantic=0 → semantic 兜底。但用户意图显然是关键词查找。

**偏差 4：`matchesAnyToken` 是子串匹配而非分词匹配。** 当前模式组中的 token 大多有足够的上下文特征（`.go`、`报错`、`::`），风险可控但需要注意未来加短词时的误匹配。

**偏差 5：没有查询长度感知。** "SQL" 和 "为什么 SQL 报错的原因是什么" 走同一套逻辑，但前者更适合 keyword，后者更适合 semantic。

### 2.3 评估结论

**hybrid 模式下 3 条通道全部启用，已经是事实上的默认行为。** semantic 和 keyword 模式在 auto 下几乎不会被选中。模式决策代码（170 行）的存在价值很低。

### 2.4 关键洞察：RRF 融合的性质

> RRF 只加分，不扣分。一个只出现在 semantic 通道的好结果，不会因为 keyword/metadata 通道没命中而降权。多通道命中同一个 chunk 会提升排名，单通道命中的 chunk 不会受惩罚。

**这意味着"无脑 hybrid"在检索质量上不会比有模式决策差。** 唯一的代价是每次检索多执行 2 条 SQL（增量约 10-20ms），可进一步通过并发优化为接近 0。

### 2.5 简化方案

**删除搜索模式决策，永远使用 hybrid（3 通道全部启用）。**

具体改动：

1. 删除 `search_mode_decision.go`（约 170 行）
2. `channels.go` 中 `Enabled()` 方法简化为纯基础设施检查：

```go
func (c *vectorGlobalChannel) Enabled(ctx SearchContext) bool {
    return c != nil && c.searcher != nil && c.embedding != nil
}
func (c *keywordChannel) Enabled(ctx SearchContext) bool {
    return c != nil && c.searcher != nil
}
func (c *metadataTitleChannel) Enabled(ctx SearchContext) bool {
    return c != nil && c.searcher != nil
}
```

3. `search_types.go` 中移除 `SearchContext.ResolvedMode / ModeDecision / QueryHints`
4. `rag_chat_service.go` 中 `resolveRetrieveSearchMode()` 直接返回 `"hybrid"`

**收益**：消除所有搜索模式误判，删除 170 行启发式规则，降低维护成本。语义盲区被关键词兜底，关键词盲区被语义兜底，元数据通道补强标题/文件名查找。

**代价**：每次检索执行 3 条 SQL（vs 单通道 1 条），增量约 10-20ms。

**保留**：通道的基础设施检查和告警容错。

---

## 三、各项改进的可行性评估

### 3.1 改动一：工具结果字段的类型安全读取

**影响面统计**：

```
接口变更: Tool 接口可选加 ResultSchema() 方法
  └─ 12 个实现需适配: 10 builtin + 2 test stubs

读取层（101 处 map[string]any 访问）:
  agent_loop.go      → 19 处
  observer_rule.go   → 35 处
  answer_guidance.go → 21 处
  observer_llm.go    → 1 处
  result_summary.go  → 2 处
  query_tools_test.go→ 23 处直接类型断言
```

**三步渐进实现**：

| 步骤 | 内容 | 工作量 | 风险 |
|---|---|---|---|
| Step 1 | 在 Result 类型上加 `GetString(key)` / `GetInt(key)` / `GetStringSlice(key)` 方法（纯增量） | 1 小时 | 极低 |
| Step 2 | 逐文件替换 `readDataString(result.Data, "xxx")` → `result.GetString("xxx")`。每个文件改完跑测试 | 半天 | 极低（机械替换） |
| Step 3 | 在 Tool 接口加 `ResultSchema() []FieldSchema`，逐工具实现 | 1 天 | 低 |

**评估**：Step 1+2 完全可行，零风险，纯机械重构。Step 3 在工具数 ≤ 10 时 ROI 不够高，建议等工具到 20+ 再做。

### 3.2 改动二：状态机逻辑去重

**核心洞察：不需要引入 Eino。** 真正的解法是把重复的 "结果 → 下一步" 映射抽成一个共享的单一决策源：

```go
// 新增文件: internal/app/rag/tool/next_action.go
func nextAction(result Result) (nextCall *Call, done bool, reasoning string)
```

然后两边的薄适配：

```go
// planCallsFromResults → 调用 nextAction，转成 []Call
// RuleObserver 各分支 → 调用 nextAction，转成 ObserveResult
```

**改动量**：

```
当前:  ~317 行分布在 agent_loop.go + observer_rule.go (两套逻辑)
改为: ~120 行集中在 tool/next_action.go (一套逻辑)
      + planCallsFromResults 退化为薄适配层 (~20 行)
      + RuleObserver 各分支退化为薄适配层 (~40 行)

净减少: ~137 行
新增文件: internal/app/rag/tool/next_action.go
改动文件: agent_loop.go, observer_rule.go
```

**评估**：

| 维度 | 判断 |
|---|---|
| 可行性 | 高。6 种工具类型的状态跳转逻辑已稳定，合并风险可控 |
| 风险 | 中。合并时可能遗漏某条分支的边缘条件 |
| 测试保护 | `agent_loop_test.go` + `query_tools_test.go` 覆盖主要路径 |
| 工作量 | 半天到一天。主要是仔细核对每条分支条件 |
| 回滚 | 改动集中在新文件 + 两个旧文件适配，容易回滚 |

### 3.3 改动三：检索模式简化

**影响面**：删除 1 个文件，修改 3 个文件中的少量代码。

**评估**：

| 维度 | 判断 |
|---|---|
| 可行性 | 极高。3 通道全部启用，不改变检索质量，只去掉了不必要的决策层 |
| 风险 | 极低。hybrid 本来就是事实默认 |
| 工作量 | 1 小时 |

---

## 四、Eino 接入评估

### 4.1 Eino 是什么

[CloudWeGo Eino](https://github.com/cloudwego/eino) 是字节跳动开源的 Go 语言 AI 应用开发框架，核心提供三样东西：

- **Graph 编排引擎**：节点 + 条件分支 + 并行 + 循环，声明式定义工作流
- **Graph as Tool**：把一个 Graph 包装成一个 Tool，让 LLM Agent 直接调用
- **多 Agent 协作模式**：Sequential / Parallel / Loop / Supervisor / Plan-Execute

### 4.2 Eino 能解决当前架构的什么问题

#### 核心收益：Graph as Tool 减少 LLM 调用次数

当前确定性诊断流程需要 LLM 驱动每一步：

```
Round 1: LLM Plan → "调 document_ingestion_diagnose" → Act → LLM Observe → "不够，继续"
Round 2: LLM Plan → "调 ingestion_task_query"       → Act → LLM Observe → "不够，继续"
Round 3: LLM Plan → "调 ingestion_task_node_query"   → Act → LLM Observe → "够了"

LLM 调用: 3 次 Planner + 3 次 Observer = 6 次
```

改为 Graph as Tool 后：

```
Round 1: LLM Plan → "调 document_root_cause_diagnosis"
           └─ Graph 内部自动完成: diagnose → task_query → node_query → 返回最终结果
         LLM Observe → "够了"

LLM 调用: 1 次 Planner + 1 次 Observer = 2 次
```

**确定性流程不应该每次决策都问 LLM。**

#### 次要收益：消灭状态机重复代码

Graph 的分支逻辑只写一次（在 Graph 定义处），不需要 `planCallsFromResults` 和 `RuleObserver` 各写一遍。

#### 远期收益：多 Agent 场景可扩展

当需要第二个 Agent 类型时，新 Agent = 新 Graph 定义，不需要复制粘贴 AgentLoop 的引擎代码。

### 4.3 不能也不该用 Eino 替代的部分

Eino 的 ReAct Agent（`ChatModelAgent`）**不能直接替代当前 AgentLoop**：

| GoAgent 独有能力 | Eino 是否提供 |
|---|---|
| 双重 Observer（LLM 主 + Rule 兜底） | 否 |
| 证据白名单校验（`validateHintAgainstEvidence`） | 否 |
| 状态冲突归一 + 深度证据升级（`answer_guidance`） | 否 |
| 多 Provider AI 路由 + 熔断（`infra-ai`） | 否（有自己的 ChatModel 抽象） |
| 多通道混合检索 + RRF 融合 | 否（有自己的 Retriever 抽象） |

### 4.4 推荐接入方式：Graph as Tool，保留 AgentLoop

```
保留: AgentLoop (Plan→Act→Observe)       ← 不改
保留: LLMObserver + RuleObserver         ← 不改
保留: 证据校验 + answer_guidance          ← 不改
保留: infra-ai（Chat/Embedding/Rerank）  ← 不改

新增: Eino Graph，封装 document→task→node 三跳确定性诊断
新增: 这个 Graph 实现 Tool 接口，注册到 Registry
删除: planCallsFromResults 中的诊断分支（Graph 替代）
删除: RuleObserver 中的诊断分支（Graph 替代）

保留: planWithBaseRules（首轮关键词路由）
保留: 开放探索场景的 LLM 驱动多轮循环
```

AgentLoop 从"执行循环 + 编排诊断流程"退化为纯粹的"执行循环 + 路由"。

### 4.5 现在接入 vs 以后接入的成本对比

| 工作项 | 现在（10 工具，1 Agent） | 以后（假设 20 工具，2 Agent） |
|---|---|---|
| 学习框架 | 0.5 天 | 0.5 天 |
| 适配工具到 Eino 接口 | 1 天 | 2 天 |
| 状态机迁移为 Graph | 1 天 | 2 天（两个场景） |
| SSE/trace/state 回调对接 | 1 天 | 1.5 天 |
| 全量测试 | 0.5 天 | 1 天 |
| **合计** | **4-5 天** | **7-8 天** |

迁移成本随工具和 Agent 数量线性增长。**现在迁移的代价最低。**

### 4.6 结论

| 维度 | 判断 |
|---|---|
| **该不该接入？** | 该。但不是全套替换，是 **Graph as Tool** 这一个模式 |
| **接入什么？** | 只接 Eino 的 Graph 编排引擎，不碰它的 Agent/ChatModel/Embedding/Retriever |
| **保留什么？** | 全部保留：AgentLoop、双重 Observer、证据校验、answer_guidance、infra-ai |
| **具体收益** | 确定性诊断流程 6 次 LLM 调用 → 2 次；消灭诊断状态机重复代码 |
| **风险控制** | Graph 只是 Tool 的一种实现方式，出问题退回到手写状态机只需改一个工具文件 |
| **工作量** | 4-5 天，可拆成 3 个独立 PR |
| **为什么现在？** | 工具数少、逻辑新鲜——迁移成本最低的窗口期 |

---

## 五、推荐执行计划

### 总体优先级排序

| 优先级 | 改动 | 可行性 | 风险 | 工作量 | 状态 |
|---|---|---|---|---|---|
| **P0** | 检索模式简化 | 极高 | 极低 | 1h | 高 | ✅ 已完成 |
| **P0** | 状态机去重（`nextAction()` 函数） | 高 | 中 | 2h | 高 | ✅ 已完成 |
| **P1** | Result 类型安全读取（Step 1+2） | 极高 | 极低 | 1h | 中 | ✅ 已完成 |
| **P1** | Eino Graph as Tool 接入 PR1 | 高 | 中 | 2h | 高 | ✅ 已完成 |
| **P1** | AgentState 合并简化 | 高 | 低 | 30min | 中 | ✅ 已完成 |
| **P1** | Graph Tool Observer 可读性（diagnosisDepth） | 高 | 低 | 30min | 中 | ✅ 已完成 |
| **P1** | Diagnose + Web Search Graph（新增能力） | 中 | 中 | 1h | 中 | ✅ 已完成 |
| **P2** | ResultSchema on Tool 接口（Step 3） | 高 | 低 | 1 天 | 低（等工具>20） | ⏸️ 推迟 |
| **P2** | Eino Graph as Tool PR2/PR3（SSE/trace） | 中 | 中 | 2-3 天 | 高 | ⏸️ 待做 |
| **P3** | 通用引擎与领域编排器分离 | 中 | 中 | 大 | 新 Agent 场景触发 | ⏸️ 推迟 |

### P0：立即执行（合计 1-2 天）

**1. 检索模式简化** ✅ 已完成（1 小时）

- 删除 `search_mode_decision.go`
- 简化 `channels.go` 的 `Enabled()` 方法
- 清理 `search_types.go` 和 `rag_chat_service.go` 中的相关字段
- 验证：`internal/app/rag/core/retrieve` 全量测试 PASS + `cmd/retrieve-eval` 评估不退化

**2. 状态机去重** ✅ 已完成（2 小时）

- 新增 `internal/app/rag/tool/next_action.go`
- 把 `planCallsFromResults` 和 `RuleObserver` 的重复决策逻辑合并为单一 `nextAction()` 函数
- `planCallsFromResults` 和 `RuleObserver` 退化为薄适配层
- 验证：现有 `internal/app/rag/tool/...` 全部测试 PASS + 手动验证 `doc_fail_01` / `doc_run_01` 场景

### 已完成（2026-05-11）

**3. Result 类型安全读取** ✅（1h）

- 在 Result 类型上加 `GetString(key)` / `GetInt(key)` / `GetStringSlice(key)` / `PreferStringSlice(primary, fallback)` 方法
- 替换 59 处 `readData*` 调用 + 18 处直接类型断言
- `readDataString` / `readDataInt` / `readDataStringSlice` 保留给原生 `map[string]any` 场景（result_summary.go, observer_llm.go 等）

**4. Eino Graph as Tool 接入** ✅ PR1（2h）

- PR1 ✅：搭建 Eino v0.8.13 依赖，`DiagnosisGraphTool`（3 跳诊断链），注册到 Registry，base rules 路由
- PR2 ⏸️：SSE/trace 回调待后续（Graph 内部 tool 调用对前端不可见）
- PR3 ⏸️：不应删除诊断状态机代码——Graph Tool 是补充路径，不是替代

**5. AgentState 合并简化** ✅（30min）

- LLM JSON schema 去重：去掉顶层 `confidence`/`nextHintCalls`/`nextHint`，统一走 `state` block
- `agent_loop.go` 合并逻辑从 20 行减到 11 行
- 5 个 few-shot 示例 + prompt template 同步更新

**6. Graph Tool Observer 可读性** ✅（30min）

- `DiagnosisGraphTool` 产出新增 `diagnosisDepth` 字段（node_level / task_level / diagnose_only）
- `RuleObserver` default 分支按 depth 分叉：node_level→0.95, task_level→0.75, 其他→0.6
- LLM data summary 新增 `diagnosisDepth` priority key

**7. Diagnose + Web Search Graph** ✅（1h）

- 新增 `DiagnoseSearchGraphTool`：Eino Graph `document_root_cause_diagnosis → web_search`
- 零 LLM 关键词提取：20 个技术错误模式 + `looksLikeTechnicalError` 启发式
- base rules 按"解决/修复/solution/fix"关键词路由到新 tool

---

## 六、不在此次改动范围内

- ingestion 模块进一步收口（已阶段性完成，短期不再主攻）
- 多 Agent 协作架构（等待 Graph as Tool 模式验证后再评估）
- 检索评估样本集扩充
- Planner/Observer 合并以减少 LLM 调用次数（P2 优先级）
- 通用引擎与领域编排器物理分离（新 Agent 场景触发时再做）
