package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/exception"
)

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
			subQuestions := ragretrieve.BuildRetrieveSubQuestions(input.Question, rewriteResult.SubQuestions)
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
	return ragretrieve.BuildRetrieveSubQuestions(question, subQuestions)
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
