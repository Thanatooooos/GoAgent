// Package stream 提供 append-only 事件流抽象，供 SSE 传输层使用。
package stream

import (
	"context"
	"encoding/json"
	"time"
)

// StreamEvent 是流中的一个原子事件，Name+Data 直接对应一条 SSE 报文。
type StreamEvent struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data,omitempty"`
	Done      bool            `json:"done"`
	Timestamp time.Time       `json:"timestamp"`
}

// StreamManager 只追加事件流：AppendEvent 写、GetEvents 从 fromOffset 增量读。
type StreamManager interface {
	AppendEvent(ctx context.Context, streamID string, event StreamEvent) error
	GetEvents(ctx context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error)
}
