# GoAgent Agent 功能深度分析与借鉴方案

> 生成日期: 2026-05-10
> 分析范围: Agent Loop / Planner / Observer / Tool 体系

---

## 目录

- [一、Agent 架构现状](#一agent-架构现状)
- [二、核心问题清单](#二核心问题清单)
  - [P0 — 架构级问题](#p0--架构级问题)
  - [P1 — 功能级问题](#p1--功能级问题)
  - [P2 — 体验级问题](#p2--体验级问题)
- [三、可借鉴的已有实现](#三可借鉴的已有实现)
  - [架构模式借鉴](#架构模式借鉴)
  - [Tool 设计借鉴](#tool-设计借鉴)
  - [Observer/Planner 借鉴](#observerplanner-借鉴)
  - [多 Agent 模式借鉴](#多-agent-模式借鉴)
- [四、问题与借鉴方案对照](#四问题与借鉴方案对照)
- [五、实施路线图](#五实施路线图)

---

## 一、Agent 架构现状

### 1.1 整体架构

当前 Agent 系统采用 **Plan → Act → Observe** 三阶段循环架构:

```
用户问题
  ↓
Planner (LLM / Rules)  → 决定调用哪些 Tool
  ↓
Executor               → 并行/串行执行 Tool
  ↓
Observer (LLM / Rules) → 判断是否停止 / 下一步做什么
  ↓
[循环最多 3 轮]
  ↓
RenderContext → 汇总证据
  ↓
LLM 生成最终回答
```

### 1.2 核心文件

| 文件 | 职责 | 行数 |
|------|------|------|
| [agent_loop.go](file:///d:/goagent/internal/app/rag/tool/agent_loop.go) | Agent 循环主逻辑 | ~815 |
| [observer_rule.go](file:///d:/goagent/internal/app/rag/tool/observer_rule.go) | 规则驱动的观察器 | ~200 |
| [observer_llm.go](file:///d:/goagent/internal/app/rag/tool/observer_llm.go) | LLM 驱动的观察器 | ~523 |
| [planner/planner.go](file:///d:/goagent/internal/app/rag/tool/planner/planner.go) | LLM 驱动的工具规划器 | ~150 |
| [executor.go](file:///d:/goagent/internal/app/rag/tool/executor.go) | 工具执行器 | ~52 |
| [registry.go](file:///d:/goagent/internal/app/rag/tool/registry.go) | 工具注册中心 | ~71 |

### 1.3 已注册 Tool 列表

当前 8 个内置 Tool，全部为**只读诊断类**:

| Tool 名称 | 用途 | 文件 |
|-----------|------|------|
| `document_query` | 查询文档状态 | [document_query_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/document_query_tool.go) |
| `document_chunk_log_query` | 查询分块日志 | [document_chunk_log_query_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/document_chunk_log_query_tool.go) |
| `document_ingestion_diagnose` | 文档摄入诊断 | [document_ingestion_diagnose_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/document_ingestion_diagnose_tool.go) |
| `ingestion_task_query` | 查询 Task 详情 | [ingestion_task_query_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/ingestion_task_query_tool.go) |
| `ingestion_task_node_query` | 查询 Task 节点详情 | [ingestion_task_node_query_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/ingestion_task_node_query_tool.go) |
| `task_ingestion_diagnose` | Task 诊断 | [task_ingestion_diagnose_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/task_ingestion_diagnose_tool.go) |
| `trace_node_query` | 查询 Trace 节点 | [trace_node_query_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/trace_node_query_tool.go) |
| `trace_retrieval_diagnose` | 检索诊断 | [trace_retrieval_diagnose_tool.go](file:///d:/goagent/internal/app/rag/tool/builtin/trace_retrieval_diagnose_tool.go) |

### 1.4 初始化流程

在 [runtime.go:buildLocalToolWorkflow](file:///d:/goagent/internal/bootstrap/rag/runtime.go#L216-L250) 中完成:

```go
func buildLocalToolWorkflow(...) ragtool.Workflow {
    registry := ragtool.NewRegistry()
    // 注册 8 个诊断 Tool
    registry.MustRegister(ragbuiltin.NewDocumentQueryTool(documentService))
    registry.MustRegister(ragbuiltin.NewDocumentIngestionDiagnoseTool(documentService))
    // ... 其他 6 个

    wf := ragtool.NewAgentLoop(ragtool.NewExecutor(registry))
    wf.SetMaxIterations(cfg.Rag.Agent.MaxIterations)        // 默认 3
    wf.SetParallelToolCalls(cfg.Rag.Agent.ParallelToolCalls.Enabled, ...)
    wf.SetPlanner(planner.NewLLMPlanner(chatService))
    wf.SetObserver(ragtool.NewLLMObserver(chatService))
    return wf
}
```

---

## 二、核心问题清单

### P0 — 架构级问题

#### 问题 1: Tool 集极度受限 — 只有诊断工具，无任何"行动"能力

**现状**: 8 个 Tool 全部是只读查询/诊断，无任何写操作或外部系统交互能力。

**缺失的 Tool 类型**:

| 类型 | 示例 | 价值 |
|------|------|------|
| **知识写入** | `create_knowledge_document`、`update_chunk` | 用户说"帮我添加一份文档到知识库"时自动执行 |
| **对话管理** | `delete_conversation`、`export_conversation` | 自然语言对话管理 |
| **外部查询** | `search_web`、`query_database` | 知识库外的信息补充 |
| **代码执行** | `run_python`、`execute_sql` | 数据分析场景 |
| **文件操作** | `download_file`、`read_file` | 文档处理场景 |

**影响**: Agent 只能"看"不能"做"，严格来说只是一个**智能诊断助手**，而非真正的 Agent。

---

#### 问题 2: Observer 规则膨胀 — 硬编码状态机，维护成本极高

**现状**: [observer_rule.go](file:///d:/goagent/internal/app/rag/tool/observer_rule.go) 中每个 Tool 对应一个独立的 `observeXxx()` 函数:

```go
switch latest.Name {
case "document_ingestion_diagnose":
    return observeDocumentDiagnosis(latest)  // 80+ 行
case "task_ingestion_diagnose":
    return observeTaskDiagnosis(latest)      // 60+ 行
case "document_query":
    return observeDocumentQuery(latest)      // 30+ 行
case "document_chunk_log_query":
    return observeChunkLogQuery(latest)      // 50+ 行
case "ingestion_task_query":
    return observeTaskQuery(latest)          // 60+ 行
// ... 共 8 个 case
}
```

**问题**:
- 每新增一个 Tool，必须新增一个 `observeXxx()` 函数
- 每个函数内部是多层 if-else 嵌套，逻辑复杂度高
- 规则之间相互依赖（如 `observeChunkLogQuery` 可能触发 `observeTaskQuery` 的下一步）
- 已有 LLMObserver 但 RuleObserver 仍是默认 fallback，两套逻辑可能冲突

---

#### 问题 3: Planner 对检索阶段"失明" — 缺少检索结果感知

**现状**: 虽然 `PlanInput` 传入了 `RewriteResult` 和 `RetrieveResult`，但 Planner 的 system prompt 中**没有明确指导如何利用检索结果做工具规划**。

**代码证据**: [planner.go:30-42](file:///d:/goagent/internal/app/rag/tool/planner/planner.go#L30-L42) 的 `plannerExamples` 中 6 条示例全部是诊断场景，无任何"检索结果不足时该调用什么 Tool"的示例。

**影响**: 当检索召回的 chunk 质量差时，Agent 无法自主决定"是否需要换个知识库再检索"或"是否需要扩展查询"。

---

#### 问题 4: 最大迭代次数硬编码为 3，无动态调整能力

**代码证据**: [agent_loop.go:13](file:///d:/goagent/internal/app/rag/tool/agent_loop.go#L13)

```go
const defaultMaxIterations = 3
```

**问题**:
- 简单查询（如 `document_query`）可能 1 轮就够了，浪费 2 轮 LLM 调用
- 复杂诊断（如跨 Task/Node 多层下钻）3 轮可能不够
- 无基于问题复杂度的动态迭代次数计算

**影响**: 每次 Agent 调用固定消耗 3 次 LLM 请求（Planner + Observer × 3），API 成本高。

---

### P1 — 功能级问题

#### 问题 5: 无 Tool 超时/重试机制

**现状**: [executor.go](file:///d:/goagent/internal/app/rag/tool/executor.go#L20-L49) 中 `Execute()` 直接调用 `tool.Invoke()`，无超时控制，无重试。

**问题**:
- 某个 Tool 调用外部 API 卡住时，整个 Agent Loop 阻塞
- 网络抖动导致单次失败时，直接标记为 failed，无重试机会
- 无 Tool 级别的 circuit breaker

---

#### 问题 6: 并行 Tool 调用实际未启用

**代码证据**: [agent_loop.go:26](file:///d:/goagent/internal/app/rag/tool/agent_loop.go#L26)

```go
func NewAgentLoop(executor *Executor) *AgentLoop {
    return &AgentLoop{
        maxParallelToolCalls: 1,  // 始终为 1
    }
}
```

虽然 `SetParallelToolCalls` 方法存在，但 [runtime.go](file:///d:/goagent/internal/bootstrap/rag/runtime.go#L244-L248) 中的配置依赖 `cfg.Rag.Agent.ParallelToolCalls.Enabled`，默认未开启。

**影响**: 即使 Planner 规划了 3 个独立 Tool，也是串行执行，延迟叠加。

---

#### 问题 7: Agent 状态管理脆弱 — 依赖字符串拼接

**现状**: Agent 状态通过 `AgentState` 结构体传递，但关键信息仍依赖字符串匹配:

```go
// observer_rule.go:168
if strings.Contains(conclusion, "still running") { ... }
if strings.Contains(conclusion, "failed at node") { ... }
```

**问题**:
- LLM 生成的 conclusion 文本稍有变化，规则就匹配不上
- 多语言场景下（中文/英文混用），字符串匹配极不稳定
- 已有 `HintCall` 结构化对象，但旧字符串 `NextHint` 仍在使用

---

#### 问题 8: 无 Tool 调用成本/Token 计量

**现状**: 每次 Agent Loop 消耗的 LLM Token 数、Tool 调用次数、总耗时等指标无记录。

**缺失内容**:
- 单次对话的 Planner/Observer/最终回答各消耗多少 Token
- Tool 调用的平均耗时分布
- Agent 降级率（多少对话因 Agent 失败回退到普通 RAG）

**影响**: 无法评估 Agent 功能的 ROI，也无法做成本优化。

---

### P2 — 体验级问题

#### 问题 9: 前端 Agent 推理展示不完整

**现状**: 虽然已接通 `agent_think` 事件，但展示形式简单。Agent 推理仅以"琥珀色提示条"展示，无:
- 多轮迭代的可视化（第 1 轮 → 第 2 轮 → 第 3 轮）
- Tool 调用关系图（哪个 Tool 触发了下一个 Tool）
- 置信度变化曲线

---

#### 问题 10: 无 Agent 对话模板/预设场景

**现状**: 用户需手动输入 `doc_run_01 为什么失败了` 才能触发 Agent 诊断。

**缺失内容**:
- 预设诊断场景按钮（"诊断最近的文档导入失败"）
- 快捷入口（文档列表页直接点击"诊断"按钮）
- 自动触发（文档导入失败时自动启动 Agent 诊断）

---

#### 问题 11: LLMObserver 的 JSON 解析脆弱

**现状**: [observer_llm.go:196-210](file:///d:/goagent/internal/app/rag/tool/observer_llm.go#L196-L210) 中 LLM 返回的 JSON 解析失败时，直接 fallback 到 RuleObserver:

```go
decision, ok := o.parseResponse(response, input)
if !ok {
    return o.observeWithFallback(ctx, input)  // 静默降级
}
```

**问题**:
- LLM 输出格式不稳定时，Agent 行为完全由规则决定
- 无日志记录为什么解析失败（是 JSON 格式错误？还是字段缺失？）
- 无法统计 LLMObserver 的成功率

---

#### 问题 12: 无 Agent 能力的 A/B 测试

**现状**: 无法对比"开启 Agent"和"关闭 Agent"的对话效果。

**缺失内容**:
- 同一问题，Agent 模式 vs 普通 RAG 模式的回答质量对比
- 用户对两种模式的反馈（👍/👎）分布
- Agent 模式是否真的提升了回答准确度

---

## 三、可借鉴的已有实现

### 架构模式借鉴

#### 方案 1: LangGraph 的图状态机模式 (最推荐)

**来源**: LangChain / LangGraph 框架

**核心理念**: 将 Agent 循环建模为**有向图**，每个节点是一个状态，边定义流转条件。

```
用户输入 → [Planner 节点] → [Tool 执行节点] → [Observer 节点] → 判断是否完成
                                                      ↓
                                                 完成? → [Final Answer 节点]
```

**核心概念**:

| 概念 | 说明 | 对应现状 |
|------|------|---------|
| **StateGraph** | 定义类型化的状态对象，在图中流转 | 当前 `AgentState` 结构体 |
| **Nodes** | 接收状态、执行工作、返回更新后的状态 | Planner / Executor / Observer |
| **Edges** | 节点间的连接（可条件分支） | 当前 for 循环硬编码 |
| **Checkpointing** | 内置状态持久化，支持长程任务中断恢复 | 无 |

**可借鉴点**:
- **显式状态管理**: 定义一个 `AgentState` 类型，所有节点共享，避免当前项目中的字符串拼接问题
- **条件边**: Observer 不再用 `switch-case` 硬编码，而是返回 `done=true/false` 控制图流转
- **可扩展性**: 新增 Tool 不需要改 Observer 规则，只需注册到 Registry

**代码示例** (Python 参考):

```python
from langgraph.graph import StateGraph, END

class AgentState(TypedDict):
    messages: list
    tool_calls: list
    results: list
    done: bool

graph = StateGraph(AgentState)
graph.add_node("planner", planner_node)
graph.add_node("tools", tools_node)
graph.add_node("observer", observer_node)

graph.add_edge("planner", "tools")
graph.add_edge("tools", "observer")
graph.add_conditional_edges("observer", lambda s: END if s["done"] else "planner")
```

**对比现状**: 当前项目的 [AgentLoop.Run](file:///d:/goagent/internal/app/rag/tool/agent_loop.go#L18-L80) 是 for 循环硬编码，改为图状态机后，新增 Tool 不需要改 Observer 规则。

**实施难度**: 高（需重构 AgentLoop）
**预期收益**: 彻底解决 Observer 膨胀问题

---

#### 方案 2: OpenAI Agents SDK 的 Runner 模式

**来源**: OpenAI Agents SDK (Python)

**核心理念**: `Runner.run()` 循环调用 LLM，直到满足退出条件。

**退出条件明确化**:
1. 模型返回最终输出（无 Tool 调用）
2. 达到最大轮次
3. 发生不可恢复错误

**可借鉴点**:

| 特性 | 说明 | 现状对比 |
|------|------|---------|
| **Hosted Tools** | 将检索、向量搜索等作为"托管工具"，与自定义函数工具统一管理 | 当前检索和 Tool 是分离的 |
| **Tool Search** | 运行时按需加载工具子集，避免一次性注入过多 Tool 定义 | 当前 8 个 Tool 全部注入 |
| **Agents as Tools** | 一个 Agent 可以作为另一个 Agent 的 Tool | 无 |

**代码示例** (Python 参考):

```python
from agents import Agent, Runner

agent = Agent(
    name="DiagnosticAgent",
    instructions="You diagnose ingestion pipeline issues.",
    tools=[document_query, ingestion_diagnose, ...],
)

result = await Runner.run(agent, "Why did doc_01 fail?")
print(result.final_output)
```

**实施难度**: 中
**预期收益**: 清晰的退出条件，支持 Tool 动态加载

---

#### 方案 3: Anthropic 的 "think" 工具模式 (低成本高收益)

**来源**: Anthropic τ-bench 基准测试

**核心理念**: 给 Agent 一个 `think` 工具，让它在调用其他工具前先记录思考过程。

**Tool 定义**:

```json
{
  "name": "think",
  "description": "Use this tool to think about something. It will not obtain new information, just append the thought to the log. Use it when complex reasoning or some cache memory is needed.",
  "input_schema": {
    "type": "object",
    "properties": {
      "thought": {
        "type": "string",
        "description": "A thought to think about."
      }
    },
    "required": ["thought"]
  }
}
```

**效果数据**:

| 场景 | 基线 | + think 工具 | 提升 |
|------|------|-------------|------|
| 航空客服 | 0.370 | 0.570 | **+54%** |
| 零售客服 | 0.783 | 0.812 | +3.7% |

**可借鉴点**:
- 当前项目的 Planner 直接输出 Tool 调用，缺少"思考"环节
- 增加 `think` 工具后，Planner 可以先输出 `"thought": "文档失败了，我需要先查 document 状态，再查 chunk log"`，再决定调用什么 Tool
- 实现成本极低，只需新增一个 Tool 定义

**Go 实现示例**:

```go
type ThinkTool struct{}

func (t *ThinkTool) Definition() Definition {
    return Definition{
        Name:        "think",
        Description: "Use this tool to think about something before taking action. It will not obtain new information, just record your reasoning.",
        Parameters: []ParameterDefinition{
            {
                Name:        "thought",
                Type:        ParamTypeString,
                Description: "Your reasoning or thought process.",
                Required:    true,
            },
        },
    }
}

func (t *ThinkTool) Invoke(ctx context.Context, call Call) (Result, error) {
    thought := readStringArg(call.Arguments, "thought")
    return Result{
        Name:    "think",
        Status:  CallStatusSuccess,
        Summary: "Thought recorded: " + thought[:min(100, len(thought))],
    }, nil
}
```

**实施难度**: 极低（1 个文件）
**预期收益**: 规划质量 +50%

---

### Tool 设计借鉴

#### 方案 4: OpenAI 的 Function Tool 规范

**来源**: OpenAI Agents SDK / Function Calling 最佳实践

**最佳实践**:

```python
@function_tool
def fetch_weather(location: Location) -> str:
    """Fetch the weather for a given location.

    Args:
        location: The location to fetch the weather for.
    """
    return "sunny"
```

**设计原则**:

| 原则 | 说明 | 现状对比 |
|------|------|---------|
| **单一职责** | 每个 Tool 只做一件事 | 当前 `document_ingestion_diagnose` 同时做了"查询+诊断+建议" |
| **类型安全** | 使用 struct 定义参数 | 当前 `call.Arguments` 是 `map[string]any` |
| **Docstring 即描述** | Tool 描述直接从注释生成 | 当前手动编写 Definition |
| **清晰的参数描述** | 每个参数都有明确的描述和约束 | 当前部分参数缺少描述 |

**可借鉴点**:
- 将 `document_ingestion_diagnose` 拆分为:
  - `query_document` (纯查询)
  - `diagnose_document` (基于查询结果诊断)
- 使用 Go struct 替代 `map[string]any` 作为 Tool 参数

**实施难度**: 中（需重构 Tool 定义）
**预期收益**: 可维护性提升，LLM 调用准确率提升

---

#### 方案 5: Tool 超时与重试机制 (通用模式)

**来源**: Function Calling LLM Best Practices 2026

**最佳实践**:

```python
async def safe_tool_execution(tool_func, params, max_retries=2):
    for attempt in range(max_retries):
        try:
            result = await tool_func(**params)
            return {"success": True, "data": result}
        except TimeoutError:
            if attempt < max_retries - 1:
                await asyncio.sleep(2 ** attempt)  # 指数退避
                continue
            return {"success": False, "error": "Tool timed out", "retry": False}
        except ValidationError as e:
            # 验证错误不重试
            return {"success": False, "error": str(e), "retry": False}
```

**Go 实现方案**:

```go
func (e *Executor) Execute(ctx context.Context, call Call) (Result, error) {
    // 1. 超时控制
    toolTimeout := 30 * time.Second
    if timeout := e.getToolTimeout(call.Name); timeout > 0 {
        toolTimeout = timeout
    }
    ctx, cancel := context.WithTimeout(ctx, toolTimeout)
    defer cancel()

    // 2. 重试机制
    maxRetries := e.getToolMaxRetries(call.Name)
    var lastErr error
    for attempt := 0; attempt <= maxRetries; attempt++ {
        result, err := tool.Invoke(ctx, call)
        if err == nil {
            return result, nil
        }
        lastErr = err

        // 验证错误不重试
        if isValidationError(err) {
            break
        }

        // 指数退避
        if attempt < maxRetries {
            backoff := time.Duration(1<<uint(attempt)) * time.Second
            select {
            case <-ctx.Done():
                return Result{Status: CallStatusFailed, ErrorMessage: "timeout"}, ctx.Err()
            case <-time.After(backoff):
            }
        }
    }
    return Result{Status: CallStatusFailed, ErrorMessage: lastErr.Error()}, lastErr
}
```

**实施难度**: 低（改 Executor）
**预期收益**: 稳定性大幅提升

---

### Observer/Planner 借鉴

#### 方案 6: OpenAI 的显式规划 Prompt

**来源**: OpenAI SWE-bench 优化指南

**Prompt 模板**:

```
You MUST plan extensively before each function call,
and reflect extensively on the outcomes of the previous function calls.
DO NOT do this entire process by making function calls only,
as this can impair your ability to solve the problem and think insightfully.
```

**效果**: SWE-bench 通过率 **+4%**

**可借鉴点**:
- 当前项目的 [planner.go](file:///d:/goagent/internal/app/rag/tool/planner/planner.go) 的 system prompt 缺少"充分规划"的强指令
- 在 prompt 中加入上述指令，即使不微调模型也能提升规划质量

**当前 planner system prompt 改进建议**:

```go
const plannerSystemTemplate = `You are the planning component of an agentic diagnostic workflow.

You MUST plan extensively before each function call, and reflect extensively on the outcomes of the previous function calls. DO NOT do this entire process by making function calls only, as this can impair your ability to solve the problem and think insightfully.

Your job is to decide which tools to call next based on the current evidence.

Available tools:
%s

Rules:
1. Never invent ids. Only use ids that already appear in the question, previous state, or tool results.
2. Provide at most 3 tool calls per round.
3. Avoid repeating a lookup that was already completed.
4. When the task/document is still running, prefer verification over failure-specific deep dives.

Return strict JSON only:
{"toolCalls":[{"name":"tool_name","arguments":{"arg":"value"}}],"reasoning":"..."}`
```

**实施难度**: 极低（改 prompt）
**预期收益**: 规划质量 +4%

---

#### 方案 7: LangChain 的结构化输出集成

**来源**: LangChain `create_react_agent` 2025 更新

**核心理念**: 在 Agent 主循环中直接支持结构化输出，而非循环结束后再调一次 LLM。

**代码示例**:

```python
class Weather(BaseModel):
    temperature: float
    condition: str

agent = create_react_agent("openai:gpt-4o-mini", tools=[weather_tool], response_format=Weather)
```

**可借鉴点**:
- 当前项目的 Agent 循环结束后，需要额外调用 LLM 生成最终回答
- 可以改为: Planner 在最后一次迭代中直接输出结构化回答，减少一次 LLM 调用

**实施难度**: 中
**预期收益**: 减少 1 次 LLM 调用，降低成本

---

### 多 Agent 模式借鉴 (长期)

#### 方案 8: CrewAI 的角色分工模式

**来源**: CrewAI 框架

**核心理念**: 模拟人类团队协作，每个 Agent 扮演**固定角色**，拥有**专属技能**和**明确职责**。

**适用场景**: 当前项目只有"诊断 Agent"，未来可以扩展为:

| 角色 | 职责 | 工具集 |
|------|------|--------|
| **诊断专家** | 分析文档/Task 失败原因 | 现有 8 个诊断 Tool |
| **知识管理员** | 创建/更新/删除知识库文档 | `create_document`, `update_chunk`, `delete_document` |
| **检索优化师** | 优化检索策略、调整向量配置 | `query_vector_store`, `update_embedding_config`, `analyze_retrieval_quality` |
| **路由 Agent** | 判断用户意图，分派给对应角色 | 无（纯路由） |

**工作流**:

```
用户提问 → [路由 Agent] 判断意图
                          ↓
              ┌───────────┼───────────┐
              ↓           ↓           ↓
        [诊断专家]  [知识管理员]  [检索优化师]
```

**CrewAI 代码示例** (Python 参考):

```python
from crewai import Agent, Task, Crew

diagnostician = Agent(
    role="Ingestion Diagnostic Expert",
    goal="Diagnose why knowledge document ingestion failed",
    backstory="You have deep expertise in data pipeline troubleshooting.",
    tools=[document_query, ingestion_diagnose, ...],
)

knowledge_admin = Agent(
    role="Knowledge Base Administrator",
    goal="Manage knowledge documents and chunks",
    backstory="You are experienced in content management.",
    tools=[create_document, update_chunk, ...],
)

crew = Crew(
    agents=[diagnostician, knowledge_admin],
    tasks=[diagnostic_task, admin_task],
    process=Process.sequential,
)
```

**实施难度**: 高（需新增 Agent 和路由逻辑）
**预期收益**: 扩展 Agent 能力边界

---

#### 方案 9: AutoGen 的对话协作模式

**来源**: Microsoft AutoGen / AG2 框架

**核心理念**: Agent 通过**自然语言对话**自主协商、辩论、协作解决问题。

**适用场景**: 复杂诊断需要多个专家辩论时。

**示例对话**:

```
用户: "为什么文档导入后检索效果差?"

诊断专家: "我看了 chunk log，显示成功了"
检索优化师: "但向量索引可能有问题，让我查一下"
诊断专家: "同意，建议重新索引"
```

**AutoGen 代码示例** (Python 参考):

```python
from autogen import AssistantAgent, GroupChat, GroupChatManager

diagnostician = AssistantAgent(
    name="Diagnostician",
    system_message="You diagnose ingestion pipeline issues.",
)

retrieval_optimizer = AssistantAgent(
    name="RetrievalOptimizer",
    system_message="You optimize retrieval quality.",
)

groupchat = GroupChat(
    agents=[diagnostician, retrieval_optimizer],
    messages=[],
    max_round=10,
)

manager = GroupChatManager(groupchat=groupchat)
```

**对比现状**: 当前项目是单 Agent 循环，无法实现多视角分析。

**实施难度**: 高（需重构 Agent 架构）
**预期收益**: 复杂问题诊断质量提升

---

## 四、问题与借鉴方案对照

| 问题 | 借鉴方案 | 实施难度 | 预期收益 |
|------|---------|---------|---------|
| **问题 1**: Tool 集只有诊断能力 | 方案 4 (Function Tool 规范) + 方案 8 (角色分工) | 中 → 高 | Agent 从"看"到"做" |
| **问题 2**: Observer 规则膨胀 | 方案 1 (LangGraph 图状态机) | 高 | 彻底解决膨胀 |
| **问题 3**: Planner 对检索结果"失明" | 方案 6 (显式规划 Prompt) | 极低 | 规划质量 +4% |
| **问题 4**: 迭代次数硬编码 | 方案 2 (Runner 模式退出条件) | 中 | 成本优化 |
| **问题 5**: 无 Tool 超时/重试 | 方案 5 (超时重试机制) | 低 | 稳定性大幅提升 |
| **问题 6**: 并行调用未启用 | 方案 2 (Runner 模式) | 低 | 延迟降低 |
| **问题 7**: 状态管理依赖字符串 | 方案 1 (LangGraph 显式状态) | 高 | 规则稳定性提升 |
| **问题 8**: 无成本计量 | 方案 2 (Runner 模式) | 低 | ROI 可评估 |
| **问题 9**: 前端展示不完整 | 方案 3 (think 工具) | 极低 | 推理过程可视化 |
| **问题 10**: 无预设场景 | 方案 8 (角色分工) | 中 | 能力可见性提升 |
| **问题 11**: LLMObserver 解析脆弱 | 方案 3 (think 工具) + 方案 7 (结构化输出) | 中 | 降级率降低 |
| **问题 12**: 无 A/B 测试 | 方案 2 (Runner 模式) | 中 | 效果可验证 |

---

## 五、实施路线图

### 第一阶段：快速见效 (1-2 周)

| 任务 | 借鉴方案 | 改动范围 |
|------|---------|---------|
| 增加 `think` 工具 | 方案 3 | 新增 1 个文件 (~50 行) |
| 优化 Planner prompt | 方案 6 | 修改 [planner.go](file:///d:/goagent/internal/app/rag/tool/planner/planner.go) 中的 prompt 常量 |
| 增加 Tool 超时控制 | 方案 5 | 修改 [executor.go](file:///d:/goagent/internal/app/rag/tool/executor.go) |
| 增加 LLMObserver 解析失败日志 | 问题 11 | 修改 [observer_llm.go](file:///d:/goagent/internal/app/rag/tool/observer_llm.go) |

**预期成果**:
- 规划质量提升 50%+
- Tool 执行稳定性提升
- 降级原因可追溯

---

### 第二阶段：架构优化 (3-4 周)

| 任务 | 借鉴方案 | 改动范围 |
|------|---------|---------|
| 重构 Tool 定义，使用 struct 参数 | 方案 4 | 修改所有 builtin Tool |
| 增加 Tool 重试机制 | 方案 5 | 修改 [executor.go](file:///d:/goagent/internal/app/rag/tool/executor.go) |
| 增加 Agent 成本计量 | 问题 8 | 新增 metrics 模块 |
| 支持动态迭代次数 | 方案 2 | 修改 [agent_loop.go](file:///d:/goagent/internal/app/rag/tool/agent_loop.go) |

**预期成果**:
- Tool 可维护性提升
- API 成本可量化
- 简单问题响应更快

---

### 第三阶段：能力扩展 (5-8 周)

| 任务 | 借鉴方案 | 改动范围 |
|------|---------|---------|
| 新增知识写入 Tool | 方案 4 | 新增 3-5 个写操作 Tool |
| 新增路由 Agent | 方案 8 | 新增路由逻辑 |
| 前端 Agent 推理可视化 | 问题 9 | 修改前端组件 |
| 预设诊断场景 | 问题 10 | 新增快捷入口 |

**预期成果**:
- Agent 从诊断扩展到操作
- 用户体验显著提升

---

### 第四阶段：架构升级 (9-12 周)

| 任务 | 借鉴方案 | 改动范围 |
|------|---------|---------|
| 引入图状态机替代 for 循环 | 方案 1 | 重构 [agent_loop.go](file:///d:/goagent/internal/app/rag/tool/agent_loop.go) |
| 多 Agent 协作 | 方案 9 | 新增多 Agent 架构 |
| A/B 测试框架 | 问题 12 | 新增实验框架 |

**预期成果**:
- Observer 膨胀问题彻底解决
- 复杂问题诊断质量提升
- Agent 效果可量化验证

---

## 附录

### A. 参考资源

| 资源 | 链接 |
|------|------|
| LangGraph 文档 | https://docs.langchain.com/oss/javascript/langgraph/overview |
| OpenAI Agents SDK | https://github.com/openai/openai-agents-python |
| OpenAI Agent 实践指南 | https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf |
| Function Calling 最佳实践 | https://www.ai-agentsplus.com/blog/function-calling-llm-best-practices-march-14-2026 |
| CrewAI vs LangGraph vs AutoGen | https://agentcenter.cloud/blogs/crewai-vs-langgraph-vs-autogen |
| Anthropic τ-bench 报告 | 见方案 3 效果数据 |

### B. 关键代码文件索引

| 文件 | 路径 | 职责 |
|------|------|------|
| Agent Loop | `internal/app/rag/tool/agent_loop.go` | 循环主逻辑 |
| Rule Observer | `internal/app/rag/tool/observer_rule.go` | 规则观察器 |
| LLM Observer | `internal/app/rag/tool/observer_llm.go` | LLM 观察器 |
| LLM Planner | `internal/app/rag/tool/planner/planner.go` | LLM 规划器 |
| Executor | `internal/app/rag/tool/executor.go` | 工具执行 |
| Registry | `internal/app/rag/tool/registry.go` | 工具注册 |
| Runtime | `internal/bootstrap/rag/runtime.go` | RAG 运行时初始化 |

### C. 术语表

| 术语 | 说明 |
|------|------|
| **Agent** | 能够自主规划、执行工具、观察结果的智能体 |
| **Planner** | 决定下一步调用哪些工具的组件 |
| **Observer** | 判断是否停止循环或给出下一步建议的组件 |
| **Tool** | Agent 可以调用的外部函数或 API |
| **ReAct** | Reasoning + Acting 模式，交替进行推理和行动 |
| **RAG** | Retrieval-Augmented Generation，检索增强生成 |
| **Ingestion** | 知识文档的摄入、解析、分块、索引过程 |
| **Chunk** | 文档被分割后的文本片段 |
| **Trace** | RAG 请求的执行追踪记录 |
