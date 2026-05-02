package test

import (
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
)

func TestKnowledgeDocumentStatusTransitions(t *testing.T) {
	t.Parallel()

	if !domain.CanKnowledgeDocumentTransition(domain.KnowledgeDocumentStatusPending, domain.KnowledgeDocumentStatusRunning) {
		t.Fatal("pending document should be allowed to enter running state")
	}
	if !domain.CanKnowledgeDocumentTransition(domain.KnowledgeDocumentStatusSuccess, domain.KnowledgeDocumentStatusDeleting) {
		t.Fatal("success document should be allowed to enter deleting state")
	}
	if domain.CanKnowledgeDocumentTransition(domain.KnowledgeDocumentStatusRunning, domain.KnowledgeDocumentStatusDeleting) {
		t.Fatal("running document should not be allowed to enter deleting state")
	}
	if domain.CanKnowledgeDocumentTransition(domain.KnowledgeDocumentStatusDeleting, domain.KnowledgeDocumentStatusRunning) {
		t.Fatal("deleting document should not be allowed to re-enter running state")
	}
}

func TestKnowledgeDocumentCanDelete(t *testing.T) {
	t.Parallel()

	if !(domain.KnowledgeDocument{Status: domain.KnowledgeDocumentStatusPending}).CanDelete() {
		t.Fatal("pending document should be deletable")
	}
	if (domain.KnowledgeDocument{Status: domain.KnowledgeDocumentStatusRunning}).CanDelete() {
		t.Fatal("running document should not be deletable")
	}
}
