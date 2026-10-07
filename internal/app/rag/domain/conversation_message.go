package domain

import (
	"strings"
	"time"
)

type ConversationMessage struct {
	ID               string
	ConversationID   string
	UserID           string
	Role             string
	Content          string
	RawContent       string
	ContentSummary   string
	IsSummarized     bool
	ThinkingContent  string
	ThinkingDuration *int
	Sources          []MessageSource
	CreateTime       time.Time
	UpdateTime       time.Time
}

// DisplayContent preserves the original body when Content is a model-context
// summary. History projection deliberately continues to use Content.
func (m ConversationMessage) DisplayContent() string {
	if strings.TrimSpace(m.RawContent) != "" {
		return m.RawContent
	}
	return m.Content
}

type MessageSource struct {
	Type            string `json:"type"`
	Title           string `json:"title,omitempty"`
	ChunkID         string `json:"chunkId,omitempty"`
	KnowledgeBaseID string `json:"knowledgeBaseId,omitempty"`
	Kind            string `json:"kind,omitempty"`
	URL             string `json:"url,omitempty"`
}
