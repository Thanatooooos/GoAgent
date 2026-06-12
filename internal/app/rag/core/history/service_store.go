package history

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	aichat "local/rag-project/internal/infra-ai/chat"
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
		ContentSummary:   strings.TrimSpace(message.Content),
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
	summaryRepo  port.ConversationSummaryRepository
	messageRepo  port.ConversationMessageRepository
	chatService  aichat.LLMService
	startTurns   int
	maxChars     int
	jobEnqueuer  SummaryJobEnqueuer
	asyncEnabled bool
	engine       summaryCompressionEngine
}

// SummaryCompressionOptions 描述摘要压缩的配置参数。
type SummaryCompressionOptions struct {
	MessageRepo  port.ConversationMessageRepository
	ChatService  aichat.LLMService
	StartTurns   int
	MaxChars     int
	JobEnqueuer  SummaryJobEnqueuer
	AsyncEnabled bool
}

// NewSummaryServiceAdapter 创建基于摘要 repository 的适配器（不支持压缩）。
func NewSummaryServiceAdapter(summaryRepo port.ConversationSummaryRepository) *SummaryServiceAdapter {
	return &SummaryServiceAdapter{summaryRepo: summaryRepo}
}

// NewCompressibleSummaryService 创建支持 LLM 压缩的摘要服务。
func NewCompressibleSummaryService(
	summaryRepo port.ConversationSummaryRepository,
	options SummaryCompressionOptions,
) *SummaryServiceAdapter {
	if options.StartTurns <= 0 {
		options.StartTurns = 10
	}
	if options.MaxChars <= 0 {
		options.MaxChars = 200
	}
	engine := summaryCompressionEngine{
		summaryRepo: summaryRepo,
		messageRepo: options.MessageRepo,
		chatService: options.ChatService,
		startTurns:  options.StartTurns,
		maxChars:    options.MaxChars,
		now:         time.Now,
	}
	adapter := &SummaryServiceAdapter{
		summaryRepo: summaryRepo,
		messageRepo: options.MessageRepo,
		chatService: options.ChatService,
		startTurns:  options.StartTurns,
		maxChars:    options.MaxChars,
		jobEnqueuer: options.JobEnqueuer,
		engine:      engine,
	}
	if options.AsyncEnabled {
		adapter.asyncEnabled = true
	}
	return adapter
}

// EnableAsyncSummaryJobs starts an in-memory worker and routes compression through it.
func (s *SummaryServiceAdapter) EnableAsyncSummaryJobs(queueSize int) *InMemorySummaryJobWorker {
	if s == nil {
		return nil
	}
	worker := NewInMemorySummaryJobWorker(s.engine, queueSize)
	worker.Start()
	s.jobEnqueuer = worker
	s.asyncEnabled = true
	return worker
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

// CompressIfNeeded 在消息数超过阈值时触发 LLM 摘要压缩。
func (s *SummaryServiceAdapter) CompressIfNeeded(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) error {
	if s == nil || s.messageRepo == nil || s.chatService == nil || s.summaryRepo == nil {
		return nil
	}
	if s.startTurns <= 0 {
		return nil
	}
	conversationID = strings.TrimSpace(conversationID)
	userID = strings.TrimSpace(userID)
	if conversationID == "" || userID == "" {
		return nil
	}

	_ = message
	input := SummaryJobInput{
		ConversationID: conversationID,
		UserID:         userID,
		RebuildReason:  "threshold_reached",
	}
	if s.asyncEnabled && s.jobEnqueuer != nil {
		return s.jobEnqueuer.EnqueueConversationSummary(ctx, input)
	}
	return s.engine.runConversationSummaryCompression(ctx, input)
}

const (
	compressSummarySystemPrompt = `你是一个对话摘要助手。请将以下对话历史压缩为一段简洁的摘要，保留关键事实、用户问题和结论。

要求：
1. 摘要长度不超过 %d 个字符。
2. 使用中文。
3. 按时间顺序组织，突出重要信息。
4. 不要包含无关细节和客套话。`
)

// buildCompressPrompt 构建压缩摘要的 system prompt。
func buildCompressPrompt(maxChars int, previousSummary string, historyMessages []domain.ConversationMessage) string {
	prompt := fmt.Sprintf(compressSummarySystemPrompt, maxChars)
	var builder strings.Builder
	builder.WriteString(prompt)
	if previousSummary != "" {
		builder.WriteString("\n\n上一次压缩的摘要为：\n")
		builder.WriteString(previousSummary)
		builder.WriteString("\n\n请结合之前的摘要，将以下新对话内容合并到摘要中。")
	}
	builder.WriteString("\n\n## 新对话\n")

	for _, msg := range historyMessages {
		role := msg.Role
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "user":
			role = "用户"
		case "assistant":
			role = "助手"
		case "system":
			continue
		default:
			role = "用户"
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if utf8.RuneCountInString(content) > 500 {
			content = string([]rune(content)[:500])
		}
		builder.WriteString(role)
		builder.WriteString("：")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return builder.String()
}

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

func toOptionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	result := value
	return &result
}

func nextIDString() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", fmt.Errorf("generate distributed id: %w", err)
	}
	return strconv.FormatInt(id, 10), nil
}
