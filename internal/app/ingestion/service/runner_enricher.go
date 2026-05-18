package service

import (
	"context"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// EnricherNodeRunner enriches chunk metadata so indexer can persist richer filters and summaries.
type EnricherNodeRunner struct{}

func NewEnricherNodeRunner() *EnricherNodeRunner {
	return &EnricherNodeRunner{}
}

func (r *EnricherNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeEnricher
}

func (r *EnricherNodeRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	_ = ctx

	if len(state.Chunks) == 0 {
		return state, nil, exception.NewClientException("enricher requires chunks", nil)
	}

	tasks := readEnrichmentTasks(node.Settings)
	if len(tasks) == 0 {
		tasks = []enrichmentTask{{Type: "metadata"}}
	}

	next := state.Clone()
	if next.Artifacts == nil {
		next.Artifacts = map[string]any{}
	}

	attachDocumentMetadata := readBoolSetting(node.Settings, "attachDocumentMetadata")
	documentMetadata := buildChunkDocumentMetadata(next)
	documentKeywords := extractKeywords(next.Parsed.Title, next.Parsed.Content, 5)
	documentSummary := truncateText(next.Parsed.Content, 180)
	appliedTasks := make([]string, 0, len(tasks))

	for index := range next.Chunks {
		next.Chunks[index].Metadata = ensureMetadata(cloneMetadata(next.Chunks[index].Metadata))
		if attachDocumentMetadata {
			next.Chunks[index].Metadata = mergeMetadata(next.Chunks[index].Metadata, documentMetadata, false)
			if len(documentKeywords) > 0 {
				next.Chunks[index].Metadata["document_keywords"] = documentKeywords
			}
		}
		for _, task := range tasks {
			switch task.Type {
			case "keywords":
				next.Chunks[index].Metadata["keywords"] = extractKeywords("", next.Chunks[index].Content, 5)
			case "summary":
				next.Chunks[index].Metadata["summary"] = truncateText(next.Chunks[index].Content, 120)
			case "metadata":
				next.Chunks[index].Metadata = mergeMetadata(next.Chunks[index].Metadata, buildHeuristicMetadata(next.Chunks[index].Content, ""), true)
				next.Chunks[index].Metadata["chunk_index"] = next.Chunks[index].Index
			}
		}
	}

	for _, task := range tasks {
		appliedTasks = append(appliedTasks, task.Type)
	}

	next.Parsed.Metadata = ensureMetadata(cloneMetadata(next.Parsed.Metadata))
	if len(documentKeywords) > 0 {
		next.Parsed.Metadata["keywords"] = documentKeywords
	}
	if strings.TrimSpace(documentSummary) != "" {
		next.Parsed.Metadata["summary"] = documentSummary
	}

	modelID := readStringSetting(node.Settings, "modelId")
	next.Artifacts["enricher"] = map[string]any{
		"tasks":                  append([]string(nil), appliedTasks...),
		"mode":                   "heuristic",
		"modelId":                modelID,
		"attachDocumentMetadata": attachDocumentMetadata,
		"documentSummary":        documentSummary,
	}

	output := map[string]any{
		"taskCount":              len(appliedTasks),
		"appliedTasks":           appliedTasks,
		"mode":                   "heuristic",
		"modelId":                modelID,
		"chunkCount":             len(next.Chunks),
		"attachDocumentMetadata": attachDocumentMetadata,
	}
	if len(documentKeywords) > 0 {
		output["documentKeywords"] = documentKeywords
	}
	if documentSummary != "" {
		output["documentSummary"] = documentSummary
	}
	return next, output, nil
}
