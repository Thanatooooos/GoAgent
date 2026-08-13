package runner

import (
	"context"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
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
func (r *ChunkerNodeRunner) Run(ctx context.Context, state ingestionworkflow.ExecutionState, node domain.PipelineNode) (ingestionworkflow.ExecutionState, map[string]any, error) {
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

	next := state.Clone()
	parentChildEnabled := true
	if raw, ok := node.Settings["enableParentChild"]; ok && raw != nil {
		parentChildEnabled = readBoolSetting(node.Settings, "enableParentChild")
	}
	chunkMode := "flat"
	if parentChildEnabled {
		parentOptions, childOptions := corechunk.ParentChildOptions(
			options.Strategy,
			readIntSetting(node.Settings, "parentChunkSize"),
			readIntSetting(node.Settings, "parentOverlapSize"),
			readIntSetting(node.Settings, "childChunkSize"),
			readIntSetting(node.Settings, "childOverlapSize"),
		)

		result, err := corechunk.SplitParentChild(state.Parsed.Content, parentOptions, childOptions)
		if err != nil {
			return state, nil, exception.NewServiceException("failed to split parent child chunks", err)
		}
		next.ParentChunks = make([]ingestionworkflow.ParentChunkPayload, 0, len(result.Parents))
		for _, parent := range result.Parents {
			next.ParentChunks = append(next.ParentChunks, ingestionworkflow.ParentChunkPayload{Index: parent.Index, Content: parent.Text})
		}
		next.Chunks = make([]ingestionworkflow.ChunkPayload, 0, len(result.Children))
		for _, child := range result.Children {
			parentIndex := child.ParentIndex
			next.Chunks = append(next.Chunks, ingestionworkflow.ChunkPayload{
				Index: child.Index, Content: child.Text, Metadata: child.Metadata, ParentIndex: &parentIndex,
			})
		}
		chunkMode = "parent_child"
	} else {
		chunks, err := r.selector.Chunk(state.Parsed.Content, options)
		if err != nil {
			return state, nil, exception.NewServiceException("failed to chunk parsed content", err)
		}
		next.ParentChunks = nil
		next.Chunks = make([]ingestionworkflow.ChunkPayload, 0, len(chunks))
		for _, item := range chunks {
			next.Chunks = append(next.Chunks, ingestionworkflow.ChunkPayload{Index: item.Index, Content: item.Text, Metadata: item.Metadata})
		}
	}

	output := map[string]any{
		"strategy":    string(options.Strategy),
		"chunkCount":  len(next.Chunks),
		"parentCount": len(next.ParentChunks),
		"chunkMode":   chunkMode,
	}
	return next, output, nil
}
