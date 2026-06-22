package history

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type summaryCompressionEngine struct {
	summaryRepo port.ConversationSummaryRepository
	messageRepo port.ConversationMessageRepository
	chatService aichat.LLMService
	startTurns  int
	maxChars    int
	budget      SummaryBudgetOptions
	now         func() time.Time
}

func (e summaryCompressionEngine) runConversationSummaryCompression(ctx context.Context, input SummaryJobInput) error {
	if e.summaryRepo == nil || e.messageRepo == nil || e.chatService == nil || e.startTurns <= 0 {
		return nil
	}
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	if conversationID == "" || userID == "" {
		return nil
	}

	userCount, _ := e.messageRepo.CountByConversationIDAndUserIDAndRole(ctx, conversationID, userID, string(convention.UserRole))
	assistantCount, _ := e.messageRepo.CountByConversationIDAndUserIDAndRole(ctx, conversationID, userID, string(convention.AssistantRole))
	totalMessages := int(userCount + assistantCount)
	threshold := e.startTurns * 2
	if totalMessages < threshold {
		return nil
	}

	latestSummary, _ := e.summaryRepo.FindLatestByConversationIDAndUserID(ctx, conversationID, userID)
	lastCompressedID := strings.TrimSpace(latestSummary.LastMessageID)

	historyMessages, err := e.messageRepo.List(ctx, port.ConversationMessageListFilter{
		ConversationID: conversationID,
		UserID:         userID,
		Order:          port.ConversationMessageOrderDesc,
		Limit:          threshold,
	})
	if err != nil {
		return fmt.Errorf("load messages for compression: %w", err)
	}
	if len(historyMessages) < threshold {
		return nil
	}
	if lastCompressedID != "" && historyMessages[0].ID == lastCompressedID {
		return nil
	}

	tier := SelectSummaryBudget(SummaryBudgetInput{
		MessageCount: len(historyMessages),
		TotalChars:   countMessageChars(historyMessages),
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
	_, err = e.summaryRepo.Create(ctx, summaryRecord)
	if err != nil {
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
		coveredToMessageID = strings.TrimSpace(historyMessages[0].ID)
		coveredFromMessageID = strings.TrimSpace(historyMessages[len(historyMessages)-1].ID)
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

const structuredSummarySystemPrompt = `你正在将一段对话压缩为结构化工作记忆。只返回 JSON。
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
	prompt := fmt.Sprintf(structuredSummarySystemPrompt, tier.MaxChars)

	var builder strings.Builder
	builder.WriteString(prompt)

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

func marshalStructuredSummary(summary StructuredSummary) string {
	summary.Normalize()
	payload, err := json.Marshal(summary)
	if err != nil {
		return ""
	}
	return string(payload)
}
