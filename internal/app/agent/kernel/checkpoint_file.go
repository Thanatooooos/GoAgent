package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileCheckpointStore persists kernel checkpoints on disk for cross-process resume.
type FileCheckpointStore struct {
	dir string
}

func NewFileCheckpointStore(rootDir string) (*FileCheckpointStore, error) {
	dir := filepath.Join(filepath.Clean(rootDir), "checkpoints")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create checkpoint store directory: %w", err)
	}
	return &FileCheckpointStore{dir: dir}, nil
}

func (s *FileCheckpointStore) Get(_ context.Context, checkPointID string) ([]byte, bool, error) {
	key, err := checkpointStoreKey(checkPointID)
	if err != nil {
		return nil, false, err
	}

	payload, err := os.ReadFile(s.pathForKey(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read checkpoint: %w", err)
	}
	buf := make([]byte, len(payload))
	copy(buf, payload)
	return buf, true, nil
}

func (s *FileCheckpointStore) Set(_ context.Context, checkPointID string, checkPoint []byte) error {
	key, err := checkpointStoreKey(checkPointID)
	if err != nil {
		return err
	}
	buf := make([]byte, len(checkPoint))
	copy(buf, checkPoint)
	return writeCheckpointFile(s.pathForKey(key), buf)
}

func (s *FileCheckpointStore) pathForKey(key string) string {
	return filepath.Join(s.dir, hashCheckpointStoreName(key)+".bin")
}

func checkpointStoreKey(checkPointID string) (string, error) {
	trimmed := strings.TrimSpace(checkPointID)
	if trimmed == "" {
		return "", fmt.Errorf("checkpoint id is required")
	}
	return trimmed, nil
}

func hashCheckpointStoreName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func writeCheckpointFile(path string, payload []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure checkpoint directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp checkpoint: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp checkpoint: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp checkpoint: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace checkpoint: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit checkpoint: %w", err)
	}
	return nil
}
