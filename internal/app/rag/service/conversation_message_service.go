package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

type AddConversationMessageInput struct {
	ConversationID   string
	UserID           string
	Role             convention.Role
	Content          string
	ThinkingContent  string
	ThinkingDuration *int
}

type ListConversationMessagesInput struct {
	ConversationID string
	UserID         string
	Limit          int
	Order          port.ConversationMessageOrder
}

type AddConversationSummaryInput struct {
	ConversationID string
	UserID         string
	Content        string
	LastMessageID  string
}

type GetLatestConversationSummaryInput struct {
	ConversationID string
	UserID         string
}

type ConversationMessageView struct {
	ID               string
	ConversationID   string
	Role             convention.Role
	Content          string
	ThinkingContent  string
	ThinkingDuration *int
	Vote             *int
	CreateTime       time.Time
}

type ConversationMessageService struct {
	conversationRepo port.ConversationRepository
	messageRepo      port.ConversationMessageRepository
	summaryRepo      port.ConversationSummaryRepository
	feedbackRepo     port.MessageFeedbackRepository
	now              func() time.Time
}

// NewConversationMessageService 创建会话消息服务。
func NewConversationMessageService(
	conversationRepo port.ConversationRepository,
	messageRepo port.ConversationMessageRepository,
	summaryRepo port.ConversationSummaryRepository,
	feedbackRepo port.MessageFeedbackRepository,
) *ConversationMessageService {
	return &ConversationMessageService{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
		summaryRepo:      summaryRepo,
		feedbackRepo:     feedbackRepo,
		now:              time.Now,
	}
}

// AddMessage 新增一条会话消息记录。
func (s *ConversationMessageService) AddMessage(ctx context.Context, input AddConversationMessageInput) (domain.ConversationMessage, error) {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	content := strings.TrimSpace(input.Content)
	if conversationID == "" {
		return domain.ConversationMessage{}, exception.NewClientException("conversation id is required", nil)
	}
	if userID == "" {
		return domain.ConversationMessage{}, exception.NewClientException("user id is required", nil)
	}
	if content == "" {
		return domain.ConversationMessage{}, exception.NewClientException("message content is required", nil)
	}
	if s.messageRepo == nil {
		return domain.ConversationMessage{}, exception.NewServiceException("conversation message repository is required", nil)
	}

	// 生成独立消息主键，并补齐创建时间与更新时间。
	id, err := nextConversationMessageID()
	if err != nil {
		return domain.ConversationMessage{}, err
	}
	now := s.now()
	message := domain.ConversationMessage{
		ID:               id,
		ConversationID:   conversationID,
		UserID:           userID,
		Role:             string(input.Role),
		Content:          content,
		ThinkingContent:  strings.TrimSpace(input.ThinkingContent),
		ThinkingDuration: input.ThinkingDuration,
		CreateTime:       now,
		UpdateTime:       now,
	}
	created, err := s.messageRepo.Create(ctx, message)
	if err != nil {
		return domain.ConversationMessage{}, exception.NewServiceException("failed to create conversation message", err)
	}
	return created, nil
}

// ListMessages 返回指定会话的消息列表，并补齐助手消息的投票信息。
func (s *ConversationMessageService) ListMessages(ctx context.Context, input ListConversationMessagesInput) ([]ConversationMessageView, error) {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	if conversationID == "" || userID == "" {
		return []ConversationMessageView{}, nil
	}
	if s.conversationRepo == nil || s.messageRepo == nil {
		return nil, exception.NewServiceException("conversation repositories are required", nil)
	}

	conversation, err := s.conversationRepo.GetByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return nil, exception.NewServiceException("failed to load conversation", err)
	}
	if conversation.ID == "" {
		return []ConversationMessageView{}, nil
	}

	// 统一处理排序参数，兼容默认正序和“取最近消息后再反转”的场景。
	order := input.Order
	if order == "" {
		order = port.ConversationMessageOrderAsc
	}
	queryOrder := order
	reverseResult := false
	if order == port.ConversationMessageOrderDesc {
		queryOrder = port.ConversationMessageOrderDesc
		reverseResult = true
	}

	messages, err := s.messageRepo.List(ctx, port.ConversationMessageListFilter{
		ConversationID: conversationID,
		UserID:         userID,
		Order:          queryOrder,
		Limit:          input.Limit,
	})
	if err != nil {
		return nil, exception.NewServiceException("failed to list conversation messages", err)
	}
	if len(messages) == 0 {
		return []ConversationMessageView{}, nil
	}
	if reverseResult {
		// 仓储层按倒序取最近消息后，这里反转回时间正序，方便上层直接展示和组装上下文。
		reverseConversationMessages(messages)
	}

	// 单独聚合助手消息的投票状态，避免污染消息主查询。
	votesByMessageID, err := s.loadVotes(ctx, userID, messages)
	if err != nil {
		return nil, err
	}

	result := make([]ConversationMessageView, 0, len(messages))
	for _, message := range messages {
		role, parseErr := convention.ParseRole(message.Role)
		if parseErr != nil {
			continue
		}
		result = append(result, ConversationMessageView{
			ID:               message.ID,
			ConversationID:   message.ConversationID,
			Role:             role,
			Content:          message.Content,
			ThinkingContent:  message.ThinkingContent,
			ThinkingDuration: message.ThinkingDuration,
			Vote:             votesByMessageID[message.ID],
			CreateTime:       message.CreateTime,
		})
	}
	return result, nil
}

// AddMessageSummary 为会话写入一条摘要记录。
func (s *ConversationMessageService) AddMessageSummary(ctx context.Context, input AddConversationSummaryInput) (domain.ConversationSummary, error) {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	content := strings.TrimSpace(input.Content)
	if conversationID == "" {
		return domain.ConversationSummary{}, exception.NewClientException("conversation id is required", nil)
	}
	if userID == "" {
		return domain.ConversationSummary{}, exception.NewClientException("user id is required", nil)
	}
	if content == "" {
		return domain.ConversationSummary{}, exception.NewClientException("conversation summary content is required", nil)
	}
	if s.summaryRepo == nil {
		return domain.ConversationSummary{}, exception.NewServiceException("conversation summary repository is required", nil)
	}

	id, err := nextConversationSummaryID()
	if err != nil {
		return domain.ConversationSummary{}, err
	}
	now := s.now()
	summary := domain.ConversationSummary{
		ID:             id,
		ConversationID: conversationID,
		UserID:         userID,
		Content:        content,
		LastMessageID:  strings.TrimSpace(input.LastMessageID),
		CreateTime:     now,
		UpdateTime:     now,
	}
	created, err := s.summaryRepo.Create(ctx, summary)
	if err != nil {
		return domain.ConversationSummary{}, exception.NewServiceException("failed to create conversation summary", err)
	}
	return created, nil
}

// GetLatestSummary 返回指定会话最近的一条摘要记录。
func (s *ConversationMessageService) GetLatestSummary(ctx context.Context, input GetLatestConversationSummaryInput) (domain.ConversationSummary, error) {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	if conversationID == "" {
		return domain.ConversationSummary{}, exception.NewClientException("conversation id is required", nil)
	}
	if userID == "" {
		return domain.ConversationSummary{}, exception.NewClientException("user id is required", nil)
	}
	if s.summaryRepo == nil {
		return domain.ConversationSummary{}, exception.NewServiceException("conversation summary repository is required", nil)
	}

	summary, err := s.summaryRepo.FindLatestByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return domain.ConversationSummary{}, exception.NewServiceException("failed to load latest conversation summary", err)
	}
	return summary, nil
}

// loadVotes 批量查询助手消息的投票信息，并按消息 ID 回填。
func (s *ConversationMessageService) loadVotes(ctx context.Context, userID string, messages []domain.ConversationMessage) (map[string]*int, error) {
	result := make(map[string]*int)
	if s.feedbackRepo == nil || len(messages) == 0 {
		return result, nil
	}

	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		if strings.EqualFold(message.Role, string(convention.AssistantRole)) {
			messageIDs = append(messageIDs, message.ID)
		}
	}
	if len(messageIDs) == 0 {
		return result, nil
	}

	feedbacks, err := s.feedbackRepo.ListByUserIDAndMessageIDs(ctx, userID, messageIDs)
	if err != nil {
		return nil, exception.NewServiceException("failed to load message feedback", err)
	}
	for _, feedback := range feedbacks {
		vote := feedback.Vote
		result[feedback.MessageID] = &vote
	}
	return result, nil
}

// reverseConversationMessages 原地反转消息顺序。
func reverseConversationMessages(items []domain.ConversationMessage) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

// nextConversationMessageID 生成新的消息主键。
func nextConversationMessageID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate conversation message id", err)
	}
	return strconv.FormatInt(id, 10), nil
}

// nextConversationSummaryID 生成新的摘要主键。
func nextConversationSummaryID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate conversation summary id", err)
	}
	return strconv.FormatInt(id, 10), nil
}
