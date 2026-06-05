package runtime

import (
	"context"
	"sync"
)

// MemorySessionStore is the default in-memory approval session store.
type MemorySessionStore struct {
	mu    sync.Mutex
	items map[string]*RuntimeSession
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		items: make(map[string]*RuntimeSession),
	}
}

func (s *MemorySessionStore) Put(_ context.Context, checkpointID string, session *RuntimeSession) error {
	key, err := checkpointKey(checkpointID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = CloneSession(session)
	return nil
}

func (s *MemorySessionStore) Get(_ context.Context, checkpointID string) (*RuntimeSession, bool, error) {
	key, err := checkpointKey(checkpointID)
	if err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[key]
	if !ok {
		return nil, false, nil
	}
	return CloneSession(session), true, nil
}

func (s *MemorySessionStore) Delete(_ context.Context, checkpointID string) error {
	key, err := checkpointKey(checkpointID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}
