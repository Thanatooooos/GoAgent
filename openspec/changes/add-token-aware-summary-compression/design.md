# add-token-aware-summary-compression Design

## Overview

本设计将 summary 压缩从固定轮数策略改为 token-aware 增量策略，并补齐真实聊天链路接入。

系统采用两阶段预算模型：

1. **上一轮结束后的预测预算**
   - 下一轮 retrieve / tool 内容未知。
   - 使用阶段 token 信封计算 history 可用预算。
   - 当“最新 summary + 未覆盖消息尾部”超过 history budget 时，异步生成新 summary。
2. **当前轮 prompt 构建时的实际预算**
   - retrieve、tool、memory 等内容已经生成。
   - 使用实际内容重新估算完整 prompt。
   - 动态裁剪并保证最终 prompt 不超过上限。

该模型不追求预测未知内容，而是通过上限合同与最终实测共同保证安全。

## Existing Runtime Findings

### Existing Summary Path

现有主要组件：

- `internal/app/rag/core/history/summary_compression.go`
- `internal/app/rag/core/history/service_store.go`
- `internal/app/rag/core/history/default_service.go`
- `internal/bootstrap/rag/runtime_build_conversation.go`

已有 summary 能力包括：

- structured summary generation
- previous summary 合并
- repair
- validation
- renderer
- 覆盖消息边界字段
- 可选的内存异步 worker

### Existing Delivery Gap

真实聊天链路：

```text
RagChatService
  -> MessageService.AddMessage(user)
  -> execute answer
  -> MessageService.AddMessage(assistant)
```

现有压缩调用链路：

```text
history.DefaultService.Append
  -> SummaryService.CompressIfNeeded
```

生产聊天只调用 `history.Load`，没有调用 `history.Append` 或 `LoadAndAppend`。因此压缩 helper 虽有模块测试，但没有接入真实消息写入路径。

### Existing Prompt Budget

chat 已有最终 prompt budget：

- 构建完整 prompt 后估算 token
- 超限时依次裁剪 history、tool、knowledge、session、memory
- summary message 被视为 pinned history

该能力继续保留，但 estimator 和阶段预算需要统一。

## Design Principles

### 1. Budget Unknown Work, Measure Known Work

下一轮未知内容使用阶段预算信封；当前轮已生成内容使用实际 token 估算。

### 2. Incremental, Not Full-History

summary 只消费最新 summary 未覆盖的新消息，不重复扫描和压缩全部历史。

### 3. Post-Answer and Fail-Open

压缩发生在助手回复成功落库后，不阻塞主回答。

### 4. One Estimation Contract

不同模块可以有不同预算，但必须共享 estimator 语义。

### 5. Final Prompt Budget Remains Authoritative

阶段预算是规划手段，最终完整 prompt 实测是硬保护。

## Unified Token Estimator

### Interface

保留一个稳定接口：

```go
type TokenEstimator interface {
    EstimateTokens(text string) int
}
```

chat、summary、retrieve、tool 和 evaluation 都通过该接口工作。

### Default Implementation

第一阶段使用仓库内统一的通用 estimator。现有 `tokenestimate` 和 `RoughTokenEstimator` 不应继续作为两套生产口径并存。

推荐将默认实现放在共享边界，例如：

```text
internal/app/rag/core/tokenbudget
```

上层模块依赖接口，不直接依赖具体第三方包。

### Message Accounting

单条消息估算：

```text
message_tokens =
    estimator(content)
  + role_and_envelope_overhead
```

完整历史估算：

```text
raw_history_tokens =
    summary_message_tokens
  + sum(uncovered_message_tokens)
```

触发估算：

```text
effective_history_tokens =
ceil(raw_history_tokens * safety_factor)
```

初始配置建议：

```yaml
summary-token:
  safety-factor: 1.15
  message-overhead-tokens: 4
```

安全系数只在预算决策层应用一次，不能在每个子阶段重复放大。

### Future Model-Specific Tokenizers

未来可以根据实际回答模型选择 estimator：

- model-specific tokenizer
- provider usage metadata
- calibrated generic estimator

但这些实现必须遵循同一接口，不改变 summary 触发和 stage budget 合同。

## Prompt Budget Model

### Budget Formula

```text
history_budget =
    max_prompt_tokens
  - fixed_prompt_reserve
  - memory_budget
  - session_recall_budget
  - retrieve_budget
  - tool_budget
  - safety_reserve
```

如果显式配置 `summary-trigger-tokens`，它覆盖自动计算的 history budget。

### Initial Budget

基于当前 `max-prompt-tokens = 8000`，初始建议：

| Stage | Tokens |
| --- | ---: |
| fixed prompt + current question | 800 |
| long-term memory | 500 |
| session recall | 1500 |
| retrieve context | 2000 |
| tool context | 1500 |
| safety reserve | 500 |
| summary + uncovered history | 1200 |

这组值是初始保护值，不是最终性能结论。上线后根据 trace 的 P90/P95 调整。

### Configuration

建议配置：

```yaml
rag:
  memory:
    summary-trigger-tokens: 0
    summary-token:
      safety-factor: 1.15
      message-overhead-tokens: 4
    chat-context:
      enabled: true
      max-prompt-tokens: 8000
      fixed-reserve-tokens: 800
      safety-reserve-tokens: 500
      stage-budget:
        memory-tokens: 500
        session-recall-tokens: 1500
        retrieve-tokens: 2000
        tool-tokens: 1500
```

约定：

- `summary-trigger-tokens > 0`：显式 history 阈值。
- `summary-trigger-tokens = 0`：根据阶段预算自动计算。
- 自动计算结果必须大于可配置最小 history budget。
- 配置不合法时启动失败或使用明确记录的安全默认值，不能静默生成负预算。

## Compression Trigger Data Flow

### Trigger Timing

触发点放在助手消息成功落库之后：

```text
assistant message persisted
  -> enqueue summary check
  -> return finish/done normally
  -> worker evaluates uncovered history tokens
  -> optionally generates summary
```

主回答已经成功，因此压缩失败只能记录，不能反向改变 chat 状态。

### Message Boundary

加载最新有效 summary：

```text
covered_to = latestSummary.CoveredToMessageID
```

加载消息：

- 无 summary：按时间正序加载可压缩消息。
- 有 summary：查询 `AfterID = covered_to` 的消息。
- 只包含 user / assistant。
- 不包含 system、空消息或已被长期消息处理器替换掉的无效内容。

### Trigger Input

```text
summary_tokens = tokens(rendered latest summary or structured representation)
tail_tokens = tokens(messages after covered_to)
effective_tokens = ceil((summary_tokens + tail_tokens) * safety_factor)
```

当：

```text
effective_tokens >= history_budget
```

则执行压缩。

### Compression Input

压缩输入必须与触发输入对齐：

- previous summary：最新 structured summary
- source messages：`covered_to` 之后的全部未覆盖消息

不能再出现：

- 用全部历史决定触发
- 只取固定 6 条执行压缩

如果未覆盖尾部本身大于 summary 模型安全输入上限，应按 token 边界分批增量压缩，而不是简单丢弃最旧或最新消息。

## Real Chat Integration

### Integration Point

推荐在 `RagChatService.persistAssistantMessage` 成功后调用一个只负责调度的接口：

```go
type SummaryTrigger interface {
    EnqueueCheck(ctx context.Context, conversationID, userID string) error
}
```

该接口不负责同步生成 summary。

原因：

- 此时 user 和 assistant 消息都已经落库。
- 当前轮回答已经完成。
- 不需要重复使用 `history.Store.Append` 再写一遍消息。
- 可以避免将消息持久化与 summary 调度耦合在同一 repository wrapper 中。

### Existing History Service

`history.DefaultService.Append` 不再是生产 summary 触发的唯一入口。

可选迁移方式：

- 保留 `Append` 供独立调用者使用。
- 将压缩调度抽成共享 `SummaryTrigger`。
- chat 与 `DefaultService.Append` 都可以调用同一 trigger，但必须依赖覆盖边界保证幂等。

## Concurrency and Idempotency

### Duplicate Trigger Scenarios

可能重复触发：

- 用户快速发送下一轮消息。
- assistant success/cancel 路径重复调度。
- worker 重试。
- 多实例同时消费相同会话任务。

### Required Guard

压缩任务至少携带：

- `conversation_id`
- `user_id`
- enqueue 时观察到的 latest message id

worker 执行时重新读取：

- 最新 summary coverage
- 最新消息边界

如果目标边界已被等价或更新的 summary 覆盖，则跳过。

第一阶段可复用现有进程内 worker，但必须提供会话级 in-flight 去重。多实例部署时，最终正确性不能只依赖内存锁；保存前需再次检查覆盖边界，或使用数据库条件写入/锁实现等价保护。

## Retrieve Budget

### Current Problem

retrieve 最终 context 主要由命中 chunk 数量决定，缺少明确 token 上限。

### Design

在 `KnowledgeContext` 注入 prompt 之前应用：

```text
truncate_context_to_token_budget(retrieve_context, retrieve_budget)
```

截断应尽量按 chunk 边界进行：

1. 按排序结果依次加入 chunk。
2. 达到预算后停止。
3. 单个 chunk 超预算时，按 token 截断该 chunk。
4. 保留来源 ID / 标题等最小定位信息。

输出观测：

- candidate chunk count
- retained chunk count
- estimated tokens before / after
- truncated
- truncation reason

## Tool Budget

### Current Problem

tool renderer 当前使用字符硬限制，例如 `12000 chars`。字符数对中文、英文、代码、JSON 的 token 成本不稳定。

### Design

tool context 使用双重保护：

1. token budget：正常主限制
2. character hard cap：防止异常超大输出和 estimator 故障

tool context 渲染后：

```text
tool_context = truncate_to_token_budget(rendered_context, tool_budget)
```

更优实现是让 renderer 按 tool result section 边界逐段加入，以保留结论、关键证据和来源，而不是从字符串尾部机械截断。

输出观测：

- tool call count
- context tokens before / after
- truncated
- retained sections
- dropped sections

## Current-Request Actual Recalculation

当 memory、session recall、retrieve、tool 都已经得到实际结果后，复用 prompt service 构建真实 message 列表，并统一估算：

```text
actual_prompt_tokens =
tokens(promptService.BuildMessages(actualContext))
```

阶段预算不是不可借用的硬分区。若 tool 未使用，其空间可以被 retrieve 或 history 实际占用，只要最终 prompt 不超过总上限。

### Degradation Order

推荐顺序：

1. 丢弃 summary 之后最旧的未压缩 history。
2. 裁剪低优先级 tool 细节，保留结论与来源。
3. 裁剪低排序 retrieve chunks。
4. 裁剪 session recall。
5. 裁剪 long-term memory。
6. 保留 summary、当前问题、固定 prompt 和必要 policy。

如果业务决定 Tool 证据优先于旧 history，现有顺序基本合理；但裁剪必须按结构边界，而不是统一字符缩短。

## Summary Completion Race

由于压缩异步执行，下一轮可能在 summary 完成前开始。

第一阶段行为：

- 下一轮继续使用旧 summary + 最近 history。
- 最终 prompt budget 仍可裁剪。
- 不等待正在执行的 summary。

可观测字段记录：

- summary job pending / running
- next request arrived before completion

本期不引入同步等待，以避免增加首包延迟。

## Failure Handling

- estimator 返回异常或非正值：使用安全 fallback estimator，并记录错误。
- history budget 无效：使用最小安全预算，并记录配置错误。
- enqueue 失败：记录并继续聊天成功。
- worker 失败：记录失败，不创建低质量 summary。
- summary validation 失败：保留旧 summary 和原始消息。
- retrieve/tool 截断失败：由最终 prompt budget 继续兜底。

## Observability

每次 summary 判断记录：

- conversation id
- latest summary id
- covered-to message id
- tail message count
- summary tokens
- tail tokens
- raw history tokens
- effective history tokens
- history budget
- trigger decision
- estimator name/version
- safety factor

每次实际 prompt 构建记录：

- fixed prompt tokens
- summary/history tokens
- memory tokens
- session recall tokens
- retrieve tokens before/after
- tool tokens before/after
- total prompt tokens
- degradation steps

指标建议复用现有 metrics：

- summary check count
- summary trigger count
- summary skipped count by reason
- duplicate/in-flight skip count
- summary success/failure count
- retrieve/tool truncation count
- actual prompt token histogram
- stage token histogram

## Rollout

1. 先统一 estimator 与观测，不改变触发策略。
2. 接入 retrieve/tool token 上限及当前轮实际重算。
3. 接通 assistant 落库后的异步 summary trigger。
4. 启用 token 触发，保留轮数配置但不参与决策。
5. 使用离线 strategy eval 和线上 trace 校准初始预算。
6. 稳定后清理废弃的轮数触发逻辑。

## Testing Strategy

### Unit Tests

- estimator 对中文、英文、代码、JSON 的基本稳定性
- message overhead 和 safety factor 只应用一次
- history budget 计算与非法配置
- `AfterID` 边界加载
- 无 summary / 有 summary 两种触发
- 低于、等于、高于阈值
- retrieve 按 chunk 边界截断
- tool 按 section/token 截断

### Integration Tests

- assistant message 落库后会调度 summary check
- 调度失败不影响 chat finish
- user + assistant 完整一轮进入未覆盖尾部
- 新 summary 只覆盖旧 summary 之后的消息
- 下一次 load 使用新 summary + 最近尾部
- 重复任务不会生成重复覆盖 summary
- 当前轮实际 prompt 不超过总预算

### Evaluation

复用 summary strategy mode 比较不同 history budget 下的：

- token saved ratio
- summary fidelity
- summary usefulness
- downstream equivalence
- dangerous drift

## Open Questions

实现计划开始前只剩一个需要通过数据校准的问题：

- 初始 stage budget 是否直接采用本文 `800 / 500 / 1500 / 2000 / 1500 / 500`，还是先从现有 trace 抽取一次 token 分布后再启用。

默认建议先采用本文安全值上线到开发环境，同时记录分布；不阻塞实现。
