package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/exception"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type ExplicitMemoryRecallService interface {
	RecallMemories(ctx context.Context, input RecallMemoriesInput) (RecallMemoriesResult, error)
}

const (
	memoryHitSourceKeyword           = "keyword"
	memoryHitSourceVector            = "vector"
	memoryContributionKeywordOnly    = "keyword_only"
	memoryContributionVectorOnly     = "vector_only"
	memoryContributionHybrid         = "hybrid"
	memoryContributionNoDirectSignal = "none"
)

type memoryRecallRetriever struct {
	repo          port.MemoryItemRepository
	embeddingRepo port.MemoryItemEmbeddingRepository
	embedding     aiembedding.EmbeddingService
	options       MemoryServiceOptions
}

func newMemoryRecallRetriever(repo port.MemoryItemRepository, options MemoryServiceOptions) *memoryRecallRetriever {
	return NewVectorAwareMemoryRecallRetriever(repo, nil, nil, options)
}

func NewVectorAwareMemoryRecallRetriever(
	repo port.MemoryItemRepository,
	embeddingRepo port.MemoryItemEmbeddingRepository,
	embedding aiembedding.EmbeddingService,
	options MemoryServiceOptions,
) *memoryRecallRetriever {
	if options.MaxRecallItems <= 0 {
		options.MaxRecallItems = defaultMemoryRecallItems
	}
	if options.MaxRecallChars <= 0 {
		options.MaxRecallChars = defaultMemoryRecallMaxChars
	}
	if options.MaxCandidatesPerScope <= 0 {
		options.MaxCandidatesPerScope = options.MaxRecallItems * 4
	}
	return &memoryRecallRetriever{
		repo:          repo,
		embeddingRepo: embeddingRepo,
		embedding:     embedding,
		options:       options,
	}
}

func (r *memoryRecallRetriever) RecallMemories(ctx context.Context, input RecallMemoriesInput) (RecallMemoriesResult, error) {
	if r == nil || r.repo == nil {
		return RecallMemoriesResult{}, nil
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return RecallMemoriesResult{}, exception.NewClientException("user id is required", nil)
	}

	kbItems, err := r.repo.List(ctx, port.MemoryItemListFilter{
		UserID:     userID,
		ScopeTypes: []string{domain.MemoryScopeKB},
		ScopeIDs:   trimMemoryValues(input.KnowledgeBaseIDs),
		Statuses:   []string{domain.MemoryStatusActive},
		ListOptions: port.ListOptions{
			Limit: r.options.MaxCandidatesPerScope,
		},
	})
	if err != nil {
		return RecallMemoriesResult{}, exception.NewServiceException("failed to list kb memory items", err)
	}
	globalItems, err := r.repo.List(ctx, port.MemoryItemListFilter{
		UserID:     userID,
		ScopeTypes: []string{domain.MemoryScopeGlobal},
		Statuses:   []string{domain.MemoryStatusActive},
		ListOptions: port.ListOptions{
			Limit: r.options.MaxCandidatesPerScope,
		},
	})
	if err != nil {
		return RecallMemoriesResult{}, exception.NewServiceException("failed to list global memory items", err)
	}

	query := strings.TrimSpace(input.Query)
	candidates := append(append([]domain.MemoryItem(nil), kbItems...), globalItems...)
	if len(candidates) == 0 {
		return RecallMemoriesResult{}, nil
	}

	vectorScores := map[string]float32{}
	if query != "" && r.embeddingRepo != nil && r.embedding != nil {
		vector, err := r.embedding.Embed(query)
		if err == nil && len(vector) > 0 {
			kbHits, err := r.embeddingRepo.SearchByVector(ctx, vector, port.MemoryItemEmbeddingSearchFilter{
				UserID:     userID,
				ScopeTypes: []string{domain.MemoryScopeKB},
				ScopeIDs:   trimMemoryValues(input.KnowledgeBaseIDs),
				Statuses:   []string{domain.MemoryStatusActive},
				TopK:       r.options.MaxCandidatesPerScope,
			})
			if err == nil {
				candidates = mergeMemorySearchHits(candidates, kbHits, vectorScores)
			}
			globalHits, err := r.embeddingRepo.SearchByVector(ctx, vector, port.MemoryItemEmbeddingSearchFilter{
				UserID:     userID,
				ScopeTypes: []string{domain.MemoryScopeGlobal},
				Statuses:   []string{domain.MemoryStatusActive},
				TopK:       r.options.MaxCandidatesPerScope,
			})
			if err == nil {
				candidates = mergeMemorySearchHits(candidates, globalHits, vectorScores)
			}
		}
	}

	ranked := rankRecallMemories(query, candidates, vectorScores)
	if len(vectorScores) > 0 {
		ranked = rerankRecallMemoriesWithVectorScores(ranked, vectorScores)
	}
	selected, contextText, truncated := buildMemoryRecallContext(ranked, r.options.MaxRecallItems, r.options.MaxRecallChars)
	return RecallMemoriesResult{
		Used:               len(selected) > 0,
		Context:            contextText,
		Items:              projectedMemoryItems(selected),
		SelectedEntries:    projectedMemoryEntries(selected),
		CandidateCount:     len(ranked),
		SelectedCount:      len(selected),
		Truncated:          truncated,
		ScopeCounts:        projectedScopeCounts(selected),
		SourceCounts:       projectedSourceCounts(selected),
		ContributionCounts: projectedContributionCounts(selected),
		TypeCounts:         projectedTypeCounts(selected),
		SelectedMemoryIDs:  projectedMemoryIDs(selected),
	}, nil
}

type scoredMemoryItem struct {
	item       domain.MemoryItem
	projection memoryRecallProjection
}

func rankRecallMemories(query string, items []domain.MemoryItem, vectorScores map[string]float32) []memoryRecallProjection {
	scored := make([]scoredMemoryItem, 0, len(items))
	queryPresent := strings.TrimSpace(query) != ""
	for _, item := range items {
		projection := buildMemoryRecallProjection(query, item)
		matchScore, matched := scoreMemoryText(query, projection.searchableText)
		vectorScore := vectorScores[strings.TrimSpace(item.ID)]
		if item.MemoryType == domain.MemoryTypeKnowledge && query != "" && !matched && vectorScore <= 0 {
			continue
		}
		score := matchScore + memoryScopePriority(item.ScopeType) + memoryTypePriority(item.MemoryType)
		if item.LastConfirmedAt != nil {
			score += 5
		}
		projection.keywordMatched = queryPresent && matched
		projection.keywordScore = matchScore
		if vectorScore > 0 {
			projection.vectorMatched = true
			projection.vectorScore = vectorScore
		}
		projection.finalScore = score
		scored = append(scored, scoredMemoryItem{item: item, projection: projection})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].projection.finalScore != scored[j].projection.finalScore {
			return scored[i].projection.finalScore > scored[j].projection.finalScore
		}
		if !scored[i].item.UpdateTime.Equal(scored[j].item.UpdateTime) {
			return scored[i].item.UpdateTime.After(scored[j].item.UpdateTime)
		}
		return scored[i].item.ID > scored[j].item.ID
	})
	result := make([]memoryRecallProjection, 0, len(scored))
	seen := make(map[string]struct{}, len(scored))
	for _, item := range scored {
		if _, ok := seen[item.item.ID]; ok {
			continue
		}
		seen[item.item.ID] = struct{}{}
		result = append(result, item.projection)
	}
	return result
}

func rerankRecallMemoriesWithVectorScores(items []memoryRecallProjection, vectorScores map[string]float32) []memoryRecallProjection {
	if len(items) == 0 || len(vectorScores) == 0 {
		return items
	}
	ranked := append([]memoryRecallProjection(nil), items...)
	for idx := range ranked {
		if score, ok := vectorScores[strings.TrimSpace(ranked[idx].item.ID)]; ok && score > 0 {
			ranked[idx].vectorMatched = true
			ranked[idx].vectorScore = score
			ranked[idx].finalScore = computeFusedMemoryScore(ranked[idx], score)
			continue
		}
		ranked[idx].finalScore = computeFusedMemoryScore(ranked[idx], 0)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].finalScore != ranked[j].finalScore {
			return ranked[i].finalScore > ranked[j].finalScore
		}
		if !ranked[i].item.UpdateTime.Equal(ranked[j].item.UpdateTime) {
			return ranked[i].item.UpdateTime.After(ranked[j].item.UpdateTime)
		}
		return ranked[i].item.ID > ranked[j].item.ID
	})
	return ranked
}

func computeFusedMemoryScore(item memoryRecallProjection, vectorScore float32) int {
	score := item.keywordScore + memoryScopePriority(item.item.ScopeType) + memoryTypePriority(item.item.MemoryType)
	if item.item.LastConfirmedAt != nil {
		score += 5
	}
	if vectorScore <= 0 {
		return score
	}

	boost := int(vectorScore * 100)
	switch {
	case item.keywordMatched:
		boost += 30
	default:
		boost += 80
	}
	return score + boost
}

func buildMemoryRecallContext(items []memoryRecallProjection, maxItems int, maxChars int) ([]memoryRecallProjection, string, bool) {
	if len(items) == 0 || maxItems <= 0 || maxChars <= 0 {
		return nil, "", false
	}
	selected := make([]memoryRecallProjection, 0, minMemoryInt(len(items), maxItems))
	truncated := false
	for _, item := range items {
		if len(selected) >= maxItems {
			truncated = true
			break
		}
		candidate := append(append([]memoryRecallProjection(nil), selected...), item)
		contextText := renderMemoryRecallContext(candidate)
		if strings.TrimSpace(contextText) == "" {
			continue
		}
		if utf8.RuneCountInString(contextText) > maxChars {
			truncated = true
			break
		}
		selected = append(selected, item)
	}
	return selected, renderMemoryRecallContext(selected), truncated
}

func buildMemoryRecallProjection(query string, item domain.MemoryItem) memoryRecallProjection {
	summary := strings.TrimSpace(item.Summary)
	if summary == "" {
		summary = summarizeMemoryText(item.Content, defaultMemorySummaryRunes)
	}
	detail := pickMemoryProjectionDetail(query, item, summary)
	searchParts := []string{
		summary,
		detail,
		strings.TrimSpace(item.Content),
		strings.TrimSpace(item.ScopeType),
		strings.TrimSpace(item.ScopeID),
		strings.TrimSpace(item.MemoryType),
	}
	return memoryRecallProjection{
		item:           item,
		summary:        summary,
		detail:         detail,
		searchableText: normalizeRecallText(strings.Join(searchParts, " ")),
	}
}

func scoreMemoryText(query string, text string) (int, bool) {
	query = normalizeRecallText(query)
	text = normalizeRecallText(text)
	if query == "" {
		return 1, true
	}
	if text == "" {
		return 0, false
	}
	score := 0
	matched := false
	if strings.Contains(text, query) {
		score += 120
		matched = true
	}
	for _, token := range extractRecallTokens(query) {
		if strings.Contains(text, token) {
			score += 20
			matched = true
		}
	}
	queryCompact := compactLowerString(query)
	textCompact := compactLowerString(text)
	if containsCJKString(queryCompact) {
		for _, bigram := range buildDistinctCJKBigrams(queryCompact) {
			if strings.Contains(textCompact, bigram) {
				score += 8
				matched = true
			}
		}
	}
	return score, matched
}

func pickMemoryProjectionDetail(query string, item domain.MemoryItem, summary string) string {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return ""
	}

	best := strings.TrimSpace(bestMemoryProjectionSegment(query, content))
	if best == "" {
		best = content
	}
	best = summarizeMemoryText(best, defaultMemoryDetailRunes)
	if strings.EqualFold(strings.TrimSpace(best), strings.TrimSpace(summary)) {
		return ""
	}
	return best
}

func bestMemoryProjectionSegment(query string, content string) string {
	segments := splitMemoryProjectionSegments(content)
	if len(segments) == 0 {
		return strings.TrimSpace(content)
	}

	bestSegment := strings.TrimSpace(segments[0])
	bestScore := -1
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		score, _ := scoreMemoryText(query, segment)
		if score > bestScore {
			bestScore = score
			bestSegment = segment
		}
	}
	return bestSegment
}

func splitMemoryProjectionSegments(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	segments := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		segments = append(segments, line)
	}
	if len(segments) > 0 {
		return segments
	}
	return []string{strings.TrimSpace(content)}
}

func renderMemoryRecallContext(items []memoryRecallProjection) string {
	if len(items) == 0 {
		return ""
	}

	sections := make([]string, 0, 2)
	for _, scopeType := range []string{domain.MemoryScopeKB, domain.MemoryScopeGlobal} {
		lines := make([]string, 0, len(items))
		for _, item := range items {
			if strings.TrimSpace(item.item.ScopeType) != scopeType {
				continue
			}
			line := renderMemoryContextEntry(item)
			if line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			continue
		}
		sections = append(sections, renderMemoryScopeSection(scopeType)+"\n"+strings.Join(lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func renderMemoryContextEntry(item memoryRecallProjection) string {
	summary := strings.TrimSpace(item.summary)
	if summary == "" {
		return ""
	}

	scope := renderMemoryScopeLabel(item.item)
	lines := []string{
		fmt.Sprintf("- [memory_id=%s scope=%s type=%s] %s", strings.TrimSpace(item.item.ID), scope, strings.TrimSpace(item.item.MemoryType), summary),
	}
	if detail := strings.TrimSpace(item.detail); detail != "" {
		lines = append(lines, "  Detail: "+detail)
	}
	return strings.Join(lines, "\n")
}

func renderMemoryScopeSection(scopeType string) string {
	switch strings.TrimSpace(scopeType) {
	case domain.MemoryScopeKB:
		return "KB-Scoped Memories:"
	case domain.MemoryScopeGlobal:
		return "Global Memories:"
	default:
		return "Other Memories:"
	}
}

func renderMemoryScopeLabel(item domain.MemoryItem) string {
	scope := strings.TrimSpace(item.ScopeType)
	scopeID := strings.TrimSpace(item.ScopeID)
	if scope != "" && scopeID != "" {
		return scope + ":" + scopeID
	}
	if scope != "" {
		return scope
	}
	if scopeID != "" {
		return scopeID
	}
	return "unknown"
}

func projectedMemoryItems(items []memoryRecallProjection) []domain.MemoryItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]domain.MemoryItem, 0, len(items))
	for _, item := range items {
		result = append(result, item.item)
	}
	return result
}

func projectedMemoryEntries(items []memoryRecallProjection) []RecallMemoryEntry {
	if len(items) == 0 {
		return nil
	}
	result := make([]RecallMemoryEntry, 0, len(items))
	for _, item := range items {
		result = append(result, RecallMemoryEntry{
			ID:           strings.TrimSpace(item.item.ID),
			ScopeType:    strings.TrimSpace(item.item.ScopeType),
			ScopeID:      strings.TrimSpace(item.item.ScopeID),
			MemoryType:   strings.TrimSpace(item.item.MemoryType),
			Summary:      strings.TrimSpace(item.summary),
			Detail:       strings.TrimSpace(item.detail),
			HitSources:   memoryHitSources(item),
			KeywordScore: item.keywordScore,
			VectorScore:  item.vectorScore,
			FinalScore:   item.finalScore,
		})
	}
	return result
}

func projectedScopeCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[strings.TrimSpace(item.item.ScopeType)]++
	}
	return counts
}

func projectedTypeCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[strings.TrimSpace(item.item.MemoryType)]++
	}
	return counts
}

func projectedSourceCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		for _, source := range memoryHitSources(item) {
			counts[source]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func projectedContributionCounts(items []memoryRecallProjection) map[string]int {
	if len(items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range items {
		counts[memoryContributionKind(item)]++
	}
	return counts
}

func projectedMemoryIDs(items []memoryRecallProjection) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, strings.TrimSpace(item.item.ID))
	}
	return result
}

func memoryHitSources(item memoryRecallProjection) []string {
	sources := make([]string, 0, 2)
	if item.keywordMatched {
		sources = append(sources, memoryHitSourceKeyword)
	}
	if item.vectorMatched {
		sources = append(sources, memoryHitSourceVector)
	}
	return sources
}

func memoryContributionKind(item memoryRecallProjection) string {
	switch {
	case item.keywordMatched && item.vectorMatched:
		return memoryContributionHybrid
	case item.keywordMatched:
		return memoryContributionKeywordOnly
	case item.vectorMatched:
		return memoryContributionVectorOnly
	default:
		return memoryContributionNoDirectSignal
	}
}

func mergeMemorySearchHits(items []domain.MemoryItem, hits []domain.MemoryItemSearchHit, vectorScores map[string]float32) []domain.MemoryItem {
	if len(hits) == 0 {
		return items
	}
	merged := append([]domain.MemoryItem(nil), items...)
	seen := make(map[string]struct{}, len(merged))
	for _, item := range merged {
		seen[strings.TrimSpace(item.ID)] = struct{}{}
	}
	for _, hit := range hits {
		id := strings.TrimSpace(hit.ID)
		if id == "" {
			continue
		}
		if current, ok := vectorScores[id]; !ok || hit.Score > current {
			vectorScores[id] = hit.Score
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, hit.MemoryItem)
	}
	return merged
}

func normalizeRecallText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func extractRecallTokens(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(parts) == 0 {
		return nil
	}
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if utf8.RuneCountInString(part) < 2 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func summarizeMemoryText(value string, maxRunes int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if value == "" {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = defaultMemorySummaryRunes
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
