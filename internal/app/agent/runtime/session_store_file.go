package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileSessionStore persists approval sessions on disk for cross-process resume.
type FileSessionStore struct {
	dir string
}

func NewFileSessionStore(rootDir string) (*FileSessionStore, error) {
	dir := filepath.Join(filepath.Clean(rootDir), "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session store directory: %w", err)
	}
	return &FileSessionStore{dir: dir}, nil
}

func (s *FileSessionStore) Put(_ context.Context, checkpointID string, session *RuntimeSession) error {
	key, err := checkpointKey(checkpointID)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(CloneSession(session))
	if err != nil {
		return fmt.Errorf("marshal runtime session: %w", err)
	}
	return writeStoreFile(s.pathForKey(key), payload)
}

func (s *FileSessionStore) Get(_ context.Context, checkpointID string) (*RuntimeSession, bool, error) {
	key, err := checkpointKey(checkpointID)
	if err != nil {
		return nil, false, err
	}

	payload, ok, err := readStoreFile(s.pathForKey(key))
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	var session *RuntimeSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return nil, false, fmt.Errorf("unmarshal runtime session: %w", err)
	}
	return CloneSession(session), true, nil
}

func (s *FileSessionStore) Delete(_ context.Context, checkpointID string) error {
	key, err := checkpointKey(checkpointID)
	if err != nil {
		return err
	}
	if err := os.Remove(s.pathForKey(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete runtime session: %w", err)
	}
	return nil
}

func (s *FileSessionStore) pathForKey(key string) string {
	return filepath.Join(s.dir, hashedStoreName(key)+".json")
}

func hashedStoreName(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func writeStoreFile(path string, payload []byte) error {
	dir := filepath.Dir(path)
	if err := ensureStoreDir(dir); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp store file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp store file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp store file: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace store file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit store file: %w", err)
	}
	return nil
}

func ensureStoreDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure store directory: %w", err)
	}
	return nil
}

func deleteStoreFile(path string, label string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete %s: %w", label, err)
	}
	return nil
}

func readStoreFile(path string) ([]byte, bool, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read store file: %w", err)
	}
	return payload, true, nil
}
