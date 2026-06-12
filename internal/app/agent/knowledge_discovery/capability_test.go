package knowledge_discovery

import (
	"context"
	"testing"

	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

type stubDiscoverer struct {
	lastAction string
	bases      knowledgeservice.KnowledgeBasePageResult
	documents  knowledgeservice.KnowledgeDocumentPageResult
	search     []knowledgeservice.KnowledgeDocumentSearchItem
	err        error
}

func (s *stubDiscoverer) PageBases(_ context.Context, input knowledgeservice.PageKnowledgeBaseInput) (knowledgeservice.KnowledgeBasePageResult, error) {
	s.lastAction = ActionListBases
	if s.err != nil {
		return knowledgeservice.KnowledgeBasePageResult{}, s.err
	}
	if s.bases.Total > 0 {
		return s.bases, nil
	}
	return knowledgeservice.KnowledgeBasePageResult{
		Items: []knowledgedomain.KnowledgeBase{{ID: "kb-1", Name: "Eval KB"}},
		DocumentCounts: map[string]int{
			"kb-1": 3,
		},
		Total:    1,
		Page:     input.Page,
		PageSize: input.PageSize,
	}, nil
}

func (s *stubDiscoverer) PageDocuments(_ context.Context, input knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error) {
	s.lastAction = ActionListDocuments
	if s.err != nil {
		return knowledgeservice.KnowledgeDocumentPageResult{}, s.err
	}
	if s.documents.Total > 0 {
		return s.documents, nil
	}
	return knowledgeservice.KnowledgeDocumentPageResult{
		Items: []knowledgedomain.KnowledgeDocument{
			{ID: "doc-1", Name: "Doc 1", KnowledgeBaseID: input.KnowledgeBaseID, Status: knowledgedomain.KnowledgeDocumentStatusFailed, ChunkCount: 2},
		},
		Total:    1,
		Page:     input.Page,
		PageSize: input.PageSize,
	}, nil
}

func (s *stubDiscoverer) SearchDocuments(_ context.Context, _ knowledgeservice.SearchKnowledgeDocumentsInput) ([]knowledgeservice.KnowledgeDocumentSearchItem, error) {
	s.lastAction = ActionSearchDocuments
	if s.err != nil {
		return nil, s.err
	}
	if len(s.search) > 0 {
		return s.search, nil
	}
	return []knowledgeservice.KnowledgeDocumentSearchItem{
		{ID: "doc-2", Name: "Doc 2", KnowledgeBaseID: "kb-1"},
	}, nil
}

func TestCapabilityInvokeBuildsInvocationResult(t *testing.T) {
	discoverer := &stubDiscoverer{}
	handle, err := NewCapability(discoverer)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Action: ActionListBases, Query: "eval"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	output, ok := result.Output.(CapabilityOutput)
	if !ok || len(output.Bases) != 1 || output.Bases[0].DocumentCount != 3 {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.Notes) == 0 {
		t.Fatalf("expected discovery notes in delta, got %+v", result.Delta)
	}
}

func TestCapabilityInvokeListDocuments(t *testing.T) {
	discoverer := &stubDiscoverer{}
	handle, err := NewCapability(discoverer)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Action: ActionListDocuments, KnowledgeBaseID: "kb-1"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	output, ok := result.Output.(CapabilityOutput)
	if !ok || len(output.Documents) != 1 || output.Documents[0].Status != knowledgedomain.KnowledgeDocumentStatusFailed {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
}

func TestCapabilityInvokeRejectsUnexpectedInput(t *testing.T) {
	handle, err := NewCapability(&stubDiscoverer{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{Input: 1}); err == nil {
		t.Fatal("expected unexpected input type to fail")
	}
}

func TestCapabilityInvokeRejectsEmptyActionByPrecondition(t *testing.T) {
	handle, err := NewCapability(&stubDiscoverer{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Action: "   "},
	}); err == nil {
		t.Fatal("expected empty action precondition to fail")
	}
}

func TestCapabilityInvokeDegradedOnDependencyError(t *testing.T) {
	handle, err := NewCapability(&stubDiscoverer{err: context.Canceled})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	_, err = handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Action: ActionListBases},
	})
	if err == nil {
		t.Fatal("expected dependency failure")
	}
}
