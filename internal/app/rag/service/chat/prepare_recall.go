package chat

import (
	"context"
	"strings"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

func (s *RagChatService) runSessionRecallStage(
	ctx context.Context,
	conversationID string,
	input RagChatInput,
	excludeMessageID string,
	rewriteResult ragrewrite.Result,
	traceID string,
) (ragChatSessionRecallStageResult, error) {
	if s == nil || s.sessionRecall == nil {
		return ragChatSessionRecallStageResult{}, nil
	}

	query := strings.TrimSpace(rewriteResult.RewrittenQuestion)
	if query == "" {
		query = strings.TrimSpace(input.Question)
	}

	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatSessionRecallStageResult]{
		node: ragChatTraceNode{
			NodeID:   "session_recall",
			NodeType: "memory",
			NodeName: "session_chunk_recall",
		},
		run: func(ctx context.Context) (ragChatSessionRecallStageResult, error) {
			result, err := s.sessionRecall.Recall(ctx, SessionRecallInput{
				ConversationID:   strings.TrimSpace(conversationID),
				UserID:           strings.TrimSpace(input.UserID),
				Query:            query,
				ExcludeMessageID: strings.TrimSpace(excludeMessageID),
			})
			if err != nil {
				return ragChatSessionRecallStageResult{}, err
			}
			return ragChatSessionRecallStageResult{result: result}, nil
		},
		buildExtra: func(result ragChatSessionRecallStageResult) map[string]any {
			hits := make([]map[string]any, 0, len(result.result.Hits))
			for _, hit := range result.result.Hits {
				hits = append(hits, map[string]any{
					"messageId":     strings.TrimSpace(hit.MessageID),
					"chunkIndex":    hit.ChunkIndex,
					"score":         hit.Score,
					"sourceChunkId": strings.TrimSpace(hit.SourceChunkID),
				})
			}
			return map[string]any{
				"used":                   result.result.Used,
				"candidateCount":         result.result.CandidateCount,
				"excerptCount":           len(result.result.Hits),
				"topScore":               result.result.TopScore,
				"cacheEnabled":           result.result.CacheEnabled,
				"cacheLayer":             strings.TrimSpace(result.result.CacheLayer),
				"recallFingerprint":      strings.TrimSpace(result.result.RecallFingerprint),
				"embeddingCacheLayer":    strings.TrimSpace(result.result.EmbeddingCacheLayer),
				"recomputeReason":        strings.TrimSpace(result.result.RecomputeReason),
				"excludedMessageId":      strings.TrimSpace(excludeMessageID),
				"selectedHits":           hits,
				"skippedPerMessageLimit": result.result.SkippedPerMessageLimit,
				"truncatedBy":            strings.TrimSpace(result.result.TruncatedBy),
			}
		},
	})
}

func (s *RagChatService) runLongTermMemoryStage(
	ctx context.Context,
	input RagChatInput,
	rewriteResult ragrewrite.Result,
	traceID string,
) (ragChatLongTermMemoryStageResult, error) {
	if s == nil || s.longTermMemory == nil {
		return ragChatLongTermMemoryStageResult{}, nil
	}

	query := strings.TrimSpace(rewriteResult.RewrittenQuestion)
	if query == "" {
		query = strings.TrimSpace(input.Question)
	}

	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatLongTermMemoryStageResult]{
		node: ragChatTraceNode{
			NodeID:   "long_term_memory",
			NodeType: "memory",
			NodeName: "long_term_memory_recall",
		},
		run: func(ctx context.Context) (ragChatLongTermMemoryStageResult, error) {
			result, err := s.longTermMemory.RecallMemories(ctx, longtermmemory.RecallMemoriesInput{
				UserID:           strings.TrimSpace(input.UserID),
				Query:            query,
				KnowledgeBaseIDs: append([]string(nil), input.KnowledgeBaseIDs...),
				ScopeTypes:       []string{domain.MemoryScopeGlobal},
				MemoryTypes:      []string{domain.MemoryTypePreference},
				Statuses:         []string{domain.MemoryStatusActive},
			})
			if err != nil {
				return ragChatLongTermMemoryStageResult{}, err
			}
			return ragChatLongTermMemoryStageResult{result: result}, nil
		},
		buildExtra: func(result ragChatLongTermMemoryStageResult) map[string]any {
			selected := make([]map[string]any, 0, len(result.result.SelectedEntries))
			for _, item := range result.result.SelectedEntries {
				selected = append(selected, map[string]any{
					"id":           strings.TrimSpace(item.ID),
					"scopeType":    strings.TrimSpace(item.ScopeType),
					"scopeID":      strings.TrimSpace(item.ScopeID),
					"memoryType":   strings.TrimSpace(item.MemoryType),
					"summary":      strings.TrimSpace(item.Summary),
					"detail":       strings.TrimSpace(item.Detail),
					"hitSources":   append([]string(nil), item.HitSources...),
					"keywordScore": item.KeywordScore,
					"vectorScore":  item.VectorScore,
					"finalScore":   item.FinalScore,
				})
			}
			return map[string]any{
				"used":                result.result.Used,
				"candidateCount":      result.result.CandidateCount,
				"selectedCount":       result.result.SelectedCount,
				"ruleCount":           result.result.RuleCount,
				"factCandidateCount":  result.result.FactCandidateCount,
				"factSelectedCount":   result.result.FactSelectedCount,
				"cacheEnabled":        result.result.CacheEnabled,
				"ruleCacheLayer":      strings.TrimSpace(result.result.RuleCacheLayer),
				"factCacheLayer":      strings.TrimSpace(result.result.FactCacheLayer),
				"embeddingCacheLayer": strings.TrimSpace(result.result.EmbeddingCacheLayer),
				"scopeVersions": map[string]any{
					"global": result.result.ScopeVersions.GlobalVersion,
					"kb":     result.result.ScopeVersions.KBVersions,
				},
				"recomputeReason":    strings.TrimSpace(result.result.RecomputeReason),
				"truncated":          result.result.Truncated,
				"scopeCounts":        result.result.ScopeCounts,
				"sourceCounts":       result.result.SourceCounts,
				"contributionCounts": result.result.ContributionCounts,
				"typeCounts":         result.result.TypeCounts,
				"memoryIds":          result.result.SelectedMemoryIDs,
				"ruleMemoryIds":      result.result.RuleMemoryIDs,
				"factMemoryIds":      result.result.FactMemoryIDs,
				"selectedMemories":   selected,
			}
		},
	})
}
