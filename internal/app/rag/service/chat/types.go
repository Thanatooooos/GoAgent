package chat

import (
	ragtool "local/rag-project/internal/app/rag/tool/core"
)

type RagChatInput struct {
	ConversationID   string
	UserID           string
	Question         string
	KnowledgeBaseIDs []string
	DeepThinking     bool
	UseAgentRuntime  bool
	RequireApproval  bool
}

type RagChatMeta struct {
	ConversationID string `json:"conversationId"`
	TaskID         string `json:"taskId"`
}

type RagChatFinishPayload struct {
	MessageID string
	Title     string
}

type RagChatMemoryStoredPayload struct {
	ConversationID   string `json:"conversationId"`
	MessageID        string `json:"messageId"`
	IsSummarized     bool   `json:"isSummarized"`
	ContentSummary   string `json:"contentSummary,omitempty"`
	RawContentLength int    `json:"rawContentLength,omitempty"`
}

type RagChatSessionRecallHitPayload struct {
	MessageID     string  `json:"messageId"`
	ChunkIndex    int     `json:"chunkIndex"`
	Score         float32 `json:"score"`
	Summary       string  `json:"summary,omitempty"`
	Excerpt       string  `json:"excerpt,omitempty"`
	SourceChunkID string  `json:"sourceChunkId,omitempty"`
}

type RagChatSessionRecallPayload struct {
	Query          string                           `json:"query,omitempty"`
	Used           bool                             `json:"used"`
	HitCount       int                              `json:"hitCount"`
	TopScore       float32                          `json:"topScore"`
	TruncatedBy    string                           `json:"truncatedBy,omitempty"`
	CandidateCount int                              `json:"candidateCount,omitempty"`
	Hits           []RagChatSessionRecallHitPayload `json:"hits,omitempty"`
}

type RagChatEventSink interface {
	SendMeta(meta RagChatMeta) error
	SendFallback(reason string) error
	SendAgentThink(message string) error
	SendAgentOutcome(payload RagChatAgentOutcomePayload) error
	SendApprovalPending(payload RagChatApprovalPendingPayload) error
	SendAgentServiceError(payload RagChatAgentServiceErrorPayload) error
	SendMemoryStored(payload RagChatMemoryStoredPayload) error
	SendSessionRecall(payload RagChatSessionRecallPayload) error
	SendThinking(delta string) error
	SendMessage(delta string) error
	SendToolStart(payload ragtool.ToolCallEvent) error
	SendToolResult(payload ragtool.ToolCallEvent) error
	SendTitle(title string) error
	SendTool(name string, status string, summary string) error
	SendFinish(payload RagChatFinishPayload) error
	SendCancel(payload RagChatFinishPayload) error
	SendError(err error) error
	SendDone() error
}
