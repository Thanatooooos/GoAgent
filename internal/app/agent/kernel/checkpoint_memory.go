package kernel

import (
	"context"
	"sync"
)

// MemoryCheckpointStore is the M1 in-memory checkpoint implementation.
type MemoryCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewMemoryCheckpointStore creates an in-memory checkpoint store.
func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{
		data: make(map[string][]byte),
	}
}

func (s *MemoryCheckpointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, ok := s.data[checkPointID]
	if !ok {
		return nil, false, nil
	}
	return data, true, nil
}

func (s *MemoryCheckpointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := make([]byte, len(checkPoint))
	copy(buf, checkPoint)
	s.data[checkPointID] = buf
	return nil
}
