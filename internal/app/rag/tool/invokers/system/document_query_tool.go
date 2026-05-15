package builtin

import (
	"context"
	"fmt"
	"strings"

	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type knowledgeDocumentGetter interface {
	Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error)
}

type DocumentQueryTool struct {
	service knowledgeDocumentGetter
}

func NewDocumentQueryTool(service knowledgeDocumentGetter) *DocumentQueryTool {
	return &DocumentQueryTool{service: service}
}

func (t *DocumentQueryTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "document_query",
		Description: "Query a knowledge document by documentId and return its current state.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "documentId",
				Type:        ragtool.ParamTypeString,
				Description: "Knowledge document id.",
				Required:    true,
			},
		},
	}
}

func (t *DocumentQueryTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "document_query", Status: ragtool.CallStatusFailed, ErrorMessage: "document query service is required"}, fmt.Errorf("document query service is required")
	}
	documentID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "documentId"))
	if documentID == "" {
		return ragtool.Result{Name: "document_query", Status: ragtool.CallStatusFailed, ErrorMessage: "documentId is required"}, fmt.Errorf("documentId is required")
	}

	document, err := t.service.Get(ctx, knowledgeservice.GetKnowledgeDocumentInput{DocumentID: documentID})
	if err != nil {
		return ragtool.Result{Name: "document_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	summary := fmt.Sprintf(
		"document %s (%s) status=%s enabled=%t processMode=%s pipelineId=%s chunkCount=%d",
		document.ID,
		strings.TrimSpace(document.Name),
		strings.TrimSpace(document.Status),
		document.Enabled,
		strings.TrimSpace(document.ProcessMode),
		strings.TrimSpace(document.PipelineID),
		document.ChunkCount,
	)

	return ragtool.Result{
		Name:    "document_query",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: map[string]any{
			"documentId":      document.ID,
			"name":            document.Name,
			"knowledgeBaseId": document.KnowledgeBaseID,
			"status":          document.Status,
			"enabled":         document.Enabled,
			"processMode":     document.ProcessMode,
			"pipelineId":      document.PipelineID,
			"chunkCount":      document.ChunkCount,
			"sourceType":      document.SourceType,
		},
	}, nil
}
