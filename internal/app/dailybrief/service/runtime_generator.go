package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/config"
)

// RuntimeBriefGenerator lets Daily Brief use the shared agentic runtime while
// keeping Daily Brief responsible for its typed artifact and publication.
type RuntimeBriefGenerator struct {
	runtime       conversationruntime.TaskRuntime
	generationCfg config.DailyBriefGenerationConfig
}

const agenticBriefTimeout = 2 * time.Minute

func NewRuntimeBriefGenerator(runtime conversationruntime.TaskRuntime, generationCfg config.DailyBriefGenerationConfig) *RuntimeBriefGenerator {
	return &RuntimeBriefGenerator{runtime: runtime, generationCfg: generationCfg}
}

func (g *RuntimeBriefGenerator) Generate(ctx context.Context, input port.BriefGenerationInput) (port.BriefGenerationResult, error) {
	if g == nil || g.runtime == nil {
		return port.BriefGenerationResult{}, fmt.Errorf("daily brief runtime is not configured")
	}
	if len(input.Candidates) == 0 {
		return port.BriefGenerationResult{}, fmt.Errorf("brief generation requires at least one candidate")
	}
	maxItems := g.generationCfg.MaxItems
	if maxItems <= 0 {
		maxItems = 5
	}
	topicCount := len(input.Topics)
	if topicCount == 0 {
		topicCount = countDistinctCandidateTopics(input.Candidates)
	}
	effectiveMax := EffectiveMaxItems(topicCount, maxItems, g.generationCfg.MaxItemsPerTopic)

	request := conversationruntime.TaskRequest{
		UserID:   input.UserID,
		TaskType: "daily_brief",
		TaskID:   strings.TrimSpace(input.RunID),
		Question: briefRuntimeRequest,
		System: []string{
			briefRuntimeSystemPrompt,
		},
		Sources: []conversationruntime.TaskSource{{Key: "daily-brief/candidates", Render: func(conversationruntime.TaskRequest) string {
			return BuildBriefGenerationPrompt(input.BriefDate, input.Topics, input.Candidates, g.generationCfg.MaxItemsPerTopic)
		}}},
		Policy: conversationruntime.Policy{AllowWebSearch: true, MaxTurns: 6, MaxToolCalls: 12},
	}
	attemptCtx, cancel := context.WithTimeout(ctx, agenticBriefTimeout)
	result, err := g.runtime.RunTask(attemptCtx, request)
	cancel()
	if err != nil && ctx.Err() == nil {
		primaryErr := err
		request.TaskType = "daily_brief_fallback"
		request.System = []string{
			briefRuntimeFallbackRequest,
		}
		request.Policy = conversationruntime.Policy{DisableTools: true, MaxTurns: 1}
		result, err = g.runtime.RunTask(ctx, request)
		if err != nil {
			return port.BriefGenerationResult{}, errors.Join(fmt.Errorf("agentic daily brief: %w", primaryErr), fmt.Errorf("candidate-only daily brief: %w", err))
		}
	}
	if err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("daily brief runtime run: %w", err)
	}
	artifact, err := ParseBriefArtifact(result.AssistantContent)
	if err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("parse daily brief runtime output: %w", err)
	}
	if g.generationCfg.MaxItemsPerTopic > 0 {
		artifact = TruncateBriefArtifactPerTopic(artifact, g.generationCfg.MaxItemsPerTopic, effectiveMax)
	} else {
		artifact = TruncateBriefArtifact(artifact, effectiveMax)
	}
	artifact = AlignBriefArtifactWithCandidates(artifact, input.Candidates)
	if err := ValidateBriefArtifact(artifact, effectiveMax); err != nil {
		return port.BriefGenerationResult{}, fmt.Errorf("daily brief runtime schema validation failed: %w", err)
	}
	return port.BriefGenerationResult{Output: artifact, Model: strings.TrimSpace(input.Model), PromptVersion: strings.TrimSpace(input.PromptVersion)}, nil
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

var _ port.BriefGenerator = (*RuntimeBriefGenerator)(nil)
