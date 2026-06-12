package history

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	compressPrompt := buildCompressPrompt(e.maxChars, latestSummary.Content, historyMessages)
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(compressPrompt),
			convention.UserMessage("请根据上述对话生成摘要。"),
		},
	}
	response, err := e.chatService.ChatWithRequest(request)
	if err != nil {
		return fmt.Errorf("compress summary llm call: %w", err)
	}
	summaryContent := strings.TrimSpace(response)
	if summaryContent == "" {
		return nil
	}

	rebuildReason := strings.TrimSpace(input.RebuildReason)
	if rebuildReason == "" {
		rebuildReason = "threshold_reached"
	}
	summaryRecord, err := buildConversationSummaryRecord(
		conversationID,
		userID,
		summaryContent,
		historyMessages,
		rebuildReason,
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
	historyMessages []domain.ConversationMessage,
	rebuildReason string,
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
	return domain.ConversationSummary{
		ID:                   id,
		ConversationID:       conversationID,
		UserID:               userID,
		Content:              content,
		LastMessageID:        coveredToMessageID,
		SummaryVersion:       domain.SummaryVersionV1,
		CoveredFromMessageID: coveredFromMessageID,
		CoveredToMessageID:   coveredToMessageID,
		SourceMessageCount:   len(historyMessages),
		QualityStatus:        domain.SummaryQualityUnchecked,
		LastRebuildReason:    strings.TrimSpace(rebuildReason),
		CreateTime:           now,
		UpdateTime:           now,
	}, nil
}
