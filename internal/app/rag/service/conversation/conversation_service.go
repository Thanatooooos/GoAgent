package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	defaultConversationTitleMaxLength = 30
	defaultConversationTitleFallback  = "新对话"
)

type CreateOrUpdateConversationInput struct {
	ConversationID string
	UserID         string
	Question       string
	LastTime       *time.Time
}

type RenameConversationInput struct {
	ConversationID string
	UserID         string
	Title          string
}

type DeleteConversationInput struct {
	ConversationID string
	UserID         string
}

type ListConversationsInput struct {
	UserID string
}

type ConversationService struct {
	conversationRepo port.ConversationRepository
	messageRepo      port.ConversationMessageRepository
	summaryRepo      port.ConversationSummaryRepository
	deleteTx         port.ConversationDeleteTransaction
	promptLoader     *ragprompt.TemplateLoader
	llmService       aichat.LLMService
	titleMaxLength   int
	now              func() time.Time
}

// NewConversationService 创建会话服务，并初始化标题长度等基础配置。
func NewConversationService(
	conversationRepo port.ConversationRepository,
	messageRepo port.ConversationMessageRepository,
	summaryRepo port.ConversationSummaryRepository,
	promptLoader *ragprompt.TemplateLoader,
	llmService aichat.LLMService,
	titleMaxLength int,
	deleteTx port.ConversationDeleteTransaction,
) *ConversationService {
	if titleMaxLength <= 0 {
		titleMaxLength = resolveConversationTitleMaxLength()
	}
	return &ConversationService{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
		summaryRepo:      summaryRepo,
		deleteTx:         deleteTx,
		promptLoader:     promptLoader,
		llmService:       llmService,
		titleMaxLength:   titleMaxLength,
		now:              time.Now,
	}
}

// List 返回指定用户的会话列表。
func (s *ConversationService) List(ctx context.Context, input ListConversationsInput) ([]domain.Conversation, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return nil, exception.NewClientException("user id is required", nil)
	}
	if s.conversationRepo == nil {
		return nil, exception.NewServiceException("conversation repository is required", nil)
	}

	items, err := s.conversationRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, exception.NewServiceException("failed to list conversations", err)
	}
	return items, nil
}

// CreateOrUpdate 在会话不存在时创建会话，存在时只更新时间。
func (s *ConversationService) CreateOrUpdate(ctx context.Context, input CreateOrUpdateConversationInput) (domain.Conversation, error) {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	question := strings.TrimSpace(input.Question)
	if conversationID == "" {
		return domain.Conversation{}, exception.NewClientException("conversation id is required", nil)
	}
	if userID == "" {
		return domain.Conversation{}, exception.NewClientException("user id is required", nil)
	}
	if s.conversationRepo == nil {
		return domain.Conversation{}, exception.NewServiceException("conversation repository is required", nil)
	}

	existing, err := s.conversationRepo.GetByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return domain.Conversation{}, exception.NewServiceException("failed to load conversation", err)
	}

	// 统一补齐最后活跃时间，避免上层遗漏时间字段。
	lastTime := input.LastTime
	if lastTime == nil {
		now := s.now()
		lastTime = &now
	}

	if existing.ID == "" {
		// 首次发起对话时创建会话，并生成一个可读标题。
		id, err := nextConversationID()
		if err != nil {
			return domain.Conversation{}, err
		}
		now := s.now()
		conversation := domain.Conversation{
			ID:             id,
			ConversationID: conversationID,
			UserID:         userID,
			Title:          s.generateTitle(ctx, question),
			LastTime:       lastTime,
			CreateTime:     now,
			UpdateTime:     now,
		}
		created, err := s.conversationRepo.Create(ctx, conversation)
		if err != nil {
			return domain.Conversation{}, exception.NewServiceException("failed to create conversation", err)
		}
		return created, nil
	}

	// 已存在会话时只刷新最后活跃时间，避免覆盖已有标题等信息。
	existing.LastTime = lastTime
	existing.UpdateTime = s.now()
	updated, err := s.conversationRepo.Update(ctx, existing)
	if err != nil {
		return domain.Conversation{}, exception.NewServiceException("failed to update conversation", err)
	}
	return updated, nil
}

// Rename 修改指定会话的标题。
func (s *ConversationService) Rename(ctx context.Context, input RenameConversationInput) error {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	title := strings.TrimSpace(input.Title)
	if conversationID == "" {
		return exception.NewClientException("conversation id is required", nil)
	}
	if userID == "" {
		return exception.NewClientException("user id is required", nil)
	}
	if title == "" {
		return exception.NewClientException("conversation title is required", nil)
	}
	if utf8.RuneCountInString(title) > s.titleMaxLength {
		return exception.NewClientException(fmt.Sprintf("conversation title length must be less than or equal to %d", s.titleMaxLength), nil)
	}
	if s.conversationRepo == nil {
		return exception.NewServiceException("conversation repository is required", nil)
	}

	conversation, err := s.conversationRepo.GetByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return exception.NewServiceException("failed to load conversation", err)
	}
	if conversation.ID == "" {
		return exception.NewClientException("conversation not found", nil)
	}

	conversation.Title = title
	conversation.UpdateTime = s.now()
	if _, err := s.conversationRepo.Update(ctx, conversation); err != nil {
		return exception.NewServiceException("failed to rename conversation", err)
	}
	return nil
}

// Delete 删除会话，并级联清理会话下的消息和摘要。
func (s *ConversationService) Delete(ctx context.Context, input DeleteConversationInput) error {
	conversationID := strings.TrimSpace(input.ConversationID)
	userID := strings.TrimSpace(input.UserID)
	if conversationID == "" {
		return exception.NewClientException("conversation id is required", nil)
	}
	if userID == "" {
		return exception.NewClientException("user id is required", nil)
	}
	if s.conversationRepo == nil || s.messageRepo == nil || s.summaryRepo == nil || s.deleteTx == nil {
		return exception.NewServiceException("conversation repositories and delete transaction are required", nil)
	}

	conversation, err := s.conversationRepo.GetByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return exception.NewServiceException("failed to load conversation", err)
	}
	if conversation.ID == "" {
		return exception.NewClientException("conversation not found", nil)
	}

	return s.deleteTx(ctx, func(
		txCtx context.Context,
		conversationRepo port.ConversationRepository,
		messageRepo port.ConversationMessageRepository,
		summaryRepo port.ConversationSummaryRepository,
	) error {
		if err := conversationRepo.Delete(txCtx, conversation.ID); err != nil {
			return exception.NewServiceException("failed to delete conversation", err)
		}
		if err := messageRepo.DeleteByConversationIDAndUserID(txCtx, conversationID, userID); err != nil {
			return exception.NewServiceException("failed to delete conversation messages", err)
		}
		if err := summaryRepo.DeleteByConversationIDAndUserID(txCtx, conversationID, userID); err != nil {
			return exception.NewServiceException("failed to delete conversation summaries", err)
		}
		return nil
	})
}

// generateTitle 生成会话标题，失败时回退到基于问题文本的截断标题。
func (s *ConversationService) generateTitle(ctx context.Context, question string) string {
	fallback := truncateConversationTitle(question, s.titleMaxLength)
	if fallback == "" {
		fallback = defaultConversationTitleFallback
	}
	if s.llmService == nil || strings.TrimSpace(question) == "" {
		return fallback
	}

	systemPrompt := "请根据用户问题生成一个简短的中文会话标题，只输出标题本身，不要加引号、序号或解释。"
	userPrompt := fmt.Sprintf("请为下面的问题生成一个不超过%d个中文字符的标题：\n%s", s.titleMaxLength, question)
	// 使用一个很小的中文提示词生成标题，避免把标题逻辑散落到上层。
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(systemPrompt),
			convention.UserMessage(userPrompt),
		},
	}

	title, err := s.llmService.ChatWithRequest(request)
	if err != nil {
		return fallback
	}
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'“”‘’")
	title = truncateConversationTitle(title, s.titleMaxLength)
	if title == "" {
		return fallback
	}
	return title
}

// truncateConversationTitle 按字符数截断标题，避免超出展示和配置限制。
func truncateConversationTitle(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if maxLength <= 0 {
		maxLength = defaultConversationTitleMaxLength
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return strings.TrimSpace(string(runes[:maxLength]))
}

// resolveConversationTitleMaxLength 读取会话标题长度配置，不存在时使用默认值。
func resolveConversationTitleMaxLength() int {
	cfg := config.Get()
	if cfg != nil && cfg.Rag.Memory.TitleMaxLength > 0 {
		return cfg.Rag.Memory.TitleMaxLength
	}
	return defaultConversationTitleMaxLength
}

// nextConversationID 生成新的会话主键。
func nextConversationID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate conversation id", err)
	}
	return fmt.Sprintf("%d", id), nil
}
