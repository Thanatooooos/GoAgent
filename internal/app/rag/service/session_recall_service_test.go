package service

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

type sessionChunkRepoStub struct {
	existsFn func(ctx context.Context, conversationID string, userID string, excludeMessageID string) (bool, error)
	searchFn func(ctx context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error)
}

func (s sessionChunkRepoStub) CreateBatch(context.Context, []domain.SessionChunk) error {
	return nil
}

func (s sessionChunkRepoStub) ExistsRecallable(ctx context.Context, conversationID string, userID string, excludeMessageID string) (bool, error) {
	return s.existsFn(ctx, conversationID, userID, excludeMessageID)
}

func (s sessionChunkRepoStub) SearchRecallableByVector(ctx context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error) {
	return s.searchFn(ctx, conversationID, userID, excludeMessageID, vector, topK)
}

type sessionRecallEmbeddingStub struct {
	vectors    [][]float32
	embedCalls int
}

func (s *sessionRecallEmbeddingStub) Embed(text string) ([]float32, error) {
	s.embedCalls++
	if len(s.vectors) == 0 {
		return []float32{0.1, 0.2}, nil
	}
	vector := s.vectors[0]
	s.vectors = s.vectors[1:]
	return vector, nil
}

func (s *sessionRecallEmbeddingStub) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return s.Embed(text)
}

func (s *sessionRecallEmbeddingStub) EmbedBatch(texts []string) ([][]float32, error) {
	return nil, nil
}

func (s *sessionRecallEmbeddingStub) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return nil, nil
}

func (s *sessionRecallEmbeddingStub) Dimension() int {
	return 2
}

func TestSessionRecallServiceSkipsEmbeddingWhenNoRecallableChunks(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	searchCalls := 0
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return false, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				searchCalls++
				return nil, nil
			},
		},
		embedding,
		SessionRecallOptions{Enabled: true},
	)

	result, err := service.Recall(context.Background(), SessionRecallInput{
		ConversationID:   "conv-1",
		UserID:           "user-1",
		Query:            "panic retriever timeout",
		ExcludeMessageID: "msg-current",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if result.Used {
		t.Fatalf("expected unused result, got %+v", result)
	}
	if embedding.embedCalls != 0 {
		t.Fatalf("expected no embedding call, got %d", embedding.embedCalls)
	}
	if searchCalls != 0 {
		t.Fatalf("expected no search call, got %d", searchCalls)
	}
}

func TestSessionRecallServiceBuildsContextWithPerMessageLimit(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return true, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				return []domain.SessionChunkSearchHit{
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-1", MessageID: "msg-1", ChunkIndex: 1, Content: "panic retriever timeout at line 42", ContentSummary: "chunk summary 1", TokenEstimate: 8},
						Score:        0.95,
					},
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-2", MessageID: "msg-1", ChunkIndex: 2, Content: "second detail also mentions panic retriever timeout", ContentSummary: "chunk summary 2", TokenEstimate: 9},
						Score:        0.90,
					},
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-3", MessageID: "msg-1", ChunkIndex: 3, Content: "third detail should be dropped by per-message limit", ContentSummary: "chunk summary 3", TokenEstimate: 8},
						Score:        0.85,
					},
					{
						SessionChunk: domain.SessionChunk{ID: "chunk-4", MessageID: "msg-2", ChunkIndex: 1, Content: "another message with timeout retriever detail", ContentSummary: "chunk summary 4", TokenEstimate: 8},
						Score:        0.80,
					},
				}, nil
			},
		},
		embedding,
		SessionRecallOptions{
			Enabled:             true,
			MaxExcerpts:         3,
			MaxChunksPerMessage: 2,
			MaxPromptTokens:     200,
			Estimator:           fixedTokenEstimator{factor: 1},
		},
	)

	result, err := service.Recall(context.Background(), SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if !result.Used {
		t.Fatalf("expected used result, got %+v", result)
	}
	if len(result.Hits) != 3 {
		t.Fatalf("expected 3 hits, got %d: %+v", len(result.Hits), result.Hits)
	}
	if result.Hits[0].MessageID != "msg-1" || result.Hits[1].MessageID != "msg-1" || result.Hits[2].MessageID != "msg-2" {
		t.Fatalf("unexpected hit ordering: %+v", result.Hits)
	}
	if strings.Contains(result.Context, "chunk summary 3") {
		t.Fatalf("expected third chunk from same message to be filtered out, got %q", result.Context)
	}
	if result.skippedPerMessageLimit != 1 {
		t.Fatalf("expected one hit to be skipped by per-message limit, got %+v", result)
	}
	if !strings.Contains(result.Context, "摘要：chunk summary 1") || !strings.Contains(result.Context, "原文片段：") {
		t.Fatalf("expected summary + excerpt context, got %q", result.Context)
	}
}

func TestSessionRecallServiceChoosesBestExcerptWindow(t *testing.T) {
	embedding := &sessionRecallEmbeddingStub{}
	service := NewSessionRecallService(
		sessionChunkRepoStub{
			existsFn: func(context.Context, string, string, string) (bool, error) {
				return true, nil
			},
			searchFn: func(context.Context, string, string, string, []float32, int) ([]domain.SessionChunkSearchHit, error) {
				return []domain.SessionChunkSearchHit{
					{
						SessionChunk: domain.SessionChunk{
							ID:             "chunk-1",
							MessageID:      "msg-1",
							ChunkIndex:     1,
							Content:        strings.Join([]string{"alpha filler", "panic retriever timeout detail", "omega filler"}, "\n"),
							ContentSummary: "target summary",
							TokenEstimate:  80,
						},
						Score: 0.91,
					},
				}, nil
			},
		},
		embedding,
		SessionRecallOptions{
			Enabled:              true,
			MaxExcerpts:          1,
			MaxChunksPerMessage:  1,
			ExcerptTargetTokens:  24,
			ExcerptOverlapTokens: 4,
			MaxPromptTokens:      100,
			Estimator:            fixedTokenEstimator{factor: 1},
		},
	)

	result, err := service.Recall(context.Background(), SessionRecallInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Query:          "panic retriever timeout",
	})
	if err != nil {
		t.Fatalf("Recall returned error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected one hit, got %+v", result.Hits)
	}
	if !strings.Contains(result.Hits[0].Excerpt, "panic retriever timeout") {
		t.Fatalf("expected best excerpt window to be selected, got %q", result.Hits[0].Excerpt)
	}
	if strings.Contains(result.Hits[0].Excerpt, "alpha filler") && !strings.Contains(result.Hits[0].Excerpt, "panic retriever timeout") {
		t.Fatalf("expected lexical overlap to outrank earlier filler window, got %q", result.Hits[0].Excerpt)
	}
}
