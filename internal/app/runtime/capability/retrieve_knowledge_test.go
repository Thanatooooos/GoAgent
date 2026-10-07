package capability

import (
	"context"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

func TestRetrieveKnowledgeUsesRuntimeScopeAndReturnsEvidence(t *testing.T) {
	t.Parallel()
	service := &retrieveStub{}
	def := RetrieveKnowledge(service)
	ctx := Context{Context: context.Background(), UserID: "u-1", KnowledgeBaseIDs: []string{"kb-1"}, RetrieveSearchMode: ragretrieve.SearchModeSemantic, AllowKnowledgeRetrieval: true}
	args := Value(`{"query":"  release notes ","top_k":2}`)
	if err := def.Validate(args); err != nil {
		t.Fatalf("validate: %v", err)
	}
	operation, err := def.Describe(args, ctx)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if operation.ID != RetrieveKnowledgeID {
		t.Fatalf("operation = %#v", operation)
	}
	result, err := def.Execute(args, ctx)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if service.request.UserID != "u-1" || len(service.request.KnowledgeBaseIDs) != 1 || service.request.KnowledgeBaseIDs[0] != "kb-1" || service.request.SearchMode != ragretrieve.SearchModeSemantic {
		t.Fatalf("request scope = %#v", service.request)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].DocumentID != "doc-1" {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
}

func TestRetrieveKnowledgeRejectsUnapprovedScope(t *testing.T) {
	t.Parallel()
	def := RetrieveKnowledge(&retrieveStub{})
	_, err := def.Describe(Value(`{"query":"q"}`), Context{Context: context.Background(), AllowKnowledgeRetrieval: true})
	if err == nil {
		t.Fatal("empty KB scope must be rejected")
	}
}

type retrieveStub struct{ request ragretrieve.Request }

func (s *retrieveStub) Retrieve(_ context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.request = request
	return ragretrieve.Result{KnowledgeContext: "context", Chunks: []convention.RetrievedChunk{{ID: "chunk-1", Text: "text", DocumentID: "doc-1", KnowledgeBaseID: "kb-1"}}}, nil
}
func (s *retrieveStub) RetrieveByVector(context.Context, []float32, ragretrieve.Request) (ragretrieve.Result, error) {
	return ragretrieve.Result{}, nil
}
