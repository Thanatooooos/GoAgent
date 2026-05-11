package builtin

import (
	"context"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type knowledgeDocumentDiagnoseReader interface {
	Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error)
	PageChunkLogs(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error)
}

// Optional interface for enriching diagnose with live task node data.
type ingestionTaskNodeReader interface {
	ListNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error)
}

type DocumentIngestionDiagnoseTool struct {
	service       knowledgeDocumentDiagnoseReader
	taskNodeSvc   ingestionTaskNodeReader
}

func NewDocumentIngestionDiagnoseTool(service knowledgeDocumentDiagnoseReader) *DocumentIngestionDiagnoseTool {
	return &DocumentIngestionDiagnoseTool{service: service}
}

// SetTaskNodeReader sets an optional task node reader for deeper diagnosis.
func (t *DocumentIngestionDiagnoseTool) SetTaskNodeReader(svc ingestionTaskNodeReader) {
	if t == nil {
		return
	}
	t.taskNodeSvc = svc
}

func (t *DocumentIngestionDiagnoseTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "document_ingestion_diagnose",
		Description: "Diagnose a knowledge document's ingestion state and return conclusion, evidence, confidence, and next-step suggestions.",
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

func (t *DocumentIngestionDiagnoseTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "document_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: "document ingestion diagnose service is required"}, fmt.Errorf("document ingestion diagnose service is required")
	}

	documentID := strings.TrimSpace(readStringArg(call.Arguments, "documentId"))
	if documentID == "" {
		return ragtool.Result{Name: "document_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: "documentId is required"}, fmt.Errorf("documentId is required")
	}

	document, err := t.service.Get(ctx, knowledgeservice.GetKnowledgeDocumentInput{DocumentID: documentID})
	if err != nil {
		return ragtool.Result{Name: "document_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	chunkLogs, err := t.service.PageChunkLogs(ctx, knowledgeservice.KnowledgeDocumentChunkLogPageInput{
		DocumentID: documentID,
		Page:       1,
		PageSize:   3,
	})
	if err != nil {
		return ragtool.Result{Name: "document_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	conclusion, confidence, evidence, suggestions, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError :=
		diagnoseDocumentIngestion(ctx, document, chunkLogs, t.taskNodeSvc)
	confidence = normalizeDiagnosisConfidence(confidence)

	summary := fmt.Sprintf("document=%s confidence=%s conclusion=%s", documentID, confidence, conclusion)
	if latestLogStatus != "" {
		summary = fmt.Sprintf("%s latestLogStatus=%s", summary, latestLogStatus)
	}
	if latestTaskID != "" {
		summary = fmt.Sprintf("%s latestTask=%s", summary, latestTaskID)
	}
	if latestNodeID != "" {
		summary = fmt.Sprintf("%s node=%s", summary, latestNodeID)
	}
	if latestNodeError != "" {
		summary = fmt.Sprintf("%s nodeError=%s", summary, latestNodeError)
	} else if latestLogError != "" {
		summary = fmt.Sprintf("%s logError=%s", summary, latestLogError)
	}

	return ragtool.Result{
		Name:    "document_ingestion_diagnose",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: buildDiagnosisPayload("document_ingestion", conclusion, confidence, evidence, suggestions, map[string]any{
			"documentId":      document.ID,
			"documentName":    document.Name,
			"documentStatus":  document.Status,
			"processMode":     document.ProcessMode,
			"pipelineId":      document.PipelineID,
			"chunkCount":      document.ChunkCount,
			"latestTaskId":    latestTaskID,
			"latestNodeId":    latestNodeID,
			"latestNodeError": latestNodeError,
			"latestLogStatus": latestLogStatus,
			"latestLogError":  latestLogError,
			"chunkLogCount":   len(chunkLogs.Items),
		}),
	}, nil
}

func diagnoseDocumentIngestion(
	ctx context.Context,
	document knowledgedomain.KnowledgeDocument,
	pageResult knowledgeservice.KnowledgeDocumentChunkLogPageResult,
	taskNodeSvc ingestionTaskNodeReader,
) (conclusion string, confidence string, evidence []string, suggestions []string, latestTaskID string, latestNodeID string, latestNodeError string, latestLogStatus string, latestLogError string) {
	evidence = append(evidence, fmt.Sprintf("document.status=%s", strings.TrimSpace(document.Status)))
	evidence = append(evidence, fmt.Sprintf("document.processMode=%s", strings.TrimSpace(document.ProcessMode)))
	if strings.TrimSpace(document.PipelineID) != "" {
		evidence = append(evidence, fmt.Sprintf("document.pipelineId=%s", strings.TrimSpace(document.PipelineID)))
	}
	evidence = append(evidence, fmt.Sprintf("document.chunkCount=%d", document.ChunkCount))

	if document.ProcessMode != knowledgedomain.KnowledgeDocumentProcessModePipeline {
		return "document is not using pipeline ingestion mode", "high", evidence,
			[]string{"switch the document to processMode=pipeline before using ingestion diagnosis"}, "", "", "", "", ""
	}

	if len(pageResult.Items) == 0 {
		switch strings.TrimSpace(document.Status) {
		case knowledgedomain.KnowledgeDocumentStatusRunning:
			return "document is running but no chunk log has been recorded yet", "medium", evidence,
				[]string{"check whether pipeline task creation has started", "inspect knowledge document startPipelineDocumentTask path"}, "", "", "", "", ""
		case knowledgedomain.KnowledgeDocumentStatusFailed:
			return "document is failed but no recent chunk log is available", "low", evidence,
				[]string{"check whether chunk log creation or retention is missing", "inspect ingestion task creation and chunk log write-back flow"}, "", "", "", "", ""
		default:
			return "no ingestion activity found for the document yet", "medium", evidence,
				[]string{"start the document pipeline task and re-check chunk logs", "confirm the document has a valid pipelineId"}, "", "", "", "", ""
		}
	}

	latest := pageResult.Items[0]
	logItem := latest.Log
	latestTaskID = firstNonEmpty(strings.TrimSpace(latest.Log.ID), ingestionTaskID(latest))
	latestLogStatus = strings.TrimSpace(logItem.Status)
	latestLogError = strings.TrimSpace(logItem.ErrorMessage)

	evidence = append(evidence, fmt.Sprintf("latestChunkLog.status=%s", latestLogStatus))
	evidence = append(evidence, fmt.Sprintf("latestChunkLog.chunkCount=%d", logItem.ChunkCount))
	if logItem.TotalDuration > 0 {
		evidence = append(evidence, fmt.Sprintf("latestChunkLog.totalDurationMs=%d", logItem.TotalDuration))
	}
	if latestTaskID != "" {
		evidence = append(evidence, fmt.Sprintf("latestChunkLog.taskId=%s", latestTaskID))
	}
	if latestLogError != "" {
		evidence = append(evidence, fmt.Sprintf("latestChunkLog.error=%s", latestLogError))
	}

	taskStatus := ""
	if latest.IngestionTask != nil {
		taskStatus = strings.TrimSpace(latest.IngestionTask.Status)
		evidence = append(evidence, fmt.Sprintf("ingestionTask.status=%s", taskStatus))
		evidence = append(evidence, fmt.Sprintf("ingestionTask.chunkCount=%d", latest.IngestionTask.ChunkCount))
		if strings.TrimSpace(latest.IngestionTask.ErrorMessage) != "" {
			evidence = append(evidence, fmt.Sprintf("ingestionTask.error=%s", strings.TrimSpace(latest.IngestionTask.ErrorMessage)))
		}
	}

	nodeStats := summarizeIngestionNodes(latest.IngestionNodes)
	evidence = appendNodeStatsEvidence(evidence, nodeStats, "ingestionNodes")

	failedNodeID, failedNodeError := nodeStats.FailedNodeID, nodeStats.FailedError
	runningNodeID := nodeStats.RunningNodeID
	latestNodeID = failedNodeID
	latestNodeError = failedNodeError
	if latestNodeID == "" {
		latestNodeID = runningNodeID
	}
	if failedNodeID != "" {
		evidence = append(evidence, fmt.Sprintf("failedNode=%s", failedNodeID))
		if failedNodeError != "" {
			evidence = append(evidence, fmt.Sprintf("failedNode.error=%s", failedNodeError))
		}
		return fmt.Sprintf("document ingestion failed at node %s", failedNodeID), "high", evidence,
			diagnosisSuggestionsForFailedNode(failedNodeID, failedNodeError), latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
	}

	if latestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusSuccess && logItem.ChunkCount == 0 {
		return "document ingestion finished but produced zero chunks", "medium", evidence,
			[]string{"check parser output and chunk strategy to confirm the document yielded usable text", "inspect whether extraction succeeded but chunk generation or filtering removed all content"}, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
	}

	if hasInconsistentIngestionState(taskStatus, latestLogStatus, strings.TrimSpace(document.Status)) {
		return "document, chunk log, and ingestion task states are inconsistent", "medium", evidence,
			[]string{"compare document status, latest chunk log status, and ingestion task status write-back order", "inspect retry or compensation flow for stale state overwrite"}, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
	}

	if latestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusFailed {
		// Enrich from live task nodes when chunk log node data is incomplete.
		if latestTaskID != "" && taskNodeSvc != nil {
			if liveNodes, err := taskNodeSvc.ListNodes(ctx, latestTaskID); err == nil {
				liveStats := summarizeIngestionNodes(liveNodes)
				if liveStats.FailedNodeID != "" {
					evidence = appendNodeStatsEvidence(evidence, liveStats, "liveTaskNodes")
					evidence = append(evidence, fmt.Sprintf("failedNode=%s", liveStats.FailedNodeID))
					if liveStats.FailedError != "" {
						evidence = append(evidence, fmt.Sprintf("failedNode.error=%s", liveStats.FailedError))
					}
					return fmt.Sprintf("document ingestion failed at node %s", liveStats.FailedNodeID), "high", evidence,
						diagnosisSuggestionsForFailedNode(liveStats.FailedNodeID, liveStats.FailedError),
						latestTaskID, liveStats.FailedNodeID, liveStats.FailedError, latestLogStatus, latestLogError
				}
			}
		}
		return "document ingestion failed, but no failed node was captured", "medium", evidence,
			[]string{"inspect ingestion task error and chunk log write-back path", "check whether task node failure details were persisted completely"}, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
	}

	if runningNodeID != "" || latestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusRunning {
		if runningNodeID != "" {
			evidence = append(evidence, fmt.Sprintf("runningNode=%s", runningNodeID))
		}
		return "document ingestion is still running", "high", evidence,
			[]string{"wait for the task to finish and re-check diagnosis", "inspect the running node progress and recent logs"}, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
	}

	if latestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusSuccess && strings.TrimSpace(document.Status) == knowledgedomain.KnowledgeDocumentStatusSuccess {
		return "document ingestion completed successfully", "high", evidence,
			[]string{"no immediate action needed", "if retrieval quality is poor, inspect chunk quality and indexing results next"}, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
	}

	return "document ingestion state is partially inconsistent and needs manual review", "low", evidence,
		[]string{"compare document status, chunk log status, and ingestion task status", "check whether write-back ordering or retry flow caused stale state"}, latestTaskID, latestNodeID, latestNodeError, latestLogStatus, latestLogError
}

func latestFailedNode(nodes []ingestiondomain.TaskNode) (string, string) {
	for _, node := range nodes {
		if strings.TrimSpace(node.Status) == ingestiondomain.TaskStatusFailed {
			return strings.TrimSpace(node.NodeID), strings.TrimSpace(node.ErrorMessage)
		}
	}
	return "", ""
}

func latestRunningNode(nodes []ingestiondomain.TaskNode) string {
	for _, node := range nodes {
		if strings.TrimSpace(node.Status) == ingestiondomain.TaskStatusRunning {
			return strings.TrimSpace(node.NodeID)
		}
	}
	return ""
}

func ingestionTaskID(item knowledgeservice.KnowledgeDocumentChunkLogItem) string {
	if item.IngestionTask == nil {
		return ""
	}
	return strings.TrimSpace(item.IngestionTask.ID)
}

func diagnosisSuggestionsForFailedNode(nodeID string, errorMessage string) []string {
	nodeID = strings.TrimSpace(nodeID)
	errorMessage = strings.TrimSpace(errorMessage)
	switch nodeID {
	case "fetcher":
		return []string{"check source accessibility and network connectivity", "verify source URL or file location is still valid"}
	case "parser":
		return []string{"check parser compatibility with the document type", "inspect parser error details and raw document content"}
	case "chunker":
		return []string{"review chunk strategy and chunk config", "check whether the parsed content is empty or malformed"}
	case "indexer":
		if strings.Contains(strings.ToLower(errorMessage), "connection refused") {
			return []string{"check vector store or indexing backend connectivity", "inspect indexer retry and compensation logs after the connection failure"}
		}
		return []string{"inspect indexer output and downstream persistence", "check vector store health, retry path, and compensation cleanup"}
	default:
		return []string{"inspect the failed node error details", "trace the task node output and related infrastructure logs"}
	}
}
