package domain

import "time"

type Source struct {
	ID               string    `json:"id"`
	TopicID          string    `json:"topicId"`
	ConversationID   string    `json:"conversationId,omitempty"`
	KnowledgeBaseID  string    `json:"knowledgeBaseId"`
	DocumentID       string    `json:"documentId,omitempty"`
	Name             string    `json:"name"`
	SourceType       string    `json:"sourceType"`
	SourceLocation   string    `json:"sourceLocation,omitempty"`
	Promoted         bool      `json:"promoted"`
	Status           string    `json:"status"`
	ProcessingStatus string    `json:"processingStatus,omitempty"`
	ChunkCount       int       `json:"chunkCount"`
	Error            string    `json:"error,omitempty"`
	Available        bool      `json:"available"`
	CreatedAt        time.Time `json:"createdAt"`
}
type ReserveSource struct {
	Mutation
	Name            string `json:"name"`
	SourceType      string `json:"sourceType"`
	SourceLocation  string `json:"sourceLocation"`
	ConversationID  string `json:"conversationId"`
	KnowledgeBaseID string `json:"knowledgeBaseId,omitempty"`
	ContentHash     string `json:"contentHash,omitempty"`
	Attachment      bool   `json:"attachment,omitempty"`
}
