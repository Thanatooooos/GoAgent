package retrieve

import "local/rag-project/internal/framework/convention"

// PipelineTrace captures retrieval post-processing stages for eval/debug.
type PipelineTrace struct {
	PreRerankChunkIDs []string `json:"pre_rerank_chunk_ids,omitempty"`
	FinalChunkIDs     []string `json:"final_chunk_ids,omitempty"`
	RerankApplied     bool     `json:"rerank_applied"`
	RerankModel       string   `json:"rerank_model,omitempty"`
	RerankError       string   `json:"rerank_error,omitempty"`
	SubQuestionMerge  bool     `json:"sub_question_merge,omitempty"`
}

func chunkIDs(chunks []convention.RetrievedChunk) []string {
	if len(chunks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id := chunk.ID; id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func clonePipelineTrace(trace *PipelineTrace) *PipelineTrace {
	if trace == nil {
		return nil
	}
	return &PipelineTrace{
		PreRerankChunkIDs: append([]string(nil), trace.PreRerankChunkIDs...),
		FinalChunkIDs:     append([]string(nil), trace.FinalChunkIDs...),
		RerankApplied:     trace.RerankApplied,
		RerankModel:       trace.RerankModel,
		RerankError:       trace.RerankError,
		SubQuestionMerge:  trace.SubQuestionMerge,
	}
}
