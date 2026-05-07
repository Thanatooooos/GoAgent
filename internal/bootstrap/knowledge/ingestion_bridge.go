package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	"local/rag-project/internal/framework/exception"
)

type ingestionTaskCreatorAdapter struct {
	taskService *ingestionservice.TaskService
}

func NewIngestionTaskCreator(taskService *ingestionservice.TaskService) knowledgeservice.IngestionTaskCreator {
	return &ingestionTaskCreatorAdapter{taskService: taskService}
}

func NewIngestionTaskReader(taskService *ingestionservice.TaskService) knowledgeservice.IngestionTaskReader {
	return &ingestionTaskCreatorAdapter{taskService: taskService}
}

func (a *ingestionTaskCreatorAdapter) CreateKnowledgePipelineTask(ctx context.Context, input knowledgeservice.CreateKnowledgePipelineTaskInput) (string, error) {
	if a == nil || a.taskService == nil {
		return "", exception.NewServiceException("ingestion task service is required", nil)
	}
	item, err := a.taskService.Create(ctx, ingestionservice.CreateTaskInput{
		ID:             strings.TrimSpace(input.TaskID),
		PipelineID:     strings.TrimSpace(input.PipelineID),
		SourceType:     strings.TrimSpace(input.SourceType),
		SourceLocation: strings.TrimSpace(input.SourceLocation),
		SourceFileName: strings.TrimSpace(input.SourceFileName),
		Metadata: map[string]any{
			"documentId":      strings.TrimSpace(input.DocumentID),
			"knowledgeBaseId": strings.TrimSpace(input.KnowledgeBaseID),
			"documentName":    strings.TrimSpace(input.DocumentName),
		},
		CreatedBy: strings.TrimSpace(input.OperatorID),
	})
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

func (a *ingestionTaskCreatorAdapter) GetKnowledgePipelineTask(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
	if a == nil || a.taskService == nil {
		return ingestiondomain.Task{}, exception.NewServiceException("ingestion task service is required", nil)
	}
	return a.taskService.Get(ctx, strings.TrimSpace(taskID))
}

func (a *ingestionTaskCreatorAdapter) ListKnowledgePipelineTaskNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error) {
	if a == nil || a.taskService == nil {
		return nil, exception.NewServiceException("ingestion task service is required", nil)
	}
	return a.taskService.ListNodes(ctx, strings.TrimSpace(taskID))
}

type ingestionKnowledgeTaskObserver struct {
	documentService *knowledgeservice.KnowledgeDocumentService
}

func NewIngestionTaskObserver(documentService *knowledgeservice.KnowledgeDocumentService) ingestionservice.TaskObserver {
	return &ingestionKnowledgeTaskObserver{documentService: documentService}
}

func (o *ingestionKnowledgeTaskObserver) OnTaskStarted(ctx context.Context, task ingestiondomain.Task) error {
	return nil
}

func (o *ingestionKnowledgeTaskObserver) OnTaskCompleted(ctx context.Context, task ingestiondomain.Task, state ingestionservice.ExecutionState, execErr error) error {
	if o == nil || o.documentService == nil {
		return nil
	}
	documentID := strings.TrimSpace(readTaskMetadataString(task.Metadata, "documentId"))
	if documentID == "" {
		return nil
	}
	errorMessage := ""
	if execErr != nil {
		errorMessage = execErr.Error()
	}
	return o.documentService.OnIngestionTaskCompleted(ctx, knowledgeservice.KnowledgeDocumentIngestionTaskCompletedInput{
		TaskID:       task.ID,
		DocumentID:   documentID,
		PipelineID:   task.PipelineID,
		ChunkCount:   len(state.Chunks),
		StartedAt:    task.StartedAt,
		CompletedAt:  task.CompletedAt,
		OperatorID:   pickTaskOperator(task),
		ErrorMessage: errorMessage,
	})
}

func (o *ingestionKnowledgeTaskObserver) OnNodeStarted(ctx context.Context, task ingestiondomain.Task, node ingestionservice.WorkflowNodeSpec) error {
	return nil
}

func (o *ingestionKnowledgeTaskObserver) OnNodeRetry(ctx context.Context, task ingestiondomain.Task, node ingestionservice.WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error {
	return nil
}

func (o *ingestionKnowledgeTaskObserver) OnNodeCompleted(
	ctx context.Context,
	task ingestiondomain.Task,
	node ingestionservice.WorkflowNodeSpec,
	output map[string]any,
	duration time.Duration,
	execErr error,
) error {
	return nil
}

func readTaskMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func pickTaskOperator(task ingestiondomain.Task) string {
	if strings.TrimSpace(task.UpdatedBy) != "" {
		return strings.TrimSpace(task.UpdatedBy)
	}
	return strings.TrimSpace(task.CreatedBy)
}
