package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type ObservationInput struct {
	Profile      string
	Conversation string
}
type ObservationResult struct {
	Action          string `json:"action"`
	ProfileMarkdown string `json:"profile_markdown"`
}

type Observer struct{ chat aichat.LLMService }

func NewObserver(chat aichat.LLMService) *Observer { return &Observer{chat: chat} }

func (o *Observer) Observe(ctx context.Context, input ObservationInput) (ObservationResult, error) {
	if o == nil || o.chat == nil {
		return ObservationResult{Action: "keep"}, nil
	}
	prompt := profileObservationPrompt + strings.TrimSpace(input.Profile) + profileObservationMessagesPrefix + strings.TrimSpace(input.Conversation)
	request := convention.ChatRequest{Messages: []convention.ChatMessage{{Role: convention.UserRole, Content: prompt}}}
	var raw string
	var err error
	if aware, ok := o.chat.(aichat.ContextAwareLLMService); ok {
		raw, err = aware.ChatWithRequestContext(ctx, request)
	} else {
		raw, err = o.chat.ChatWithRequest(request)
	}
	if err != nil {
		return ObservationResult{}, err
	}
	var result ObservationResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err != nil {
		return ObservationResult{}, fmt.Errorf("decode observation output: %w", err)
	}
	result.Action = strings.ToLower(strings.TrimSpace(result.Action))
	result.ProfileMarkdown = sanitizeProfileMarkdown(result.ProfileMarkdown)
	if result.Action == "keep" {
		return ObservationResult{Action: "keep"}, nil
	}
	if result.Action != "replace" || result.ProfileMarkdown == "" {
		return ObservationResult{}, fmt.Errorf("invalid observation action")
	}
	return result, nil
}

// Models occasionally turn an implicit identity into a profile title. A
// derived profile must be neutral unless identity is an explicit product
// feature, so normalize this heading instead of persisting a guessed name.
func sanitizeProfileMarkdown(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "#") && strings.Contains(lower, "user profile:") {
			prefixLength := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
			lines[index] = strings.Repeat("#", prefixLength) + " Overview"
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (s *Service) ApplyObservation(ctx context.Context, current domain.UserMemoryProfile, result ObservationResult) (domain.UserMemoryProfile, error) {
	currentContent := sanitizeProfileMarkdown(current.ContentMarkdown)
	nextContent := currentContent
	if result.Action == "replace" {
		nextContent = sanitizeProfileMarkdown(result.ProfileMarkdown)
	}
	if strings.TrimSpace(nextContent) == strings.TrimSpace(current.ContentMarkdown) {
		return current, nil
	}
	current.ContentMarkdown = nextContent
	current.Version++
	return s.repo.Save(ctx, current)
}
