package domain

import "time"

// Version lists carry metadata only; bodies are read one revision at a time.
type ArtifactVersionSummary struct {
	ArtifactID     string    `json:"artifactId"`
	Revision       int       `json:"revision"`
	Title          string    `json:"title"`
	Author         string    `json:"author"`
	Summary        string    `json:"summary"`
	RestoredFrom   int       `json:"restoredFrom,omitempty"`
	ConversationID string    `json:"conversationId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}
