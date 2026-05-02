package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
)

// MessageServiceStore 基于 repository 提供记忆层需要的历史读取与消息追加能力。
type MessageServiceStore struct {
	conversationRepo port.ConversationRepository
	messageRepo      port.ConversationMessageRepository
	now              func() time.Time
}

// NewMessageServiceStore 创建基于 repository 的记忆存储适配器。
func NewMessageServiceStore(
	conversationRepo port.ConversationRepository,
	messageRepo port.ConversationMessageRepository,
) *MessageServiceStore {
	return &MessageServiceStore{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
		now:              time.Now,
	}
}

// LoadHistory 读取最近历史，并整理成模型可用的时间正序。
func (s *MessageServiceStore) LoadHistory(ctx context.Context, conversationID string, userID string, limit int) ([]convention.ChatMessage, error) {
	if s == nil || s.conversationRepo == nil || s.messageRepo == nil {
		return []convention.ChatMessage{}, nil
	}

	conversation, err := s.conversationRepo.GetByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("load conversation: %w", err)
	}
	if conversation.ID == "" {
		return []convention.ChatMessage{}, nil
	}

	items, err := s.messageRepo.List(ctx, port.ConversationMessageListFilter{
		ConversationID: conversationID,
		UserID:         userID,
		Order:          port.ConversationMessageOrderDesc,
		Limit:          limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list conversation messages: %w", err)
	}
	if len(items) == 0 {
		return []convention.ChatMessage{}, nil
	}

	result := make([]convention.ChatMessage, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		message, ok := toChatMessage(items[i])
		if ok {
			result = append(result, message)
		}
	}
	return normalizeHistory(result), nil
}

// Append 追加一条消息，并复用统一的持久化结构。
func (s *MessageServiceStore) Append(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) (string, error) {
	if s == nil || s.messageRepo == nil {
		return "", nil
	}

	id, err := nextIDString()
	if err != nil {
		return "", err
	}
	now := s.now()
	record := domain.ConversationMessage{
		ID:               id,
		ConversationID:   conversationID,
		UserID:           userID,
		Role:             string(message.Role),
		Content:          strings.TrimSpace(message.Content),
		ThinkingContent:  strings.TrimSpace(message.ThinkingContent),
		ThinkingDuration: toOptionalInt(message.ThinkingDuration),
		CreateTime:       now,
		UpdateTime:       now,
	}
	created, err := s.messageRepo.Create(ctx, record)
	if err != nil {
		return "", fmt.Errorf("create conversation message: %w", err)
	}
	return created.ID, nil
}

// RefreshCache 为后续缓存型实现预留，当前无需处理。
func (s *MessageServiceStore) RefreshCache(context.Context, string, string) error {
	return nil
}

// SummaryServiceAdapter 基于摘要 repository 提供摘要读取能力。
type SummaryServiceAdapter struct {
	summaryRepo port.ConversationSummaryRepository
}

// NewSummaryServiceAdapter 创建基于摘要 repository 的适配器。
func NewSummaryServiceAdapter(summaryRepo port.ConversationSummaryRepository) *SummaryServiceAdapter {
	return &SummaryServiceAdapter{summaryRepo: summaryRepo}
}

// LoadLatestSummary 读取最近摘要。
func (s *SummaryServiceAdapter) LoadLatestSummary(ctx context.Context, conversationID string, userID string) (*convention.ChatMessage, error) {
	if s == nil || s.summaryRepo == nil {
		return nil, nil
	}

	summary, err := s.summaryRepo.FindLatestByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("find latest conversation summary: %w", err)
	}
	if summary.ID == "" || summary.Content == "" {
		return nil, nil
	}
	message := convention.SystemMessage(summary.Content)
	return &message, nil
}

// DecorateIfNeeded 给摘要补齐统一前缀。
func (s *SummaryServiceAdapter) DecorateIfNeeded(summary *convention.ChatMessage) *convention.ChatMessage {
	if summary == nil || summary.Content == "" {
		return summary
	}
	if strings.HasPrefix(summary.Content, "对话摘要：") {
		return summary
	}
	decorated := convention.SystemMessage("对话摘要：" + summary.Content)
	return &decorated
}

// CompressIfNeeded 为后续摘要压缩预留接口，一期先不启用。
func (s *SummaryServiceAdapter) CompressIfNeeded(context.Context, string, string, convention.ChatMessage) error {
	return nil
}
