package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type stubBriefLLM struct {
	response string
	requests []convention.ChatRequest
}

func (s *stubBriefLLM) Chat(prompt string) (string, error) {
	return s.response, nil
}

func (s *stubBriefLLM) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, nil
}

func (s *stubBriefLLM) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	s.requests = append(s.requests, request)
	return s.response, nil
}

func (s *stubBriefLLM) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *stubBriefLLM) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

type ctxAwareStubBriefLLM struct {
	stubBriefLLM
	ctxs []context.Context
}

func (s *ctxAwareStubBriefLLM) ChatWithRequestContext(ctx context.Context, request convention.ChatRequest) (string, error) {
	s.ctxs = append(s.ctxs, ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.response, nil
}

func (s *ctxAwareStubBriefLLM) ChatWithModelContext(ctx context.Context, request convention.ChatRequest, modelID string) (string, error) {
	s.ctxs = append(s.ctxs, ctx)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.response, nil
}

func (s *ctxAwareStubBriefLLM) ChatWithRequestUsageContext(ctx context.Context, request convention.ChatRequest) (string, aichat.TokenUsage, error) {
	s.ctxs = append(s.ctxs, ctx)
	if err := ctx.Err(); err != nil {
		return "", aichat.TokenUsage{}, err
	}
	return s.response, aichat.TokenUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}, nil
}

func validBriefJSON() string {
	artifact := domain.BriefArtifact{
		Headline:   "Daily AI Brief",
		TopSummary: "Top stories.",
		Sections: []domain.BriefSection{
			{
				Key:   "tech.ai.models",
				Title: "AI Models",
				Items: []domain.BriefItemDraft{
					{
						Title:        "Story",
						Summary:      "Summary",
						WhyItMatters: "Impact",
						URL:          "https://example.com/story",
						Source:       domain.SourceKeyOpenAIBlog,
						Topic:        domain.TopicKeyTechAIModels,
					},
				},
			},
		},
	}
	payload, _ := json.Marshal(artifact)
	return string(payload)
}

func TestBriefGeneratorValidatesModelOutput(t *testing.T) {
	t.Parallel()

	llm := &stubBriefLLM{response: validBriefJSON()}
	generator := NewBriefGenerator(llm, config.DailyBriefGenerationConfig{
		MaxItems:      5,
		Model:         "qwen3-32b",
		PromptVersion: "v1",
	})

	result, err := generator.Generate(context.Background(), port.BriefGenerationInput{
		UserID:    "user-1",
		BriefDate: "2026-06-29",
		Candidates: []domain.Candidate{
			{
				Title:  "Story",
				URL:    "https://example.com/story",
				Source: domain.SourceKeyOpenAIBlog,
				Topic:  domain.TopicKeyTechAIModels,
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if result.Output.Headline != "Daily AI Brief" {
		t.Fatalf("unexpected headline: %q", result.Output.Headline)
	}
	if len(llm.requests) != 1 || llm.requests[0].JSONMode == nil || !*llm.requests[0].JSONMode {
		t.Fatalf("expected JSON mode request, got %+v", llm.requests)
	}
}

func TestBriefGeneratorRejectsInvalidOutput(t *testing.T) {
	t.Parallel()

	llm := &stubBriefLLM{response: `{"headline":"","topSummary":"","sections":[]}`}
	generator := NewBriefGenerator(llm, config.DailyBriefGenerationConfig{MaxItems: 5})

	_, err := generator.Generate(context.Background(), port.BriefGenerationInput{
		BriefDate: "2026-06-29",
		Candidates: []domain.Candidate{
			{Title: "Story", URL: "https://example.com", Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels},
		},
	})
	if err == nil {
		t.Fatal("expected invalid model output to fail")
	}
}

func TestBriefGeneratorUsesIndependentLLMContext(t *testing.T) {
	t.Parallel()

	llm := &ctxAwareStubBriefLLM{stubBriefLLM: stubBriefLLM{response: validBriefJSON()}}
	generator := NewBriefGenerator(llm, config.DailyBriefGenerationConfig{MaxItems: 5, Model: "qwen3-32b"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result, err := generator.Generate(ctx, port.BriefGenerationInput{
		BriefDate: "2026-06-29",
		Candidates: []domain.Candidate{
			{Title: "Story", URL: "https://example.com/story", Source: domain.SourceKeyOpenAIBlog, Topic: domain.TopicKeyTechAIModels},
		},
	})
	if err != nil {
		t.Fatalf("expected generation to ignore canceled caller context, got %v", err)
	}
	if len(llm.ctxs) != 1 {
		t.Fatalf("expected context-aware llm path to be used once, got %d", len(llm.ctxs))
	}
	if llm.ctxs[0] == ctx {
		t.Fatal("expected generator to use an independent llm context")
	}
	if result.Output.Headline != "Daily AI Brief" {
		t.Fatalf("unexpected headline: %q", result.Output.Headline)
	}
}

func TestPublisherPublishesIssueAndItems(t *testing.T) {
	t.Parallel()

	issueRepo := &stubIssueRepo{issueByID: domain.NewIssue("issue-1", "user-1", "2026-06-29")}
	itemRepo := &stubReadItemRepo{}
	publisher := NewPublisher(func(ctx context.Context, fn func(context.Context, port.IssueRepository, port.ItemRepository) error) error {
		return fn(ctx, issueRepo, itemRepo)
	})

	publishedAt := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	issue, items, err := publisher.Publish(context.Background(), PublishInput{
		Issue:          domain.NewIssue("issue-1", "user-1", "2026-06-29"),
		PublishedRunID: "run-1",
		PublishedAt:    publishedAt,
		Artifact: domain.BriefArtifact{
			Headline:   "Daily AI Brief",
			TopSummary: "Top stories.",
			Sections: []domain.BriefSection{
				{
					Key:   "tech.ai.models",
					Title: "AI Models",
					Items: []domain.BriefItemDraft{
						{
							Title:        "Story",
							Summary:      "Summary",
							WhyItMatters: "Impact",
							URL:          "https://example.com/story",
							Source:       domain.SourceKeyOpenAIBlog,
							Topic:        domain.TopicKeyTechAIModels,
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if issue.Status != domain.IssueStatusReady {
		t.Fatalf("expected ready issue, got %q", issue.Status)
	}
	if issue.PublishedRunID != "run-1" {
		t.Fatalf("expected published run id, got %q", issue.PublishedRunID)
	}
	if len(items) != 1 {
		t.Fatalf("expected one published item, got %d", len(items))
	}
}
