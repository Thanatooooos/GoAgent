package evaluation

import (
	"context"
	"fmt"
	"strings"

	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type SummaryAnswerConfig struct {
	Model                      string
	Temperature                float64
	MaxTokens                  int
	EnableRetrieval            bool
	EnableTools                bool
	EnableExternalCompensation bool
}

type SummaryAnswerInput struct {
	Question string
	Context  string
	Config   SummaryAnswerConfig
}

type SummaryAnswerOutput struct {
	Answer string
}

type SummaryAnswerGenerator interface {
	Answer(ctx context.Context, input SummaryAnswerInput) (SummaryAnswerOutput, error)
}

type PromptSummaryAnswerGenerator struct {
	promptService *ragprompt.Service
	chatService   aichat.LLMService
	config        SummaryAnswerConfig
}

func NewPromptSummaryAnswerGenerator(promptService *ragprompt.Service, chatService aichat.LLMService, config SummaryAnswerConfig) *PromptSummaryAnswerGenerator {
	if promptService == nil {
		promptService = ragprompt.NewService(nil)
	}
	return &PromptSummaryAnswerGenerator{
		promptService: promptService,
		chatService:   chatService,
		config:        config,
	}
}

func (g *PromptSummaryAnswerGenerator) Answer(_ context.Context, input SummaryAnswerInput) (SummaryAnswerOutput, error) {
	if g == nil || g.chatService == nil {
		return SummaryAnswerOutput{}, fmt.Errorf("chat service is required")
	}
	cfg := g.answerConfig(input.Config)
	messages, err := g.promptService.BuildMessages(ragprompt.Context{
		Question:       strings.TrimSpace(input.Question),
		SessionContext: strings.TrimSpace(input.Context),
		AnswerGuidance: "Answer based only on the provided session context. If the context is insufficient, say so clearly. Respond in Chinese.",
	})
	if err != nil {
		return SummaryAnswerOutput{}, err
	}
	toolsEnabled := cfg.EnableTools
	temperature := cfg.Temperature
	maxTokens := cfg.MaxTokens
	request := convention.ChatRequest{
		Messages:    messages,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		EnableTools: &toolsEnabled,
	}
	var answer string
	if strings.TrimSpace(cfg.Model) != "" {
		answer, err = g.chatService.ChatWithModel(request, cfg.Model)
	} else {
		answer, err = g.chatService.ChatWithRequest(request)
	}
	if err != nil {
		return SummaryAnswerOutput{}, err
	}
	return SummaryAnswerOutput{Answer: strings.TrimSpace(answer)}, nil
}

func (g *PromptSummaryAnswerGenerator) answerConfig(input SummaryAnswerConfig) SummaryAnswerConfig {
	cfg := g.config
	if strings.TrimSpace(input.Model) != "" {
		cfg.Model = strings.TrimSpace(input.Model)
	}
	cfg.Temperature = input.Temperature
	if input.MaxTokens > 0 {
		cfg.MaxTokens = input.MaxTokens
	}
	cfg.EnableRetrieval = input.EnableRetrieval
	cfg.EnableTools = input.EnableTools
	cfg.EnableExternalCompensation = input.EnableExternalCompensation
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 512
	}
	return cfg
}

type SummaryEquivalenceQueryResult struct {
	ID                   string  `json:"id"`
	Query                string  `json:"query"`
	FullContextAnswer    string  `json:"full_context_answer"`
	SummaryContextAnswer string  `json:"summary_context_answer"`
	Passed               bool    `json:"passed"`
	Score                float64 `json:"score"`
	DangerousDrift       bool    `json:"dangerous_drift"`
	Reason               string  `json:"reason,omitempty"`
}

type SummaryEquivalenceEvaluation struct {
	Queries []SummaryEquivalenceQueryResult `json:"queries"`
	Passed  bool                            `json:"passed"`
	Score   float64                         `json:"score"`
}

func RunSummaryEquivalence(ctx context.Context, answerGen SummaryAnswerGenerator, judge Judge, sample SummarySample, generated SummaryGenerationOutput) (SummaryEquivalenceEvaluation, error) {
	if answerGen == nil {
		return SummaryEquivalenceEvaluation{}, fmt.Errorf("summary answer generator is required")
	}
	if judge == nil {
		return SummaryEquivalenceEvaluation{}, fmt.Errorf("judge is required")
	}

	config := fixedSummaryAnswerConfig()
	fullContext := renderSourceMessages(sample.Input.SourceMessages)
	summaryContext := strings.TrimSpace(generated.Rendered)

	results := make([]SummaryEquivalenceQueryResult, 0, len(sample.NextTurnEval.Queries))
	totalScore := 0.0
	allPassed := true
	for _, query := range sample.NextTurnEval.Queries {
		fullAnswer, err := answerGen.Answer(ctx, SummaryAnswerInput{
			Question: query.Query,
			Context:  fullContext,
			Config:   config,
		})
		if err != nil {
			return SummaryEquivalenceEvaluation{}, err
		}
		summaryAnswer, err := answerGen.Answer(ctx, SummaryAnswerInput{
			Question: query.Query,
			Context:  summaryContext,
			Config:   config,
		})
		if err != nil {
			return SummaryEquivalenceEvaluation{}, err
		}
		judgeResult, err := judge.Evaluate(ctx, JudgeRequest{
			PromptRef: "summary.equivalence.v1",
			RubricRef: "summary.equivalence.v1",
			Payload: map[string]any{
				"query":                    query.Query,
				"equivalence_expectations": query.EquivalenceExpectations,
				"full_context_answer":      fullAnswer.Answer,
				"summary_context_answer":   summaryAnswer.Answer,
			},
			Config: fixedSummaryEquivalenceJudgeConfig(),
		})
		if err != nil {
			return SummaryEquivalenceEvaluation{}, err
		}

		dangerousDrift, _ := judgeResult.Details["dangerous_drift"].(bool)
		queryResult := SummaryEquivalenceQueryResult{
			ID:                   query.ID,
			Query:                query.Query,
			FullContextAnswer:    fullAnswer.Answer,
			SummaryContextAnswer: summaryAnswer.Answer,
			Passed:               judgeResult.Passed,
			Score:                judgeResult.Score,
			DangerousDrift:       dangerousDrift,
			Reason:               judgeResult.Reason,
		}
		if dangerousDrift || !judgeResult.Passed {
			allPassed = false
		}
		totalScore += judgeResult.Score
		results = append(results, queryResult)
	}

	score := 0.0
	if len(results) > 0 {
		score = totalScore / float64(len(results))
	}
	return SummaryEquivalenceEvaluation{
		Queries: results,
		Passed:  allPassed,
		Score:   score,
	}, nil
}

func renderSourceMessages(messages []SummaryMessage) string {
	if len(messages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		parts = append(parts, role+": "+content)
	}
	return strings.Join(parts, "\n")
}
