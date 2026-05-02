package service

import (
	"context"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// FetcherNodeRunner 提供最小来源归一化能力。
type FetcherNodeRunner struct{}

// NewFetcherNodeRunner 创建 fetcher 占位运行器。
func NewFetcherNodeRunner() *FetcherNodeRunner {
	return &FetcherNodeRunner{}
}

// NodeType 返回当前运行器负责的节点类型。
func (r *FetcherNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeFetcher
}

// Run 把 task source 归一化为统一 SourcePayload。
func (r *FetcherNodeRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	_ = ctx
	_ = node

	sourceType := pickFirstNonEmpty(
		readStringSetting(node.Settings, "sourceType"),
		state.Task.SourceType,
		state.Source.Type,
	)
	if err := validateTaskSourceType(sourceType); err != nil {
		return state, nil, err
	}

	location := pickFirstNonEmpty(
		readStringSetting(node.Settings, "sourceLocation"),
		state.Task.SourceLocation,
		state.Source.Location,
	)
	fileName := pickFirstNonEmpty(
		readStringSetting(node.Settings, "fileName"),
		state.Task.SourceFileName,
		state.Source.FileName,
	)
	contentType := pickFirstNonEmpty(
		readStringSetting(node.Settings, "contentType"),
		readStringSetting(state.Task.Metadata, "contentType"),
		state.Source.ContentType,
	)

	rawText := pickFirstNonEmpty(
		readStringSetting(node.Settings, "rawText"),
		readStringSetting(state.Task.Metadata, "rawText"),
		readStringSetting(state.Task.Metadata, "content"),
	)

	if sourceType == domain.TaskSourceTypeFile && location == "" && fileName == "" && rawText == "" {
		return state, nil, exception.NewClientException("file source requires source location, file name or inline content", nil)
	}
	if sourceType == domain.TaskSourceTypeURL && location == "" && rawText == "" {
		return state, nil, exception.NewClientException("url source requires source location or inline content", nil)
	}
	if sourceType == domain.TaskSourceTypeFeishu || sourceType == domain.TaskSourceTypeS3 {
		return state, nil, exception.NewClientException("source type is recognized but fetcher placeholder does not implement remote fetching yet", nil)
	}
	if rawText == "" {
		return state, nil, exception.NewClientException("fetcher placeholder does not implement remote reading yet; provide inline content in metadata or node settings", nil)
	}

	next := state.Clone()
	next.Source = SourcePayload{
		Type:        sourceType,
		Location:    location,
		FileName:    fileName,
		ContentType: contentType,
		Metadata: map[string]any{
			"placeholder": true,
		},
	}
	if rawText != "" {
		next.Source.Bytes = []byte(rawText)
	}

	output := map[string]any{
		"sourceType":  sourceType,
		"location":    location,
		"fileName":    fileName,
		"contentType": contentType,
		"hasBytes":    len(next.Source.Bytes) > 0,
		"placeholder": true,
	}
	return next, output, nil
}
