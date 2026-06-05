package document_investigation

import (
	"context"
	"errors"
	"strings"
	"testing"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

type stubInvestigator struct {
	getFn      func(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error)
	pageLogsFn func(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error)
}

func (s stubInvestigator) Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return s.getFn(ctx, input)
}

func (s stubInvestigator) PageChunkLogs(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
	return s.pageLogsFn(ctx, input)
}

func TestCapabilityInvokeDiagnosesDocumentFailure(t *testing.T) {
	handle, err := NewCapability(stubInvestigator{
		getFn: func(_ context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
			if input.DocumentID != "doc-1" {
				t.Fatalf("unexpected document id: %q", input.DocumentID)
			}
			return knowledgedomain.KnowledgeDocument{
				ID:          "doc-1",
				Name:        "Product Spec",
				ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
				Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
				PipelineID:  "pipe-1",
				ChunkCount:  0,
			}, nil
		},
		pageLogsFn: func(_ context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
			if input.DocumentID != "doc-1" || input.Page != 1 || input.PageSize != 3 {
				t.Fatalf("unexpected page input: %+v", input)
			}
			return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
				Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
					{
						Log: knowledgedomain.KnowledgeDocumentChunkLog{
							Status:       knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
							ChunkCount:   0,
							ErrorMessage: "pipeline failed",
						},
						IngestionTask: &ingestiondomain.Task{
							ID:     "task-1",
							Status: ingestiondomain.TaskStatusFailed,
						},
						IngestionNodes: []ingestiondomain.TaskNode{
							{NodeID: "indexer", Status: ingestiondomain.TaskStatusFailed, ErrorMessage: "connection refused"},
						},
					},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{DocumentID: "doc-1"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if handle.Spec().Name != agentcapability.NameDocumentInvestigation || handle.Spec().Family != agentcapability.FamilyDocumentInvestigation {
		t.Fatalf("unexpected capability spec: %+v", handle.Spec())
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	output, ok := result.Output.(CapabilityOutput)
	if !ok {
		t.Fatalf("expected capability output type, got %T", result.Output)
	}
	if output.Conclusion != "document ingestion failed at node indexer" || output.Confidence != "high" {
		t.Fatalf("unexpected output: %+v", output)
	}
	if len(result.Delta.Evidence.AddItems) != 1 || !strings.Contains(result.Delta.Evidence.AddItems[0].Content, "failed at node") {
		t.Fatalf("expected evidence delta, got %+v", result.Delta)
	}
}

func TestCapabilityInvokeReturnsDependencyFailure(t *testing.T) {
	handle, err := NewCapability(stubInvestigator{
		getFn: func(_ context.Context, _ knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
			return knowledgedomain.KnowledgeDocument{}, errors.New("repo unavailable")
		},
		pageLogsFn: func(_ context.Context, _ knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
			t.Fatal("PageChunkLogs should not be called when Get fails")
			return knowledgeservice.KnowledgeDocumentChunkLogPageResult{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, invokeErr := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{DocumentID: "doc-1"},
	})
	if invokeErr == nil {
		t.Fatal("expected invoke error")
	}
	if result.ErrorClass != agentcapability.ErrorClassDependency || result.Status != agentcapability.StatusDegraded {
		t.Fatalf("expected dependency failure result, got %+v", result)
	}
}
