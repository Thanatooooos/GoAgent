package evaluation

import (
	"context"

	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/app/rag/domain"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type SummaryGenerationInput struct {
	SourceMessages  []SummaryMessage
	PreviousSummary *raghistory.StructuredSummary
}

type SummaryGenerationOutput struct {
	Structured raghistory.StructuredSummary
	Rendered   string
	Raw        string
}

type SummaryGenerator interface {
	Generate(ctx context.Context, input SummaryGenerationInput) (SummaryGenerationOutput, error)
}

type HistorySummaryGenerator struct {
	chatService aichat.LLMService
	budget      raghistory.SummaryBudgetOptions
	variant     raghistory.StructuredSummaryPromptVariant
}

type HistorySummaryGeneratorOption func(*HistorySummaryGenerator)

func WithHistorySummaryPromptVariant(variant raghistory.StructuredSummaryPromptVariant) HistorySummaryGeneratorOption {
	return func(generator *HistorySummaryGenerator) {
		generator.variant = raghistory.NormalizeStructuredSummaryPromptVariant(variant)
	}
}

func NewHistorySummaryGenerator(chatService aichat.LLMService, budget raghistory.SummaryBudgetOptions, opts ...HistorySummaryGeneratorOption) *HistorySummaryGenerator {
	generator := &HistorySummaryGenerator{
		chatService: chatService,
		budget:      budget,
		variant:     raghistory.StructuredSummaryPromptVariantStateAware,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(generator)
		}
	}
	return generator
}

func (g *HistorySummaryGenerator) Generate(ctx context.Context, input SummaryGenerationInput) (SummaryGenerationOutput, error) {
	output, err := raghistory.GenerateStructuredSummary(ctx, g.chatService, raghistory.GenerateStructuredSummaryInput{
		PreviousSummary: input.PreviousSummary,
		SourceMessages:  toSummaryDomainMessages(input.SourceMessages),
		Budget:          g.budget,
		PromptVariant:   g.variant,
	})
	if err != nil {
		return SummaryGenerationOutput{}, err
	}
	return SummaryGenerationOutput{
		Structured: output.Structured,
		Rendered:   output.Rendered,
		Raw:        output.Raw,
	}, nil
}

func toSummaryDomainMessages(messages []SummaryMessage) []domain.ConversationMessage {
	if len(messages) == 0 {
		return nil
	}
	result := make([]domain.ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "" || msg.Content == "" {
			continue
		}
		result = append(result, domain.ConversationMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return result
}
