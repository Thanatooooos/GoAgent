package knowledge_discovery

import (
	"context"

	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
)

// ServiceDiscoverer adapts knowledge base and document services to KnowledgeDiscoverer.
type ServiceDiscoverer struct {
	BaseService     basePager
	DocumentService documentPager
}

type basePager interface {
	Page(ctx context.Context, input knowledgeservice.PageKnowledgeBaseInput) (knowledgeservice.KnowledgeBasePageResult, error)
}

type documentPager interface {
	Page(ctx context.Context, input knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error)
	Search(ctx context.Context, input knowledgeservice.SearchKnowledgeDocumentsInput) ([]knowledgeservice.KnowledgeDocumentSearchItem, error)
}

// NewServiceDiscoverer builds a discoverer from knowledge services.
func NewServiceDiscoverer(baseService basePager, documentService documentPager) *ServiceDiscoverer {
	if baseService == nil || documentService == nil {
		return nil
	}
	return &ServiceDiscoverer{
		BaseService:     baseService,
		DocumentService: documentService,
	}
}

func (s *ServiceDiscoverer) PageBases(ctx context.Context, input knowledgeservice.PageKnowledgeBaseInput) (knowledgeservice.KnowledgeBasePageResult, error) {
	return s.BaseService.Page(ctx, input)
}

func (s *ServiceDiscoverer) PageDocuments(ctx context.Context, input knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error) {
	return s.DocumentService.Page(ctx, input)
}

func (s *ServiceDiscoverer) SearchDocuments(ctx context.Context, input knowledgeservice.SearchKnowledgeDocumentsInput) ([]knowledgeservice.KnowledgeDocumentSearchItem, error) {
	return s.DocumentService.Search(ctx, input)
}

var (
	_ KnowledgeDiscoverer = (*ServiceDiscoverer)(nil)
	_                     = knowledgedomain.KnowledgeBase{}
)
