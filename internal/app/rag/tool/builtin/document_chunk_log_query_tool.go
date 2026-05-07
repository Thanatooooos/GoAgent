package builtin

import (
	"context"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type knowledgeDocumentChunkLogPager interface {
	PageChunkLogs(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error)
}

type DocumentChunkLogQueryTool struct {
	service knowledgeDocumentChunkLogPager
}

func NewDocumentChunkLogQueryTool(service knowledgeDocumentChunkLogPager) *DocumentChunkLogQueryTool {
	return &DocumentChunkLogQueryTool{service: service}
}

func (t *DocumentChunkLogQueryTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "document_chunk_log_query",
		Description: "Query latest chunk logs for a knowledge document to diagnose processing and ingestion issues.",
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

func (t *DocumentChunkLogQueryTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "document_chunk_log_query", Status: ragtool.CallStatusFailed, ErrorMessage: "document chunk log service is required"}, fmt.Errorf("document chunk log service is required")
	}
	documentID := strings.TrimSpace(readStringArg(call.Arguments, "documentId"))
	if documentID == "" {
		return ragtool.Result{Name: "document_chunk_log_query", Status: ragtool.CallStatusFailed, ErrorMessage: "documentId is required"}, fmt.Errorf("documentId is required")
	}

	pageResult, err := t.service.PageChunkLogs(ctx, knowledgeservice.KnowledgeDocumentChunkLogPageInput{
		DocumentID: documentID,
		Page:       1,
		PageSize:   3,
	})
	if err != nil {
		return ragtool.Result{Name: "document_chunk_log_query", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	logItems := make([]map[string]any, 0, len(pageResult.Items))
	failedLogs := make([]string, 0)
	runningLogs := make([]string, 0)
	latestStatus := ""
	latestError := ""
	latestTaskID := ""

	for idx, item := range pageResult.Items {
		logItem := item.Log
		entry := map[string]any{
			"logId":           logItem.ID,
			"documentId":      logItem.DocumentID,
			"status":          logItem.Status,
			"processMode":     logItem.ProcessMode,
			"chunkStrategy":   logItem.ChunkStrategy,
			"pipelineId":      logItem.PipelineID,
			"extractDuration": logItem.ExtractDuration,
			"chunkDuration":   logItem.ChunkDuration,
			"embedDuration":   logItem.EmbedDuration,
			"persistDuration": logItem.PersistDuration,
			"totalDuration":   logItem.TotalDuration,
			"chunkCount":      logItem.ChunkCount,
			"errorMessage":    logItem.ErrorMessage,
			"startTime":       logItem.StartTime,
			"endTime":         logItem.EndTime,
			"ingestionTaskId": "",
			"ingestionStatus": "",
			"failedNodeIds":   []string{},
			"runningNodeIds":  []string{},
		}

		if item.IngestionTask != nil {
			entry["ingestionTaskId"] = item.IngestionTask.ID
			entry["ingestionStatus"] = item.IngestionTask.Status
			latestTaskID = firstNonEmpty(latestTaskID, item.IngestionTask.ID)
		}

		failedNodeIDs := make([]string, 0)
		failedNodeErrors := make([]string, 0)
		runningNodeIDs := make([]string, 0)
		for _, node := range item.IngestionNodes {
			switch strings.TrimSpace(node.Status) {
			case ingestiondomain.TaskStatusFailed:
				failedNodeIDs = append(failedNodeIDs, node.NodeID)
				failedNodeErrors = append(failedNodeErrors, fmt.Sprintf("%s(%s)", node.NodeID, strings.TrimSpace(node.ErrorMessage)))
			case ingestiondomain.TaskStatusRunning:
				runningNodeIDs = append(runningNodeIDs, node.NodeID)
			}
		}
		entry["failedNodeIds"] = failedNodeIDs
		entry["runningNodeIds"] = runningNodeIDs
		logItems = append(logItems, entry)

		if idx == 0 {
			latestStatus = strings.TrimSpace(logItem.Status)
			latestError = strings.TrimSpace(logItem.ErrorMessage)
			if latestTaskID == "" {
				latestTaskID = strings.TrimSpace(logItem.ID)
			}
		}

		switch strings.TrimSpace(logItem.Status) {
		case "failed":
			failedSummary := strings.TrimSpace(logItem.ID)
			if len(failedNodeErrors) > 0 {
				failedSummary = fmt.Sprintf("%s[%s]", failedSummary, strings.Join(failedNodeErrors, ", "))
			} else if strings.TrimSpace(logItem.ErrorMessage) != "" {
				failedSummary = fmt.Sprintf("%s(%s)", failedSummary, strings.TrimSpace(logItem.ErrorMessage))
			}
			failedLogs = append(failedLogs, failedSummary)
		case "running":
			runningLogs = append(runningLogs, strings.TrimSpace(logItem.ID))
		}
	}

	summary := fmt.Sprintf("document=%s chunkLogs=%d", documentID, len(pageResult.Items))
	if latestStatus != "" {
		summary = fmt.Sprintf("%s latestStatus=%s", summary, latestStatus)
	}
	if latestTaskID != "" {
		summary = fmt.Sprintf("%s latestTask=%s", summary, latestTaskID)
	}
	if latestError != "" {
		summary = fmt.Sprintf("%s latestError=%s", summary, latestError)
	}
	if len(failedLogs) > 0 {
		summary = fmt.Sprintf("%s failed=[%s]", summary, strings.Join(failedLogs, ", "))
	}
	if len(runningLogs) > 0 {
		summary = fmt.Sprintf("%s running=[%s]", summary, strings.Join(runningLogs, ", "))
	}

	return ragtool.Result{
		Name:    "document_chunk_log_query",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: map[string]any{
			"documentId":      documentID,
			"total":           pageResult.Total,
			"page":            pageResult.Page,
			"pageSize":        pageResult.PageSize,
			"logCount":        len(pageResult.Items),
			"latestStatus":    latestStatus,
			"latestTaskId":    latestTaskID,
			"latestError":     latestError,
			"failedLogCount":  len(failedLogs),
			"runningLogCount": len(runningLogs),
			"chunkLogs":       logItems,
		},
	}, nil
}
