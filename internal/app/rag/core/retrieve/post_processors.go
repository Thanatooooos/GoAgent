package retrieve

import (
	"context"
	"sort"
	"strings"

	"local/rag-project/internal/framework/convention"
	airerank "local/rag-project/internal/infra-ai/rerank"
)

type fusionPostProcessor struct{}

func NewFusionPostProcessor() SearchResultPostProcessor {
	return &fusionPostProcessor{}
}

func (p *fusionPostProcessor) Name() string               { return "fusion" }
func (p *fusionPostProcessor) Order() int                 { return 10 }
func (p *fusionPostProcessor) Enabled(SearchContext) bool { return true }
func (p *fusionPostProcessor) Process(_ context.Context, input SearchProcessInput) ([]convention.RetrievedChunk, error) {
	if len(input.ChannelResults) == 0 {
		return []convention.RetrievedChunk{}, nil
	}
	if len(input.ChannelResults) == 1 {
		return cloneChunks(input.ChannelResults[0].Chunks), nil
	}
	return rrfFuseChannelResults(input.ChannelResults), nil
}

type dedupPostProcessor struct{}

func NewDedupPostProcessor() SearchResultPostProcessor {
	return &dedupPostProcessor{}
}

func (p *dedupPostProcessor) Name() string               { return "dedup" }
func (p *dedupPostProcessor) Order() int                 { return 20 }
func (p *dedupPostProcessor) Enabled(SearchContext) bool { return true }
func (p *dedupPostProcessor) Process(_ context.Context, input SearchProcessInput) ([]convention.RetrievedChunk, error) {
	chunkMap := map[string]convention.RetrievedChunk{}
	for _, chunk := range input.Chunks {
		if existing, ok := chunkMap[chunk.ID]; ok {
			if chunk.Score > existing.Score {
				chunkMap[chunk.ID] = chunk
			}
			continue
		}
		chunkMap[chunk.ID] = chunk
	}
	chunks := make([]convention.RetrievedChunk, 0, len(chunkMap))
	for _, chunk := range chunkMap {
		chunks = append(chunks, chunk)
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})
	if input.Context.TopK > 0 && len(chunks) > input.Context.TopK {
		chunks = chunks[:input.Context.TopK]
	}
	return chunks, nil
}

type rerankPostProcessor struct {
	reranker airerank.RerankService
}

func NewRerankPostProcessor(reranker airerank.RerankService) SearchResultPostProcessor {
	return &rerankPostProcessor{reranker: reranker}
}

func (p *rerankPostProcessor) Name() string               { return "rerank" }
func (p *rerankPostProcessor) Order() int                 { return 30 }
func (p *rerankPostProcessor) Enabled(SearchContext) bool { return p != nil && p.reranker != nil }
func (p *rerankPostProcessor) Process(_ context.Context, input SearchProcessInput) ([]convention.RetrievedChunk, error) {
	chunks := cloneChunks(input.Chunks)
	if p == nil || p.reranker == nil || len(chunks) <= 1 {
		return chunks, nil
	}
	topN := input.Context.RerankTopN
	if topN <= 0 || topN > len(chunks) {
		topN = len(chunks)
	}
	reranked, err := p.reranker.Rerank(strings.TrimSpace(input.Context.Query), chunks, topN)
	if err != nil || len(reranked) == 0 {
		return chunks, nil
	}
	return reranked, nil
}

func rrfFuseChannelResults(results []SearchChannelResult) []convention.RetrievedChunk {
	if len(results) == 0 {
		return []convention.RetrievedChunk{}
	}

	type fusionEntry struct {
		chunk    convention.RetrievedChunk
		rrfScore float32
	}
	merged := map[string]*fusionEntry{}
	for _, result := range results {
		weight := readChannelRRFWeight(result)
		for rank, chunk := range result.Chunks {
			score := weight * (float32(1.0) / float32(defaultRRFK+rank+1))
			if existing, ok := merged[chunk.ID]; ok {
				existing.rrfScore += score
				if chunk.Score > existing.chunk.Score {
					existing.chunk = chunk
				}
				continue
			}
			merged[chunk.ID] = &fusionEntry{
				chunk:    chunk,
				rrfScore: score,
			}
		}
	}

	entries := make([]*fusionEntry, 0, len(merged))
	for _, entry := range merged {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].rrfScore > entries[j].rrfScore
	})

	fused := make([]convention.RetrievedChunk, 0, len(entries))
	for _, entry := range entries {
		chunk := entry.chunk
		chunk.Score = entry.rrfScore
		fused = append(fused, chunk)
	}
	return fused
}

func readChannelRRFWeight(result SearchChannelResult) float32 {
	if result.Metadata != nil {
		if value, ok := result.Metadata["rrfWeight"]; ok {
			switch typed := value.(type) {
			case float32:
				if typed > 0 {
					return typed
				}
			case float64:
				if typed > 0 {
					return float32(typed)
				}
			case int:
				if typed > 0 {
					return float32(typed)
				}
			}
		}
	}
	return defaultChannelRRFWeight(result.ChannelName)
}

func cloneChunks(chunks []convention.RetrievedChunk) []convention.RetrievedChunk {
	if len(chunks) == 0 {
		return []convention.RetrievedChunk{}
	}
	cloned := make([]convention.RetrievedChunk, len(chunks))
	copy(cloned, chunks)
	return cloned
}
