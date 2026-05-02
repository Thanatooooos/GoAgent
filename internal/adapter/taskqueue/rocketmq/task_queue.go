package rocketmq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	rocketmqclient "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"

	"local/rag-project/internal/app/knowledge/port"
)

type producerClient interface {
	Start() error
	Shutdown() error
	SendSync(ctx context.Context, mq ...*primitive.Message) (*primitive.SendResult, error)
}

type TaskQueue struct {
	producer                   producerClient
	chunkDocumentTopic         string
	refreshRemoteDocumentTopic string
}

var _ port.TaskQueue = (*TaskQueue)(nil)

func NewTaskQueue(options TaskQueueOptions) (*TaskQueue, error) {
	options = normalizeTaskQueueOptions(options)
	configureClientLogger()
	p, err := rocketmqclient.NewProducer(
		producer.WithNameServer(options.NameServers),
		producer.WithGroupName(cleanConfigValue(options.ProducerGroup)),
		producer.WithSendMsgTimeout(options.SendMessageTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq producer: %w", err)
	}
	return NewTaskQueueWithProducer(p, options), nil
}

func NewTaskQueueWithProducer(producer producerClient, options TaskQueueOptions) *TaskQueue {
	options = normalizeTaskQueueOptions(options)
	return &TaskQueue{
		producer:                   producer,
		chunkDocumentTopic:         cleanConfigValue(options.ChunkDocumentTopic),
		refreshRemoteDocumentTopic: cleanConfigValue(options.RefreshRemoteDocumentTopic),
	}
}

func (q *TaskQueue) Start() error {
	if q == nil || q.producer == nil {
		return fmt.Errorf("rocketmq producer is required")
	}
	if err := q.producer.Start(); err != nil {
		return fmt.Errorf("start rocketmq producer: %w", err)
	}
	return nil
}

func (q *TaskQueue) Shutdown() error {
	if q == nil || q.producer == nil {
		return nil
	}
	if err := q.producer.Shutdown(); err != nil {
		return fmt.Errorf("shutdown rocketmq producer: %w", err)
	}
	return nil
}

func (q *TaskQueue) SubmitChunkDocument(ctx context.Context, task port.ChunkDocumentTask) error {
	message := ChunkDocumentMessage{
		Type:        MessageTypeChunkDocument,
		TaskID:      strings.TrimSpace(task.TaskID),
		DocumentID:  strings.TrimSpace(task.DocumentID),
		TriggeredBy: strings.TrimSpace(task.TriggeredBy),
		CreatedAt:   time.Now(),
	}
	if message.TaskID == "" {
		return fmt.Errorf("chunk document task id is required")
	}
	if message.DocumentID == "" {
		return fmt.Errorf("chunk document id is required")
	}

	return q.sendJSON(ctx, q.chunkDocumentTopic, message.TaskID, message.DocumentID, message)
}

func (q *TaskQueue) SubmitRefreshRemoteDocument(ctx context.Context, task port.RefreshRemoteDocumentTask) error {
	message := RefreshRemoteDocumentMessage{
		Type:       MessageTypeRefreshRemoteDocument,
		TaskID:     strings.TrimSpace(task.TaskID),
		DocumentID: strings.TrimSpace(task.DocumentID),
		ScheduleID: strings.TrimSpace(task.ScheduleID),
		CreatedAt:  time.Now(),
	}
	if message.TaskID == "" {
		return fmt.Errorf("refresh remote document task id is required")
	}
	if message.ScheduleID == "" {
		return fmt.Errorf("refresh remote document schedule id is required")
	}

	return q.sendJSON(ctx, q.refreshRemoteDocumentTopic, message.TaskID, message.DocumentID, message.ScheduleID, message)
}

func (q *TaskQueue) sendJSON(ctx context.Context, topic string, keys ...any) error {
	if q == nil || q.producer == nil {
		return fmt.Errorf("rocketmq producer is required")
	}
	topic = cleanConfigValue(topic)
	if topic == "" {
		return fmt.Errorf("rocketmq topic is required")
	}
	if len(keys) == 0 {
		return fmt.Errorf("rocketmq message body is required")
	}

	bodyValue := keys[len(keys)-1]
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return fmt.Errorf("marshal rocketmq message: %w", err)
	}

	messageKeys := make([]string, 0, len(keys)-1)
	for _, key := range keys[:len(keys)-1] {
		text := strings.TrimSpace(fmt.Sprint(key))
		if text != "" {
			messageKeys = append(messageKeys, text)
		}
	}

	msg := primitive.NewMessage(topic, body)
	if len(messageKeys) > 0 {
		msg.WithKeys(messageKeys)
	}
	if _, err := q.producer.SendSync(ctx, msg); err != nil {
		return fmt.Errorf("send rocketmq message: topic=%s err=%w", topic, err)
	}
	return nil
}

func cleanConfigValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "${unique-name:}", "")
	return value
}
