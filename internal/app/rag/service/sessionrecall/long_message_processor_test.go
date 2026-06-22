package sessionrecall

import (
	"context"
	"strings"
	"testing"

	ragconversation "local/rag-project/internal/app/rag/service/conversation"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type fixedTokenEstimator struct {
	factor int
}

func (f fixedTokenEstimator) EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if f.factor <= 0 {
		return len([]rune(text))
	}
	return len([]rune(text)) / f.factor
}

func TestLongMessageContentProcessorLeavesSmallMessageUntouched(t *testing.T) {
	processor := NewLongMessageContentProcessor(LongMessageProcessorOptions{
		Enabled:                     true,
		DirectContextMaxTokens:      10,
		ChunkSummaryThresholdTokens: 20,
		Estimator:                   fixedTokenEstimator{factor: 1},
	})

	result, err := processor.ProcessAddMessage(context.Background(), ragconversation.AddConversationMessageInput{
		Content: "short text",
	})
	if err != nil {
		t.Fatalf("ProcessAddMessage returned error: %v", err)
	}
	if result.IsSummarized {
		t.Fatalf("expected small message to stay unsummarized: %+v", result)
	}
	if result.Content != "short text" {
		t.Fatalf("unexpected content: %q", result.Content)
	}
}

func TestLongMessageContentProcessorSummarizesMediumMessage(t *testing.T) {
	processor := NewLongMessageContentProcessor(LongMessageProcessorOptions{
		Enabled:                     true,
		DirectContextMaxTokens:      10,
		ChunkSummaryThresholdTokens: 50,
		LargeChunkTargetTokens:      12,
		LargeChunkOverlapTokens:     2,
		MediumSummaryMaxChars:       120,
		ChunkSummaryMaxChars:        80,
		Estimator:                   fixedTokenEstimator{factor: 1},
	})

	input := strings.Repeat("abcde\n", 6)
	result, err := processor.ProcessAddMessage(context.Background(), ragconversation.AddConversationMessageInput{
		Content: input,
	})
	if err != nil {
		t.Fatalf("ProcessAddMessage returned error: %v", err)
	}
	if !result.IsSummarized {
		t.Fatalf("expected medium message to be summarized: %+v", result)
	}
	if result.RawContent != strings.TrimSpace(input) {
		t.Fatalf("expected raw content to preserve original message")
	}
	if !strings.Contains(result.Content, "长消息摘要") {
		t.Fatalf("expected medium summary marker, got %q", result.Content)
	}
	if len(result.SessionChunks) == 0 {
		t.Fatalf("expected medium message to produce session chunks: %+v", result)
	}
	if strings.TrimSpace(result.SessionChunks[0].ContentSummary) == "" {
		t.Fatalf("expected medium session chunk summary, got %+v", result.SessionChunks[0])
	}
}

type stubSummaryLLMService struct {
	responses []string
}

func (s *stubSummaryLLMService) Chat(prompt string) (string, error) {
	return s.next(), nil
}

func (s *stubSummaryLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	return s.next(), nil
}

func (s *stubSummaryLLMService) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	return s.next(), nil
}

func (s *stubSummaryLLMService) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *stubSummaryLLMService) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *stubSummaryLLMService) next() string {
	if len(s.responses) == 0 {
		return ""
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp
}

func TestLongMessageContentProcessorUsesLLMForMediumSummary(t *testing.T) {
	llm := &stubSummaryLLMService{responses: []string{"这是 LLM 生成的摘要"}}
	processor := NewLongMessageContentProcessor(LongMessageProcessorOptions{
		Enabled:                     true,
		DirectContextMaxTokens:      10,
		ChunkSummaryThresholdTokens: 50,
		MediumSummaryMaxChars:       120,
		Estimator:                   fixedTokenEstimator{factor: 1},
		ChatService:                 llm,
	})

	input := strings.Repeat("abcde\n", 6)
	result, err := processor.ProcessAddMessage(context.Background(), ragconversation.AddConversationMessageInput{
		Content: input,
	})
	if err != nil {
		t.Fatalf("ProcessAddMessage returned error: %v", err)
	}
	if result.Content != "这是 LLM 生成的摘要" {
		t.Fatalf("expected llm summary, got %q", result.Content)
	}
}

func TestLongMessageContentProcessorSummarizesLargeMessageByChunks(t *testing.T) {
	processor := NewLongMessageContentProcessor(LongMessageProcessorOptions{
		Enabled:                     true,
		DirectContextMaxTokens:      10,
		ChunkSummaryThresholdTokens: 20,
		LargeChunkTargetTokens:      8,
		ChunkSummaryMaxChars:        80,
		LargeSummaryMaxChars:        200,
		Estimator:                   fixedTokenEstimator{factor: 1},
	})

	input := strings.Join([]string{
		"line-1-abcdefghijklmnopqrstuvwxyz",
		"line-2-abcdefghijklmnopqrstuvwxyz",
		"line-3-abcdefghijklmnopqrstuvwxyz",
		"line-4-abcdefghijklmnopqrstuvwxyz",
	}, "\n")
	result, err := processor.ProcessAddMessage(context.Background(), ragconversation.AddConversationMessageInput{
		Content: input,
	})
	if err != nil {
		t.Fatalf("ProcessAddMessage returned error: %v", err)
	}
	if !result.IsSummarized {
		t.Fatalf("expected large message to be summarized: %+v", result)
	}
	if !strings.Contains(result.Content, "超长消息摘要") {
		t.Fatalf("expected large summary marker, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "分段1/") {
		t.Fatalf("expected chunk-level summaries, got %q", result.Content)
	}
	if len(result.SessionChunks) == 0 {
		t.Fatalf("expected session chunks for large message, got %+v", result)
	}
	if result.SessionChunks[0].ChunkIndex != 1 || strings.TrimSpace(result.SessionChunks[0].Content) == "" {
		t.Fatalf("unexpected first session chunk: %+v", result.SessionChunks[0])
	}
}

func TestLongMessageContentProcessorUsesLLMForLargeSummary(t *testing.T) {
	llm := &stubSummaryLLMService{responses: []string{
		"第一段摘要",
		"第二段摘要",
		"第三段摘要",
		"第四段摘要",
		"合并后的总摘要",
	}}
	processor := NewLongMessageContentProcessor(LongMessageProcessorOptions{
		Enabled:                     true,
		DirectContextMaxTokens:      10,
		ChunkSummaryThresholdTokens: 20,
		LargeChunkTargetTokens:      35,
		ChunkSummaryMaxChars:        80,
		LargeSummaryMaxChars:        200,
		Estimator:                   fixedTokenEstimator{factor: 1},
		ChatService:                 llm,
	})

	input := strings.Join([]string{
		"line-1-abcdefghijklmnopqrstuvwxyz",
		"line-2-abcdefghijklmnopqrstuvwxyz",
		"line-3-abcdefghijklmnopqrstuvwxyz",
		"line-4-abcdefghijklmnopqrstuvwxyz",
	}, "\n")
	result, err := processor.ProcessAddMessage(context.Background(), ragconversation.AddConversationMessageInput{
		Content: input,
	})
	if err != nil {
		t.Fatalf("ProcessAddMessage returned error: %v", err)
	}
	if result.Content != "合并后的总摘要" {
		t.Fatalf("expected merged llm summary, got %q", result.Content)
	}
}

func TestSplitTextByTokenBudgetAddsOverlap(t *testing.T) {
	chunks := splitTextByTokenBudget(strings.Join([]string{
		"aa",
		"bb",
		"cc",
		"dd",
	}, "\n"), 5, 2, fixedTokenEstimator{factor: 1})

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks with overlap, got %d: %#v", len(chunks), chunks)
	}
	if chunks[0] != "aa\nbb" {
		t.Fatalf("unexpected first chunk: %q", chunks[0])
	}
	if chunks[1] != "bb\ncc" {
		t.Fatalf("expected overlap in second chunk, got %q", chunks[1])
	}
	if chunks[2] != "cc\ndd" {
		t.Fatalf("expected overlap in third chunk, got %q", chunks[2])
	}
}
