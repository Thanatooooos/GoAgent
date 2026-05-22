package system

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type DiagnosisResultView struct {
	Conclusion      string
	Confidence      string
	Facts           []string
	Inferences      []string
	RiskHints       []string
	NextActions     []string
	TraceID         string
	TaskID          string
	LatestTaskID    string
	LatestNodeID    string
	LatestNodeError string
}

func ViewDiagnosisResult(result ragcore.Result) (DiagnosisResultView, bool) {
	switch strings.TrimSpace(result.Name) {
	case "document_ingestion_diagnose", "task_ingestion_diagnose", "trace_retrieval_diagnose":
	default:
		return DiagnosisResultView{}, false
	}

	return DiagnosisResultView{
		Conclusion:      result.GetString("conclusion"),
		Confidence:      result.GetString("confidence"),
		Facts:           result.PreferStringSlice("facts", "evidence"),
		Inferences:      result.GetStringSlice("inferences"),
		RiskHints:       result.GetStringSlice("riskHints"),
		NextActions:     result.PreferStringSlice("nextActions", "suggestions"),
		TraceID:         result.GetString("traceId"),
		TaskID:          result.GetString("taskId"),
		LatestTaskID:    result.GetString("latestTaskId"),
		LatestNodeID:    result.GetString("latestNodeId"),
		LatestNodeError: result.GetString("latestNodeError"),
	}, true
}

type DocumentQueryResultView struct {
	DocumentID      string
	Name            string
	KnowledgeBaseID string
	Status          string
	Enabled         bool
	ProcessMode     string
	PipelineID      string
	ChunkCount      int
	SourceType      string
}

func ViewDocumentQueryResult(result ragcore.Result) (DocumentQueryResultView, bool) {
	if strings.TrimSpace(result.Name) != "document_query" {
		return DocumentQueryResultView{}, false
	}
	return DocumentQueryResultView{
		DocumentID:      result.GetString("documentId"),
		Name:            result.GetString("name"),
		KnowledgeBaseID: result.GetString("knowledgeBaseId"),
		Status:          result.GetString("status"),
		Enabled:         ragcore.ReadDataBool(result.Data, "enabled"),
		ProcessMode:     result.GetString("processMode"),
		PipelineID:      result.GetString("pipelineId"),
		ChunkCount:      result.GetInt("chunkCount"),
		SourceType:      result.GetString("sourceType"),
	}, true
}

type DocumentChunkLogItemView struct {
	LogID           string
	DocumentID      string
	Status          string
	ProcessMode     string
	ChunkStrategy   string
	PipelineID      string
	ChunkCount      int
	ErrorMessage    string
	IngestionTaskID string
	IngestionStatus string
	FailedNodeIDs   []string
	RunningNodeIDs  []string
}

type DocumentChunkLogQueryResultView struct {
	DocumentID      string
	Total           int
	Page            int
	PageSize        int
	LogCount        int
	LatestStatus    string
	LatestTaskID    string
	LatestError     string
	FailedLogCount  int
	RunningLogCount int
	ChunkLogs       []DocumentChunkLogItemView
}

func ViewDocumentChunkLogQueryResult(result ragcore.Result) (DocumentChunkLogQueryResultView, bool) {
	if strings.TrimSpace(result.Name) != "document_chunk_log_query" {
		return DocumentChunkLogQueryResultView{}, false
	}
	view := DocumentChunkLogQueryResultView{
		DocumentID:      result.GetString("documentId"),
		Total:           result.GetInt("total"),
		Page:            result.GetInt("page"),
		PageSize:        result.GetInt("pageSize"),
		LogCount:        result.GetInt("logCount"),
		LatestStatus:    result.GetString("latestStatus"),
		LatestTaskID:    result.GetString("latestTaskId"),
		LatestError:     result.GetString("latestError"),
		FailedLogCount:  result.GetInt("failedLogCount"),
		RunningLogCount: result.GetInt("runningLogCount"),
	}
	for _, item := range ragcore.ReadMapItems(result.Data["chunkLogs"]) {
		entry := DocumentChunkLogItemView{
			LogID:           ragcore.ReadDataString(item, "logId"),
			DocumentID:      ragcore.ReadDataString(item, "documentId"),
			Status:          ragcore.ReadDataString(item, "status"),
			ProcessMode:     ragcore.ReadDataString(item, "processMode"),
			ChunkStrategy:   ragcore.ReadDataString(item, "chunkStrategy"),
			PipelineID:      ragcore.ReadDataString(item, "pipelineId"),
			ChunkCount:      ragcore.ReadDataInt(item, "chunkCount"),
			ErrorMessage:    ragcore.ReadDataString(item, "errorMessage"),
			IngestionTaskID: ragcore.ReadDataString(item, "ingestionTaskId"),
			IngestionStatus: ragcore.ReadDataString(item, "ingestionStatus"),
			FailedNodeIDs:   ragcore.ReadDataStringSlice(item, "failedNodeIds"),
			RunningNodeIDs:  ragcore.ReadDataStringSlice(item, "runningNodeIds"),
		}
		if entry.LogID == "" && entry.DocumentID == "" && entry.IngestionTaskID == "" {
			continue
		}
		view.ChunkLogs = append(view.ChunkLogs, entry)
	}
	if view.LogCount == 0 {
		view.LogCount = len(view.ChunkLogs)
	}
	return view, true
}

type DocumentListItemView struct {
	DocumentID      string
	Name            string
	Status          string
	ProcessMode     string
	KnowledgeBaseID string
	ChunkCount      int
}

type DocumentListResultView struct {
	Total           int
	FailedCount     int
	RunningCount    int
	KnowledgeBaseID string
	Query           string
	Status          string
	Items           []DocumentListItemView
}

func ViewDocumentListResult(result ragcore.Result) (DocumentListResultView, bool) {
	if strings.TrimSpace(result.Name) != "document_list" {
		return DocumentListResultView{}, false
	}
	view := DocumentListResultView{
		Total:           result.GetInt("total"),
		FailedCount:     result.GetInt("failedCount"),
		RunningCount:    result.GetInt("runningCount"),
		KnowledgeBaseID: result.GetString("knowledgeBaseId"),
		Query:           result.GetString("query"),
		Status:          result.GetString("status"),
	}
	for _, item := range ragcore.ReadMapItems(result.Data["items"]) {
		entry := DocumentListItemView{
			DocumentID:      ragcore.ReadDataString(item, "documentId"),
			Name:            ragcore.ReadDataString(item, "name"),
			Status:          ragcore.ReadDataString(item, "status"),
			ProcessMode:     ragcore.ReadDataString(item, "processMode"),
			KnowledgeBaseID: ragcore.ReadDataString(item, "knowledgeBaseId"),
			ChunkCount:      ragcore.ReadDataInt(item, "chunkCount"),
		}
		if entry.DocumentID == "" && entry.Name == "" {
			continue
		}
		view.Items = append(view.Items, entry)
	}
	if view.Total == 0 && len(view.Items) > 0 {
		view.Total = len(view.Items)
	}
	return view, true
}

type IngestionTaskNodeSummaryView struct {
	NodeID   string
	NodeType string
	Status   string
}

type IngestionTaskQueryResultView struct {
	TaskID          string
	PipelineID      string
	Status          string
	SourceType      string
	SourceLocation  string
	SourceFileName  string
	ErrorMessage    string
	ChunkCount      int
	TaskNodeCount   int
	TaskNodeSummary []IngestionTaskNodeSummaryView
}

func ViewIngestionTaskQueryResult(result ragcore.Result) (IngestionTaskQueryResultView, bool) {
	if strings.TrimSpace(result.Name) != "ingestion_task_query" {
		return IngestionTaskQueryResultView{}, false
	}
	view := IngestionTaskQueryResultView{
		TaskID:         result.GetString("taskId"),
		PipelineID:     result.GetString("pipelineId"),
		Status:         result.GetString("status"),
		SourceType:     result.GetString("sourceType"),
		SourceLocation: result.GetString("sourceLocation"),
		SourceFileName: result.GetString("sourceFileName"),
		ErrorMessage:   result.GetString("errorMessage"),
		ChunkCount:     result.GetInt("chunkCount"),
		TaskNodeCount:  result.GetInt("taskNodeCount"),
	}
	for _, item := range ragcore.ReadMapItems(result.Data["taskNodeSummary"]) {
		entry := IngestionTaskNodeSummaryView{
			NodeID:   ragcore.ReadDataString(item, "nodeId"),
			NodeType: ragcore.ReadDataString(item, "nodeType"),
			Status:   ragcore.ReadDataString(item, "status"),
		}
		if entry.NodeID == "" {
			continue
		}
		view.TaskNodeSummary = append(view.TaskNodeSummary, entry)
	}
	if view.TaskNodeCount == 0 {
		view.TaskNodeCount = len(view.TaskNodeSummary)
	}
	return view, true
}

func (v IngestionTaskQueryResultView) LatestInterestingNode() (IngestionTaskNodeSummaryView, bool) {
	for _, node := range v.TaskNodeSummary {
		status := strings.ToLower(strings.TrimSpace(node.Status))
		if strings.TrimSpace(node.NodeID) == "" {
			continue
		}
		if status == "failed" || status == "running" {
			return node, true
		}
	}
	return IngestionTaskNodeSummaryView{}, false
}

type IngestionTaskNodeItemView struct {
	NodeID       string
	NodeType     string
	NodeOrder    int
	Status       string
	DurationMs   int
	Message      string
	ErrorMessage string
}

type IngestionTaskNodeQueryResultView struct {
	TaskID       string
	NodeID       string
	NodeType     string
	NodeOrder    int
	Status       string
	DurationMs   int
	Message      string
	ErrorMessage string
	NodeCount    int
	Nodes        []IngestionTaskNodeItemView
}

func ViewIngestionTaskNodeQueryResult(result ragcore.Result) (IngestionTaskNodeQueryResultView, bool) {
	if strings.TrimSpace(result.Name) != "ingestion_task_node_query" {
		return IngestionTaskNodeQueryResultView{}, false
	}
	view := IngestionTaskNodeQueryResultView{
		TaskID:       result.GetString("taskId"),
		NodeID:       result.GetString("nodeId"),
		NodeType:     result.GetString("nodeType"),
		NodeOrder:    result.GetInt("nodeOrder"),
		Status:       result.GetString("status"),
		DurationMs:   result.GetInt("durationMs"),
		Message:      result.GetString("message"),
		ErrorMessage: result.GetString("errorMessage"),
		NodeCount:    result.GetInt("nodeCount"),
	}
	for _, item := range ragcore.ReadMapItems(result.Data["nodes"]) {
		entry := IngestionTaskNodeItemView{
			NodeID:       ragcore.ReadDataString(item, "nodeId"),
			NodeType:     ragcore.ReadDataString(item, "nodeType"),
			NodeOrder:    ragcore.ReadDataInt(item, "nodeOrder"),
			Status:       ragcore.ReadDataString(item, "status"),
			DurationMs:   ragcore.ReadDataInt(item, "durationMs"),
			ErrorMessage: ragcore.ReadDataString(item, "errorMessage"),
		}
		if entry.NodeID == "" {
			continue
		}
		view.Nodes = append(view.Nodes, entry)
	}
	if view.NodeCount == 0 && len(view.Nodes) > 0 {
		view.NodeCount = len(view.Nodes)
	}
	return view, true
}

type TaskListItemView struct {
	TaskID         string
	PipelineID     string
	Status         string
	SourceFileName string
	ChunkCount     int
	ErrorMessage   string
}

type TaskListResultView struct {
	Total        int
	FailedCount  int
	RunningCount int
	PipelineID   string
	Status       string
	Items        []TaskListItemView
}

func ViewTaskListResult(result ragcore.Result) (TaskListResultView, bool) {
	if strings.TrimSpace(result.Name) != "task_list" {
		return TaskListResultView{}, false
	}
	view := TaskListResultView{
		Total:        result.GetInt("total"),
		FailedCount:  result.GetInt("failedCount"),
		RunningCount: result.GetInt("runningCount"),
		PipelineID:   result.GetString("pipelineId"),
		Status:       result.GetString("status"),
	}
	for _, item := range ragcore.ReadMapItems(result.Data["items"]) {
		entry := TaskListItemView{
			TaskID:         ragcore.ReadDataString(item, "taskId"),
			PipelineID:     ragcore.ReadDataString(item, "pipelineId"),
			Status:         ragcore.ReadDataString(item, "status"),
			SourceFileName: ragcore.ReadDataString(item, "sourceFileName"),
			ChunkCount:     ragcore.ReadDataInt(item, "chunkCount"),
			ErrorMessage:   ragcore.ReadDataString(item, "errorMessage"),
		}
		if entry.TaskID == "" {
			continue
		}
		view.Items = append(view.Items, entry)
	}
	if view.Total == 0 && len(view.Items) > 0 {
		view.Total = len(view.Items)
	}
	return view, true
}
