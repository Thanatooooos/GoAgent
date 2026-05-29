package service

import (
	"context"
	"errors"
	"sync"
	"strings"
	"time"
	"unicode/utf8"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
)

const defaultTopK = 5

const (
	subQuestionStatusSuccess = "success"
	subQuestionStatusEmpty   = "empty"
	subQuestionStatusFailed  = "failed"
	subQuestionStatusCancel  = "cancelled"

	retrieveExecutionModeSerial                 = "serial"
	retrieveExecutionModeParallel               = "parallel"
	retrieveExecutionModeSerialDependencyRisk   = "serial_due_to_dependency_risk"
	defaultSubquestionConcurrency               = 2
)

type subQuestionRetrieveResult struct {
	Query        string
	Status       string
	DurationMs   int64
	ChunkCount   int
	Error        string
	FallbackUsed bool
	Result       ragretrieve.Result
}

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
			startedAt := time.Now()
			subQuestions := normalizedRetrieveSubQuestions(rewriteResult.SubQuestions, input.Question)
			executionMode := s.determineSubQuestionExecutionMode(subQuestions)

			var subResults []subQuestionRetrieveResult
			if executionMode == retrieveExecutionModeParallel {
				subResults = s.retrieveSubQuestionsParallel(ctx, input, subQuestions)
			} else {
				subResults = s.retrieveSubQuestionsSerial(ctx, input, subQuestions)
			}

			successResults := collectSuccessfulRetrieveResults(subResults)
			if len(successResults) > 0 {
				merged := ragretrieve.MergeResults(successResults, defaultTopK)
				return ragChatRetrieveStageResult{
					result:              merged,
					used:                true,
					executionMode:       executionMode,
					wallClockDurationMs: time.Since(startedAt).Milliseconds(),
					subQuestions:        subResults,
				}, nil
			}

			retrieveResult, err := s.retrieveOriginalQuestionFallback(ctx, input)
			if err != nil {
				return ragChatRetrieveStageResult{}, exception.NewServiceException("failed to retrieve rag knowledge", err)
			}
			return ragChatRetrieveStageResult{
				result:              retrieveResult,
				used:                true,
				executionMode:       executionMode,
				wallClockDurationMs: time.Since(startedAt).Milliseconds(),
				subQuestions:        subResults,
			}, nil
		},
		buildExtra: func(result ragChatRetrieveStageResult) map[string]any {
			extra := map[string]any{
				"used":                result.used,
				"chunkCount":          len(result.result.Chunks),
				"topScore":            topChunkScore(result.result),
				"executionMode":       strings.TrimSpace(result.executionMode),
				"wallClockDurationMs": result.wallClockDurationMs,
			}
			appendSubQuestionTraceExtra(extra, result.subQuestions)
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

func normalizedRetrieveSubQuestions(subQuestions []string, question string) []string {
	normalized := make([]string, 0, len(subQuestions))
	for _, q := range subQuestions {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		normalized = append(normalized, q)
	}
	if len(normalized) > 0 {
		return normalized
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil
	}
	return []string{question}
}

func (s *RagChatService) determineSubQuestionExecutionMode(subQuestions []string) string {
	if len(subQuestions) <= 1 || !s.parallelSubquestions || s.subquestionMaxConcurrency() <= 1 {
		return retrieveExecutionModeSerial
	}
	if shouldSerializeSubQuestions(subQuestions) {
		return retrieveExecutionModeSerialDependencyRisk
	}
	return retrieveExecutionModeParallel
}

func (s *RagChatService) subquestionMaxConcurrency() int {
	if s == nil || s.subquestionConcurrency <= 0 {
		return defaultSubquestionConcurrency
	}
	return s.subquestionConcurrency
}

func shouldSerializeSubQuestions(subQuestions []string) bool {
	for _, q := range subQuestions {
		if isDependencyRiskSubQuestion(q) {
			return true
		}
	}
	return false
}

func isDependencyRiskSubQuestion(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	lower := strings.ToLower(question)
	riskMarkers := []string{
		"这个", "它", "该", "其", "上面", "前面",
		"这个节点", "这个任务", "这个文档", "它的报错", "其错误",
		"this", "it", "that", "above", "previous",
		"this node", "this task", "this document", "its error",
	}
	for _, marker := range riskMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	shortRiskQuestions := []string{
		"报错是什么", "错误是什么", "在哪一步", "哪个节点", "为什么失败",
		"what error was returned", "which node failed", "where did it timeout", "why did it fail",
	}
	for _, marker := range shortRiskQuestions {
		if lower == marker {
			return true
		}
	}
	return utf8.RuneCountInString(question) <= 6 && !strings.ContainsAny(question, " ，。,.?!？!")
}

func (s *RagChatService) retrieveSubQuestionsSerial(ctx context.Context, input RagChatInput, subQuestions []string) []subQuestionRetrieveResult {
	results := make([]subQuestionRetrieveResult, 0, len(subQuestions))
	for _, q := range subQuestions {
		results = append(results, s.executeSingleSubQuestionRetrieve(ctx, input, q))
	}
	return results
}

func (s *RagChatService) retrieveSubQuestionsParallel(ctx context.Context, input RagChatInput, subQuestions []string) []subQuestionRetrieveResult {
	results := make([]subQuestionRetrieveResult, len(subQuestions))
	maxConcurrency := s.subquestionMaxConcurrency()
	if maxConcurrency > len(subQuestions) {
		maxConcurrency = len(subQuestions)
	}
	if maxConcurrency <= 1 {
		return s.retrieveSubQuestionsSerial(ctx, input, subQuestions)
	}

	type retrieveJob struct {
		index int
		query string
	}

	jobs := make(chan retrieveJob)
	var wg sync.WaitGroup
	for worker := 0; worker < maxConcurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results[job.index] = s.executeSingleSubQuestionRetrieve(ctx, input, job.query)
			}
		}()
	}

	for idx, q := range subQuestions {
		select {
		case <-ctx.Done():
			for remaining := idx; remaining < len(subQuestions); remaining++ {
				results[remaining] = subQuestionRetrieveResult{
					Query:        strings.TrimSpace(subQuestions[remaining]),
					Status:       subQuestionStatusCancel,
					Error:        ctx.Err().Error(),
					FallbackUsed: false,
				}
			}
			close(jobs)
			wg.Wait()
			return results
		case jobs <- retrieveJob{index: idx, query: q}:
		}
	}
	close(jobs)
	wg.Wait()
	return results
}

func (s *RagChatService) executeSingleSubQuestionRetrieve(ctx context.Context, input RagChatInput, question string) subQuestionRetrieveResult {
	question = strings.TrimSpace(question)
	startedAt := time.Now()
	result, err := s.retrieveService.Retrieve(ctx, ragretrieve.Request{
		UserID:           strings.TrimSpace(input.UserID),
		Query:            question,
		KnowledgeBaseIDs: input.KnowledgeBaseIDs,
		SearchMode:       ragretrieve.SearchModeHybrid,
	})
	durationMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		status := subQuestionStatusFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = subQuestionStatusCancel
		}
		return subQuestionRetrieveResult{
			Query:        question,
			Status:       status,
			DurationMs:   durationMs,
			Error:        err.Error(),
			FallbackUsed: false,
		}
	}

	status := subQuestionStatusSuccess
	if len(result.Chunks) == 0 {
		status = subQuestionStatusEmpty
	}
	return subQuestionRetrieveResult{
		Query:        question,
		Status:       status,
		DurationMs:   durationMs,
		ChunkCount:   len(result.Chunks),
		FallbackUsed: false,
		Result:       result,
	}
}

func collectSuccessfulRetrieveResults(subResults []subQuestionRetrieveResult) []ragretrieve.Result {
	results := make([]ragretrieve.Result, 0, len(subResults))
	for _, item := range subResults {
		if item.Status != subQuestionStatusSuccess {
			continue
		}
		results = append(results, item.Result)
	}
	return results
}

func appendSubQuestionTraceExtra(extra map[string]any, subResults []subQuestionRetrieveResult) {
	if len(subResults) == 0 {
		return
	}

	details := make([]map[string]any, 0, len(subResults))
	var succeeded, emptyCount, failed int
	for _, item := range subResults {
		switch item.Status {
		case subQuestionStatusSuccess:
			succeeded++
		case subQuestionStatusEmpty:
			emptyCount++
		case subQuestionStatusFailed, subQuestionStatusCancel:
			failed++
		}

		detail := map[string]any{
			"query":        item.Query,
			"status":       item.Status,
			"durationMs":   item.DurationMs,
			"chunkCount":   item.ChunkCount,
			"fallbackUsed": item.FallbackUsed,
		}
		if strings.TrimSpace(item.Error) != "" {
			detail["error"] = strings.TrimSpace(item.Error)
		}
		details = append(details, detail)
	}

	extra["subQuestionTotal"] = len(subResults)
	extra["subQuestionSucceeded"] = succeeded
	extra["subQuestionEmpty"] = emptyCount
	extra["subQuestionFailed"] = failed
	extra["partialFailure"] = failed > 0
	extra["subQuestions"] = details
}

func (s *RagChatService) retrieveOriginalQuestionFallback(ctx context.Context, input RagChatInput) (ragretrieve.Result, error) {
	return s.retrieveService.Retrieve(ctx, ragretrieve.Request{
		UserID:           strings.TrimSpace(input.UserID),
		Query:            strings.TrimSpace(input.Question),
		KnowledgeBaseIDs: input.KnowledgeBaseIDs,
		SearchMode:       ragretrieve.SearchModeHybrid,
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
