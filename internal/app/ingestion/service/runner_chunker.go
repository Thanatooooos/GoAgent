package service

import (
	"context"
	"strings"

	corechunk "local/rag-project/internal/app/core/chunk"
	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// ChunkerNodeRunner 提供最小分块实现。
type ChunkerNodeRunner struct {
	selector *corechunk.Selector
}

// NewChunkerNodeRunner 创建 chunker 运行器。
func NewChunkerNodeRunner(selector *corechunk.Selector) *ChunkerNodeRunner {
	if selector == nil {
		selector = corechunk.NewDefaultSelector()
	}
	return &ChunkerNodeRunner{selector: selector}
}

// NodeType 返回当前运行器负责的节点类型。
func (r *ChunkerNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeChunker
}

// Run 使用现有 chunk selector 生成最小分块结果。
func (r *ChunkerNodeRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	_ = ctx

	if r == nil || r.selector == nil {
		return state, nil, exception.NewServiceException("chunk selector is required", nil)
	}
	if strings.TrimSpace(state.Parsed.Content) == "" {
		return state, nil, exception.NewClientException("chunker requires parsed content", nil)
	}

	strategy := corechunk.Strategy(readStringSetting(node.Settings, "strategy"))
	if strategy == "" {
		strategy = corechunk.StrategyFixedSize
	}
	options := corechunk.Options{
		Strategy:     strategy,
		ChunkSize:    readIntSetting(node.Settings, "chunkSize"),
		OverlapSize:  readIntSetting(node.Settings, "overlapSize"),
		MinChunkSize: readIntSetting(node.Settings, "minChunkSize"),
	}.Normalize()

	chunks, err := r.selector.Chunk(state.Parsed.Content, options)
	if err != nil {
		return state, nil, exception.NewServiceException("failed to chunk parsed content", err)
	}

	next := state.Clone()
	next.Chunks = make([]ChunkPayload, 0, len(chunks))
	for _, item := range chunks {
		next.Chunks = append(next.Chunks, ChunkPayload{
			Index:    item.Index,
			Content:  item.Text,
			Metadata: item.Metadata,
		})
	}

	output := map[string]any{
		"strategy":    string(options.Strategy),
		"chunkCount":  len(next.Chunks),
		"placeholder": true,
	}
	return next, output, nil
}
