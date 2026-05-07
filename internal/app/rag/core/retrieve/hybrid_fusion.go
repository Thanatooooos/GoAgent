package retrieve

import (
	"sort"

	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/framework/convention"
)

const defaultRRFK = 60

// RRFusion 使用 Reciprocal Rank Fusion 融合两路检索结果。
// vectorHits 和 keywordHits 分别为语义检索和关键词检索的结果。
// k 为平滑参数，默认 60。
func RRFusion(vectorHits, keywordHits []corevector.SearchHit, k int) []convention.RetrievedChunk {
	if k <= 0 {
		k = defaultRRFK
	}

	// 合并两路结果，按 chunk_id 聚合 RRF 分数。
	chunkScore := map[string]*fusionEntry{}
	accumulate := func(hits []corevector.SearchHit) {
		for rank, hit := range hits {
			rrfScore := float32(1.0) / float32(k+rank+1)
			id := hit.ChunkID
			if existing, ok := chunkScore[id]; ok {
				existing.rrfScore += rrfScore
				// 保留两路中较高的原始分数和更完整的元数据。
				if hit.Score > existing.hit.Score {
					existing.hit = hit
				}
			} else {
				chunkScore[id] = &fusionEntry{
					hit:      hit,
					rrfScore: rrfScore,
				}
			}
		}
	}

	accumulate(vectorHits)
	accumulate(keywordHits)

	// 按 RRF 融合分降序排列。
	entries := make([]*fusionEntry, 0, len(chunkScore))
	for _, entry := range chunkScore {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].rrfScore > entries[j].rrfScore
	})

	result := make([]convention.RetrievedChunk, 0, len(entries))
	for _, entry := range entries {
		result = append(result, convention.RetrievedChunk{
			ID:              entry.hit.ChunkID,
			Text:            entry.hit.Text,
			Score:           entry.rrfScore,
			DocumentID:      entry.hit.DocumentID,
			KnowledgeBaseID: entry.hit.KnowledgeBaseID,
			ChunkIndex:      entry.hit.Index,
			Metadata:        entry.hit.Metadata,
		})
	}
	return result
}

type fusionEntry struct {
	hit      corevector.SearchHit
	rrfScore float32
}

// MergeChunks 合并去重多个检索结果，按 chunk ID 取最高分。
func MergeChunks(results []Result) []convention.RetrievedChunk {
	seen := map[string]convention.RetrievedChunk{}
	for _, result := range results {
		for _, chunk := range result.Chunks {
			if existing, ok := seen[chunk.ID]; ok {
				if chunk.Score > existing.Score {
					seen[chunk.ID] = chunk
				}
			} else {
				seen[chunk.ID] = chunk
			}
		}
	}
	chunks := make([]convention.RetrievedChunk, 0, len(seen))
	for _, chunk := range seen {
		chunks = append(chunks, chunk)
	}
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})
	return chunks
}
