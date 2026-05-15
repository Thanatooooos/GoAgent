package builtin

import (
	"context"
	"fmt"
	"strings"

	ingestionservice "local/rag-project/internal/app/ingestion/service"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type ingestionTaskPager interface {
	Page(ctx context.Context, input ingestionservice.PageTasksInput) (ingestionservice.TaskPageResult, error)
}

type TaskListTool struct {
	service ingestionTaskPager
}

func NewTaskListTool(service ingestionTaskPager) *TaskListTool {
	return &TaskListTool{service: service}
}

func (t *TaskListTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "task_list",
		Description: "List ingestion tasks, optionally filtered by status or pipeline. Use this to discover tasks that failed, are running, or belong to a specific pipeline — especially when the user asks open-ended questions like 'which tasks failed recently?' without providing specific task IDs.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "status",
				Type:        ragtool.ParamTypeString,
				Description: "Filter by task status. Common values: failed, running, success, pending. Leave empty to list all.",
				Required:    false,
			},
			{
				Name:        "pipelineId",
				Type:        ragtool.ParamTypeString,
				Description: "Filter by pipeline id. Optional.",
				Required:    false,
			},
		},
	}
}

func (t *TaskListTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.service == nil {
		return ragtool.Result{Name: "task_list", Status: ragtool.CallStatusFailed, ErrorMessage: "task list service is required"}, nil
	}

	status := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "status"))
	pipelineID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "pipelineId"))

	pageInput := ingestionservice.PageTasksInput{
		Page:       1,
		PageSize:   20,
		PipelineID: pipelineID,
		Status:     status,
	}

	result, err := t.service.Page(ctx, pageInput)
	if err != nil {
		return ragtool.Result{Name: "task_list", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, nil
	}

	if result.Total == 0 {
		return ragtool.Result{
			Name:    "task_list",
			Status:  ragtool.CallStatusSuccess,
			Summary: fmt.Sprintf("no tasks found (status=%q pipelineId=%q)", status, pipelineID),
			Data: map[string]any{
				"total": 0,
				"items": []map[string]any{},
			},
		}, nil
	}

	items := make([]map[string]any, 0, len(result.Items))
	failedCount := 0
	runningCount := 0
	for _, task := range result.Items {
		item := map[string]any{
			"taskId":         task.ID,
			"pipelineId":     task.PipelineID,
			"status":         task.Status,
			"sourceFileName": task.SourceFileName,
			"chunkCount":     task.ChunkCount,
		}
		if task.ErrorMessage != "" {
			item["errorMessage"] = task.ErrorMessage
		}
		items = append(items, item)

		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case "failed":
			failedCount++
		case "running":
			runningCount++
		}
	}

	summary := fmt.Sprintf("found %d tasks (total=%d)", len(items), result.Total)
	if status == "" {
		summary = fmt.Sprintf("%s, failed=%d running=%d", summary, failedCount, runningCount)
	}

	data := map[string]any{
		"total":        result.Total,
		"items":        items,
		"failedCount":  failedCount,
		"runningCount": runningCount,
	}
	if pipelineID != "" {
		data["pipelineId"] = pipelineID
	}
	if status != "" {
		data["status"] = status
	}

	return ragtool.Result{
		Name:    "task_list",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data:    data,
	}, nil
}

// Ensure TaskListTool implements Tool.
var _ ragtool.Tool = (*TaskListTool)(nil)
