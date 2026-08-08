package port

import (
	"context"

	"local/rag-project/internal/app/dailybrief/domain"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type BriefGenerationInput struct {
	UserID        string
	BriefDate     string
	Topics        []string
	Candidates    []domain.Candidate
	PromptVersion string
	Model         string
}

type BriefGenerationResult struct {
	Output      domain.BriefArtifact
	TokenUsage  aichat.TokenUsage
	Model       string
	PromptVersion string
}

type BriefGenerator interface {
	Generate(ctx context.Context, input BriefGenerationInput) (BriefGenerationResult, error)
}
