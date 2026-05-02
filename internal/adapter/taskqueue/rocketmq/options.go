package rocketmq

import (
	"strings"
	"time"

	"local/rag-project/internal/framework/config"
)

type TaskQueueOptions struct {
	NameServers                []string
	ProducerGroup              string
	SendMessageTimeout         time.Duration
	ChunkDocumentTopic         string
	RefreshRemoteDocumentTopic string
}

type ChunkDocumentConsumerOptions struct {
	NameServers []string
	Group       string
	Topic       string
}

func TaskQueueOptionsFromConfig(cfg *config.Config) TaskQueueOptions {
	if cfg == nil {
		return TaskQueueOptions{}
	}
	timeout := time.Duration(cfg.RocketMQ.Producer.SendMessageTimeout) * time.Millisecond
	return TaskQueueOptions{
		NameServers:                parseNameServers(cfg.RocketMQ.NameServer),
		ProducerGroup:              cfg.RocketMQ.Producer.Group,
		SendMessageTimeout:         timeout,
		ChunkDocumentTopic:         cfg.RocketMQ.Topics.ChunkDocument,
		RefreshRemoteDocumentTopic: cfg.RocketMQ.Topics.RefreshRemoteDocument,
	}
}

func ChunkDocumentConsumerOptionsFromConfig(cfg *config.Config) ChunkDocumentConsumerOptions {
	if cfg == nil {
		return ChunkDocumentConsumerOptions{}
	}
	return ChunkDocumentConsumerOptions{
		NameServers: parseNameServers(cfg.RocketMQ.NameServer),
		Group:       cfg.RocketMQ.Consumer.ChunkDocumentGroup,
		Topic:       cfg.RocketMQ.Topics.ChunkDocument,
	}
}

func normalizeTaskQueueOptions(options TaskQueueOptions) TaskQueueOptions {
	if len(options.NameServers) == 0 {
		options.NameServers = []string{"127.0.0.1:9876"}
	}
	if strings.TrimSpace(options.ProducerGroup) == "" {
		options.ProducerGroup = DefaultProducerGroup
	}
	if options.SendMessageTimeout <= 0 {
		options.SendMessageTimeout = 3 * time.Second
	}
	if strings.TrimSpace(options.ChunkDocumentTopic) == "" {
		options.ChunkDocumentTopic = DefaultChunkDocumentTopic
	}
	if strings.TrimSpace(options.RefreshRemoteDocumentTopic) == "" {
		options.RefreshRemoteDocumentTopic = DefaultRefreshRemoteDocumentTopic
	}
	return options
}

func normalizeChunkDocumentConsumerOptions(options ChunkDocumentConsumerOptions) ChunkDocumentConsumerOptions {
	if len(options.NameServers) == 0 {
		options.NameServers = []string{"127.0.0.1:9876"}
	}
	if strings.TrimSpace(options.Group) == "" {
		options.Group = DefaultChunkDocumentConsumerGroup
	}
	if strings.TrimSpace(options.Topic) == "" {
		options.Topic = DefaultChunkDocumentTopic
	}
	return options
}

func parseNameServers(raw string) []string {
	parts := strings.Split(raw, ";")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	if len(result) == 0 && strings.TrimSpace(raw) != "" {
		result = append(result, strings.TrimSpace(raw))
	}
	return result
}
