package evaluation

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

// evalMinPreRerankCandidates is the minimum number of candidates to feed into
// the rerank step. When the caller's desired TopK is smaller than this value,
// the evaluator expands TopK to this value for the pre-rerank pool and sets
// RerankTopN to the original TopK so the final output size stays the same.
// Set EVAL_PRERANK_CANDIDATES=0 to disable expansion and use the original TopK.
var evalMinPreRerankCandidates = readEvalPreRerankCandidates()

func readEvalPreRerankCandidates() int {
	if v := os.Getenv("EVAL_PRERANK_CANDIDATES"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			return n
		}
	}
	return 20
}

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

	prerankTopK := topK
	rerankTopN := 0
	if prerankTopK < evalMinPreRerankCandidates {
		prerankTopK = evalMinPreRerankCandidates
		rerankTopN = topK
	}

	request := ragretrieve.Request{
		UserID:           strings.TrimSpace(sample.UserID),
		Query:            strings.TrimSpace(sample.Query),
		KnowledgeBaseIDs: append([]string(nil), sample.KnowledgeBaseIDs...),
		SearchMode:       searchMode,
		TopK:             prerankTopK,
		RerankTopN:       rerankTopN,
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
