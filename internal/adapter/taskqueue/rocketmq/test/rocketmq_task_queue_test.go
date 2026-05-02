package rocketmq_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"

	taskrocketmq "local/rag-project/internal/adapter/taskqueue/rocketmq"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/app/knowledge/service"
)

type fakeProducer struct {
	started bool
	sent    []*primitive.Message
}

func (f *fakeProducer) Start() error {
	f.started = true
	return nil
}

func (f *fakeProducer) Shutdown() error {
	return nil
}

func (f *fakeProducer) SendSync(ctx context.Context, mq ...*primitive.Message) (*primitive.SendResult, error) {
	f.sent = append(f.sent, mq...)
	return &primitive.SendResult{}, nil
}

func TestTaskQueueSubmitChunkDocumentSendsRocketMQMessage(t *testing.T) {
	producer := &fakeProducer{}
	queue := taskrocketmq.NewTaskQueueWithProducer(producer, taskrocketmq.TaskQueueOptions{
		ChunkDocumentTopic: "chunk-topic",
	})

	err := queue.SubmitChunkDocument(context.Background(), port.ChunkDocumentTask{
		TaskID:      "task-1",
		DocumentID:  "doc-1",
		TriggeredBy: "u-1",
	})
	if err != nil {
		t.Fatalf("SubmitChunkDocument() error = %v", err)
	}
	if len(producer.sent) != 1 {
		t.Fatalf("expected one message, got %d", len(producer.sent))
	}
	msg := producer.sent[0]
	if msg.Topic != "chunk-topic" {
		t.Fatalf("unexpected topic: %q", msg.Topic)
	}

	var payload taskrocketmq.ChunkDocumentMessage
	if err := json.Unmarshal(msg.Body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload.Type != taskrocketmq.MessageTypeChunkDocument || payload.TaskID != "task-1" || payload.DocumentID != "doc-1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

type fakePushConsumer struct {
	started bool
	topic   string
	handler func(context.Context, ...*primitive.MessageExt) (consumer.ConsumeResult, error)
}

func (f *fakePushConsumer) Start() error {
	f.started = true
	return nil
}

func (f *fakePushConsumer) Shutdown() error {
	return nil
}

func (f *fakePushConsumer) Subscribe(topic string, selector consumer.MessageSelector, handler func(context.Context, ...*primitive.MessageExt) (consumer.ConsumeResult, error)) error {
	f.topic = topic
	f.handler = handler
	return nil
}

type fakeChunkProcessor struct {
	inputs []service.ExecuteChunkInput
}

func (f *fakeChunkProcessor) ExecuteChunk(ctx context.Context, input service.ExecuteChunkInput) error {
	f.inputs = append(f.inputs, input)
	return nil
}

func TestChunkDocumentConsumerDispatchesToProcessor(t *testing.T) {
	consumerClient := &fakePushConsumer{}
	processor := &fakeChunkProcessor{}
	chunkConsumer := taskrocketmq.NewChunkDocumentConsumerWithClient(consumerClient, taskrocketmq.ChunkDocumentConsumerOptions{
		Topic: "chunk-topic",
	}, processor)

	if err := chunkConsumer.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !consumerClient.started || consumerClient.topic != "chunk-topic" {
		t.Fatalf("consumer not started/subscribed correctly: %+v", consumerClient)
	}

	body, err := json.Marshal(taskrocketmq.ChunkDocumentMessage{
		Type:        taskrocketmq.MessageTypeChunkDocument,
		TaskID:      "task-1",
		DocumentID:  "doc-1",
		TriggeredBy: "u-1",
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	result, err := consumerClient.handler(context.Background(), &primitive.MessageExt{
		Message: primitive.Message{Body: body},
		MsgId:   "msg-1",
	})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result != consumer.ConsumeSuccess {
		t.Fatalf("unexpected consume result: %v", result)
	}
	if len(processor.inputs) != 1 || processor.inputs[0].DocumentID != "doc-1" || processor.inputs[0].TriggeredBy != "u-1" {
		t.Fatalf("unexpected processor inputs: %+v", processor.inputs)
	}
}
