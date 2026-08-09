package runner

import (
	"context"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	wikiservice "local/rag-project/internal/app/knowledge/service/wiki"
)

// WikiPageServicePort 是 runner 依赖的 wiki 持久化接口（最小化，便于测试注入）。
type WikiPageServicePort interface {
	UpsertPagesFromDocument(ctx context.Context, kbID string, pages []knowledgedomain.WikiPage, links []knowledgedomain.WikiLink) error
}

// WikiGeneratorNodeRunner 由 LLM 从解析出的文档生成 wiki 页面并持久化。
type WikiGeneratorNodeRunner struct {
	service   WikiPageServicePort
	generator wikiservice.WikiGenerator
}

func NewWikiGeneratorNodeRunner(service WikiPageServicePort, generator wikiservice.WikiGenerator) *WikiGeneratorNodeRunner {
	return &WikiGeneratorNodeRunner{service: service, generator: generator}
}

func (r *WikiGeneratorNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeWikiGenerator
}

func (r *WikiGeneratorNodeRunner) Run(ctx context.Context, state ingestionworkflow.ExecutionState, node domain.PipelineNode) (ingestionworkflow.ExecutionState, map[string]any, error) {
	if r == nil || r.generator == nil || r.service == nil {
		return state, map[string]any{"degraded": true}, nil
	}
	kbID := strings.TrimSpace(readStringSetting(state.Task.Metadata, "knowledgeBaseId"))
	if kbID == "" || strings.TrimSpace(state.Parsed.Content) == "" {
		return state, map[string]any{"degraded": true}, nil
	}
	options := wikiservice.WikiGenerationOptions{
		MaxPages: readIntSetting(node.Settings, "maxPages"),
		PageType: readStringSetting(node.Settings, "pageType"),
	}
	result, err := r.generator.GenerateFromDocument(ctx, state.Parsed.Title, state.Parsed.Content, options)
	if err != nil || len(result.Pages) == 0 {
		return state, map[string]any{"degraded": true, "pageCount": 0}, nil
	}
	if err := r.service.UpsertPagesFromDocument(ctx, kbID, result.Pages, result.Links); err != nil {
		return state, map[string]any{"degraded": true, "error": err.Error()}, nil
	}
	return state, map[string]any{"pageCount": len(result.Pages), "linkCount": len(result.Links)}, nil
}
