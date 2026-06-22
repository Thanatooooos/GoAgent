package runner

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"strings"

	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// ParserNodeRunner 提供最小解析占位实现。
type ParserNodeRunner struct {
	selector *coreparser.Selector
}

// NewParserNodeRunner 创建 parser 运行器。
func NewParserNodeRunner(selector *coreparser.Selector) *ParserNodeRunner {
	if selector == nil {
		selector = coreparser.NewDefaultSelector(nil)
	}
	return &ParserNodeRunner{selector: selector}
}

// NodeType 返回当前运行器负责的节点类型。
func (r *ParserNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeParser
}

// Run 将 fetcher 输出转为标准文本载荷。
func (r *ParserNodeRunner) Run(ctx context.Context, state ingestionworkflow.ExecutionState, node domain.PipelineNode) (ingestionworkflow.ExecutionState, map[string]any, error) {
	_ = ctx

	if r == nil || r.selector == nil {
		return state, nil, exception.NewServiceException("parser selector is required", nil)
	}

	content := state.Source.Bytes
	if len(content) == 0 {
		inline := pickFirstNonEmpty(
			readStringSetting(node.Settings, "rawText"),
			readStringSetting(state.Task.Metadata, "rawText"),
			readStringSetting(state.Task.Metadata, "content"),
		)
		if inline != "" {
			content = []byte(inline)
		}
	}
	if len(content) == 0 {
		return state, nil, exception.NewClientException("parser placeholder requires source bytes or inline text", nil)
	}

	mimeType := pickFirstNonEmpty(
		readStringSetting(node.Settings, "mimeType"),
		state.Source.ContentType,
	)
	fileName := pickFirstNonEmpty(
		readStringSetting(node.Settings, "fileName"),
		state.Source.FileName,
	)
	parserType := readStringSetting(node.Settings, "parserType")

	result := coreparser.OfText(string(content))
	selectedType := "plain_text"
	if parserType != "" {
		if parserImpl, ok := r.selector.Select(parserType); ok {
			parsed, err := parserImpl.Parse(content, mimeType, node.Settings)
			if err != nil {
				return state, nil, exception.NewServiceException("failed to parse source content", err)
			}
			result = parsed
			selectedType = parserImpl.ParserType()
		}
	} else if parserImpl := r.selector.SelectFor(mimeType, fileName); parserImpl != nil {
		parsed, err := parserImpl.Parse(content, mimeType, node.Settings)
		if err == nil && strings.TrimSpace(parsed.Text) != "" {
			result = parsed
			selectedType = parserImpl.ParserType()
		}
	}

	next := state.Clone()
	next.Parsed = ingestionworkflow.ParsedDocument{
		Content:  result.Text,
		Title:    fileName,
		Metadata: result.Metadata,
	}

	output := map[string]any{
		"parserType":    selectedType,
		"contentLength": len(next.Parsed.Content),
		"title":         next.Parsed.Title,
		"placeholder":   true,
	}
	return next, output, nil
}
