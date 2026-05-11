package builtin

import (
	"context"
	"fmt"
	"strings"

	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type knowledgeDocumentPager interface {
	Page(ctx context.Context, input knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error)
}

type DocumentListTool struct {
	service knowledgeDocumentPager
}

func NewDocumentListTool(service knowledgeDocumentPager) *DocumentListTool {
	return &DocumentListTool{service: service}
}

func (t *DocumentListTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "document_list",
		Description: "List knowledge documents, optionally filtered by status. Use this to discover documents that failed, are running, or match a keyword — especially when the user asks open-ended questions like 'which documents failed recently?' without providing specific document IDs.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "status",
				Type:        ragtool.ParamTypeString,
				Description: "Filter by document status. Common values: failed, running, success, pending. Leave empty to list all.",
				Required:    false,
			},
			{
				Name:        "query",
				Type:        ragtool.ParamTypeString,
				Description: "Keyword search across document names. Leave empty to match all.",
				Required:    false,
			},
			{
				Name:        "knowledgeBaseId",
				Type:        ragtool.ParamTypeString,
				Description: "Filter by knowledge base id. Optional.",
				Required:    false,
			},
		},
	}
}

func (t *DocumentListTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "document_list", Status: ragtool.CallStatusFailed, ErrorMessage: "document list service is required"}, nil
	}

	status := strings.TrimSpace(readStringArg(call.Arguments, "status"))
	query := strings.TrimSpace(readStringArg(call.Arguments, "query"))
	knowledgeBaseID := strings.TrimSpace(readStringArg(call.Arguments, "knowledgeBaseId"))

	pageInput := knowledgeservice.PageKnowledgeDocumentInput{
		KnowledgeBaseID: knowledgeBaseID,
		Page:            1,
		PageSize:        20,
		Status:          status,
		Query:           query,
	}

	result, err := t.service.Page(ctx, pageInput)
	if err != nil {
		return ragtool.Result{Name: "document_list", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, nil
	}

	if result.Total == 0 {
		return ragtool.Result{
			Name:    "document_list",
			Status:  ragtool.CallStatusSuccess,
			Summary: fmt.Sprintf("no documents found (status=%q query=%q)", status, query),
			Data: map[string]any{
				"total": 0,
				"items": []map[string]any{},
			},
		}, nil
	}

	items := make([]map[string]any, 0, len(result.Items))
	failedCount := 0
	runningCount := 0
	for _, doc := range result.Items {
		item := map[string]any{
			"documentId":      doc.ID,
			"name":            doc.Name,
			"status":          doc.Status,
			"processMode":     doc.ProcessMode,
			"knowledgeBaseId": doc.KnowledgeBaseID,
			"chunkCount":      doc.ChunkCount,
		}
		items = append(items, item)

		switch strings.ToLower(strings.TrimSpace(doc.Status)) {
		case "failed":
			failedCount++
		case "running":
			runningCount++
		}
	}

	scope := knowledgeBaseID
	if scope == "" {
		scope = "all knowledge bases"
	}
	summary := fmt.Sprintf("found %d documents in %s (total=%d)", len(items), scope, result.Total)
	if status == "" {
		summary = fmt.Sprintf("%s, failed=%d running=%d", summary, failedCount, runningCount)
	}

	data := map[string]any{
		"total":        result.Total,
		"items":        items,
		"failedCount":  failedCount,
		"runningCount": runningCount,
	}
	if knowledgeBaseID != "" {
		data["knowledgeBaseId"] = knowledgeBaseID
	}
	if query != "" {
		data["query"] = query
	}
	if status != "" {
		data["status"] = status
	}

	return ragtool.Result{
		Name:    "document_list",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data:    data,
	}, nil
}

// Ensure DocumentListTool implements Tool.
var _ ragtool.Tool = (*DocumentListTool)(nil)
