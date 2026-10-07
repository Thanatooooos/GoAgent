package runtimeadapter

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/convention"
)

// ConversationHistory projects only durable user and final assistant messages.
// Runtime adds the current user message and its own journal after this source.
type ConversationHistory struct {
	messages  port.ConversationMessageRepository
	summaries port.ConversationSummaryRepository
}

func NewConversationHistory(messages port.ConversationMessageRepository, summaries port.ConversationSummaryRepository) ConversationHistory {
	return ConversationHistory{messages: messages, summaries: summaries}
}

func (h ConversationHistory) Messages(ctx context.Context, request conversationruntime.RunRequest) ([]conversationruntime.ModelMessage, error) {
	snapshot, err := h.Snapshot(ctx, request)
	if err != nil {
		return nil, err
	}
	result := make([]conversationruntime.ModelMessage, 0, len(snapshot.Messages)+1)
	if content := strings.TrimSpace(snapshot.Summary.Content); content != "" {
		result = append(result, conversationruntime.ModelMessage{Role: conversationruntime.ModelRoleSystem, Content: conversationSummaryHeader + content})
	}
	for _, message := range snapshot.Messages {
		result = append(result, conversationruntime.ModelMessage{Role: message.Role, Content: message.Content})
	}
	return result, nil
}

func (h ConversationHistory) Snapshot(ctx context.Context, request conversationruntime.RunRequest) (conversationruntime.HistorySnapshot, error) {
	if h.messages == nil {
		return conversationruntime.HistorySnapshot{}, fmt.Errorf("conversation message service is required")
	}
	snapshot := conversationruntime.HistorySnapshot{}
	if h.summaries != nil {
		summary, err := h.summaries.FindLatestByConversationIDAndUserID(ctx, request.ConversationID, request.UserID)
		if err != nil {
			return conversationruntime.HistorySnapshot{}, fmt.Errorf("load conversation summary: %w", err)
		}
		snapshot.Summary = conversationruntime.HistorySummary{
			Content:              strings.TrimSpace(summary.Content),
			StructuredJSON:       strings.TrimSpace(summary.StructuredSummaryJSON),
			CoveredFromMessageID: strings.TrimSpace(summary.CoveredFromMessageID),
			CoveredToMessageID:   strings.TrimSpace(summary.CoveredToMessageID),
			SourceMessageCount:   summary.SourceMessageCount,
		}
	}
	views, err := h.messages.List(ctx, port.ConversationMessageListFilter{ConversationID: request.ConversationID, UserID: request.UserID, AfterID: snapshot.Summary.CoveredToMessageID, Order: port.ConversationMessageOrderAsc, Limit: 500})
	if err != nil {
		return conversationruntime.HistorySnapshot{}, err
	}
	for _, view := range views {
		if view.ID == request.UserMessageID {
			continue
		}
		switch view.Role {
		case string(convention.UserRole):
			snapshot.Messages = append(snapshot.Messages, conversationruntime.HistoryMessage{ID: view.ID, Role: conversationruntime.ModelRoleUser, Content: view.Content})
		case string(convention.AssistantRole):
			snapshot.Messages = append(snapshot.Messages, conversationruntime.HistoryMessage{ID: view.ID, Role: conversationruntime.ModelRoleAssistant, Content: view.Content})
		}
	}
	return snapshot, nil
}

func (h ConversationHistory) StoreSummary(ctx context.Context, request conversationruntime.RunRequest, summary conversationruntime.HistorySummary) (bool, error) {
	if h.summaries == nil {
		return false, fmt.Errorf("conversation summary repository is required")
	}
	coverage, ok := h.summaries.(port.ConversationSummaryCoverageRepository)
	if !ok {
		return false, fmt.Errorf("conversation summary repository does not support coverage advancement")
	}
	coveredTo := strings.TrimSpace(summary.CoveredToMessageID)
	if coveredTo == "" {
		return false, fmt.Errorf("summary coverage boundary is required")
	}
	accepted, err := coverage.CreateIfCoverageAdvances(ctx, domain.ConversationSummary{
		ID:                    coveredTo,
		ConversationID:        request.ConversationID,
		UserID:                request.UserID,
		Content:               strings.TrimSpace(summary.Content),
		StructuredSummaryJSON: strings.TrimSpace(summary.StructuredJSON),
		LastMessageID:         coveredTo,
		SummaryVersion:        1,
		CoveredFromMessageID:  strings.TrimSpace(summary.CoveredFromMessageID),
		CoveredToMessageID:    coveredTo,
		SourceMessageCount:    summary.SourceMessageCount,
		QualityStatus:         "runtime",
		LastRebuildReason:     "context_budget",
	})
	if err != nil {
		return false, fmt.Errorf("advance conversation summary: %w", err)
	}
	return accepted, nil
}

var _ conversationruntime.ConversationHistory = ConversationHistory{}
var _ conversationruntime.CompactionHistory = ConversationHistory{}
