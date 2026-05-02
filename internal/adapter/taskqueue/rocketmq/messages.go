package rocketmq

import "time"

const (
	DefaultChunkDocumentTopic         = "knowledge-document-chunk_topic"
	DefaultRefreshRemoteDocumentTopic = "knowledge-document-refresh_topic"
	DefaultProducerGroup              = "ragent-producer"
	DefaultChunkDocumentConsumerGroup = "ragent-document-chunk-consumer"

	MessageTypeChunkDocument         = "chunk_document"
	MessageTypeRefreshRemoteDocument = "refresh_remote_document"
)

type ChunkDocumentMessage struct {
	Type        string    `json:"type"`
	TaskID      string    `json:"taskId"`
	DocumentID  string    `json:"documentId"`
	TriggeredBy string    `json:"triggeredBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

type RefreshRemoteDocumentMessage struct {
	Type       string    `json:"type"`
	TaskID     string    `json:"taskId"`
	DocumentID string    `json:"documentId"`
	ScheduleID string    `json:"scheduleId"`
	CreatedAt  time.Time `json:"createdAt"`
}
