package memory

import (
	"context"
	"fmt"

	"local/rag-project/internal/framework/convention"
)

const DefaultHistoryLimit = 10

type DefaultService struct {
	store        Store
	summary      SummaryService
	historyLimit int
}

// NewDefaultService 创建一期默认的会话记忆服务。
func NewDefaultService(store Store, summary SummaryService, historyLimit int) *DefaultService {
	if historyLimit <= 0 {
		historyLimit = DefaultHistoryLimit
	}
	return &DefaultService{
		store:        store,
		summary:      summary,
		historyLimit: historyLimit,
	}
}

// Load 先读取消息历史，再按需把摘要插入到上下文最前面。
func (s *DefaultService) Load(ctx context.Context, conversationID string, userID string) ([]convention.ChatMessage, error) {
	if s == nil || s.store == nil {
		return []convention.ChatMessage{}, nil
	}

	history, err := s.store.LoadHistory(ctx, conversationID, userID, s.historyLimit)
	if err != nil {
		return nil, fmt.Errorf("load memory history: %w", err)
	}
	if s.summary == nil {
		return history, nil
	}

	summaryMessage, err := s.summary.LoadLatestSummary(ctx, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("load memory summary: %w", err)
	}
	summaryMessage = s.summary.DecorateIfNeeded(summaryMessage)
	if summaryMessage == nil {
		return history, nil
	}

	// 摘要存在时放在最前面，方便模型先看到压缩后的全局背景。
	result := make([]convention.ChatMessage, 0, len(history)+1)
	result = append(result, *summaryMessage)
	result = append(result, history...)
	return result, nil
}

// Append 把新消息写入存储，并为后续摘要压缩预留调用点。
func (s *DefaultService) Append(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) (string, error) {
	if s == nil || s.store == nil {
		return "", nil
	}

	messageID, err := s.store.Append(ctx, conversationID, userID, message)
	if err != nil {
		return "", fmt.Errorf("append memory message: %w", err)
	}
	if s.summary != nil {
		if err := s.summary.CompressIfNeeded(ctx, conversationID, userID, message); err != nil {
			return "", fmt.Errorf("compress memory summary: %w", err)
		}
	}
	return messageID, nil
}

// LoadAndAppend 用于聊天链路中“先取上下文，再落当前消息”的标准流程。
func (s *DefaultService) LoadAndAppend(ctx context.Context, conversationID string, userID string, message convention.ChatMessage) ([]convention.ChatMessage, error) {
	history, err := s.Load(ctx, conversationID, userID)
	if err != nil {
		return nil, err
	}
	if _, err := s.Append(ctx, conversationID, userID, message); err != nil {
		return nil, err
	}
	return history, nil
}
