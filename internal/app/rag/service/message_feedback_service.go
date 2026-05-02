package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

// SubmitMessageFeedbackInput 描述消息反馈提交参数。
type SubmitMessageFeedbackInput struct {
	MessageID string
	UserID    string
	Vote      int
}

// MessageFeedbackService 负责保存消息点赞点踩结果。
type MessageFeedbackService struct {
	messageRepo  port.ConversationMessageRepository
	feedbackRepo port.MessageFeedbackRepository
	now          func() time.Time
}

// NewMessageFeedbackService 创建消息反馈服务。
func NewMessageFeedbackService(
	messageRepo port.ConversationMessageRepository,
	feedbackRepo port.MessageFeedbackRepository,
) *MessageFeedbackService {
	return &MessageFeedbackService{
		messageRepo:  messageRepo,
		feedbackRepo: feedbackRepo,
		now:          time.Now,
	}
}

// Submit 保存或更新一条消息反馈。
func (s *MessageFeedbackService) Submit(ctx context.Context, input SubmitMessageFeedbackInput) error {
	if s == nil || s.messageRepo == nil || s.feedbackRepo == nil {
		return exception.NewServiceException("message feedback repositories are required", nil)
	}

	messageID := strings.TrimSpace(input.MessageID)
	userID := strings.TrimSpace(input.UserID)
	if messageID == "" {
		return exception.NewClientException("message id is required", nil)
	}
	if userID == "" {
		return exception.NewClientException("user id is required", nil)
	}
	if input.Vote != 1 && input.Vote != -1 {
		return exception.NewClientException("vote must be 1 or -1", nil)
	}

	// 先确认消息属于当前用户，避免跨会话写反馈。
	message, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil {
		return exception.NewServiceException("failed to load conversation message", err)
	}
	if message.ID == "" || strings.TrimSpace(message.UserID) != userID {
		return exception.NewClientException("conversation message not found", nil)
	}

	existing, err := s.feedbackRepo.GetByMessageIDAndUserID(ctx, messageID, userID)
	if err != nil {
		return exception.NewServiceException("failed to load message feedback", err)
	}

	now := s.now()
	if existing.ID == "" {
		id, err := nextMessageFeedbackID()
		if err != nil {
			return err
		}
		_, err = s.feedbackRepo.Create(ctx, domain.MessageFeedback{
			ID:             id,
			MessageID:      message.ID,
			ConversationID: message.ConversationID,
			UserID:         userID,
			Vote:           input.Vote,
			CreateTime:     now,
			UpdateTime:     now,
		})
		if err != nil {
			return exception.NewServiceException("failed to create message feedback", err)
		}
		return nil
	}

	existing.Vote = input.Vote
	existing.UpdateTime = now
	if _, err := s.feedbackRepo.Update(ctx, existing); err != nil {
		return exception.NewServiceException("failed to update message feedback", err)
	}
	return nil
}

// nextMessageFeedbackID 生成新的反馈主键。
func nextMessageFeedbackID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate message feedback id", err)
	}
	return fmt.Sprintf("%d", id), nil
}
