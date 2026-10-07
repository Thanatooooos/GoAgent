package rag

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/distributedid"
	infraembedding "local/rag-project/internal/infra-ai/embedding"
)

type ConversationMessageChunkSink struct {
	db        *gorm.DB
	embedding infraembedding.EmbeddingService
	now       func() time.Time
}

func NewConversationMessageChunkSink(db *gorm.DB, embedding infraembedding.EmbeddingService) *ConversationMessageChunkSink {
	return &ConversationMessageChunkSink{
		db:        db,
		embedding: embedding,
		now:       time.Now,
	}
}

func (s *ConversationMessageChunkSink) PersistMessageChunks(ctx context.Context, message domain.ConversationMessage, chunks []port.ProcessedConversationMessageChunk) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	prepared, err := s.PrepareChunks(ctx, message, chunks)
	if err != nil {
		return err
	}
	return PersistPreparedChunks(ctx, s.db, prepared)
}

// PreparedSessionChunks contains database writes only; embedding runs before locks.
type PreparedSessionChunks struct {
	Chunks     []domain.SessionChunk
	Embeddings []domain.SessionChunkEmbedding
}

func (s *ConversationMessageChunkSink) PrepareChunks(ctx context.Context, message domain.ConversationMessage, chunks []port.ProcessedConversationMessageChunk) (PreparedSessionChunks, error) {
	if len(chunks) == 0 {
		return PreparedSessionChunks{}, nil
	}
	if s == nil || s.embedding == nil {
		return PreparedSessionChunks{}, fmt.Errorf("embedding service is required")
	}
	now := s.now()
	sessionChunks := make([]domain.SessionChunk, 0, len(chunks))
	sessionEmbeddings := make([]domain.SessionChunkEmbedding, 0, len(chunks))
	for _, chunk := range chunks {
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}
		chunkID, err := nextSessionChunkID()
		if err != nil {
			return PreparedSessionChunks{}, err
		}
		vector, err := s.embedding.Embed(content)
		if err != nil {
			return PreparedSessionChunks{}, fmt.Errorf("embed session chunk %d: %w", chunk.ChunkIndex, err)
		}
		sessionChunks = append(sessionChunks, domain.SessionChunk{
			ID:             chunkID,
			ConversationID: strings.TrimSpace(message.ConversationID),
			MessageID:      strings.TrimSpace(message.ID),
			UserID:         strings.TrimSpace(message.UserID),
			ChunkIndex:     chunk.ChunkIndex,
			Content:        content,
			ContentSummary: strings.TrimSpace(chunk.ContentSummary),
			TokenEstimate:  chunk.TokenEstimate,
			CreateTime:     now,
			UpdateTime:     now,
		})
		sessionEmbeddings = append(sessionEmbeddings, domain.SessionChunkEmbedding{
			ChunkID:    chunkID,
			Embedding:  vector,
			CreateTime: now,
			UpdateTime: now,
		})
	}
	return PreparedSessionChunks{Chunks: sessionChunks, Embeddings: sessionEmbeddings}, nil
}

func PersistPreparedChunks(ctx context.Context, db *gorm.DB, prepared PreparedSessionChunks) error {
	if len(prepared.Chunks) == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewSessionChunkRepository(tx).CreateBatch(ctx, prepared.Chunks); err != nil {
			return err
		}
		return NewSessionChunkEmbeddingRepository(tx).UpsertBatch(ctx, prepared.Embeddings)
	})
}

func nextSessionChunkID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", fmt.Errorf("generate session chunk id: %w", err)
	}
	return strconv.FormatInt(id, 10), nil
}
