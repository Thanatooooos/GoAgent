package ingestion

import (
	"encoding/json"
	"fmt"

	postgrescommon "local/rag-project/internal/adapter/repository/postgres/common"
	"local/rag-project/internal/app/ingestion/domain"
)

func conditionalUpdateRequiresConditions(entity string) error {
	return postgrescommon.ConditionalUpdateRequiresConditions(entity)
}

// marshalPipelineNodes 把 pipeline 节点定义编码为 json。
func marshalPipelineNodes(nodes []domain.PipelineNode) ([]byte, error) {
	if len(nodes) == 0 {
		return []byte("[]"), nil
	}
	value, err := json.Marshal(nodes)
	if err != nil {
		return nil, fmt.Errorf("marshal ingestion pipeline nodes: %w", err)
	}
	return value, nil
}

// unmarshalPipelineNodes 从 json 解码 pipeline 节点定义。
func unmarshalPipelineNodes(value []byte) ([]domain.PipelineNode, error) {
	if len(value) == 0 {
		return nil, nil
	}
	var result []domain.PipelineNode
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, fmt.Errorf("unmarshal ingestion pipeline nodes: %w", err)
	}
	return result, nil
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
