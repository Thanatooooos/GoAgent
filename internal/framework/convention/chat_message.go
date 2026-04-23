package convention

import (
	"fmt"
	"strings"
)

type Role string

const (
	SystemRole    Role = "system"
	UserRole      Role = "user"
	AssistantRole Role = "assistant"
)

func ParseRole(s string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(s)))
	switch role {
	case SystemRole, UserRole, AssistantRole:
		return role, nil
	default:
		return "", fmt.Errorf("invalid role: %s", s)
	}
}

type ChatMessage struct {
	Role             Role   `json:"role"`
	Content          string `json:"content"`
	ThinkingContent  string `json:"thinkingContent,omitempty"`
	ThinkingDuration int    `json:"thinkingDuration,omitempty"`
}

func SystemMessage(content string) ChatMessage {
	return ChatMessage{Role: SystemRole, Content: content}
}

func UserMessage(content string) ChatMessage {
	return ChatMessage{Role: UserRole, Content: content}
}

func AssistantMessage(content string) ChatMessage {
	return ChatMessage{Role: AssistantRole, Content: content}
}
