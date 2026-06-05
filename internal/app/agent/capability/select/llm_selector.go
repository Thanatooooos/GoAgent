package selectcapability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type LLMSelector struct {
	chatService aichat.LLMService
}

func NewLLMSelector(chatService aichat.LLMService) *LLMSelector {
	if chatService == nil {
		return nil
	}
	return &LLMSelector{chatService: chatService}
}

func (s *LLMSelector) Select(ctx context.Context, input SelectionInput) (SelectionOutput, error) {
	if s == nil || s.chatService == nil {
		return SelectionOutput{}, nil
	}
	summary := buildPromptInput(input)
	payload, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return SelectionOutput{}, fmt.Errorf("marshal capability selector input: %w", err)
	}

	jsonMode := true
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(selectorSystemPrompt),
			convention.UserMessage("Capability selection input:\n" + string(payload) + "\n\nReturn JSON only."),
		},
		JSONMode: &jsonMode,
	}
	response, err := s.chatService.ChatWithRequest(request)
	if err != nil {
		return SelectionOutput{}, fmt.Errorf("llm capability selector call: %w", err)
	}

	output, err := parseSelectionOutput(response)
	if err != nil {
		return SelectionOutput{}, err
	}
	if err := validateSelectionOutput(output, input.Capabilities, input.MaxSelections); err != nil {
		return SelectionOutput{}, err
	}
	return output, nil
}

type selectorPromptInput struct {
	UserRequest   string              `json:"user_request"`
	ContextNotes  []string            `json:"context_notes,omitempty"`
	MaxSelections int                 `json:"max_selections,omitempty"`
	Capabilities  []agentcatalog.Card `json:"capabilities"`
}

func buildPromptInput(input SelectionInput) selectorPromptInput {
	return selectorPromptInput{
		UserRequest:   strings.TrimSpace(input.UserRequest),
		ContextNotes:  append([]string(nil), input.ContextNotes...),
		MaxSelections: maxSelectionsOrDefault(input.MaxSelections),
		Capabilities:  append([]agentcatalog.Card(nil), input.Capabilities...),
	}
}

func parseSelectionOutput(raw string) (SelectionOutput, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SelectionOutput{}, fmt.Errorf("capability selector response is empty")
	}
	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}
	var output SelectionOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return SelectionOutput{}, fmt.Errorf("parse capability selector json: %w", err)
	}
	return output, nil
}

func validateSelectionOutput(output SelectionOutput, cards []agentcatalog.Card, maxSelections int) error {
	limit := maxSelectionsOrDefault(maxSelections)
	if len(output.Selections) > limit {
		return fmt.Errorf("capability selector returned %d selections, limit is %d", len(output.Selections), limit)
	}
	knownNames := make(map[string]struct{}, len(cards))
	knownFamilies := make(map[string]struct{}, len(cards))
	knownRoles := make(map[string]struct{})
	knownKinds := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		knownNames[card.Name] = struct{}{}
		if card.Family != "" {
			knownFamilies[card.Family] = struct{}{}
		}
		if card.Kind != "" {
			knownKinds[card.Kind] = struct{}{}
		}
		for _, role := range card.Roles {
			knownRoles[role] = struct{}{}
		}
	}
	for _, selection := range output.Selections {
		if strings.TrimSpace(selection.Name) == "" && strings.TrimSpace(selection.Family) == "" && strings.TrimSpace(selection.Role) == "" {
			return fmt.Errorf("capability selector selection must include at least one of name, family, or role")
		}
		if selection.Name != "" {
			if _, ok := knownNames[selection.Name]; !ok {
				return fmt.Errorf("capability selector chose unknown capability %q", selection.Name)
			}
		}
		if selection.Family != "" {
			if _, ok := knownFamilies[selection.Family]; !ok {
				return fmt.Errorf("capability selector chose unknown family %q", selection.Family)
			}
		}
		if selection.Role != "" {
			if _, ok := knownRoles[selection.Role]; !ok {
				return fmt.Errorf("capability selector chose unknown role %q", selection.Role)
			}
		}
		if selection.Kind != "" {
			if _, ok := knownKinds[selection.Kind]; !ok {
				return fmt.Errorf("capability selector chose unknown kind %q", selection.Kind)
			}
		}
	}
	return nil
}

func extractJSONBlock(raw string) string {
	marker := "```json"
	start := strings.Index(raw, marker)
	if start == -1 {
		marker = "```"
		start = strings.Index(raw, marker)
	}
	if start == -1 {
		return ""
	}
	contentStart := strings.IndexByte(raw[start:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += start + 1
	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}

func maxSelectionsOrDefault(maxSelections int) int {
	if maxSelections > 0 {
		return maxSelections
	}
	return 1
}

const selectorSystemPrompt = `You are the capability-selection layer of an agent runtime.

Choose at most max_selections capabilities from the provided capability catalog.
Prefer higher-level workflow capabilities when they directly solve the request.
Use only capabilities that are present in the provided catalog.
Extract input fields only from the user request or provided context notes.
Do not invent document ids, URLs, trace ids, or other identifiers.
If no capability is a strong match, return {"selections": []}.

Return strict JSON only.`
