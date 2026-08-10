package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisStreamManager 基于 Redis List 的跨实例事件流。
// key = prefix:streamID；Append=RPush，Get=LRange(fromOffset,-1)。
type RedisStreamManager struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

func NewRedisStreamManager(client *goredis.Client, prefix string, ttl time.Duration) *RedisStreamManager {
	if prefix == "" {
		prefix = "stream:events"
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &RedisStreamManager{client: client, prefix: prefix, ttl: ttl}
}

func (m *RedisStreamManager) key(streamID string) string {
	return fmt.Sprintf("%s:%s", m.prefix, streamID)
}

func (m *RedisStreamManager) AppendEvent(ctx context.Context, streamID string, event StreamEvent) error {
	if m == nil || m.client == nil {
		return nil
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	key := m.key(streamID)
	if err := m.client.RPush(ctx, key, raw).Err(); err != nil {
		return fmt.Errorf("stream append %q: %w", key, err)
	}
	if err := m.client.Expire(ctx, key, m.ttl).Err(); err != nil {
		return fmt.Errorf("stream expire %q: %w", key, err)
	}
	return nil
}

// GetEvents 从 fromOffset 增量读取事件。任一事件解码失败时中止整批读取并返回错误，
// 不返回部分解码结果，避免静默丢事件或偏移错位。
func (m *RedisStreamManager) GetEvents(ctx context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error) {
	if m == nil || m.client == nil {
		return nil, fromOffset, nil
	}
	if fromOffset < 0 {
		fromOffset = 0
	}
	key := m.key(streamID)
	values, err := m.client.LRange(ctx, key, int64(fromOffset), -1).Result()
	if err != nil {
		return nil, fromOffset, fmt.Errorf("stream read %q: %w", key, err)
	}
	out := make([]StreamEvent, 0, len(values))
	for _, v := range values {
		var e StreamEvent
		if err := json.Unmarshal([]byte(v), &e); err != nil {
			return nil, fromOffset, fmt.Errorf("stream decode %q: %w", key, err)
		}
		out = append(out, e)
	}
	return out, fromOffset + len(values), nil
}

// Close 关闭底层 Redis 客户端。
func (m *RedisStreamManager) Close() error {
	if m == nil || m.client == nil {
		return nil
	}
	return m.client.Close()
}
