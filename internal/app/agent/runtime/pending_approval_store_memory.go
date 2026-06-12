package runtime

import (
	"context"
	"sync"
)

// MemoryPendingApprovalStore is the default in-memory pending approval index.
type MemoryPendingApprovalStore struct {
	mu    sync.Mutex
	items map[string]PendingApprovalRef
}

func NewMemoryPendingApprovalStore() *MemoryPendingApprovalStore {
	return &MemoryPendingApprovalStore{
		items: make(map[string]PendingApprovalRef),
	}
}

func (s *MemoryPendingApprovalStore) Put(_ context.Context, conversationID string, userID string, ref PendingApprovalRef) error {
	key, err := pendingApprovalLookupKey(conversationID, userID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = ref
	return nil
}

func (s *MemoryPendingApprovalStore) Get(_ context.Context, conversationID string, userID string) (PendingApprovalRef, bool, error) {
	key, err := pendingApprovalLookupKey(conversationID, userID)
	if err != nil {
		return PendingApprovalRef{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.items[key]
	if !ok {
		return PendingApprovalRef{}, false, nil
	}
	return ref, true, nil
}

func (s *MemoryPendingApprovalStore) Delete(_ context.Context, conversationID string, userID string) error {
	key, err := pendingApprovalLookupKey(conversationID, userID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}
