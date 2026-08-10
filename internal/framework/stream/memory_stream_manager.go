package stream

import (
	"context"
	"sync"
	"time"
)

type memoryStream struct {
	events      []StreamEvent
	lastUpdated time.Time
}

// MemoryStreamManager 进程内追加式事件流，仅单实例可见。
type MemoryStreamManager struct {
	mu      sync.RWMutex
	streams map[string]*memoryStream
}

func NewMemoryStreamManager() *MemoryStreamManager {
	return &MemoryStreamManager{streams: map[string]*memoryStream{}}
}

func (m *MemoryStreamManager) AppendEvent(_ context.Context, streamID string, event StreamEvent) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.streams[streamID]
	if s == nil {
		s = &memoryStream{}
		m.streams[streamID] = s
	}
	s.events = append(s.events, event)
	s.lastUpdated = time.Now()
	return nil
}

func (m *MemoryStreamManager) GetEvents(_ context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error) {
	if m == nil {
		return nil, fromOffset, nil
	}
	if fromOffset < 0 {
		fromOffset = 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.streams[streamID]
	if s == nil {
		return nil, 0, nil
	}
	if fromOffset >= len(s.events) {
		return nil, len(s.events), nil
	}
	out := make([]StreamEvent, len(s.events)-fromOffset)
	copy(out, s.events[fromOffset:])
	return out, len(s.events), nil
}

// PruneOlderThan 删除 lastUpdated 早于 cutoff 的流，返回删除数量。
func (m *MemoryStreamManager) PruneOlderThan(_ context.Context, cutoff time.Time) int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, s := range m.streams {
		if s.lastUpdated.Before(cutoff) {
			delete(m.streams, id)
			removed++
		}
	}
	return removed
}
