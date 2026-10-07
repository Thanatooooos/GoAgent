package sessionrecall

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/port"
	ragconversation "local/rag-project/internal/app/rag/service/conversation"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type LongMessageProcessorOptions struct {
	Enabled                     bool
	DirectContextMaxTokens      int
	ChunkSummaryThresholdTokens int
	LargeChunkTargetTokens      int
	LargeChunkOverlapTokens     int
	MediumSummaryMaxChars       int
	ChunkSummaryMaxChars        int
	LargeSummaryMaxChars        int
	Estimator                   TokenEstimator
	ChatService                 aichat.LLMService
}

type LongMessageContentProcessor struct {
	options LongMessageProcessorOptions
}

func NewLongMessageContentProcessor(options LongMessageProcessorOptions) *LongMessageContentProcessor {
	if options.DirectContextMaxTokens <= 0 {
		options.DirectContextMaxTokens = 3000
	}
	if options.ChunkSummaryThresholdTokens <= 0 {
		options.ChunkSummaryThresholdTokens = 12000
	}
	if options.LargeChunkTargetTokens <= 0 {
		options.LargeChunkTargetTokens = 3000
	}
	if options.MediumSummaryMaxChars <= 0 {
		options.MediumSummaryMaxChars = 600
	}
	if options.ChunkSummaryMaxChars <= 0 {
		options.ChunkSummaryMaxChars = 240
	}
	if options.LargeSummaryMaxChars <= 0 {
		options.LargeSummaryMaxChars = 900
	}
	if options.Estimator == nil {
		options.Estimator = RoughTokenEstimator{}
	}
	return &LongMessageContentProcessor{options: options}
}

func (p *LongMessageContentProcessor) ProcessAddMessage(ctx context.Context, input ragconversation.AddConversationMessageInput) (ragconversation.ProcessedConversationMessageContent, error) {
	content := strings.TrimSpace(input.Content)
	if p == nil || !p.options.Enabled || content == "" {
		return ragconversation.ProcessedConversationMessageContent{
			Content:        content,
			RawContent:     strings.TrimSpace(input.RawContent),
			ContentSummary: strings.TrimSpace(input.ContentSummary),
			IsSummarized:   input.IsSummarized,
		}, nil
	}

	tokenCount := p.options.Estimator.EstimateTokens(content)
	if tokenCount <= p.options.DirectContextMaxTokens {
		return ragconversation.ProcessedConversationMessageContent{
			Content:        content,
			ContentSummary: content,
		}, nil
	}

	if tokenCount <= p.options.ChunkSummaryThresholdTokens {
		summary := p.summarizeMediumMessage(content, tokenCount)
		sessionChunks := p.buildRuleBasedSessionChunks(content)
		return ragconversation.ProcessedConversationMessageContent{
			Content:        summary,
			RawContent:     content,
			ContentSummary: summary,
			IsSummarized:   true,
			SessionChunks:  sessionChunks,
		}, nil
	}

	sessionChunks, chunkSummaries := p.buildSummarizedSessionChunks(ctx, content)
	summary := p.mergeChunkSummaries(tokenCount, chunkSummaries)
	return ragconversation.ProcessedConversationMessageContent{
		Content:        summary,
		RawContent:     content,
		ContentSummary: summary,
		IsSummarized:   true,
		SessionChunks:  sessionChunks,
	}, nil
}

func (p *LongMessageContentProcessor) buildRuleBasedSessionChunks(content string) []port.ProcessedConversationMessageChunk {
	chunks := splitTextByTokenBudget(content, p.options.LargeChunkTargetTokens, p.options.LargeChunkOverlapTokens, p.options.Estimator)
	if len(chunks) == 0 {
		return nil
	}
	result := make([]port.ProcessedConversationMessageChunk, 0, len(chunks))
	for idx, chunk := range chunks {
		tokenCount := p.options.Estimator.EstimateTokens(chunk)
		result = append(result, port.ProcessedConversationMessageChunk{
			ChunkIndex:     idx + 1,
			Content:        chunk,
			ContentSummary: buildChunkSummary(idx+1, len(chunks), chunk, tokenCount, p.options.ChunkSummaryMaxChars),
			TokenEstimate:  tokenCount,
		})
	}
	return result
}

func (p *LongMessageContentProcessor) buildSummarizedSessionChunks(ctx context.Context, content string) ([]port.ProcessedConversationMessageChunk, []string) {
	chunks := splitTextByTokenBudget(content, p.options.LargeChunkTargetTokens, p.options.LargeChunkOverlapTokens, p.options.Estimator)
	if len(chunks) == 0 {
		return nil, nil
	}
	chunkSummaries := make([]string, 0, len(chunks))
	sessionChunks := make([]port.ProcessedConversationMessageChunk, 0, len(chunks))
	for idx, chunk := range chunks {
		tokenCount := p.options.Estimator.EstimateTokens(chunk)
		summary := p.summarizeChunk(ctx, idx+1, len(chunks), chunk)
		chunkSummaries = append(chunkSummaries, summary)
		sessionChunks = append(sessionChunks, port.ProcessedConversationMessageChunk{
			ChunkIndex:     idx + 1,
			Content:        chunk,
			ContentSummary: summary,
			TokenEstimate:  tokenCount,
		})
	}
	return sessionChunks, chunkSummaries
}

func buildMediumMessageSummary(content string, tokenCount int, maxChars int) string {
	lines := splitNonEmptyLines(content)
	lineCount := len(lines)
	head := firstNonEmptyPreview(lines, 3, maxChars/2)
	tail := lastNonEmptyPreview(lines, 2, maxChars/3)
	kind := detectLongMessageKind(content)

	summary := fmt.Sprintf("长消息摘要：类型=%s，预估tokens=%d，行数=%d。开头重点：%s", kind, tokenCount, lineCount, head)
	if tail != "" && tail != head {
		summary += "。结尾片段：" + tail
	}
	return truncateRunes(summary, maxChars)
}

func buildChunkSummary(index int, total int, chunk string, tokenCount int, maxChars int) string {
	lines := splitNonEmptyLines(chunk)
	head := firstNonEmptyPreview(lines, 2, maxChars/2)
	tail := lastNonEmptyPreview(lines, 1, maxChars/3)
	summary := fmt.Sprintf("分段%d/%d：预估tokens=%d，重点=%s", index, total, tokenCount, head)
	if tail != "" && tail != head {
		summary += "；尾部=" + tail
	}
	return truncateRunes(summary, maxChars)
}

func mergeChunkSummaries(totalTokens int, chunkSummaries []string, maxChars int) string {
	if len(chunkSummaries) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("超长消息摘要：预估tokens=%d，共%d段。", totalTokens, len(chunkSummaries)))
	for idx, item := range chunkSummaries {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if idx > 0 {
			builder.WriteString(" ")
		}
		builder.WriteString(item)
	}
	return truncateRunes(builder.String(), maxChars)
}

func detectLongMessageKind(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "exception"), strings.Contains(lower, "traceback"), strings.Contains(lower, "error"), strings.Contains(lower, "panic:"):
		return "log_or_error"
	case strings.Contains(text, "```"), strings.Contains(lower, "func "), strings.Contains(lower, "package "), strings.Contains(lower, "class "):
		return "code"
	default:
		return "text"
	}
}

func (p *LongMessageContentProcessor) summarizeMediumMessage(content string, tokenCount int) string {
	if summary, err := p.callLLMSummaryPrompt(buildMediumSummaryPrompt(content, tokenCount, p.options.MediumSummaryMaxChars), p.options.MediumSummaryMaxChars); err == nil && summary != "" {
		return summary
	}
	return buildMediumMessageSummary(content, tokenCount, p.options.MediumSummaryMaxChars)
}

func (p *LongMessageContentProcessor) summarizeChunk(_ context.Context, index int, total int, chunk string) string {
	tokenCount := p.options.Estimator.EstimateTokens(chunk)
	if summary, err := p.callLLMSummaryPrompt(buildChunkSummaryPrompt(index, total, chunk, tokenCount, p.options.ChunkSummaryMaxChars), p.options.ChunkSummaryMaxChars); err == nil && summary != "" {
		return summary
	}
	return buildChunkSummary(index, total, chunk, tokenCount, p.options.ChunkSummaryMaxChars)
}

func (p *LongMessageContentProcessor) mergeChunkSummaries(totalTokens int, chunkSummaries []string) string {
	if summary, err := p.callLLMSummaryPrompt(buildMergeSummaryPrompt(totalTokens, chunkSummaries, p.options.LargeSummaryMaxChars), p.options.LargeSummaryMaxChars); err == nil && summary != "" {
		return summary
	}
	return mergeChunkSummaries(totalTokens, chunkSummaries, p.options.LargeSummaryMaxChars)
}

func (p *LongMessageContentProcessor) callLLMSummaryPrompt(prompt string, maxChars int) (string, error) {
	if p == nil || p.options.ChatService == nil {
		return "", fmt.Errorf("chat service unavailable")
	}
	maxTokens := estimateSummaryMaxTokens(maxChars)
	response, err := p.options.ChatService.ChatWithRequest(convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(prompt),
			convention.UserMessage(longMessageSummaryRequest),
		},
		MaxTokens: &maxTokens,
	})
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(response)
	if summary == "" {
		return "", fmt.Errorf("empty summary response")
	}
	return truncateRunes(summary, maxChars), nil
}

func buildMediumSummaryPrompt(content string, tokenCount int, maxChars int) string {
	return fmt.Sprintf(mediumMessageSummaryPromptTemplate, maxChars, tokenCount, content)
}

func buildChunkSummaryPrompt(index int, total int, chunk string, tokenCount int, maxChars int) string {
	return fmt.Sprintf(chunkMessageSummaryPromptTemplate, index, total, maxChars, tokenCount, chunk)
}

func buildMergeSummaryPrompt(totalTokens int, chunkSummaries []string, maxChars int) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(mergedMessageSummaryPromptTemplate, maxChars, totalTokens))
	for idx, item := range chunkSummaries {
		if strings.TrimSpace(item) == "" {
			continue
		}
		builder.WriteString(fmt.Sprintf(messageChunkSummaryTemplate, idx+1, strings.TrimSpace(item)))
	}
	return builder.String()
}

func estimateSummaryMaxTokens(maxChars int) int {
	if maxChars <= 0 {
		return 256
	}
	estimated := maxChars * 2
	if estimated < 128 {
		return 128
	}
	return estimated
}

func splitNonEmptyLines(text string) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func firstNonEmptyPreview(lines []string, maxLines int, maxChars int) string {
	if maxLines <= 0 || len(lines) == 0 {
		return ""
	}
	selected := make([]string, 0, maxLines)
	for _, line := range lines {
		if len(selected) >= maxLines {
			break
		}
		if line != "" {
			selected = append(selected, line)
		}
	}
	return truncateRunes(strings.Join(selected, " | "), maxChars)
}

func lastNonEmptyPreview(lines []string, maxLines int, maxChars int) string {
	if maxLines <= 0 || len(lines) == 0 {
		return ""
	}
	selected := make([]string, 0, maxLines)
	for i := len(lines) - 1; i >= 0 && len(selected) < maxLines; i-- {
		if lines[i] != "" {
			selected = append([]string{lines[i]}, selected...)
		}
	}
	return truncateRunes(strings.Join(selected, " | "), maxChars)
}
