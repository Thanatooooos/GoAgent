package runtimeadapter

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
)

// GlobalKnowledgeBaseAccess makes the current product policy explicit: every
// authenticated user may search every non-deleted knowledge base. Replace it
// with a membership-aware adapter when per-KB permissions are introduced.
type GlobalKnowledgeBaseAccess struct {
	repository knowledgeport.KnowledgeBaseRepository
}

func NewGlobalKnowledgeBaseAccess(db *gorm.DB) *GlobalKnowledgeBaseAccess {
	return &GlobalKnowledgeBaseAccess{repository: postgresknowledge.NewKnowledgeBaseRepository(db)}
}

func (a *GlobalKnowledgeBaseAccess) AccessibleIDs(ctx context.Context, userID string) ([]string, error) {
	if a == nil || a.repository == nil {
		return nil, fmt.Errorf("knowledge base access repository is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}
	bases, err := a.repository.List(ctx, knowledgeport.KnowledgeBaseListFilter{})
	if err != nil {
		return nil, fmt.Errorf("list globally accessible knowledge bases: %w", err)
	}
	ids := make([]string, 0, len(bases))
	for _, base := range bases {
		if id := strings.TrimSpace(base.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
