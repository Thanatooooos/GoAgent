package history

import (
	"context"
	"testing"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/convention"
)

func TestSelectSummaryBudgetPrefersTokenTiers(t *testing.T) {
	options := SummaryBudgetOptions{
		SmallMaxChars:  400,
		MediumMaxChars: 600,
		LargeMaxChars:  800,
	}

	small := SelectSummaryBudget(SummaryBudgetInput{TotalTokens: 500}, options)
	if small.Name != "small" || small.MaxChars != 400 {
		t.Fatalf("expected small tier, got %+v", small)
	}

	medium := SelectSummaryBudget(SummaryBudgetInput{
		TotalTokens: 2500,
		Messages:    []string{"plain conversation"},
	}, options)
	if medium.Name != "medium" || medium.MaxChars != 600 {
		t.Fatalf("expected medium tier, got %+v", medium)
	}

	large := SelectSummaryBudget(SummaryBudgetInput{TotalTokens: 4500}, options)
	if large.Name != "large" || large.MaxChars != 800 {
		t.Fatalf("expected large tier, got %+v", large)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	messages := []domain.ConversationMessage{
		{Content: "hello"},
		{Content: "world"},
	}
	total := estimateMessagesTokens(messages, NewTokenEstimateAdapter())
	if total <= 0 {
		t.Fatalf("expected positive token estimate, got %d", total)
	}
}

func TestCompressIfNeededSkipsWhenBelowTokenThreshold(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "短回复"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "短问题"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "短回复2"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "短问题2"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService:   &mockChatServiceForCompress{response: "should not be called"},
		StartTurns:    2,
		TriggerTokens: 5000,
		Estimator:     NewTokenEstimateAdapter(),
		MaxChars:      200,
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryRepo.created {
		t.Fatal("expected no summary when token threshold is not met")
	}
}
