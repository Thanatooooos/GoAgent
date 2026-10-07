package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/config"
)

type taskRuntimeStub struct {
	request  conversationruntime.TaskRequest
	requests []conversationruntime.TaskRequest
	result   conversationruntime.RunResult
	err      error
	results  []conversationruntime.RunResult
	errors   []error
}

func (s *taskRuntimeStub) RunTask(_ context.Context, request conversationruntime.TaskRequest) (conversationruntime.RunResult, error) {
	s.request = request
	s.requests = append(s.requests, request)
	if len(s.results) > 0 {
		index := len(s.requests) - 1
		return s.results[index], s.errors[index]
	}
	return s.result, s.err
}

func TestRuntimeBriefGeneratorUsesCandidateSeedAndAcceptsWebSource(t *testing.T) {
	stub := &taskRuntimeStub{result: conversationruntime.RunResult{Status: conversationruntime.StatusCompleted, AssistantContent: `{"headline":"今日要闻","topSummary":"模型和产品动态持续推进。","sections":[{"key":"tech.ai.models","title":"模型","items":[{"title":"新模型发布","summary":"发布了新的模型能力。","whyItMatters":"开发团队可以据此评估能力。","url":"https://example.com/news","source":"example.com","topic":"tech.ai.models"}]}]}`}}
	generator := NewRuntimeBriefGenerator(stub, config.DailyBriefGenerationConfig{MaxItems: 5})
	result, err := generator.Generate(context.Background(), port.BriefGenerationInput{UserID: "u1", BriefDate: "2026-09-09", Topics: []string{"tech.ai.models"}, Candidates: []domain.Candidate{{Title: "seed", URL: "https://seed.example/item", Source: "openai-blog", Topic: "tech.ai.models"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !stub.request.Policy.AllowWebSearch || len(stub.request.System) != 1 || len(stub.request.Sources) != 1 || stub.request.Question != "Research and write the daily brief now." {
		t.Fatalf("task request = %+v", stub.request)
	}
	if result.Output.Sections[0].Items[0].Source != "example.com" {
		t.Fatalf("item = %+v", result.Output.Sections[0].Items[0])
	}
}

func TestRuntimeBriefGeneratorFallsBackToCandidatesWhenToolBudgetExhausted(t *testing.T) {
	output := `{"headline":"今日要闻","topSummary":"模型能力继续推进。","sections":[{"key":"tech.ai.models","title":"模型","items":[{"title":"新模型发布","summary":"发布了新的模型能力。","whyItMatters":"开发团队可以据此评估能力。","url":"https://seed.example/item","source":"openai-blog","topic":"tech.ai.models"}]}]}`
	stub := &taskRuntimeStub{
		results: []conversationruntime.RunResult{{}, {Status: conversationruntime.StatusCompleted, AssistantContent: output}},
		errors:  []error{conversationruntime.ErrTaskToolCallBudgetExhausted, nil},
	}
	generator := NewRuntimeBriefGenerator(stub, config.DailyBriefGenerationConfig{MaxItems: 5})
	_, err := generator.Generate(context.Background(), port.BriefGenerationInput{UserID: "u1", RunID: "r1", BriefDate: "2026-09-29", Topics: []string{"tech.ai.models"}, Candidates: []domain.Candidate{{Title: "新模型发布", URL: "https://seed.example/item", Source: "openai-blog", Topic: "tech.ai.models"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.requests) != 2 || stub.requests[1].TaskType != "daily_brief_fallback" || !stub.requests[1].Policy.DisableTools {
		t.Fatalf("fallback requests = %+v", stub.requests)
	}
}

func TestRuntimeBriefGeneratorFallsBackToCandidatesWhenTurnBudgetExhausted(t *testing.T) {
	output := `{"headline":"今日要闻","topSummary":"模型能力继续推进。","sections":[{"key":"tech.ai.models","title":"模型","items":[{"title":"新模型发布","summary":"发布了新的模型能力。","whyItMatters":"开发团队可以据此评估能力。","url":"https://seed.example/item","source":"openai-blog","topic":"tech.ai.models"}]}]}`
	stub := &taskRuntimeStub{
		results: []conversationruntime.RunResult{{}, {Status: conversationruntime.StatusCompleted, AssistantContent: output}},
		errors:  []error{conversationruntime.ErrTaskTurnBudgetExhausted, nil},
	}
	generator := NewRuntimeBriefGenerator(stub, config.DailyBriefGenerationConfig{MaxItems: 5})
	_, err := generator.Generate(context.Background(), port.BriefGenerationInput{UserID: "u1", RunID: "r1", BriefDate: "2026-09-29", Topics: []string{"tech.ai.models"}, Candidates: []domain.Candidate{{Title: "新模型发布", URL: "https://seed.example/item", Source: "openai-blog", Topic: "tech.ai.models"}}})
	if err != nil || len(stub.requests) != 2 || !stub.requests[1].Policy.DisableTools {
		t.Fatalf("err=%v requests=%+v", err, stub.requests)
	}
}

func TestRuntimeBriefGeneratorFallsBackToCandidatesWhenAgenticAttemptTimesOut(t *testing.T) {
	output := `{"headline":"今日要闻","topSummary":"模型能力继续推进。","sections":[{"key":"tech.ai.models","title":"模型","items":[{"title":"新模型发布","summary":"发布了新的模型能力。","whyItMatters":"开发团队可以据此评估能力。","url":"https://seed.example/item","source":"openai-blog","topic":"tech.ai.models"}]}]}`
	stub := &taskRuntimeStub{
		results: []conversationruntime.RunResult{{}, {Status: conversationruntime.StatusCompleted, AssistantContent: output}},
		errors:  []error{context.DeadlineExceeded, nil},
	}
	generator := NewRuntimeBriefGenerator(stub, config.DailyBriefGenerationConfig{MaxItems: 5})
	_, err := generator.Generate(context.Background(), port.BriefGenerationInput{UserID: "u1", RunID: "r1", BriefDate: "2026-09-29", Topics: []string{"tech.ai.models"}, Candidates: []domain.Candidate{{Title: "新模型发布", URL: "https://seed.example/item", Source: "openai-blog", Topic: "tech.ai.models"}}})
	if err != nil || len(stub.requests) != 2 || !stub.requests[1].Policy.DisableTools {
		t.Fatalf("err=%v requests=%+v", err, stub.requests)
	}
}

func TestRuntimeBriefGeneratorReportsBothErrorsWhenFallbackFails(t *testing.T) {
	stub := &taskRuntimeStub{results: []conversationruntime.RunResult{{}, {}}, errors: []error{errors.New("stream interrupted"), errors.New("model unavailable")}}
	generator := NewRuntimeBriefGenerator(stub, config.DailyBriefGenerationConfig{})
	_, err := generator.Generate(context.Background(), port.BriefGenerationInput{UserID: "u1", RunID: "r1", BriefDate: "2026-09-29", Topics: []string{"tech.ai.models"}, Candidates: []domain.Candidate{{Title: "seed", URL: "https://seed.example/item", Source: "openai-blog", Topic: "tech.ai.models"}}})
	if err == nil || len(stub.requests) != 2 || !strings.Contains(err.Error(), "stream interrupted") || !strings.Contains(err.Error(), "model unavailable") {
		t.Fatalf("err=%v requests=%d", err, len(stub.requests))
	}
}
