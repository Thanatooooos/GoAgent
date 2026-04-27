package service

import "local/rag-project/internal/app/knowledge/port"

type KnowledgeDocumentService struct {
	baseRepo     port.KnowledgeBaseRepository
	documentRepo port.KnowledgeDocumentRepository
}
