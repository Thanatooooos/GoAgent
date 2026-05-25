package recall

import (
	"context"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
)

const factMemoryChunkPrefix = "memory_fact:"

var _ ragretrieve.FactMemoryRetriever = (*recallService)(nil)

func (r *recallService) SearchFacts(ctx context.Context, request ragretrieve.FactMemorySearchRequest) (ragretrieve.FactMemorySearchResult, error) {
	if r == nil || r.repo == nil {
		return ragretrieve.FactMemorySearchResult{}, nil
	}

	userID := strings.TrimSpace(request.UserID)
	if userID == "" {
		return ragretrieve.FactMemorySearchResult{}, exception.NewClientException("user id is required", nil)
	}

	query := strings.TrimSpace(request.Query)
	if query == "" {
		return ragretrieve.FactMemorySearchResult{}, nil
	}

	topK := request.TopK
	if topK <= 0 {
		topK = ragretrieve.DefaultTopK
	}
	candidateLimit := maxInt(r.options.MaxCandidatesPerScope, topK*4)
	knowledgeBaseIDs := trimMemoryValues(request.KnowledgeBaseIDs)

	ranked, candidateCount, _, _, _, _, err := r.loadFactRankingProjections(ctx, userID, query, knowledgeBaseIDs, candidateLimit)
	if err != nil {
		return ragretrieve.FactMemorySearchResult{}, err
	}
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}

	return ragretrieve.FactMemorySearchResult{
		Chunks:             projectFactMemoryChunks(ranked),
		CandidateCount:     candidateCount,
		SelectedCount:      len(ranked),
		SelectedMemoryIDs:  projectedMemoryIDs(ranked),
		ContributionCounts: projectedContributionCounts(ranked),
		SourceCounts:       projectedSourceCounts(ranked),
		ScopeCounts:        projectedScopeCounts(ranked),
	}, nil
}

func (r *recallService) loadFactMemoryCandidatesWithLimit(
	ctx context.Context,
	userID string,
	query string,
	knowledgeBaseIDs []string,
	limit int,
) ([]domain.MemoryItem, map[string]float32, string, error) {
	searchText := strings.TrimSpace(query)
	searchTokens := buildRecallSearchTokens(searchText)
	limit = normalizeMemoryCandidateLimit(limit)

	var kbItems []domain.MemoryItem
	var err error
	if len(knowledgeBaseIDs) > 0 {
		kbItems, err = r.repo.List(ctx, port.MemoryItemListFilter{
			UserID:       userID,
			ScopeTypes:   []string{domain.MemoryScopeKB},
			ScopeIDs:     knowledgeBaseIDs,
			MemoryTypes:  []string{domain.MemoryTypeKnowledge},
			Statuses:     []string{domain.MemoryStatusActive},
			SearchText:   searchText,
			SearchTokens: searchTokens,
			ListOptions: port.ListOptions{
				Limit: limit,
			},
		})
		if err != nil {
			return nil, nil, "skipped", exception.NewServiceException("failed to list kb fact memory items", err)
		}
	}

	globalItems, err := r.repo.List(ctx, port.MemoryItemListFilter{
		UserID:       userID,
		ScopeTypes:   []string{domain.MemoryScopeGlobal},
		MemoryTypes:  []string{domain.MemoryTypeKnowledge},
		Statuses:     []string{domain.MemoryStatusActive},
		SearchText:   searchText,
		SearchTokens: searchTokens,
		ListOptions: port.ListOptions{
			Limit: limit,
		},
	})
	if err != nil {
		return nil, nil, "skipped", exception.NewServiceException("failed to list global fact memory items", err)
	}

	candidates := dedupeMemoryItems(append(append([]domain.MemoryItem(nil), kbItems...), globalItems...))
	vectorScores := map[string]float32{}
	if searchText == "" || r.embeddingRepo == nil || r.embedding == nil {
		return candidates, vectorScores, "skipped", nil
	}

	vector, embeddingLayer, err := r.embedQuery(ctx, searchText)
	if err != nil || len(vector) == 0 {
		return candidates, vectorScores, embeddingLayer, nil
	}

	if len(knowledgeBaseIDs) > 0 {
		kbHits, err := r.embeddingRepo.SearchByVector(ctx, vector, port.MemoryItemEmbeddingSearchFilter{
			UserID:      userID,
			ScopeTypes:  []string{domain.MemoryScopeKB},
			ScopeIDs:    knowledgeBaseIDs,
			MemoryTypes: []string{domain.MemoryTypeKnowledge},
			Statuses:    []string{domain.MemoryStatusActive},
			TopK:        limit,
		})
		if err == nil {
			candidates = mergeMemorySearchHits(candidates, kbHits, vectorScores)
		}
	}

	globalHits, err := r.embeddingRepo.SearchByVector(ctx, vector, port.MemoryItemEmbeddingSearchFilter{
		UserID:      userID,
		ScopeTypes:  []string{domain.MemoryScopeGlobal},
		MemoryTypes: []string{domain.MemoryTypeKnowledge},
		Statuses:    []string{domain.MemoryStatusActive},
		TopK:        limit,
	})
	if err == nil {
		candidates = mergeMemorySearchHits(candidates, globalHits, vectorScores)
	}

	return candidates, vectorScores, embeddingLayer, nil
}

func dedupeMemoryItems(items []domain.MemoryItem) []domain.MemoryItem {
	if len(items) <= 1 {
		return items
	}
	result := make([]domain.MemoryItem, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		result = append(result, item)
	}
	return result
}

func normalizeMemoryCandidateLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return memorytypes.DefaultMemoryRecallItems * 4
}

func projectFactMemoryChunks(items []memoryRecallProjection) []convention.RetrievedChunk {
	if len(items) == 0 {
		return nil
	}
	chunks := make([]convention.RetrievedChunk, 0, len(items))
	for _, item := range items {
		text := renderFactMemoryChunkText(item)
		if strings.TrimSpace(text) == "" {
			continue
		}
		chunks = append(chunks, convention.RetrievedChunk{
			ID:              factMemoryChunkID(item.item.ID),
			Text:            text,
			Score:           float32(item.finalScore),
			DocumentID:      strings.TrimSpace(item.item.ID),
			KnowledgeBaseID: factMemoryKnowledgeBaseID(item.item),
			ChunkIndex:      0,
			Metadata:        buildFactMemoryChunkMetadata(item),
		})
	}
	return chunks
}

func factMemoryChunkID(memoryID string) string {
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return factMemoryChunkPrefix
	}
	return factMemoryChunkPrefix + memoryID
}

func factMemoryKnowledgeBaseID(item domain.MemoryItem) string {
	if strings.TrimSpace(item.ScopeType) != domain.MemoryScopeKB {
		return ""
	}
	return strings.TrimSpace(item.ScopeID)
}

func renderFactMemoryChunkText(item memoryRecallProjection) string {
	parts := make([]string, 0, 3)
	if summary := strings.TrimSpace(item.summary); summary != "" {
		parts = append(parts, summary)
	}
	if detail := strings.TrimSpace(item.detail); detail != "" && detail != strings.TrimSpace(item.summary) {
		parts = append(parts, "Detail: "+detail)
	}
	if displayValue := strings.TrimSpace(item.item.DisplayValue); displayValue != "" && !strings.Contains(strings.Join(parts, "\n"), displayValue) {
		parts = append(parts, "Value: "+displayValue)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildFactMemoryChunkMetadata(item memoryRecallProjection) map[string]any {
	return map[string]any{
		"source":            ragretrieve.ChannelMemoryFact,
		"memory_id":         strings.TrimSpace(item.item.ID),
		"memory_type":       strings.TrimSpace(item.item.MemoryType),
		"scope_type":        strings.TrimSpace(item.item.ScopeType),
		"scope_id":          strings.TrimSpace(item.item.ScopeID),
		"namespace":         strings.TrimSpace(item.item.Namespace),
		"category":          strings.TrimSpace(item.item.Category),
		"canonical_key":     strings.TrimSpace(item.item.CanonicalKey),
		"display_value":     strings.TrimSpace(item.item.DisplayValue),
		"section":           renderFactMemorySection(item.item),
		"hit_sources":       memoryHitSources(item),
		"contribution_kind": memoryContributionKind(item),
		"keyword_score":     item.keywordScore,
		"vector_score":      item.vectorScore,
		"final_score":       item.finalScore,
	}
}

func renderFactMemorySection(item domain.MemoryItem) string {
	parts := []string{"Fact Memory"}
	switch strings.TrimSpace(item.ScopeType) {
	case domain.MemoryScopeKB:
		parts = append(parts, "KB Scoped")
	case domain.MemoryScopeGlobal:
		parts = append(parts, "Global")
	default:
		parts = append(parts, "Other")
	}
	if key := strings.TrimSpace(item.CanonicalKey); key != "" {
		parts = append(parts, key)
	} else if category := strings.TrimSpace(item.Category); category != "" {
		parts = append(parts, category)
	}
	return strings.TrimSpace(strings.Join(parts, " > "))
}

var _ ragretrieve.FactMemoryRetriever = (*recallService)(nil)
