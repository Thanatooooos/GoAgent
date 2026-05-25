package service

import (
	"context"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
)

const defaultTopK = 5

func (s *RagChatService) prepareChat(ctx context.Context, input RagChatInput) (ragChatPreparedState, error) {
	conversationStage, err := s.runConversationStage(ctx, input)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	memoryStage, err := s.runMemoryStage(ctx, conversationStage.conversationID, strings.TrimSpace(input.UserID))
	if err != nil {
		return ragChatPreparedState{}, err
	}

	userMessageStage, err := s.runUserMessageStage(ctx, input, conversationStage.conversationID)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	runtimeStage, err := s.runRuntimeStage(ctx, input, conversationStage, userMessageStage)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	rewriteStage, err := s.runRewriteStage(ctx, input.Question, memoryStage.history, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}

	longTermMemoryStage, err := s.runLongTermMemoryStage(ctx, input, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		log.Warnf("rag chat long-term memory stage failed open: userID=%s traceID=%s err=%v", strings.TrimSpace(input.UserID), runtimeStage.state.traceID, err)
		longTermMemoryStage = ragChatLongTermMemoryStageResult{}
	}

	sessionRecallStage, err := s.runSessionRecallStage(ctx, conversationStage.conversationID, input, userMessageStage.message.ID, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		log.Warnf(
			"rag chat session recall stage failed open: conversationID=%s userID=%s traceID=%s err=%v",
			conversationStage.conversationID,
			strings.TrimSpace(input.UserID),
			runtimeStage.state.traceID,
			err,
		)
		sessionRecallStage = ragChatSessionRecallStageResult{}
	}

	retrieveStage, err := s.runRetrieveStage(ctx, input, rewriteStage.result, runtimeStage.state.traceID)
	if err != nil {
		return ragChatPreparedState{}, err
	}
	return ragChatPreparedState{
		state:          runtimeStage.state,
		history:        memoryStage.history,
		userMessage:    userMessageStage.message,
		rewriteResult:  rewriteStage.result,
		memoryContext:  longTermMemoryStage.result.Context,
		sessionRecall:  sessionRecallStage.result,
		sessionContext: sessionRecallStage.result.Context,
		retrieveResult: retrieveStage.result,
		retrievalUsed:  retrieveStage.used,
	}, nil
}

func (s *RagChatService) runConversationStage(ctx context.Context, input RagChatInput) (ragChatConversationStageResult, error) {
	question := strings.TrimSpace(input.Question)
	userID := strings.TrimSpace(input.UserID)
	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		nextID, err := nextConversationExternalID()
		if err != nil {
			return ragChatConversationStageResult{}, err
		}
		conversationID = nextID
	}

	conversation, err := s.conversationService.CreateOrUpdate(ctx, CreateOrUpdateConversationInput{
		ConversationID: conversationID,
		UserID:         userID,
		Question:       question,
	})
	if err != nil {
		return ragChatConversationStageResult{}, err
	}

	return ragChatConversationStageResult{
		conversationID: conversationID,
		conversation:   conversation,
	}, nil
}

func (s *RagChatService) runMemoryStage(ctx context.Context, conversationID string, userID string) (ragChatMemoryStageResult, error) {
	history, err := s.historyService.Load(ctx, conversationID, userID)
	if err != nil {
		return ragChatMemoryStageResult{}, exception.NewServiceException("failed to load rag memory", err)
	}
	return ragChatMemoryStageResult{history: history}, nil
}

func (s *RagChatService) runUserMessageStage(ctx context.Context, input RagChatInput, conversationID string) (ragChatUserMessageStageResult, error) {
	userMessage, err := s.messageService.AddMessage(ctx, AddConversationMessageInput{
		ConversationID: conversationID,
		UserID:         strings.TrimSpace(input.UserID),
		Role:           convention.UserRole,
		Content:        strings.TrimSpace(input.Question),
	})
	if err != nil {
		return ragChatUserMessageStageResult{}, err
	}
	return ragChatUserMessageStageResult{message: userMessage}, nil
}

func (s *RagChatService) runRuntimeStage(
	ctx context.Context,
	input RagChatInput,
	conversationStage ragChatConversationStageResult,
	userMessageStage ragChatUserMessageStageResult,
) (ragChatRuntimeStageResult, error) {
	traceID, err := nextRagTraceID()
	if err != nil {
		return ragChatRuntimeStageResult{}, err
	}
	taskID, err := nextRagTaskID()
	if err != nil {
		return ragChatRuntimeStageResult{}, err
	}

	state := ragChatRuntimeState{
		meta: RagChatMeta{
			ConversationID: conversationStage.conversationID,
			TaskID:         taskID,
		},
		title:         conversationStage.conversation.Title,
		userMessageID: userMessageStage.message.ID,
		traceID:       traceID,
		startTime:     s.tracer.now(),
	}
	_ = s.tracer.startTraceRunAt(ctx, traceID, conversationStage.conversationID, taskID, strings.TrimSpace(input.UserID), state.startTime)

	return ragChatRuntimeStageResult{state: state}, nil
}

func (s *RagChatService) runRewriteStage(ctx context.Context, question string, history []convention.ChatMessage, traceID string) (ragChatRewriteStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatRewriteStageResult]{
		node: ragChatTraceNode{
			NodeID:   "rewrite",
			NodeType: "rewrite",
			NodeName: "query_rewrite",
		},
		run: func(context.Context) (ragChatRewriteStageResult, error) {
			if s.rewriteService == nil {
				result := ragrewrite.Result{
					RewrittenQuestion: question,
					SubQuestions:      []string{question},
					NeedRetrieval:     ragrewrite.InferNeedRetrieval(question),
				}
				return ragChatRewriteStageResult{result: result}, nil
			}
			result := s.rewriteService.RewriteWithHistory(question, history)
			return ragChatRewriteStageResult{result: result}, nil
		},
		buildExtra: func(result ragChatRewriteStageResult) map[string]any {
			return map[string]any{
				"subQuestionCount": len(result.result.SubQuestions),
			}
		},
	})
}

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
				"candidateCount":         result.result.candidateCount,
				"excerptCount":           len(result.result.Hits),
				"topScore":               result.result.TopScore,
				"cacheEnabled":           result.result.CacheEnabled,
				"cacheLayer":             strings.TrimSpace(result.result.CacheLayer),
				"recallFingerprint":      strings.TrimSpace(result.result.RecallFingerprint),
				"embeddingCacheLayer":    strings.TrimSpace(result.result.EmbeddingCacheLayer),
				"recomputeReason":        strings.TrimSpace(result.result.RecomputeReason),
				"excludedMessageId":      strings.TrimSpace(excludeMessageID),
				"selectedHits":           hits,
				"skippedPerMessageLimit": result.result.skippedPerMessageLimit,
				"truncatedBy":            strings.TrimSpace(result.result.truncatedBy),
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

func (s *RagChatService) runRetrieveStage(ctx context.Context, input RagChatInput, rewriteResult ragrewrite.Result, traceID string) (ragChatRetrieveStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatRetrieveStageResult]{
		node: ragChatTraceNode{
			NodeID:   "retrieve",
			NodeType: "retrieve",
			NodeName: "vector_retrieve",
		},
		run: func(ctx context.Context) (ragChatRetrieveStageResult, error) {
			if !shouldRunRetrieve(input, rewriteResult) {
				return ragChatRetrieveStageResult{}, nil
			}
			subQuestions := rewriteResult.SubQuestions
			if len(subQuestions) == 0 {
				subQuestions = []string{strings.TrimSpace(input.Question)}
			}

			results := make([]ragretrieve.Result, 0, len(subQuestions))
			for _, q := range subQuestions {
				retrieveResult, err := s.retrieveService.Retrieve(ctx, ragretrieve.Request{
					UserID:           strings.TrimSpace(input.UserID),
					Query:            strings.TrimSpace(q),
					KnowledgeBaseIDs: input.KnowledgeBaseIDs,
					SearchMode:       ragretrieve.SearchModeHybrid,
				})
				if err != nil {
					continue
				}
				results = append(results, retrieveResult)
			}
			if len(results) == 0 {
				retrieveResult, err := s.retrieveService.Retrieve(ctx, ragretrieve.Request{
					UserID:           strings.TrimSpace(input.UserID),
					Query:            strings.TrimSpace(input.Question),
					KnowledgeBaseIDs: input.KnowledgeBaseIDs,
					SearchMode:       ragretrieve.SearchModeHybrid,
				})
				if err != nil {
					return ragChatRetrieveStageResult{}, exception.NewServiceException("failed to retrieve rag knowledge", err)
				}
				return ragChatRetrieveStageResult{result: retrieveResult, used: true}, nil
			}

			merged := ragretrieve.MergeResults(results, defaultTopK)
			return ragChatRetrieveStageResult{result: merged, used: true}, nil
		},
		buildExtra: func(result ragChatRetrieveStageResult) map[string]any {
			extra := map[string]any{
				"used":       result.used,
				"chunkCount": len(result.result.Chunks),
				"topScore":   topChunkScore(result.result),
			}
			if len(result.result.SearchChannels) > 0 {
				extra["searchChannels"] = append([]string(nil), result.result.SearchChannels...)
			}

			if len(result.result.ChannelStats) > 0 {
				stats := make([]map[string]any, 0, len(result.result.ChannelStats))
				for _, stat := range result.result.ChannelStats {
					item := map[string]any{
						"name":       stat.Name,
						"chunkCount": stat.ChunkCount,
						"latencyMs":  stat.LatencyMs,
					}
					if stat.Error != "" {
						item["error"] = stat.Error
					}
					if len(stat.Metadata) > 0 {
						item["metadata"] = stat.Metadata
					}
					stats = append(stats, item)
				}
				extra["channelStats"] = stats
			}
			return extra
		},
	})
}

func shouldRunRetrieve(input RagChatInput, rewriteResult ragrewrite.Result) bool {
	if len(input.KnowledgeBaseIDs) == 0 {
		return false
	}
	return rewriteResult.NeedRetrieval
}

func topChunkScore(result ragretrieve.Result) float32 {
	if len(result.Chunks) == 0 {
		return 0
	}
	maxScore := result.Chunks[0].Score
	for _, c := range result.Chunks[1:] {
		if c.Score > maxScore {
			maxScore = c.Score
		}
	}
	return maxScore
}

func shouldRunToolWorkflow(input RagChatInput, rewriteResult ragrewrite.Result, retrievalUsed bool) bool {
	if retrievalUsed {
		return true
	}
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return false
	}
	if ragtool.FirstMatchedID(ragtool.DocumentIDPattern, question) != "" {
		return true
	}
	if ragtool.FirstMatchedID(ragtool.TaskIDPattern, question) != "" {
		return true
	}
	if ragtool.FirstMatchedID(ragtool.TraceIDPattern, question) != "" {
		return true
	}
	return rewriteResult.NeedRetrieval
}
