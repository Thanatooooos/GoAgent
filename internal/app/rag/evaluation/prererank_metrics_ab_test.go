package evaluation

import (
	"fmt"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

// relevanceAwareReranker simulates a real reranker: it boosts chunks whose IDs
// match expected IDs to the top, then fills remaining slots by score.
type relevanceAwareReranker struct {
	expectedIDs map[string]struct{}
	inputCount  int
}

func (r *relevanceAwareReranker) Rerank(_ string, candidates []convention.RetrievedChunk, topN int) ([]convention.RetrievedChunk, error) {
	r.inputCount = len(candidates)
	if topN <= 0 || topN > len(candidates) {
		topN = len(candidates)
	}
	matches := make([]convention.RetrievedChunk, 0)
	others := make([]convention.RetrievedChunk, 0)
	for _, c := range candidates {
		if _, ok := r.expectedIDs[c.ID]; ok {
			matches = append(matches, c)
		} else {
			others = append(others, c)
		}
	}
	result := make([]convention.RetrievedChunk, 0, topN)
	result = append(result, matches...)
	need := topN - len(result)
	if need > 0 && len(others) > 0 {
		if need > len(others) {
			need = len(others)
		}
		result = append(result, others[:need]...)
	}
	return result, nil
}

// TestABMetrics_TopK5_vs_TopK20 compares retrieval metrics when the reranker
// operates on 5 vs 20 pre-rerank candidates.
//
// Scenario: 3 channels (vec×12, kw×12, title×12), RRF-fused into 36 unique
// chunks. Using the real RRF formula weight/(60+rank+1):
//
//	vec weight 1.0, kw weight 0.85, title weight 0.8
//
// Expected IDs and their fused ranks (pre-rerank):
//
//	vec_00 → rank  1  (RRF=0.01639) — always survives
//	vec_08 → rank  9  (RRF=0.01449) — cut at TopK=5, survives TopK=20
//	kw_04  → rank 18  (RRF=0.01308) — cut at TopK=5, survives TopK=20
//
// At TopK=5:  only vec_00 survives dedup → rerank can't recover the other 2
// At TopK=20: all 3 survive dedup → rerank pulls them to top 3
func TestABMetrics_TopK5_vs_TopK20(t *testing.T) {
	expectedIDs := map[string]struct{}{
		"vec_00": {},
		"vec_08": {},
		"kw_04":  {},
	}
	expectedIDList := []string{"vec_00", "vec_08", "kw_04"}
	expectedRelevance := map[string]int{
		"vec_00": 3,
		"vec_08": 2,
		"kw_04":  1,
	}

	build := func(prefix string, n int) []convention.RetrievedChunk {
		out := make([]convention.RetrievedChunk, n)
		for i := 0; i < n; i++ {
			out[i] = convention.RetrievedChunk{
				ID:    fmt.Sprintf("%s_%02d", prefix, i),
				Score: float32(n - i),
			}
		}
		return out
	}

	const perChan = 12
	vector := build("vec", perChan)
	keyword := build("kw", perChan)
	title := build("title", perChan)

	channels := []ragretrieve.SearchChannelResult{
		{ChannelName: ragretrieve.ChannelVectorGlobal, Chunks: vector, Metadata: map[string]any{"rrfWeight": float32(1.0)}},
		{ChannelName: ragretrieve.ChannelKeyword, Chunks: keyword, Metadata: map[string]any{"rrfWeight": float32(0.85)}},
		{ChannelName: ragretrieve.ChannelMetadataTitle, Chunks: title, Metadata: map[string]any{"rrfWeight": float32(0.8)}},
	}

	type testCase struct {
		name       string
		topK       int
		rerankTopN int
	}

	cases := []testCase{
		{name: "TopK=5_before", topK: 5, rerankTopN: 0},
		{name: "TopK=20_RerankTopN=5_after", topK: 20, rerankTopN: 5},
	}

	ks := []int{1, 3, 5}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reranker := &relevanceAwareReranker{expectedIDs: expectedIDs}

			fused := rrfFuseChannelResultsIR(channels)
			deduped := dedupToTopKIR(fused, tc.topK)
			reranked, err := reranker.Rerank("test query", deduped, tc.rerankTopN)
			if err != nil {
				t.Fatalf("Rerank error: %v", err)
			}

			retrieved := make([]RetrievedItem, 0, len(reranked))
			for _, c := range reranked {
				retrieved = append(retrieved, RetrievedItem{ChunkID: c.ID, Score: float64(c.Score)})
			}

			sample := Sample{
				Name:              tc.name,
				Query:             "test query",
				Target:            TargetChunk,
				ExpectedIDs:       append([]string(nil), expectedIDList...),
				ExpectedRelevance: expectedRelevance,
				Retrieved:         retrieved,
			}

			summary, err := Evaluate([]Sample{sample}, ks)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}

			result := summary.Samples[0]
			o := summary.Overall

			t.Logf("Pre-rerank candidates: %d", len(deduped))
			t.Logf("Rerank input:  %d", reranker.inputCount)
			t.Logf("Rerank output: %d", len(reranked))
			t.Logf("Final IDs: %v", chunkIDsFromItems(retrieved))
			t.Logf("")
			t.Logf("FirstRelevantRank: %d", result.FirstRelevantRank)
			t.Logf("MRR:              %.4f", o.MRR)
			for _, k := range ks {
				t.Logf("  Hit@%d:  %-5v  rate=%.2f", k, result.HitAtK[k], o.HitRateAtK[k])
				t.Logf("  Recall@%d: %.4f  avg=%.4f", k, result.RecallAtK[k], o.AverageRecallAtK[k])
				t.Logf("  NDCG@%d:  %.4f  avg=%.4f", k, result.NDCGAtK[k], o.AverageNDCGAtK[k])
			}
		})
	}
}

// Minimal RRF fusion clone for test use (avoids depending on unexported
// retrieve.rrfFuseChannelResults).
func rrfFuseChannelResultsIR(results []ragretrieve.SearchChannelResult) []convention.RetrievedChunk {
	const k = 60
	type entry struct {
		chunk    convention.RetrievedChunk
		rrfScore float32
	}
	merged := map[string]*entry{}
	for _, result := range results {
		weight := float32(1.0)
		if w, ok := result.Metadata["rrfWeight"]; ok {
			switch v := w.(type) {
			case float32:
				if v > 0 {
					weight = v
				}
			case float64:
				if v > 0 {
					weight = float32(v)
				}
			}
		}
		for rank, chunk := range result.Chunks {
			score := weight * (1.0 / float32(k+rank+1))
			if existing, ok := merged[chunk.ID]; ok {
				existing.rrfScore += score
				if chunk.Score > existing.chunk.Score {
					existing.chunk = chunk
				}
			} else {
				merged[chunk.ID] = &entry{chunk: chunk, rrfScore: score}
			}
		}
	}
	entries := make([]*entry, 0, len(merged))
	for _, e := range merged {
		entries = append(entries, e)
	}
	// Sort by RRF score descending.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].rrfScore > entries[i].rrfScore {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	out := make([]convention.RetrievedChunk, 0, len(entries))
	for _, e := range entries {
		c := e.chunk
		c.Score = e.rrfScore
		out = append(out, c)
	}
	return out
}

func dedupToTopKIR(chunks []convention.RetrievedChunk, topK int) []convention.RetrievedChunk {
	seen := map[string]convention.RetrievedChunk{}
	for _, c := range chunks {
		if existing, ok := seen[c.ID]; ok {
			if c.Score > existing.Score {
				seen[c.ID] = c
			}
		} else {
			seen[c.ID] = c
		}
	}
	deduped := make([]convention.RetrievedChunk, 0, len(seen))
	for _, c := range seen {
		deduped = append(deduped, c)
	}
	// Sort by score descending.
	for i := 0; i < len(deduped); i++ {
		for j := i + 1; j < len(deduped); j++ {
			if deduped[j].Score > deduped[i].Score {
				deduped[i], deduped[j] = deduped[j], deduped[i]
			}
		}
	}
	if topK > 0 && len(deduped) > topK {
		deduped = deduped[:topK]
	}
	return deduped
}

func chunkIDsFromItems(items []RetrievedItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ChunkID
	}
	return ids
}
