package domain

import "time"

type ChatRequest struct {
	SourceIDs        []string `json:"sourceIds,omitempty"`
	RequestID        string   `json:"requestId"`
	ConversationID   string   `json:"conversationId,omitempty"`
	ContinueFrom     string   `json:"continueFrom,omitempty"`
	ItemID           string   `json:"itemId,omitempty"`
	ArtifactID       string   `json:"artifactId,omitempty"`
	ArtifactRevision int      `json:"artifactRevision,omitempty"`
	Action           string   `json:"action,omitempty"`
	Question         string   `json:"question"`
}
type TurnSnapshot struct {
	TopicName      string     `json:"topicName"`
	Description    string     `json:"description"`
	State          State      `json:"state"`
	Artifacts      []Artifact `json:"artifacts"`
	OmittedEntries int        `json:"omittedEntries,omitempty"`
}
type Turn struct {
	Outputs            []TurnOutput `json:"outputs"`
	ID                 string       `json:"id"`
	TopicID            string       `json:"topicId"`
	UserID             string       `json:"-"`
	ConversationID     string       `json:"conversationId"`
	ItemID             string       `json:"itemId,omitempty"`
	ArtifactID         string       `json:"artifactId,omitempty"`
	ArtifactRevision   int          `json:"artifactRevision"`
	StateRevision      int          `json:"stateRevision"`
	UserMessageID      string       `json:"userMessageId"`
	AssistantMessageID string       `json:"assistantMessageId,omitempty"`
	Question           string       `json:"question"`
	Action             string       `json:"action"`
	Snapshot           TurnSnapshot `json:"snapshot"`
	Status             string       `json:"status"`
	Error              string       `json:"error,omitempty"`
	DeadlineAt         time.Time    `json:"deadlineAt"`
	CreatedAt          time.Time    `json:"createdAt"`
}
type TurnOutput struct {
	Kind       string `json:"kind"`
	ArtifactID string `json:"artifactId,omitempty"`
	Revision   int    `json:"revision,omitempty"`
	Title      string `json:"title,omitempty"`
	Summary    string `json:"summary,omitempty"`
	ProposalID string `json:"proposalId,omitempty"`
}
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}
