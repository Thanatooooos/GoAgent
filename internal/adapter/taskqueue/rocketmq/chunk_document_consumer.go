package rocketmq

import (
	"context"
	"encoding/json"
	"fmt"

	rocketmqclient "github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"

	"local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/log"
)

type pushConsumerClient interface {
	Start() error
	Shutdown() error
	Subscribe(topic string, selector consumer.MessageSelector, f func(context.Context, ...*primitive.MessageExt) (consumer.ConsumeResult, error)) error
}

type ChunkDocumentProcessor interface {
	ExecuteChunk(ctx context.Context, input service.ExecuteChunkInput) error
}

type ChunkDocumentConsumer struct {
	consumer  pushConsumerClient
	processor ChunkDocumentProcessor
	topic     string
}

func NewChunkDocumentConsumer(options ChunkDocumentConsumerOptions, processor ChunkDocumentProcessor) (*ChunkDocumentConsumer, error) {
	options = normalizeChunkDocumentConsumerOptions(options)
	configureClientLogger()
	c, err := rocketmqclient.NewPushConsumer(
		consumer.WithNameServer(options.NameServers),
		consumer.WithGroupName(cleanConfigValue(options.Group)),
	)
	if err != nil {
		return nil, fmt.Errorf("create rocketmq chunk document consumer: %w", err)
	}
	return NewChunkDocumentConsumerWithClient(c, options, processor), nil
}

func NewChunkDocumentConsumerWithClient(consumer pushConsumerClient, options ChunkDocumentConsumerOptions, processor ChunkDocumentProcessor) *ChunkDocumentConsumer {
	options = normalizeChunkDocumentConsumerOptions(options)
	return &ChunkDocumentConsumer{
		consumer:  consumer,
		processor: processor,
		topic:     cleanConfigValue(options.Topic),
	}
}

func (c *ChunkDocumentConsumer) Start() error {
	if c == nil || c.consumer == nil {
		return fmt.Errorf("rocketmq consumer is required")
	}
	if c.processor == nil {
		return fmt.Errorf("chunk document processor is required")
	}
	if c.topic == "" {
		return fmt.Errorf("chunk document topic is required")
	}

	if err := c.consumer.Subscribe(c.topic, consumer.MessageSelector{}, c.consume); err != nil {
		return fmt.Errorf("subscribe rocketmq chunk document topic: %w", err)
	}
	if err := c.consumer.Start(); err != nil {
		return fmt.Errorf("start rocketmq chunk document consumer: %w", err)
	}
	return nil
}

func (c *ChunkDocumentConsumer) Shutdown() error {
	if c == nil || c.consumer == nil {
		return nil
	}
	if err := c.consumer.Shutdown(); err != nil {
		return fmt.Errorf("shutdown rocketmq chunk document consumer: %w", err)
	}
	return nil
}

func (c *ChunkDocumentConsumer) consume(ctx context.Context, messages ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, message := range messages {
		if message == nil {
			continue
		}
		var payload ChunkDocumentMessage
		if err := json.Unmarshal(message.Body, &payload); err != nil {
			log.Errorf("decode chunk document message failed: msgId=%s err=%v", message.MsgId, err)
			return consumer.ConsumeRetryLater, err
		}
		if payload.Type != "" && payload.Type != MessageTypeChunkDocument {
			err := fmt.Errorf("unexpected chunk document message type: %s", payload.Type)
			log.Errorf("consume chunk document message failed: msgId=%s err=%v", message.MsgId, err)
			return consumer.ConsumeRetryLater, err
		}
		if err := c.processor.ExecuteChunk(ctx, service.ExecuteChunkInput{
			DocumentID:  payload.DocumentID,
			TriggeredBy: payload.TriggeredBy,
		}); err != nil {
			log.Errorf("execute chunk document task failed: taskId=%s documentId=%s err=%v", payload.TaskID, payload.DocumentID, err)
			return consumer.ConsumeRetryLater, err
		}
	}
	return consumer.ConsumeSuccess, nil
}
