package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/config"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type BriefGenerator struct {
	llm           aichat.LLMService
	generationCfg config.DailyBriefGenerationConfig
}

func NewBriefGenerator(llm aichat.LLMService, generationCfg config.DailyBriefGenerationConfig) *BriefGenerator {
	return &BriefGenerator{
		llm:           llm,
		generationCfg: generationCfg,
	}
}

func (g *BriefGenerator) Generate(ctx context.Context, input port.BriefGenerationInput) (port.BriefGenerationResult, error) {
	if g == nil || g.llm == nil {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generator llm is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generation llm call: %w", err)
	}
	if len(input.Candidates) == 0 {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generation requires at least one candidate")
	}

	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = strings.TrimSpace(g.generationCfg.Model)
	}
	promptVersion := strings.TrimSpace(input.PromptVersion)
	if promptVersion == "" {
		promptVersion = strings.TrimSpace(g.generationCfg.PromptVersion)
	}

	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(BuildBriefGenerationSystemPrompt(g.generationCfg.MaxItemsPerTopic)),
			convention.UserMessage(BuildBriefGenerationPrompt(input.BriefDate, input.Topics, input.Candidates, g.generationCfg.MaxItemsPerTopic)),
		},
	}
	jsonMode := true
	request.JSONMode = &jsonMode

	llmCtx, llmCancel := g.llmContext(ctx)
	defer llmCancel()

	var (
		raw    string
		usage  aichat.TokenUsage
		err    error
	)
	if model != "" {
		if ctxAware, ok := g.llm.(aichat.ContextAwareLLMService); ok {
			raw, err = ctxAware.ChatWithModelContext(llmCtx, request, model)
		} else {
			raw, err = g.llm.ChatWithModel(request, model)
		}
	} else if usageAware, ok := g.llm.(aichat.ContextAwareUsageAwareLLMService); ok {
		raw, usage, err = usageAware.ChatWithRequestUsageContext(llmCtx, request)
	} else if ctxAware, ok := g.llm.(aichat.ContextAwareLLMService); ok {
		raw, err = ctxAware.ChatWithRequestContext(llmCtx, request)
	} else if usageAware, ok := g.llm.(aichat.UsageAwareLLMService); ok {
		raw, usage, err = usageAware.ChatWithRequestUsage(request)
	} else {
		raw, err = g.llm.ChatWithRequest(request)
	}
	if err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generation llm call: %w", err)
	}

	artifact, err := ParseBriefArtifact(raw)
	if err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generation output invalid: %w", err)
	}
	maxItems := g.generationCfg.MaxItems
	maxItemsPerTopic := g.generationCfg.MaxItemsPerTopic
	if maxItems <= 0 {
		maxItems = 5
	}
	topicCount := len(input.Topics)
	if topicCount == 0 {
		topicCount = countDistinctCandidateTopics(input.Candidates)
	}
	effectiveMax := EffectiveMaxItems(topicCount, maxItems, maxItemsPerTopic)
	if maxItemsPerTopic > 0 {
		artifact = TruncateBriefArtifactPerTopic(artifact, maxItemsPerTopic, effectiveMax)
	} else {
		artifact = TruncateBriefArtifact(artifact, effectiveMax)
	}
	artifact = AlignBriefArtifactWithCandidates(artifact, input.Candidates)
	if err := ValidateBriefArtifact(artifact, effectiveMax); err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generation schema validation failed: %w", err)
	}

	return port.BriefGenerationResult{
		Output:        artifact,
		TokenUsage:    usage.Normalized(),
		Model:         model,
		PromptVersion: promptVersion,
	}, nil
}

func countDistinctCandidateTopics(candidates []domain.Candidate) int {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		topic := strings.TrimSpace(candidate.Topic)
		if topic == "" {
			continue
		}
		seen[topic] = struct{}{}
	}
	return len(seen)
}

func (g *BriefGenerator) llmContext(parent context.Context) (context.Context, context.CancelFunc) {
	_ = parent
	timeout := 10 * time.Minute
	if cfg := config.Get(); cfg != nil && cfg.AI.HTTP.TimeoutMs > 0 {
		timeout = time.Duration(cfg.AI.HTTP.TimeoutMs) * time.Millisecond
	}
	return context.WithTimeout(context.Background(), timeout)
}

var _ port.BriefGenerator = (*BriefGenerator)(nil)
