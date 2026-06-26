package history

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type GenerateStructuredSummaryInput struct {
	PreviousSummary *StructuredSummary
	SourceMessages  []domain.ConversationMessage
	Budget          SummaryBudgetOptions
	PromptVariant   StructuredSummaryPromptVariant
}

type GenerateStructuredSummaryOutput struct {
	Structured StructuredSummary
	Rendered   string
	Raw        string
	Validation SummaryValidationResult
}

func GenerateStructuredSummary(
	ctx context.Context,
	chatService aichat.LLMService,
	input GenerateStructuredSummaryInput,
) (GenerateStructuredSummaryOutput, error) {
	if chatService == nil {
		return GenerateStructuredSummaryOutput{}, fmt.Errorf("chat service is required")
	}

	tier := SelectSummaryBudget(SummaryBudgetInput{
		MessageCount: len(input.SourceMessages),
		TotalChars:   countMessageChars(input.SourceMessages),
		Messages:     messageContents(input.SourceMessages),
	}, input.Budget)

	latestSummary := domain.ConversationSummary{}
	if input.PreviousSummary != nil {
		previous := RepairStructuredSummary(*input.PreviousSummary)
		latestSummary.StructuredSummaryJSON = marshalStructuredSummary(previous)
		latestSummary.Content = RenderStructuredSummary(previous, tier.MaxChars)
	}

	jsonMode := true
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(buildStructuredSummaryPromptWithVariant(
				tier,
				latestSummary,
				input.SourceMessages,
				NormalizeStructuredSummaryPromptVariant(input.PromptVariant),
			)),
			convention.UserMessage("现在请直接返回结构化工作记忆 JSON。"),
		},
		JSONMode: &jsonMode,
	}
	response, err := chatService.ChatWithRequest(request)
	if err != nil {
		return GenerateStructuredSummaryOutput{}, fmt.Errorf("generate structured summary chat call: %w", err)
	}

	structured, err := ParseStructuredSummary(strings.TrimSpace(response))
	if err != nil {
		return GenerateStructuredSummaryOutput{}, fmt.Errorf("parse structured summary: %w", err)
	}
	repaired := RepairStructuredSummary(structured)
	validation := ValidateStructuredSummary(repaired, input.SourceMessages)

	return GenerateStructuredSummaryOutput{
		Structured: repaired,
		Rendered:   RenderStructuredSummary(repaired, tier.MaxChars),
		Raw:        strings.TrimSpace(response),
		Validation: validation,
	}, nil
}
