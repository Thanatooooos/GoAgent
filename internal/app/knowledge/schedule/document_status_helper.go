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

type StoredFileDTO struct {
	Url            string
	DetectedType   string
	Size           int64
	OriginFileName string
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

	rows, err := d.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(docID),
		port.KnowledgeDocument.Enabled.Eq(true),
		port.KnowledgeDocument.Deleted.Eq(false),
		port.KnowledgeDocument.Status.In(
			domain.KnowledgeDocumentStatusPending,
			domain.KnowledgeDocumentStatusFailed,
			domain.KnowledgeDocumentStatusSuccess,
		),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusRunning),
		port.KnowledgeDocument.UpdatedBy.To(systemUser),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	if err != nil {
		return false, exception.NewServiceException("failed to mark knowledge document running", err)
	}
	return rows > 0, nil
}

func (d *DocumentStatusHelper) MarkFailedIfRunning(ctx context.Context, docID string) error {
	_, err := d.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(docID),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusFailed),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	return err
}

func (d *DocumentStatusHelper) MarkSuccessIfRunning(ctx context.Context, docID string) error {
	_, err := d.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(strings.TrimSpace(docID)),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusSuccess),
		port.KnowledgeDocument.UpdatedBy.To(systemUser),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	return err
}

func (d *DocumentStatusHelper) RecoverStuckRunning(ctx context.Context, timeoutMinutes int64) (int64, error) {
	timeout := max(timeoutMinutes, 10)
	threshold := time.Now().Add(-time.Duration(timeout) * time.Minute)
	result, err := d.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
		port.KnowledgeDocument.UpdatedAt.Lt(threshold),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusFailed),
		port.KnowledgeDocument.UpdatedBy.To(systemUser),
	))
	return result, err
}

func (d *DocumentStatusHelper) ApplyRefreshedFileMetadata(ctx context.Context, docID string, stored StoredFileDTO) error {
	result, err := d.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(docID),
	), port.Set(
		port.KnowledgeDocument.FileSize.To(stored.Size),
		port.KnowledgeDocument.FileURL.To(stored.Url),
		port.KnowledgeDocument.FileType.To(stored.DetectedType),
		port.KnowledgeDocument.Name.To(stored.OriginFileName),
		port.KnowledgeDocument.UpdatedBy.To(systemUser),
	))
	if result == 0 {
		return exception.NewClientException("non-existed file", err)
	}
	return err
}
