package chat

import (
	"encoding/json"
	"strings"
)

const (
	openAIStyleSSEDataPrefix = "data:"
	openAIStyleSSEDoneMarker = "[DONE]"
)

type OpenAIStyleSseParser struct {
	reasoningEnabled bool
}

type ParsedEvent struct {
	Content   string
	Reasoning string
	Completed bool
}

type openAIStyleSsePayload struct {
	Choices []openAIStyleChoice `json:"choices"`
}

type openAIStyleChoice struct {
	Delta        *openAIStyleMessage `json:"delta"`
	Message      *openAIStyleMessage `json:"message"`
	FinishReason any                 `json:"finish_reason"`
}

type openAIStyleMessage struct {
	Content          *string `json:"content"`
	ReasoningContent *string `json:"reasoning_content"`
}

func NewOpenAIStyleSseParser(reasoningEnabled bool) *OpenAIStyleSseParser {
	return &OpenAIStyleSseParser{reasoningEnabled: reasoningEnabled}
}

func ParseOpenAIStyleSseLine(line string, reasoningEnabled bool) (ParsedEvent, error) {
	return NewOpenAIStyleSseParser(reasoningEnabled).ParseLine(line)
}

func (p *OpenAIStyleSseParser) ParseLine(line string) (ParsedEvent, error) {
	if strings.TrimSpace(line) == "" {
		return ParsedEvent{}, nil
	}

	payload := strings.TrimSpace(line)
	if strings.HasPrefix(payload, openAIStyleSSEDataPrefix) {
		payload = strings.TrimSpace(payload[len(openAIStyleSSEDataPrefix):])
	}
	if strings.EqualFold(payload, openAIStyleSSEDoneMarker) {
		return ParsedEvent{Completed: true}, nil
	}

	var parsed openAIStyleSsePayload
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return ParsedEvent{}, err
	}
	if len(parsed.Choices) == 0 {
		return ParsedEvent{}, nil
	}

	choice := parsed.Choices[0]
	event := ParsedEvent{
		Content:   extractOpenAIStyleText(choice, func(m *openAIStyleMessage) *string { return m.Content }),
		Completed: choice.FinishReason != nil,
	}
	if p != nil && p.reasoningEnabled {
		event.Reasoning = extractOpenAIStyleText(choice, func(m *openAIStyleMessage) *string { return m.ReasoningContent })
	}

	return event, nil
}

func (e ParsedEvent) HasContent() bool {
	return strings.TrimSpace(e.Content) != ""
}

func (e ParsedEvent) HasReasoning() bool {
	return strings.TrimSpace(e.Reasoning) != ""
}

func extractOpenAIStyleText(choice openAIStyleChoice, field func(*openAIStyleMessage) *string) string {
	for _, message := range []*openAIStyleMessage{choice.Delta, choice.Message} {
		if message == nil {
			continue
		}
		value := field(message)
		if value != nil {
			return *value
		}
	}
	return ""
}
