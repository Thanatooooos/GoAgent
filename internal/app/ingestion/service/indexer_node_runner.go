package service

import (
	"context"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// IndexerNodeRunner 提供最小索引写入占位实现。
type IndexerNodeRunner struct{}

// NewIndexerNodeRunner 创建 indexer 占位运行器。
func NewIndexerNodeRunner() *IndexerNodeRunner {
	return &IndexerNodeRunner{}
}

// NodeType 返回当前运行器负责的节点类型。
func (r *IndexerNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeIndexer
}

// Run 汇总 chunk 结果，生成第一版 index result 占位。
func (r *IndexerNodeRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	_ = ctx

	if len(state.Chunks) == 0 {
		return state, nil, exception.NewClientException("indexer requires chunks", nil)
	}

	target := pickFirstNonEmpty(
		readStringSetting(node.Settings, "target"),
		"placeholder",
	)

	next := state.Clone()
	next.IndexResult = IndexResult{
		Target:     target,
		ChunkCount: len(state.Chunks),
		Metadata: map[string]any{
			"placeholder": true,
		},
	}

	output := map[string]any{
		"target":      strings.TrimSpace(target),
		"chunkCount":  len(state.Chunks),
		"placeholder": true,
	}
	return next, output, nil
}
