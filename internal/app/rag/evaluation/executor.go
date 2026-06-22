package evaluation

import (
	"context"
	"fmt"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

type ExecuteConfig struct {
	Retrieve           ragretrieve.Service
	Rewrite            ragrewrite.Service
	UseRewrite         bool
	SearchModeOverride string
	SubQuestionOptions ragretrieve.SubQuestionOptions
}

func ExecuteSample(ctx context.Context, sample *Sample, cfg ExecuteConfig) error {
	if sample == nil {
		return fmt.Errorf("evaluation sample is required")
	}
	if cfg.Retrieve == nil {
		return fmt.Errorf("retrieve service is required")
	}

	searchMode := resolveSearchMode(sample.SearchMode, cfg.SearchModeOverride)
	topK := sample.TopK
	if topK <= 0 {
		topK = ragretrieve.DefaultTopK
	}

	request := ragretrieve.Request{
		UserID:           strings.TrimSpace(sample.UserID),
		Query:            strings.TrimSpace(sample.Query),
		KnowledgeBaseIDs: append([]string(nil), sample.KnowledgeBaseIDs...),
		SearchMode:       searchMode,
		TopK:             topK,
	}

	var (
		result        ragretrieve.Result
		executionMode string
		err           error
	)

	if cfg.UseRewrite {
		if cfg.Rewrite == nil {
			return fmt.Errorf("rewrite service is required when useRewrite is enabled")
		}
		rewriteResult := cfg.Rewrite.RewriteWithSplit(request.Query)
		sample.RewrittenQuery = strings.TrimSpace(rewriteResult.RewrittenQuestion)
		sample.SubQuestions = append([]string(nil), rewriteResult.SubQuestions...)
		sample.NeedRetrieval = rewriteResult.NeedRetrieval

		subQuestions := ragretrieve.BuildRetrieveSubQuestions(request.Query, rewriteResult.SubQuestions)
		executor := ragretrieve.NewSubQuestionExecutor(cfg.Retrieve, cfg.SubQuestionOptions)
		result, executionMode, _, err = executor.RetrieveMerged(ctx, request, subQuestions, topK)
		sample.ExecutionMode = executionMode
	} else {
		result, err = cfg.Retrieve.Retrieve(ctx, request)
	}

	if err != nil {
		return fmt.Errorf("execute sample %q: %w", sample.Name, err)
	}

	sample.Retrieved = retrievedItemsFromChunks(result.Chunks)
	sample.ChannelRetrieved = channelRetrievedFromResult(result)
	sample.PipelineTrace = pipelineTraceToMap(result.PipelineTrace)
	return nil
}

func retrievedItemsFromChunks(chunks []convention.RetrievedChunk) []RetrievedItem {
	retrieved := make([]RetrievedItem, 0, len(chunks))
	for _, chunk := range chunks {
		retrieved = append(retrieved, RetrievedItem{
			ChunkID:    chunk.ID,
			DocumentID: chunk.DocumentID,
			Metadata:   chunk.Metadata,
			Score:      float64(chunk.Score),
		})
	}
	return retrieved
}

func channelRetrievedFromResult(result ragretrieve.Result) map[string][]RetrievedItem {
	if len(result.ChannelRetrieved) == 0 {
		return nil
	}
	channelRetrieved := make(map[string][]RetrievedItem, len(result.ChannelRetrieved))
	for channel, chunks := range result.ChannelRetrieved {
		channelRetrieved[channel] = retrievedItemsFromChunks(chunks)
	}
	return channelRetrieved
}

func resolveSearchMode(sampleMode, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return strings.TrimSpace(sampleMode)
}

func pipelineTraceToMap(trace *ragretrieve.PipelineTrace) map[string]any {
	if trace == nil {
		return nil
	}
	out := map[string]any{
		"rerank_applied": trace.RerankApplied,
	}
	if len(trace.PreRerankChunkIDs) > 0 {
		out["pre_rerank_chunk_ids"] = append([]string(nil), trace.PreRerankChunkIDs...)
	}
	if len(trace.FinalChunkIDs) > 0 {
		out["final_chunk_ids"] = append([]string(nil), trace.FinalChunkIDs...)
	}
	if trace.RerankModel != "" {
		out["rerank_model"] = trace.RerankModel
	}
	if trace.RerankError != "" {
		out["rerank_error"] = trace.RerankError
	}
	if trace.SubQuestionMerge {
		out["sub_question_merge"] = true
	}
	return out
}
