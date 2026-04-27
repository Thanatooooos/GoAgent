package schedule

import (
	"context"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

const systemUser = "system"

type DocumentStatusHelper struct {
	documentRepo port.KnowledgeDocumentRepository
}

func NewDocumentStatusHelper(documentRepo port.KnowledgeDocumentRepository) *DocumentStatusHelper {
	return &DocumentStatusHelper{documentRepo: documentRepo}
}

func (d *DocumentStatusHelper) TryMarkRunning(ctx context.Context, docID string) (bool, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return false, exception.NewClientException("document id is required", nil)
	}
	if d == nil || d.documentRepo == nil {
		return false, exception.NewServiceException("knowledge document repository is required", nil)
	}

	rows, err := d.documentRepo.UpdateWhere(ctx, port.KnowledgeDocumentConditions{
		ID:       docID,
		Enabled:  boolPointer(true),
		Deleted:  boolPointer(false),
		StatusNE: domain.KnowledgeDocumentStatusRunning,
	}, port.KnowledgeDocumentPatch{
		Status:    port.ValueOf(domain.KnowledgeDocumentStatusRunning),
		UpdatedBy: port.ValueOf(systemUser),
		UpdatedAt: port.ValueOf(time.Now()),
	})
	if err != nil {
		return false, exception.NewServiceException("failed to mark knowledge document running", err)
	}
	return rows > 0, nil
}

func (d *DocumentStatusHelper) MarkFailedIfRunning(ctx context.Context, docID string) error {
	_, err := d.documentRepo.UpdateWhere(ctx, port.KnowledgeDocumentConditions{
		ID:       docID,
		StatusEQ: domain.KnowledgeDocumentChunkLogStatusRunning,
	}, port.KnowledgeDocumentPatch{
		Status:    port.ValueOf(string(domain.KnowledgeDocumentChunkLogStatusFailed)),
		UpdatedAt: port.ValueOf(time.Now()),
	})
	return err
}

func boolPointer(value bool) *bool {
	return &value
}
