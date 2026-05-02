package memory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
)

type RepositoryStore struct {
	messageRepo port.ConversationMessageRepository
	now         func() time.Time
}

// NewRepositoryStore 创建基于 repository 的记忆存储实现。
func NewRepositoryStore(messageRepo port.ConversationMessageRepository) *RepositoryStore {
	return &RepositoryStore{
		messageRepo: messageRepo,
		now:         time.Now,
	}
}

// LoadHistory 直接从消息仓储读取最近历史，并整理成时间正序上下文。
func (s *RepositoryStore) LoadHistory(ctx context.Context, conversationID string, userID string, limit int) ([]convention.ChatMessage, error) {
	if s == nil || s.messageRepo == nil {
		return []convention.ChatMessage{}, nil
	}

	messages, err := s.messageRepo.List(ctx, port.ConversationMessageListFilter{
		ConversationID: conversationID,
		UserID:         userID,
		Order:          port.ConversationMessageOrderDesc,
		Limit:          limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list conversation messages: %w", err)
	}
	if len(messages) == 0 {
		return []convention.ChatMessage{}, nil
	}

	// 仓储按倒序取最近消息，这里翻转回正序，方便直接拼到模型上下文中。
	result := make([]convention.ChatMessage, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		message, ok := toChatMessage(messages[i])
		if ok {
			result = append(result, message)
		}
	}
	return normalizeHistory(result), nil
}

// Append 把一条聊天消息转换为持久化记录并写入仓储。
func (s *RepositoryStore) Append(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) (string, error) {
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
		Content:          message.Content,
		ThinkingContent:  message.ThinkingContent,
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

// RefreshCache 为缓存型实现预留，当前 repository 直读模式无需额外处理。
func (s *RepositoryStore) RefreshCache(context.Context, string, string) error {
	return nil
}

type RepositorySummaryService struct {
	summaryRepo port.ConversationSummaryRepository
}

// NewRepositorySummaryService 创建基于 repository 的摘要读取实现。
func NewRepositorySummaryService(summaryRepo port.ConversationSummaryRepository) *RepositorySummaryService {
	return &RepositorySummaryService{summaryRepo: summaryRepo}
}

// LoadLatestSummary 从摘要仓储读取最近摘要。
func (s *RepositorySummaryService) LoadLatestSummary(ctx context.Context, conversationID string, userID string) (*convention.ChatMessage, error) {
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

// DecorateIfNeeded 给摘要补齐统一中文前缀，方便模型区分摘要与普通系统消息。
func (s *RepositorySummaryService) DecorateIfNeeded(summary *convention.ChatMessage) *convention.ChatMessage {
	if summary == nil || summary.Content == "" {
		return summary
	}
	if strings.HasPrefix(summary.Content, "对话摘要：") {
		return summary
	}
	decorated := convention.SystemMessage("对话摘要：" + summary.Content)
	return &decorated
}

// CompressIfNeeded 为后续摘要压缩能力预留接口，一期先不启用。
func (s *RepositorySummaryService) CompressIfNeeded(context.Context, string, string, convention.ChatMessage) error {
	return nil
}

// toChatMessage 把持久化消息转换成模型可消费的聊天消息。
func toChatMessage(message domain.ConversationMessage) (convention.ChatMessage, bool) {
	role, err := convention.ParseRole(message.Role)
	if err != nil || message.Content == "" {
		return convention.ChatMessage{}, false
	}
	result := convention.ChatMessage{
		Role:            role,
		Content:         message.Content,
		ThinkingContent: message.ThinkingContent,
	}
	if message.ThinkingDuration != nil {
		result.ThinkingDuration = *message.ThinkingDuration
	}
	return result, true
}

// normalizeHistory 去掉开头连续的 assistant 消息，避免缺失用户提问时污染上下文。
func normalizeHistory(messages []convention.ChatMessage) []convention.ChatMessage {
	if len(messages) == 0 {
		return []convention.ChatMessage{}
	}
	start := 0
	for start < len(messages) && messages[start].Role == convention.AssistantRole {
		start++
	}
	if start >= len(messages) {
		return []convention.ChatMessage{}
	}
	return messages[start:]
}

// toOptionalInt 把 0 值时长转换为 nil，避免无意义写库。
func toOptionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	result := value
	return &result
}

// nextIDString 生成一条新的分布式字符串主键。
func nextIDString() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", fmt.Errorf("generate distributed id: %w", err)
	}
	return strconv.FormatInt(id, 10), nil
}
