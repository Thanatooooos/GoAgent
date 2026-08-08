package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RewriteSampleCheckpoint struct {
	SampleName         string                    `json:"sample_name"`
	Query              string                    `json:"query"`
	Sample             RewriteSample             `json:"sample"`
	SampleResult       SharedSampleResult        `json:"sample_result"`
	Artifact           map[string]any            `json:"artifact"`
	BaselineRetrieval  *SampleResult             `json:"baseline_retrieval,omitempty"`
	CandidateRetrieval *SampleResult             `json:"candidate_retrieval,omitempty"`
	CompletedAt        string                    `json:"completed_at"`
}

type RewriteArtifactStore struct {
	dir    string
	resume bool
}

func NewRewriteArtifactStore(dir string, resume bool) *RewriteArtifactStore {
	return &RewriteArtifactStore{
		dir:    strings.TrimSpace(dir),
		resume: resume,
	}
}

func (s *RewriteArtifactStore) Enabled() bool {
	return s != nil && s.dir != ""
}

func (s *RewriteArtifactStore) ResumeEnabled() bool {
	return s.Enabled() && s.resume
}

func (s *RewriteArtifactStore) Load(sampleName, query string) (RewriteSampleCheckpoint, bool, error) {
	if !s.ResumeEnabled() {
		return RewriteSampleCheckpoint{}, false, nil
	}
	path := s.samplePath(sampleName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RewriteSampleCheckpoint{}, false, nil
		}
		return RewriteSampleCheckpoint{}, false, fmt.Errorf("read rewrite checkpoint %q: %w", path, err)
	}
	var checkpoint RewriteSampleCheckpoint
	if err := json.Unmarshal(raw, &checkpoint); err != nil {
		return RewriteSampleCheckpoint{}, false, fmt.Errorf("decode rewrite checkpoint %q: %w", path, err)
	}
	if strings.TrimSpace(checkpoint.Query) != strings.TrimSpace(query) {
		return RewriteSampleCheckpoint{}, false, nil
	}
	return checkpoint, true, nil
}

func (s *RewriteArtifactStore) Save(checkpoint RewriteSampleCheckpoint) error {
	if !s.Enabled() {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create rewrite artifact dir: %w", err)
	}
	checkpoint.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	path := s.samplePath(checkpoint.SampleName)
	payload, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("encode rewrite checkpoint %q: %w", checkpoint.SampleName, err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("write rewrite checkpoint %q: %w", path, err)
	}
	return s.updateSemanticIndex(checkpoint)
}

func (s *RewriteArtifactStore) updateSemanticIndex(checkpoint RewriteSampleCheckpoint) error {
	indexPath := filepath.Join(s.dir, "semantic_evaluations.json")
	index := map[string]any{}
	if raw, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(raw, &index)
	}
	if checkpoint.Artifact == nil {
		return nil
	}
	if semantic, ok := checkpoint.Artifact["semantic_evaluation"]; ok && semantic != nil {
		index[checkpoint.SampleName] = semantic
	}
	payload, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode semantic index: %w", err)
	}
	if err := os.WriteFile(indexPath, payload, 0o644); err != nil {
		return fmt.Errorf("write semantic index: %w", err)
	}
	return nil
}

func (s *RewriteArtifactStore) samplePath(sampleName string) string {
	safeName := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(strings.TrimSpace(sampleName))
	if safeName == "" {
		safeName = "sample"
	}
	return filepath.Join(s.dir, safeName+".json")
}
