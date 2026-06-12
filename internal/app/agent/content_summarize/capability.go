package content_summarize

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/framework/convention"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	defaultMaxChars = 500
	maxAllowedChars = 2000

	purposeAnswerPrep      = "answer_prep"
	purposeEvidenceDigest  = "evidence_digest"
	purposeGeneral         = "general"
)

// ChatCompleter summarizes content through the configured LLM service.
type ChatCompleter interface {
	ChatWithRequest(request convention.ChatRequest) (string, error)
}

// CapabilityInput describes a summarize request.
type CapabilityInput struct {
	Content  string `json:"content"`
	MaxChars int    `json:"max_chars,omitempty"`
	Language string `json:"language,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
}

// CapabilityOutput is the normalized summarize result.
type CapabilityOutput struct {
	Summary       string `json:"summary"`
	OriginalChars int    `json:"original_chars"`
	SummaryChars  int    `json:"summary_chars"`
	Language      string `json:"language"`
}

type capabilityAdapter struct {
	spec     agentcapability.Spec
	completer ChatCompleter
}

// NewCapability builds the content summarize capability.
func NewCapability(completer ChatCompleter, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if completer == nil {
		return nil, fmt.Errorf("chat completer is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameContentSummarize,
		Kind:             agentcapability.KindTool,
		Family:           agentcapability.FamilyGeneration,
		Roles:            []string{agentcapability.RoleSummarize},
		Description:      "Summarizes long content into a concise digest for downstream answer preparation.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: true,
		SupportsResume:   false,
		ProducesEvidence: false,
		Idempotency:      agentcapability.IdempotencyBestEffort,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "content",
				Requirement: agentcapability.PreconditionRequirementNonEmpty,
				Description: "Content summarize requires non-empty content.",
			},
		},
	}
	agentcapability.ApplyOptions(&spec, options...)
	return capabilityAdapter{spec: spec, completer: completer}, nil
}

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "content summarize input is required", "content summarize input")
}

func (c capabilityAdapter) Invoke(_ context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "content summarize input is required", "content summarize input")
	if err != nil {
		return agentcapability.ValidationFailureResult(c.spec, "content summarize rejected", err), err
	}

	content := strings.TrimSpace(input.Content)
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = "zh"
	}
	maxChars := input.MaxChars
	if maxChars <= 0 {
		maxChars = defaultMaxChars
	}
	if maxChars > maxAllowedChars {
		maxChars = maxAllowedChars
	}

	prompt := buildSummarizePrompt(content, language, maxChars, strings.TrimSpace(strings.ToLower(input.Purpose)))
	summary, invokeErr := c.completer.ChatWithRequest(convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(prompt),
			convention.UserMessage("请输出摘要。"),
		},
	})
	summary = strings.TrimSpace(summary)
	if invokeErr != nil {
		return agentcapability.ExternalFailureResult(c.spec, "summarize content", invokeErr), invokeErr
	}
	if summary == "" {
		err = fmt.Errorf("empty summarize response")
		return agentcapability.ExternalFailureResult(c.spec, "summarize content", err), err
	}

	originalChars := len([]rune(content))
	summaryChars := len([]rune(summary))
	output := CapabilityOutput{
		Summary:       summary,
		OriginalChars: originalChars,
		SummaryChars:  summaryChars,
		Language:      language,
	}
	note := fmt.Sprintf("Summarized %d chars -> %d chars", originalChars, summaryChars)
	actionSummary := summary
	if summaryChars > 120 {
		actionSummary = string([]rune(summary)[:120]) + "..."
	}

	return agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: actionSummary,
		},
		Observation: agentcapability.ObservationRecord{
			Summary: actionSummary,
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: agentcapability.AppendNonEmpty(nil, note),
			},
		},
		Status: agentcapability.StatusSucceeded,
	}, nil
}

func buildSummarizePrompt(content, language string, maxChars int, purpose string) string {
	var instruction string
	switch purpose {
	case purposeAnswerPrep:
		instruction = "将内容压缩为可直接用于回答用户的简洁摘要，保留关键事实和结论。"
	case purposeEvidenceDigest:
		instruction = "将内容提炼为证据摘要，突出可引用的关键信息，忽略无关细节。"
	default:
		instruction = "将内容压缩为简洁摘要，保留关键事实。"
	}
	return fmt.Sprintf("%s\n要求：\n1. 使用%s。\n2. 摘要不超过%d个字符。\n3. 不要编造原文没有的信息。\n\n## 内容\n%s", instruction, language, maxChars, content)
}
