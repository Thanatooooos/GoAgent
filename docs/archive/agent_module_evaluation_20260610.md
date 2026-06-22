# Agent 模块评估报告（internal/app/agent）

评估时间：`2026-06-10`

评估范围：`internal/app/agent/` 全部 164 个 Go 文件（生产代码 121 个 / 约 14,000 行，测试代码 43 个 / 约 10,200 行），结合 `docs/agent_tool_parity_matrix.md`、`docs/codebase_evaluation_20260608.md`、`internal/bootstrap/rag/agent_runtime.go` 的实际装配路径。

---

## 一、总体结论

**agent 模块是当前仓库中架构质量最高的代码，但它目前是一台"放在架子上的发动机"——构建质量领先于实际部署程度。**

- 设计层面：分层正确、依赖方向干净、状态管理采用 event-sourcing、capability 契约元数据完整
- 测试层面：契约测试 + 服务级回归 + checkpoint/replay 覆盖，是全仓库测试质量最好的模块
- 落地层面：生产 bootstrap 只注册了部分能力，`plan_execute` 在生产不可达，审批会话不能跨重启存活，没有增量事件流
- 风险不在设计，而在"已建成"与"已上线"之间的差距随时间扩大，旧 runtime 持续承载生产流量造成双轨税

下一阶段的正确投入方向是**装轮子，不是加马力**：bootstrap 接线、事件流、持久化、双 runtime 收口。

---

## 二、分维度评估

### 2.1 架构与设计 — A−

**优点：**

1. **分层清晰，依赖方向正确**

   ```text
   kernel（eino graph 封装）
     → runtime（session / checkpoint / replay）
     → state（snapshot / delta / reducer / journal）
     → pattern（reactive / plan_execute）
     → capability + 8 个能力包
   ```

   无 dot import，无 `SetXXX` 注入，全部依赖通过 `ServiceOptions` 构造时注入。这是对旧 `rag/tool` runtime 所有教训的系统性修正。

2. **Event-sourcing 状态管理是真正的差异化设计**

   - 节点对 session 只读，变更通过 `StateDelta` 表达，经 `Reducer` 应用，落入 `Journal` 事件
   - `runtime/replay.go`（502 行）可重建运行过程
   - approval / resume / checkpoint 回归测试因此真正可信

3. **Capability 契约元数据完整**

   `capability.Spec` 携带 `RiskLevel / RequiresApproval / SupportsParallel / SupportsResume / Idempotency / Preconditions / ProducesEvidence`——这是 planner 和 policy 层做决策真正需要的元数据，而不只是名字加描述。

4. **文件纪律好**

   生产代码最大文件 `planner/llm_planner.go` 595 行，其余均在 510 行以下，无 god file。

**弱点：**

1. **层次深度超前于实际规模**（`codebase_evaluation_20260608.md` §4.1 的批评部分仍成立）：`capability/catalog`、`capability/select`、`capability/resolve` 三个子包合计约 400 行，服务于约 8 个 capability。
2. **`Schema` 只是 Go 类型名，不是 schema**：`NewSchema()` 仅记录 `pkgpath.TypeName`。LLM selector / planner 看不到字段级结构，只能依赖 `Description` 文本——这是旧 runtime 参数幻觉问题在新架构中的残留根因。
3. **Pattern 在构造期冻结**：`service_assembly.go::compileRunner` 一个 Service 只编译一张图，没有按请求在 reactive / plan_execute 之间路由的能力。

### 2.2 生产落地 — C−（最重要的发现）

模块质量领先于生产暴露程度，具体差距：

1. **`bootstrap/rag/agent_runtime.go` 硬编码 `Pattern: PatternReactive`**

   `plan_execute` 及其 2026-06-07 完成的全部泛化工作（synthesizer、artifacts、assessment policy、混合能力计划）**在生产环境不可达**。

2. **`DocumentInvestigator` 与 `KnowledgeDiscoverer` 未在 bootstrap 注入**

   生产实际注册的 capability 只有：`web_search`、`web_fetch`、`external_evidence_collect`、`think`、`content_summarize`、`memory_recall`（依赖 memoryService 存在）。整个诊断族——旧 runtime 的主要工作——能力代码已存在但没有接线。

3. **等价矩阵现状**（`agent_tool_parity_matrix.md`，2026-06-09）

   - ready：2（`web_search`、`web_fetch`）
   - partial：约 10（外部证据工作流、文档/任务诊断族）
   - missing：4（`trace_node_query`、`trace_retrieval_diagnose`、`document_root_cause_diagnosis`、think 当时未对齐）
   - 矩阵结论"当前不建议删除旧 `rag/tool/`"是正确的——意味着双 runtime 税继续存在

4. **默认 `maxIterations = 2`**（`service.go::defaultAgentMaxIterations`）

   对 search→fetch 够用，对多跳诊断明显不足——侧面印证新 runtime 尚未承接旧 runtime 的真实工作负载。

### 2.3 运维就绪度 — C（两个硬阻塞）

1. **持久化只有内存实现**

   `runtime.MemorySessionStore` 与 `kernel.MemoryCheckpointStore` 是默认且唯一实现：

   - 待审批会话**不能跨进程重启存活**
   - 无法水平扩展（resume 必须命中同一实例）
   - 对一个核心卖点是"暂停等待人工确认"的功能，这是最高优先级的生产阻塞项

2. **没有增量事件流**

   - runtime 在运行期间积累 `Journal`，运行结束后整体返回
   - 旧 runtime 实时下发 `tool_start / tool_result / agent_think` SSE 事件；新链路在完成前前端什么都看不到
   - 今天把 chat 切到新 runtime 等于**用户体验回退**——Journal 是做流式的正确底座，缺的只是 emitter

3. **chat → agent 交接有损**

   `service.go::seedRuntimeSessionFromToolStage` 把 rewrite / memory / session / knowledge 上下文压平成 320~400 字符截断的 `notes` 字符串——结构化上下文在 agent 看到之前就退化成了散文。

### 2.4 测试质量 — A−

**优点：**

- 测试/生产代码比约 10.2k : 14k，且构成正确：
  - `capability/contract_test.go`（518 行）对所有 capability 强制 spec 不变量
  - 服务级回归覆盖 approval / resume / error path / session boundary
  - `checkpoint_regression_test.go` + `replay_test.go` 锁定核心状态机行为
  - 2026-06-07 新增的混合能力 plan_execute 服务级回归
- `service_test_helpers.go` 模式压住了装配重复，避免了老模块的"SetXXX 测试地狱"

**缺口：**

- `pattern/reactive/pattern_test.go` 已达 1223 行，正在滑向被批评过的巨型测试文件
- **没有任务级评估**：只有行为正确性测试，没有"`doc_fail_01` 类场景成功率 / 回合数 / 降级率"的量化评估（对应 P2-4）

### 2.5 评级汇总

- 架构 / 分层：**A−**（全仓库最佳；轻微过深）
- 类型安全 / 契约：**B+**（全程类型化 I/O，但 schema 有名无实）
- 生产集成：**C−**（五周的工作被一个未注入的依赖和一行硬编码挡在生产之外）
- 运维就绪：**C**（内存存储、无流式）
- 测试：**A−**
- 战略方向：**正确**——风险不是设计错误，而是"建成"与"上线"的差距扩大

---

## 三、未来方向

### 3.1 近期：关闭部署差距（比任何新功能都紧急）

1. **bootstrap 接线**
   - `agent_runtime.go` 注入 `DocumentInvestigator` + `KnowledgeDiscoverer`
   - 新增 `trace_investigation` capability，补齐 trace 诊断链路
   - 完成后等价矩阵大部分 partial → ready
2. **Journal-tap 事件流**
   - 在 runner 上挂事件监听器，把 journal 事件实时翻译为现有 SSE `tool_start / tool_result` 契约
   - 没有这一步，P0-4 灰度会单纯因 UX 回退而失败
3. **持久化 SessionStore / CheckpointStore**
   - Postgres 或 Redis 适配器各一个；接口已经干净，工作量约两个小 adapter
4. **执行 P0-4 双 runtime 收口**
   - 观测 → 灰度 → 删除旧 AgentLoop（回收约 3000+ 行）

### 3.2 中期：能力与规划深度

5. **真 JSON Schema**：从 struct tag 派生字段级 schema 进 `Spec`，LLM selector / planner 按字段校验而不是信任描述——参数幻觉的结构性修复
6. **按请求路由 pattern**：启动时编译两张图，按问题特征在 reactive / plan_execute 间路由（selector 已存在，只是还不能选 pattern）
7. **第一批写操作 capability**：`memory_save` → `document_create`，全部走 approval 流程，把 approval/resume 从演示特性变成知识库助手的安全机制
8. **任务级评估基建**（P2-4）：固定场景集，按 pattern 统计成功率 / 回合数 / 降级率——retrieve 侧有 `retrieve-eval`，agent 侧目前没有对应物
9. **结构化 ToolStageContext 交接**：替换压平的 notes 字符串

### 3.3 长期：架构想去的地方

10. **`KindSubAgent` 已声明但未使用**：多 agent 委托（诊断子 agent、研究子 agent）是自然的下一个 pattern，handoff 与 state 的设计已为此预留
11. **定时自治运行**：plan_execute + 持久化 checkpoint 支撑知识库健康巡检、订阅源轮询——这是"知识库管家"产品目标的 runtime 底座
12. **情景记忆（episodic memory）**：把运行 journal 沉淀为可召回经验，重复诊断更快收敛——与 `docs/memory_improvement_plan.md` 的 L3 直接衔接

---

## 四、与其他计划文档的衔接

- 双 runtime 收口：`docs/structural_improvement_plan.md` P0-4（本报告 3.1 第 4 项是其前置补强）
- 等价矩阵与阻塞删除清单：`docs/agent_tool_parity_matrix.md`
- memory_save capability 与情景记忆：`docs/memory_improvement_plan.md` M3 / L3
- 任务级评估：`docs/functional_improvement_todo_20260609.md` 第 14 项
