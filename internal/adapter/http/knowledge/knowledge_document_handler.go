package knowledge

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/service"
)

type KnowledgeDocumentService interface {
	Upload(ctx context.Context, input service.UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	StartChunk(ctx context.Context, input service.StartChunkKnowledgeDocumentInput) error
	Get(ctx context.Context, input service.GetKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	Update(ctx context.Context, input service.UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error)
	Page(ctx context.Context, input service.PageKnowledgeDocumentInput) (service.KnowledgeDocumentPageResult, error)
	Search(ctx context.Context, input service.SearchKnowledgeDocumentsInput) ([]service.KnowledgeDocumentSearchItem, error)
	Enable(ctx context.Context, input service.EnableKnowledgeDocumentInput) error
	Delete(ctx context.Context, input service.DeleteKnowledgeDocumentInput) error
	PageChunkLogs(ctx context.Context, input service.KnowledgeDocumentChunkLogPageInput) (service.KnowledgeDocumentChunkLogPageResult, error)
	PageScheduleExecs(ctx context.Context, input service.PageKnowledgeDocumentScheduleExecInput) (service.KnowledgeDocumentScheduleExecPageResult, error)
}

type KnowledgeDocumentHandler struct {
	service KnowledgeDocumentService
}

func NewKnowledgeDocumentHandler(service KnowledgeDocumentService) *KnowledgeDocumentHandler {
	return &KnowledgeDocumentHandler{service: service}
}
