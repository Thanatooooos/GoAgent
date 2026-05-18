package ingestion

import (
	"fmt"

	"local/rag-project/internal/adapter/repository/postgres/ingestion/models"
	"local/rag-project/internal/app/ingestion/domain"
)

func toPipelineModel(item domain.Pipeline) (models.PipelineModel, error) {
	definitionJSON, err := marshalPipelineDefinition(item.Definition, item.Nodes)
	if err != nil {
		return models.PipelineModel{}, err
	}
	return models.PipelineModel{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		NodesJSON:   definitionJSON,
		CreatedBy:   item.CreatedBy,
		UpdatedBy:   item.UpdatedBy,
		CreateTime:  item.CreatedAt,
		UpdateTime:  item.UpdatedAt,
	}, nil
}

func toPipelineDomain(item models.PipelineModel) (domain.Pipeline, error) {
	definition, err := unmarshalPipelineDefinition(item.NodesJSON)
	if err != nil {
		return domain.Pipeline{}, err
	}
	return domain.Pipeline{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		Definition:  definition,
		Nodes:       domain.ClonePipelineNodes(definition.Nodes),
		CreatedBy:   item.CreatedBy,
		UpdatedBy:   item.UpdatedBy,
		CreatedAt:   item.CreateTime,
		UpdatedAt:   item.UpdateTime,
	}, nil
}

func toTaskModel(item domain.Task) (models.TaskModel, error) {
	metadata, err := marshalMap(item.Metadata, "task metadata")
	if err != nil {
		return models.TaskModel{}, err
	}
	return models.TaskModel{
		ID:             item.ID,
		PipelineID:     item.PipelineID,
		SourceType:     item.SourceType,
		SourceLocation: item.SourceLocation,
		SourceFileName: item.SourceFileName,
		Status:         item.Status,
		ChunkCount:     item.ChunkCount,
		ErrorMessage:   item.ErrorMessage,
		Metadata:       metadata,
		StartedAt:      item.StartedAt,
		CompletedAt:    item.CompletedAt,
		CreatedBy:      item.CreatedBy,
		UpdatedBy:      item.UpdatedBy,
		CreateTime:     item.CreatedAt,
		UpdateTime:     item.UpdatedAt,
	}, nil
}

func toTaskDomain(item models.TaskModel) (domain.Task, error) {
	metadata, err := unmarshalMap(item.Metadata, "task metadata")
	if err != nil {
		return domain.Task{}, err
	}
	return domain.Task{
		ID:             item.ID,
		PipelineID:     item.PipelineID,
		SourceType:     item.SourceType,
		SourceLocation: item.SourceLocation,
		SourceFileName: item.SourceFileName,
		Status:         item.Status,
		ChunkCount:     item.ChunkCount,
		ErrorMessage:   item.ErrorMessage,
		Metadata:       metadata,
		StartedAt:      item.StartedAt,
		CompletedAt:    item.CompletedAt,
		CreatedBy:      item.CreatedBy,
		UpdatedBy:      item.UpdatedBy,
		CreatedAt:      item.CreateTime,
		UpdatedAt:      item.UpdateTime,
	}, nil
}

func toTaskNodeModel(item domain.TaskNode) (models.TaskNodeModel, error) {
	output, err := marshalMap(item.Output, "task node output")
	if err != nil {
		return models.TaskNodeModel{}, err
	}
	return models.TaskNodeModel{
		ID:           item.ID,
		TaskID:       item.TaskID,
		PipelineID:   item.PipelineID,
		NodeID:       item.NodeID,
		NodeType:     item.NodeType,
		NodeOrder:    item.NodeOrder,
		Status:       item.Status,
		DurationMs:   item.DurationMs,
		Message:      item.Message,
		ErrorMessage: item.ErrorMessage,
		Output:       output,
		CreateTime:   item.CreatedAt,
		UpdateTime:   item.UpdatedAt,
	}, nil
}

func toTaskNodeDomain(item models.TaskNodeModel) (domain.TaskNode, error) {
	output, err := unmarshalMap(item.Output, "task node output")
	if err != nil {
		return domain.TaskNode{}, err
	}
	return domain.TaskNode{
		ID:           item.ID,
		TaskID:       item.TaskID,
		PipelineID:   item.PipelineID,
		NodeID:       item.NodeID,
		NodeType:     item.NodeType,
		NodeOrder:    item.NodeOrder,
		Status:       item.Status,
		DurationMs:   item.DurationMs,
		Message:      item.Message,
		ErrorMessage: item.ErrorMessage,
		Output:       output,
		CreatedAt:    item.CreateTime,
		UpdatedAt:    item.UpdateTime,
	}, nil
}

func mustToPipelineDomains(items []models.PipelineModel) ([]domain.Pipeline, error) {
	result := make([]domain.Pipeline, 0, len(items))
	for _, item := range items {
		mapped, err := toPipelineDomain(item)
		if err != nil {
			return nil, fmt.Errorf("map ingestion pipeline domain: %w", err)
		}
		result = append(result, mapped)
	}
	return result, nil
}

func mustToTaskDomains(items []models.TaskModel) ([]domain.Task, error) {
	result := make([]domain.Task, 0, len(items))
	for _, item := range items {
		mapped, err := toTaskDomain(item)
		if err != nil {
			return nil, fmt.Errorf("map ingestion task domain: %w", err)
		}
		result = append(result, mapped)
	}
	return result, nil
}

func mustToTaskNodeDomains(items []models.TaskNodeModel) ([]domain.TaskNode, error) {
	result := make([]domain.TaskNode, 0, len(items))
	for _, item := range items {
		mapped, err := toTaskNodeDomain(item)
		if err != nil {
			return nil, fmt.Errorf("map ingestion task node domain: %w", err)
		}
		result = append(result, mapped)
	}
	return result, nil
}
