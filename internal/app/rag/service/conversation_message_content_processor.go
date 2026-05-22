package service

import "context"

// NoopConversationMessageContentProcessor 保持当前写入行为不变，
// 作为长消息处理能力落地前的默认实现。
type RealConversationMessageContentProcessor struct{}

func RoughTokenEstimate(input AddConversationMessageInput) int {
	runes := []rune(input.Content)
	cjk := 0
	ascii := 0
	for _, r := range runes {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF:
			cjk++
		case r <= 127:
			ascii++
		default:
			cjk++
		}
	}
	return cjk + ascii/4
}

func (RealConversationMessageContentProcessor) ProcessAddMessage(_ context.Context, input AddConversationMessageInput) (ProcessedConversationMessageContent, error) {
	count := RoughTokenEstimate(input)
	switch {
	case count < 3000:
		return ProcessedConversationMessageContent{
			Content:        input.Content,
			RawContent:     input.RawContent,
			ContentSummary: input.ContentSummary,
			IsSummarized:   false,
		}, nil
	case count >= 3000 && count < 12000:

	}

	return ProcessedConversationMessageContent{
		Content:        input.Content,
		RawContent:     input.RawContent,
		ContentSummary: input.ContentSummary,
		IsSummarized:   input.IsSummarized,
	}, nil
}
