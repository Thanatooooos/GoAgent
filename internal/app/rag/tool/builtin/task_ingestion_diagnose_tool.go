package builtin

import (
	"context"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type ingestionTaskDiagnoseReader interface {
	Get(ctx context.Context, id string) (ingestiondomain.Task, error)
	ListNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error)
}

type TaskIngestionDiagnoseTool struct {
	service ingestionTaskDiagnoseReader
}

func NewTaskIngestionDiagnoseTool(service ingestionTaskDiagnoseReader) *TaskIngestionDiagnoseTool {
	return &TaskIngestionDiagnoseTool{service: service}
}

func (t *TaskIngestionDiagnoseTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "task_ingestion_diagnose",
		Description: "Diagnose an ingestion task and return conclusion, evidence, confidence, and next-step suggestions.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "taskId",
				Type:        ragtool.ParamTypeString,
				Description: "Ingestion task id.",
				Required:    true,
			},
		},
	}
}

func (t *TaskIngestionDiagnoseTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "task_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: "task ingestion diagnose service is required"}, fmt.Errorf("task ingestion diagnose service is required")
	}

	taskID := strings.TrimSpace(readStringArg(call.Arguments, "taskId"))
	if taskID == "" {
		return ragtool.Result{Name: "task_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: "taskId is required"}, fmt.Errorf("taskId is required")
	}

	task, err := t.service.Get(ctx, taskID)
	if err != nil {
		return ragtool.Result{Name: "task_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	nodes, err := t.service.ListNodes(ctx, taskID)
	if err != nil {
		return ragtool.Result{Name: "task_ingestion_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	conclusion, confidence, evidence, suggestions, latestNodeID, latestNodeError := diagnoseTaskIngestion(task, nodes)

	summary := fmt.Sprintf("task=%s confidence=%s conclusion=%s", taskID, confidence, conclusion)
	if latestNodeID != "" {
		summary = fmt.Sprintf("%s node=%s", summary, latestNodeID)
	}
	if latestNodeError != "" {
		summary = fmt.Sprintf("%s nodeError=%s", summary, latestNodeError)
	} else if strings.TrimSpace(task.ErrorMessage) != "" {
		summary = fmt.Sprintf("%s taskError=%s", summary, strings.TrimSpace(task.ErrorMessage))
	}

	return ragtool.Result{
		Name:    "task_ingestion_diagnose",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: map[string]any{
			"taskId":          task.ID,
			"pipelineId":      task.PipelineID,
			"taskStatus":      task.Status,
			"sourceType":      task.SourceType,
			"chunkCount":      task.ChunkCount,
			"errorMessage":    task.ErrorMessage,
			"conclusion":      conclusion,
			"confidence":      confidence,
			"evidence":        evidence,
			"suggestions":     suggestions,
			"latestNodeId":    latestNodeID,
			"latestNodeError": latestNodeError,
			"nodeCount":       len(nodes),
		},
	}, nil
}

func diagnoseTaskIngestion(
	task ingestiondomain.Task,
	nodes []ingestiondomain.TaskNode,
) (conclusion string, confidence string, evidence []string, suggestions []string, latestNodeID string, latestNodeError string) {
	evidence = append(evidence, fmt.Sprintf("task.status=%s", strings.TrimSpace(task.Status)))
	evidence = append(evidence, fmt.Sprintf("task.pipelineId=%s", strings.TrimSpace(task.PipelineID)))
	evidence = append(evidence, fmt.Sprintf("task.sourceType=%s", strings.TrimSpace(task.SourceType)))
	evidence = append(evidence, fmt.Sprintf("task.chunkCount=%d", task.ChunkCount))
	if strings.TrimSpace(task.ErrorMessage) != "" {
		evidence = append(evidence, fmt.Sprintf("task.error=%s", strings.TrimSpace(task.ErrorMessage)))
	}
	evidence = append(evidence, fmt.Sprintf("task.nodeCount=%d", len(nodes)))

	nodeStats := summarizeIngestionNodes(nodes)
	evidence = appendNodeStatsEvidence(evidence, nodeStats, "taskNodes")

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
		return fmt.Sprintf("ingestion task failed at node %s", failedNodeID), "high", evidence, diagnosisSuggestionsForFailedNode(failedNodeID, failedNodeError), latestNodeID, latestNodeError
	}

	if strings.TrimSpace(task.Status) == ingestiondomain.TaskStatusSuccess && task.ChunkCount == 0 {
		return "ingestion task completed successfully but produced zero chunks", "medium", evidence,
			[]string{"inspect parser output and chunker configuration", "confirm the input document produced usable text before indexing"}, latestNodeID, latestNodeError
	}

	if strings.TrimSpace(task.Status) == ingestiondomain.TaskStatusSuccess && nodeStats.RunningCount > 0 {
		return "task status is success, but some nodes still appear running", "medium", evidence,
			[]string{"inspect task/node state write-back ordering", "check whether late node updates were persisted after task completion"}, latestNodeID, latestNodeError
	}

	if strings.TrimSpace(task.Status) == ingestiondomain.TaskStatusFailed && nodeStats.FailedCount == 0 && nodeStats.RunningCount > 0 {
		return "task is marked failed, but node records only show running state", "medium", evidence,
			[]string{"inspect executor cancellation and node final-state persistence", "check whether the failing node error was lost during retry or shutdown"}, latestNodeID, latestNodeError
	}

	if strings.TrimSpace(task.Status) == ingestiondomain.TaskStatusFailed {
		return "ingestion task failed, but no failed node was captured", "medium", evidence,
			[]string{"inspect task-level error and node persistence flow", "check whether task node failure details were persisted completely"}, latestNodeID, latestNodeError
	}

	if runningNodeID != "" || strings.TrimSpace(task.Status) == ingestiondomain.TaskStatusRunning {
		if runningNodeID != "" {
			evidence = append(evidence, fmt.Sprintf("runningNode=%s", runningNodeID))
		}
		return "ingestion task is still running", "high", evidence,
			[]string{"wait for task completion and re-run diagnosis", "inspect the running node progress and recent logs"}, latestNodeID, latestNodeError
	}

	if strings.TrimSpace(task.Status) == ingestiondomain.TaskStatusSuccess {
		return "ingestion task completed successfully", "high", evidence,
			[]string{"no immediate action needed", "if document state is still inconsistent, compare task result with document and chunk log write-back"}, latestNodeID, latestNodeError
	}

	if len(nodes) == 0 {
		return "task exists but no node execution records were found", "medium", evidence,
			[]string{"check whether executor started the task", "inspect task dispatch and node persistence flow"}, latestNodeID, latestNodeError
	}

	return "ingestion task state is partially inconsistent and needs manual review", "low", evidence,
		[]string{"compare task status with node statuses", "check retry flow and task state write-back ordering"}, latestNodeID, latestNodeError
}
