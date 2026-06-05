package document_investigation

import (
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
)

func diagnose(document knowledgedomain.KnowledgeDocument, pageResult knowledgeservice.KnowledgeDocumentChunkLogPageResult) CapabilityOutput {
	output := CapabilityOutput{
		DocumentID:     strings.TrimSpace(document.ID),
		DocumentName:   strings.TrimSpace(document.Name),
		DocumentStatus: strings.TrimSpace(document.Status),
		ProcessMode:    strings.TrimSpace(document.ProcessMode),
		PipelineID:     strings.TrimSpace(document.PipelineID),
		ChunkCount:     document.ChunkCount,
	}
	output.Evidence = append(output.Evidence,
		fmt.Sprintf("document.status=%s", output.DocumentStatus),
		fmt.Sprintf("document.processMode=%s", output.ProcessMode),
		fmt.Sprintf("document.chunkCount=%d", output.ChunkCount),
	)
	if output.PipelineID != "" {
		output.Evidence = append(output.Evidence, fmt.Sprintf("document.pipelineId=%s", output.PipelineID))
	}

	if document.ProcessMode != knowledgedomain.KnowledgeDocumentProcessModePipeline {
		output.Conclusion = "document is not using pipeline ingestion mode"
		output.Confidence = "high"
		output.Suggestions = []string{"switch the document to processMode=pipeline before using ingestion diagnosis"}
		return output
	}

	if len(pageResult.Items) == 0 {
		output.Confidence = "medium"
		switch output.DocumentStatus {
		case knowledgedomain.KnowledgeDocumentStatusRunning:
			output.Conclusion = "document is running but no chunk log has been recorded yet"
			output.Suggestions = []string{"check whether pipeline task creation has started"}
		case knowledgedomain.KnowledgeDocumentStatusFailed:
			output.Conclusion = "document is failed but no recent chunk log is available"
			output.Confidence = "low"
			output.Suggestions = []string{"inspect ingestion task creation and chunk-log write-back flow"}
		default:
			output.Conclusion = "no ingestion activity found for the document yet"
			output.Suggestions = []string{"start the document pipeline task and re-check chunk logs"}
		}
		return output
	}

	latest := pageResult.Items[0]
	output.LatestTaskID = strings.TrimSpace(taskID(latest))
	output.LatestLogStatus = strings.TrimSpace(latest.Log.Status)
	output.LatestLogError = strings.TrimSpace(latest.Log.ErrorMessage)
	output.Evidence = append(output.Evidence,
		fmt.Sprintf("latestChunkLog.status=%s", output.LatestLogStatus),
		fmt.Sprintf("latestChunkLog.chunkCount=%d", latest.Log.ChunkCount),
	)
	if output.LatestTaskID != "" {
		output.Evidence = append(output.Evidence, fmt.Sprintf("latestChunkLog.taskId=%s", output.LatestTaskID))
	}
	if output.LatestLogError != "" {
		output.Evidence = append(output.Evidence, fmt.Sprintf("latestChunkLog.error=%s", output.LatestLogError))
	}

	failedNodeID, failedNodeError := latestFailedNode(latest.IngestionNodes)
	runningNodeID := latestRunningNode(latest.IngestionNodes)
	output.LatestNodeID = failedNodeID
	output.LatestNodeError = failedNodeError
	if output.LatestNodeID == "" {
		output.LatestNodeID = runningNodeID
	}
	if failedNodeID != "" {
		output.Conclusion = fmt.Sprintf("document ingestion failed at node %s", failedNodeID)
		output.Confidence = "high"
		output.Suggestions = suggestionsForFailedNode(failedNodeID, failedNodeError)
		if failedNodeError != "" {
			output.Evidence = append(output.Evidence, fmt.Sprintf("failedNode.error=%s", failedNodeError))
		}
		return output
	}

	if output.LatestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusRunning || runningNodeID != "" {
		output.Conclusion = "document ingestion is still running"
		output.Confidence = "high"
		output.Suggestions = []string{"wait for the task to finish and re-check diagnosis"}
		if runningNodeID != "" {
			output.Evidence = append(output.Evidence, fmt.Sprintf("runningNode=%s", runningNodeID))
		}
		return output
	}

	if output.LatestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusSuccess &&
		output.DocumentStatus == knowledgedomain.KnowledgeDocumentStatusSuccess {
		output.Conclusion = "document ingestion completed successfully"
		output.Confidence = "high"
		output.Suggestions = []string{"no immediate action needed"}
		return output
	}

	if output.LatestLogStatus == knowledgedomain.KnowledgeDocumentChunkLogStatusFailed {
		output.Conclusion = "document ingestion failed, but no failed node was captured"
		output.Confidence = "medium"
		output.Suggestions = []string{"inspect ingestion task error and chunk-log write-back path"}
		return output
	}

	output.Conclusion = "document ingestion state is partially inconsistent and needs manual review"
	output.Confidence = "low"
	output.Suggestions = []string{"compare document status, chunk-log status, and ingestion task status"}
	return output
}

func taskID(item knowledgeservice.KnowledgeDocumentChunkLogItem) string {
	if item.IngestionTask == nil {
		return ""
	}
	return item.IngestionTask.ID
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

func suggestionsForFailedNode(nodeID string, errorMessage string) []string {
	nodeID = strings.TrimSpace(nodeID)
	errorMessage = strings.TrimSpace(errorMessage)
	switch nodeID {
	case "fetcher":
		return []string{"check source accessibility and network connectivity"}
	case "parser":
		return []string{"check parser compatibility with the document type"}
	case "chunker":
		return []string{"review chunk strategy and chunk config"}
	case "indexer":
		if strings.Contains(strings.ToLower(errorMessage), "connection refused") {
			return []string{"check vector store or indexing backend connectivity"}
		}
		return []string{"inspect indexer output and downstream persistence"}
	default:
		return []string{"inspect the failed node error details"}
	}
}
