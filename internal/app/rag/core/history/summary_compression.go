package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/log"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type summaryCompressionEngine struct {
	summaryRepo           port.ConversationSummaryRepository
	messageRepo           port.ConversationMessageRepository
	chatService           aichat.LLMService
	triggerTokens         int
	estimator             TokenEstimator
	safetyFactor          float64
	messageOverheadTokens int
	maxChars              int
	budget                SummaryBudgetOptions
	now                   func() time.Time
}

func (e summaryCompressionEngine) runConversationSummaryCompression(ctx context.Context, input SummaryJobInput) error {
	if e.summaryRepo == nil || e.messageRepo == nil || e.chatService == nil || e.triggerTokens <= 0 {
		return nil
	}
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	if conversationID == "" || userID == "" {
		return nil
	}

	latestSummary, err := e.summaryRepo.FindLatestByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return fmt.Errorf("load latest summary: %w", err)
	}
	coveredToMessageID := strings.TrimSpace(latestSummary.CoveredToMessageID)
	if coveredToMessageID == "" {
		coveredToMessageID = strings.TrimSpace(latestSummary.LastMessageID)
	}
	historyMessages, err := e.messageRepo.List(ctx, port.ConversationMessageListFilter{
		ConversationID: conversationID,
		UserID:         userID,
		Roles: []string{
			string(convention.UserRole),
			string(convention.AssistantRole),
		},
		AfterID:   coveredToMessageID,
		ThroughID: strings.TrimSpace(input.TargetMessageID),
		Order:     port.ConversationMessageOrderAsc,
		Limit:     500,
	})
	if err != nil {
		return fmt.Errorf("load messages for compression: %w", err)
	}
	if len(historyMessages) == 0 {
		return nil
	}

	summaryTokens := e.estimator.EstimateTokens(strings.TrimSpace(latestSummary.Content))
	tailTokens := estimateMessagesTokensWithOverhead(historyMessages, e.estimator, e.messageOverheadTokens)
	rawHistoryTokens := summaryTokens + tailTokens
	effectiveHistoryTokens := tokenbudget.ApplySafetyFactor(rawHistoryTokens, e.safetyFactor)
	triggered := effectiveHistoryTokens >= e.triggerTokens
	estimatorName, estimatorVersion := tokenbudget.DescribeEstimator(e.estimator)
	log.FromContext(ctx).Infow(
		"summary compression token check",
		"conversation_id", conversationID,
		"user_id", userID,
		"covered_to_message_id", coveredToMessageID,
		"target_message_id", strings.TrimSpace(input.TargetMessageID),
		"tail_message_count", len(historyMessages),
		"summary_tokens", summaryTokens,
		"tail_tokens", tailTokens,
		"raw_history_tokens", rawHistoryTokens,
		"effective_history_tokens", effectiveHistoryTokens,
		"trigger_tokens", e.triggerTokens,
		"estimator_name", estimatorName,
		"estimator_version", estimatorVersion,
		"safety_factor", e.safetyFactor,
		"triggered", triggered,
	)
	if !triggered {
		return nil
	}

	tier := SelectSummaryBudget(SummaryBudgetInput{
		MessageCount: len(historyMessages),
		TotalChars:   countMessageChars(historyMessages),
		TotalTokens:  effectiveHistoryTokens,
		Messages:     messageContents(historyMessages),
	}, e.budget)

	jsonMode := true
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(buildStructuredSummaryPrompt(tier, latestSummary, historyMessages)),
			convention.UserMessage("现在请直接返回结构化工作记忆 JSON。"),
		},
		JSONMode: &jsonMode,
	}
	response, err := e.chatService.ChatWithRequest(request)
	if err != nil {
		return fmt.Errorf("compress summary llm call: %w", err)
	}

	structured, err := ParseStructuredSummary(strings.TrimSpace(response))
	if err != nil {
		return fmt.Errorf("parse structured summary: %w", err)
	}
	repaired := RepairStructuredSummary(structured)
	validation := ValidateStructuredSummary(repaired, historyMessages)
	if !validation.Accepted {
		return nil
	}

	rendered := RenderStructuredSummary(repaired, tier.MaxChars)
	if strings.TrimSpace(rendered) == "" {
		return nil
	}

	rebuildReason := strings.TrimSpace(input.RebuildReason)
	if rebuildReason == "" {
		rebuildReason = "threshold_reached"
	}
	summaryRecord, err := buildConversationSummaryRecord(
		conversationID,
		userID,
		rendered,
		marshalStructuredSummary(repaired),
		historyMessages,
		rebuildReason,
		domain.SummaryQualityAccepted,
		e.now(),
	)
	if err != nil {
		return err
	}
	if coverageRepo, ok := e.summaryRepo.(port.ConversationSummaryCoverageRepository); ok {
		accepted, createErr := coverageRepo.CreateIfCoverageAdvances(ctx, summaryRecord)
		if createErr != nil {
			return fmt.Errorf("save compressed summary: %w", createErr)
		}
		if !accepted {
			return nil
		}
		return nil
	}
	if _, err = e.summaryRepo.Create(ctx, summaryRecord); err != nil {
		return fmt.Errorf("save compressed summary: %w", err)
	}
	return nil
}

func buildConversationSummaryRecord(
	conversationID string,
	userID string,
	content string,
	structuredSummaryJSON string,
	historyMessages []domain.ConversationMessage,
	rebuildReason string,
	qualityStatus string,
	now time.Time,
) (domain.ConversationSummary, error) {
	id, err := nextIDString()
	if err != nil {
		return domain.ConversationSummary{}, err
	}
	coveredToMessageID := ""
	coveredFromMessageID := ""
	if len(historyMessages) > 0 {
		coveredFromMessageID = strings.TrimSpace(historyMessages[0].ID)
		coveredToMessageID = strings.TrimSpace(historyMessages[len(historyMessages)-1].ID)
	}
	if strings.TrimSpace(qualityStatus) == "" {
		qualityStatus = domain.SummaryQualityUnchecked
	}
	return domain.ConversationSummary{
		ID:                    id,
		ConversationID:        conversationID,
		UserID:                userID,
		Content:               content,
		StructuredSummaryJSON: strings.TrimSpace(structuredSummaryJSON),
		LastMessageID:         coveredToMessageID,
		SummaryVersion:        domain.SummaryVersionV1,
		CoveredFromMessageID:  coveredFromMessageID,
		CoveredToMessageID:    coveredToMessageID,
		SourceMessageCount:    len(historyMessages),
		QualityStatus:         qualityStatus,
		LastRebuildReason:     strings.TrimSpace(rebuildReason),
		CreateTime:            now,
		UpdateTime:            now,
	}, nil
}

type StructuredSummaryPromptVariant string

const (
	StructuredSummaryPromptVariantStateAware StructuredSummaryPromptVariant = "state-aware"
	StructuredSummaryPromptVariantLegacy     StructuredSummaryPromptVariant = "legacy"
)

func NormalizeStructuredSummaryPromptVariant(variant StructuredSummaryPromptVariant) StructuredSummaryPromptVariant {
	switch variant {
	case StructuredSummaryPromptVariantLegacy:
		return StructuredSummaryPromptVariantLegacy
	default:
		return StructuredSummaryPromptVariantStateAware
	}
}

func ParseStructuredSummaryPromptVariant(raw string) (StructuredSummaryPromptVariant, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(StructuredSummaryPromptVariantStateAware):
		return StructuredSummaryPromptVariantStateAware, nil
	case string(StructuredSummaryPromptVariantLegacy):
		return StructuredSummaryPromptVariantLegacy, nil
	default:
		return "", fmt.Errorf("unsupported summary prompt variant %q", raw)
	}
}

const stateAwareStructuredSummarySystemPrompt = `你正在将一段对话压缩为结构化工作记忆。只返回严格 JSON，不允许输出任何额外内容。

========================
一、JSON Schema
========================
允许字段如下（必须严格遵守）：

- schema_version: number（必须为 1）
- goal: string
- active_priorities: string[]
- user_preferences: string[]
- constraints: string[]
- established_facts: string[]
- recent_progress: string[]
- open_questions: string[]
- background_issues: string[]

禁止：
- null
- undefined
- 额外字段
- markdown
- 解释文本

所有数组字段必须存在；无内容时返回 []。

========================
二、核心原则（最高优先级）
========================
优先级从高到低：

1. 不得编造任何信息（不确定内容必须进入 open_questions）
2. 保持用户显式意图（目标 / 优先级 / 不做什么）
3. 保留关键可验证事实（错误码 / config / version / 数值 / 决策）
4. 保持时间一致性（最新覆盖旧信息）
5. 保留状态边界事实（已确认但未执行、候选但非生产、已设计但未校准）
6. 满足字段容量限制（超过则执行淘汰策略）

========================
三、状态边界事实（非常重要）
========================
以下信息必须显式保留，不得弱化、推断或省略：

- 已确认但未执行：
  例如“PostgreSQL 已决定，但迁移尚未执行”。
  必须进入 established_facts。

- 已允许但未开始实现：
  例如“允许生成样本，但 production summary prompt/schema 实现尚未开始”。
  必须进入 established_facts 或 constraints。

- 候选 / 诊断 / 压力测试 / 生产最终：
  数值或方案必须绑定状态标签。
  禁止只写“8000”；必须写“8000 是 diagnostic run parameter，不是 production final threshold”。

- 仍开放 / 未校准 / 未采集：
  例如“trace 字段方案已确认，但 P90/P95 未校准”。
  已确认的未完成状态进入 established_facts；待决策问题进入 open_questions。

- 不确定性合同：
  例如“retrieve/tool token 在 summary 触发时不能精确预测，必须在 retrieve/tool 后重算完整 prompt”。
  必须进入 established_facts 或 constraints。

========================
四、禁止完成态漂移
========================
如果对话只表达“计划 / 建议 / 允许 / 讨论 / 诊断 / 样本建设 / 待 review”，禁止写成：

- 已完成
- 已上线
- 已实现
- 已执行
- 已确定
- 已证明有效

只有用户明确确认或实际结果明确出现时，才能使用完成态。

========================
五、时间规则
========================
- recent_progress：最近发生的动作、结果、失败、状态变化
- established_facts：已确认且需要后续稳定遵守的事实
- constraints：当前必须遵守的硬约束，包括“不做什么”
- 若信息发生冲突 → 以最新为准，旧信息必须删除或覆盖
- 如果最近变化形成了后续必须遵守的稳定状态，也必须用不同措辞写入 established_facts 或 constraints

========================
六、字段定义（严格约束）
========================

goal：
- 一句话
- 必须具体（包含对象/系统）
- 必须能指导下一步行动
- 禁止抽象总结（如“优化系统”“提升体验”）

active_priorities（≤5）：
仅允许来源：
- open_questions 中关键阻塞问题
- recent_progress 中未完成任务
- 用户明确要求的下一步动作
按执行优先级排序。

user_preferences：
- 用户明确表达的偏好
- 技术选型 / 架构偏好 / 工作方式

constraints（≤5）：
- 当前必须遵守的硬约束
- 包括“不能做什么”
- 必须保留具体配置 / key / 数值

established_facts（≤5）：
- 已确认且稳定的信息
- 必须可验证
- 必须保留原始关键字段（错误码 / version / config / 数值 / 状态标签）
- “已确认但未执行”“候选但非生产”“已设计但未校准”也属于 established_facts

recent_progress（≤5）：
- 最近发生 / 最近变化 / 新确认 / 任务进展
- 包含状态变化（成功 / 失败 / 修改）
- 不得把计划、建议、样本建设写成生产实现完成

open_questions：
- 如果对话中存在明确未确认、待验证、仍开放的问题，则必须非空
- 如果没有开放问题，返回 []
- 禁止为了满足格式而编造泛化疑问
- 禁止放入已确认事实

background_issues（≤5）：
- 明确提到但当前不处理的问题
- 不能进入 active_priorities

========================
七、去重与合并规则
========================
- 语义重复必须合并
- 同一事实不同表达 → 保留最新
- 冲突信息 → 以最新为准
- 可推导信息禁止重复存储
- established_facts 和 recent_progress 不得用同一句重复；如果都需要保留，recent_progress 写事件，established_facts 写稳定状态

========================
八、容量淘汰策略
========================
当超过限制时：

优先保留：
1. 具体数值 / 错误码 / config / version
2. 用户明确指令
3. 状态边界事实
4. 最近发生变化
5. 阻塞性问题

优先删除：
1. 泛化描述
2. 重复信息
3. 可推导信息
4. 低信息密度内容

========================
九、输出约束
========================
- 必须是合法 JSON
- 禁止 markdown / 注释 / 解释
- 禁止 trailing comma
- 所有数组字段必须存在（无则 []）
- schema_version 必须为 number 且 = 1
- 本摘要用于恢复当前对话的工作状态，不得改变当前对话自身的业务目标
- 最终渲染预算约 %d 字符。`

const legacyStructuredSummarySystemPrompt = `你正在将一段对话压缩为结构化工作记忆。只返回 JSON。
JSON 类型约定：允许字段：
- schema_version: 整数 (number)，固定为 1
- goal: 字符串
- active_priorities: 字符串数组，无该项时返回 []
- user_preferences: 字符串数组，无该项时返回 []
- constraints: 字符串数组，无该项时返回 []
- established_facts: 字符串数组，无该项时返回 []
- recent_progress: 字符串数组，无该项时返回 []
- open_questions: 字符串数组，无该项时返回 []
- background_issues: 字符串数组，无该项时返回 []

各字段内容指南：
- schema_version 必须是数字 1。不要写成 1.0、"1"、1.1 等形式。
- goal：一句话描述当前对话的主要目标。只保留当前仍然有效的目标和约束，保持当前边界；目标变更时保留最新的。
- active_priorities：当前下一步应该优先推进的事项。只写当前范围内有效且应主导后续计划的问题，active_priorities 按执行优先级排序。最多 5 项。
- user_preferences：用户明确表达的偏好（技术选型、工作流等）。
- constraints：当前有效的硬性约束。当前不做什么也属于 constraints。每条独立一项。保留具体数值、名称、配置 key。最多 5 项。
- established_facts：已确认的事实。不要把猜测写成 established_facts。特别关注决策变更（"从 A 改为 B"、"X 已作废"）。范围变化、优先级变化、作废决定，除了 recent_progress，也要在 established_facts 中保留可验证表述。错误码（如 ERR_POOL_TIMEOUT）、配置 key（如 pool.max_active=50）、版本号（如 v2.4.1）必须逐字保留。最多 5 项。
- recent_progress：最近取得的进展，最近刚确认或刚变化的状态优先写入 recent_progress。每条具体可验证。提及错误码、参数值、文件名。最多 5 项。
- open_questions：仍未解决的问题。未确认、待验证、候选信息放进 open_questions。如果对话中存在不确定性或未解决的问题，此字段不能为空。保留问题原文的关键措辞。
- background_issues：对话中明确提到但不是当前重点的问题。它们需要保留，但不要写进 active_priorities。最多 5 项。

规则：
1. 不要编造事实。不确定的信息放进 open_questions。
2. 已被新信息覆盖、作废的旧事实不要保留。
3. 只保留当前边界内仍然有效的信息，不要把更早阶段已经结束或已经过期的内容带回来。不要把较窄的未做事项扩写成更泛的阶段结论。
4. 如果某个根因、方案、因果链条仍未证实，保留“尚未确认/待验证”的措辞，不要把因果猜测改写成结论。
5. 错误码（如 ERR_POOL_TIMEOUT）、配置 key（如 pool.max_active）、版本号（如 v2.4.1）、具体决策必须逐字保留在摘要文本中。
6. 如果对话明确说某事项“不是当前重点/只是背景问题/暂不处理”，不要写进 active_priorities。
7. 如果事项只是待确认方向，但并未被设为当前主线，放进 open_questions，不要抬升为 active_priorities。
8. 如果用户已经总结当前重点，优先保持该重点顺序。
9. 最终渲染预算约 %d 字符。`

func buildStructuredSummaryPrompt(tier SummaryBudgetTier, latestSummary domain.ConversationSummary, historyMessages []domain.ConversationMessage) string {
	return buildStructuredSummaryPromptWithVariant(tier, latestSummary, historyMessages, StructuredSummaryPromptVariantStateAware)
}

func buildStructuredSummaryPromptWithVariant(
	tier SummaryBudgetTier,
	latestSummary domain.ConversationSummary,
	historyMessages []domain.ConversationMessage,
	variant StructuredSummaryPromptVariant,
) string {
	prompt := fmt.Sprintf(structuredSummaryPromptTemplate(variant), tier.MaxChars)

	var builder strings.Builder
	builder.WriteString(prompt)
	builder.WriteString("\n\u8865\u5145\u89c4\u5219：\u5982\u679c\u67d0\u9879\u7ed3\u8bba\u53ea\u6765\u81ea\u52a9\u624b\u5efa\u8bae\u3001\u793a\u4f8b\u4ee3\u7801\u6216\u901a\u7528\u65b9\u6848\u8bf4\u660e，\u800c\u6ca1\u6709\u88ab\u7528\u6237\u786e\u8ba4\u6216\u5b9e\u9645\u843d\u5730，\u4e0d\u8981\u5199\u6210 established_facts\u3002\n")

	previousStructured := strings.TrimSpace(latestSummary.StructuredSummaryJSON)
	if previousStructured != "" {
		builder.WriteString("\n上一次结构化摘要 JSON：\n")
		builder.WriteString(previousStructured)
		builder.WriteString("\n")
	} else if previousContent := strings.TrimSpace(latestSummary.Content); previousContent != "" {
		builder.WriteString("\n上一轮压缩摘要：\n")
		builder.WriteString(previousContent)
		builder.WriteString("\n")
	}

	builder.WriteString("\n最近消息：\n")
	for _, msg := range historyMessages {
		role := normalizeSummaryRoleLabel(msg.Role)
		if role == "" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if utf8.RuneCountInString(content) > 500 {
			content = trimRunes(content, 500)
		}
		builder.WriteString(role)
		builder.WriteString("：")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return builder.String()
}

func structuredSummaryPromptTemplate(variant StructuredSummaryPromptVariant) string {
	switch NormalizeStructuredSummaryPromptVariant(variant) {
	case StructuredSummaryPromptVariantLegacy:
		return legacyStructuredSummarySystemPrompt
	default:
		return stateAwareStructuredSummarySystemPrompt
	}
}

func normalizeSummaryRoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "用户"
	case "assistant":
		return "助手"
	default:
		return ""
	}
}

func countMessageChars(messages []domain.ConversationMessage) int {
	total := 0
	for _, message := range messages {
		total += utf8.RuneCountInString(strings.TrimSpace(message.Content))
	}
	return total
}

func messageContents(messages []domain.ConversationMessage) []string {
	if len(messages) == 0 {
		return nil
	}
	result := make([]string, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content != "" {
			result = append(result, content)
		}
	}
	return result
}

func estimateMessagesTokens(messages []domain.ConversationMessage, estimator TokenEstimator) int {
	return estimateMessagesTokensWithOverhead(messages, estimator, 0)
}

func estimateMessagesTokensWithOverhead(messages []domain.ConversationMessage, estimator TokenEstimator, overhead int) int {
	if estimator == nil {
		return 0
	}
	if overhead < 0 {
		overhead = 0
	}
	total := 0
	for _, message := range messages {
		total += estimator.EstimateTokens(strings.TrimSpace(message.Content)) + overhead
	}
	return total
}

func marshalStructuredSummary(summary StructuredSummary) string {
	summary.Normalize()
	payload, err := json.Marshal(summary)
	if err != nil {
		return ""
	}
	return string(payload)
}
