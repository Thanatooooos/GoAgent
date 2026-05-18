package ingestion

import (
	"encoding/json"
	"fmt"
	"strings"

	postgrescommon "local/rag-project/internal/adapter/repository/postgres/common"
	"local/rag-project/internal/app/ingestion/domain"
)

func conditionalUpdateRequiresConditions(entity string) error {
	return postgrescommon.ConditionalUpdateRequiresConditions(entity)
}

// marshalPipelineDefinition encodes the normalized pipeline definition as json.
func marshalPipelineDefinition(definition domain.PipelineDefinition, nodes []domain.PipelineNode) ([]byte, error) {
	normalized := domain.NormalizePipelineDefinition(definition, nodes)
	value, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal ingestion pipeline definition: %w", err)
	}
	return value, nil
}

// unmarshalPipelineDefinition decodes either the v1 graph definition or the legacy node array.
func unmarshalPipelineDefinition(value []byte) (domain.PipelineDefinition, error) {
	if len(value) == 0 {
		return domain.PipelineDefinition{}, nil
	}
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		return domain.PipelineDefinition{}, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var nodes []domain.PipelineNode
		if err := json.Unmarshal(value, &nodes); err != nil {
			return domain.PipelineDefinition{}, fmt.Errorf("unmarshal legacy ingestion pipeline nodes: %w", err)
		}
		return domain.NormalizePipelineDefinition(domain.PipelineDefinition{}, nodes), nil
	}
	var result domain.PipelineDefinition
	if err := json.Unmarshal(value, &result); err != nil {
		return domain.PipelineDefinition{}, fmt.Errorf("unmarshal ingestion pipeline definition: %w", err)
	}
	return domain.NormalizePipelineDefinition(result, result.Nodes), nil
}

// marshalMap 编码 map 字段。
func marshalMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal ingestion %s: %w", field, err)
	}
	return data, nil
}

// unmarshalMap 解码 map 字段。
func unmarshalMap(value []byte, field string) (map[string]any, error) {
	if len(value) == 0 {
		return nil, nil
	}
	var result map[string]any
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, fmt.Errorf("unmarshal ingestion %s: %w", field, err)
	}
	return result, nil
}

