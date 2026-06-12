package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// FilePendingApprovalStore persists pending approval lookup refs on disk.
type FilePendingApprovalStore struct {
	dir string
}

func NewFilePendingApprovalStore(rootDir string) (*FilePendingApprovalStore, error) {
	dir := filepath.Join(filepath.Clean(rootDir), "pending-approvals")
	if err := ensureStoreDir(dir); err != nil {
		return nil, fmt.Errorf("create pending approval store directory: %w", err)
	}
	return &FilePendingApprovalStore{dir: dir}, nil
}

func (s *FilePendingApprovalStore) Put(_ context.Context, conversationID string, userID string, ref PendingApprovalRef) error {
	key, err := pendingApprovalLookupKey(conversationID, userID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(ref)
	if err != nil {
		return fmt.Errorf("marshal pending approval ref: %w", err)
	}
	return writeStoreFile(s.pathForKey(key), payload)
}

func (s *FilePendingApprovalStore) Get(_ context.Context, conversationID string, userID string) (PendingApprovalRef, bool, error) {
	key, err := pendingApprovalLookupKey(conversationID, userID)
	if err != nil {
		return PendingApprovalRef{}, false, err
	}
	payload, ok, err := readStoreFile(s.pathForKey(key))
	if err != nil {
		return PendingApprovalRef{}, false, err
	}
	if !ok {
		return PendingApprovalRef{}, false, nil
	}
	var ref PendingApprovalRef
	if err := json.Unmarshal(payload, &ref); err != nil {
		return PendingApprovalRef{}, false, fmt.Errorf("unmarshal pending approval ref: %w", err)
	}
	return ref, true, nil
}

func (s *FilePendingApprovalStore) Delete(_ context.Context, conversationID string, userID string) error {
	key, err := pendingApprovalLookupKey(conversationID, userID)
	if err != nil {
		return err
	}
	return deleteStoreFile(s.pathForKey(key), "pending approval ref")
}

func (s *FilePendingApprovalStore) pathForKey(key string) string {
	return filepath.Join(s.dir, hashedStoreName(key)+".json")
}
