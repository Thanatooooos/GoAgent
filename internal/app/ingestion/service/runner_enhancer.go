package service

import (
	"context"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// EnhancerNodeRunner provides a lightweight content-enhancement stage before/after chunking.
type EnhancerNodeRunner struct{}

func NewEnhancerNodeRunner() *EnhancerNodeRunner {
	return &EnhancerNodeRunner{}
}

func (r *EnhancerNodeRunner) NodeType() string {
	return domain.PipelineNodeTypeEnhancer
}

func (r *EnhancerNodeRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	_ = ctx

	if strings.TrimSpace(state.Parsed.Content) == "" && len(state.Chunks) == 0 {
		return state, nil, exception.NewClientException("enhancer requires parsed content or chunks", nil)
	}

	tasks := readEnrichmentTasks(node.Settings)
	if len(tasks) == 0 {
		tasks = []enrichmentTask{{Type: "context_enhance"}}
	}

	next := state.Clone()
	next.Parsed.Metadata = ensureMetadata(cloneMetadata(next.Parsed.Metadata))
	if next.Artifacts == nil {
		next.Artifacts = map[string]any{}
	}

	documentKeywords := extractKeywords(next.Parsed.Title, next.Parsed.Content, 5)
	documentQuestions := generateQuestions(next.Parsed.Title, next.Parsed.Content, documentKeywords, 3)
	documentMetadata := buildHeuristicMetadata(next.Parsed.Content, next.Parsed.Title)
	appliedTasks := make([]string, 0, len(tasks))

	for _, task := range tasks {
		switch task.Type {
		case "context_enhance":
			header := buildDocumentContextHeader(next)
			if strings.TrimSpace(next.Parsed.Content) != "" {
				next.Parsed.Content = prependIfMissing(next.Parsed.Content, header)
			}
			if len(next.Chunks) > 0 && strings.TrimSpace(header) != "" {
				for index := range next.Chunks {
					next.Chunks[index].Content = prependIfMissing(next.Chunks[index].Content, header)
				}
			}
			appliedTasks = append(appliedTasks, task.Type)
		case "keywords":
			next.Parsed.Metadata["keywords"] = documentKeywords
			if len(next.Chunks) > 0 {
				for index := range next.Chunks {
					next.Chunks[index].Metadata = ensureMetadata(cloneMetadata(next.Chunks[index].Metadata))
					next.Chunks[index].Metadata["keywords"] = extractKeywords("", next.Chunks[index].Content, 4)
				}
			}
			appliedTasks = append(appliedTasks, task.Type)
		case "questions":
			next.Parsed.Metadata["questions"] = documentQuestions
			if len(next.Chunks) > 0 {
				for index := range next.Chunks {
					next.Chunks[index].Metadata = ensureMetadata(cloneMetadata(next.Chunks[index].Metadata))
					keywords := extractKeywords("", next.Chunks[index].Content, 3)
					next.Chunks[index].Metadata["questions"] = generateQuestions("", next.Chunks[index].Content, keywords, 2)
				}
			}
			appliedTasks = append(appliedTasks, task.Type)
		case "metadata":
			next.Parsed.Metadata = mergeMetadata(next.Parsed.Metadata, documentMetadata, true)
			if len(next.Chunks) > 0 {
				for index := range next.Chunks {
					next.Chunks[index].Metadata = ensureMetadata(cloneMetadata(next.Chunks[index].Metadata))
					next.Chunks[index].Metadata = mergeMetadata(next.Chunks[index].Metadata, buildHeuristicMetadata(next.Chunks[index].Content, ""), true)
				}
			}
			appliedTasks = append(appliedTasks, task.Type)
		}
	}

	modelID := readStringSetting(node.Settings, "modelId")
	next.Artifacts["enhancer"] = map[string]any{
		"tasks":           append([]string(nil), appliedTasks...),
		"mode":            "heuristic",
		"modelId":         modelID,
		"parsedKeywords":  documentKeywords,
		"parsedQuestions": documentQuestions,
	}

	output := map[string]any{
		"taskCount":           len(appliedTasks),
		"appliedTasks":        appliedTasks,
		"mode":                "heuristic",
		"modelId":             modelID,
		"parsedContentLength": len(next.Parsed.Content),
		"chunkCount":          len(next.Chunks),
	}
	if len(documentKeywords) > 0 {
		output["keywords"] = documentKeywords
	}
	if len(documentQuestions) > 0 {
		output["questions"] = documentQuestions
	}
	return next, output, nil
}
