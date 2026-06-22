package retrieve

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	ExecutionModeSerial               = "serial"
	ExecutionModeParallel             = "parallel"
	ExecutionModeSerialDependencyRisk = "serial_due_to_dependency_risk"

	defaultSubQuestionConcurrency = 2
	maxRetrieveSubQuestions         = 4
	originalSubQuestionRRFWeight    = float32(1.5)
)

type SubQuestionOptions struct {
	ParallelEnabled bool
	MaxConcurrency  int
}

type SubQuestionStatus string

const (
	SubQuestionStatusSuccess SubQuestionStatus = "success"
	SubQuestionStatusEmpty   SubQuestionStatus = "empty"
	SubQuestionStatusFailed  SubQuestionStatus = "failed"
	SubQuestionStatusCancel  SubQuestionStatus = "cancel"
)

type SubQuestionResult struct {
	Query      string
	Status     SubQuestionStatus
	DurationMs int64
	ChunkCount int
	Error      string
	Result     Result
}

type SubQuestionExecutor struct {
	retrieve        Service
	parallelEnabled bool
	maxConcurrency  int
}

func NewSubQuestionExecutor(retrieve Service, opts SubQuestionOptions) *SubQuestionExecutor {
	maxConcurrency := opts.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultSubQuestionConcurrency
	}
	return &SubQuestionExecutor{
		retrieve:        retrieve,
		parallelEnabled: opts.ParallelEnabled,
		maxConcurrency:  maxConcurrency,
	}
}

func NormalizeSubQuestions(subQuestions []string, fallbackQuestion string) []string {
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
	fallbackQuestion = strings.TrimSpace(fallbackQuestion)
	if fallbackQuestion == "" {
		return nil
	}
	return []string{fallbackQuestion}
}

// BuildRetrieveSubQuestions keeps the original user query as the first retrieval
// anchor, then appends rewrite sub-questions without duplicates.
// When the original query is a pronoun-led follow-up and rewrite produced
// standalone sub-questions, the original is omitted so retrieval uses the
// resolved rewrite instead of the ambiguous short query.
func BuildRetrieveSubQuestions(original string, rewriteSubQuestions []string) []string {
	original = strings.TrimSpace(original)
	rewriteSubQuestions = NormalizeSubQuestions(rewriteSubQuestions, original)

	result := make([]string, 0, maxRetrieveSubQuestions)
	seen := map[string]struct{}{}
	appendQuestion := func(question string) {
		question = strings.TrimSpace(question)
		if question == "" {
			return
		}
		key := strings.ToLower(question)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		result = append(result, question)
	}

	skipOriginal := len(rewriteSubQuestions) > 0 && isDependencyRiskSubQuestion(original)
	if !skipOriginal {
		appendQuestion(original)
	}
	for _, question := range rewriteSubQuestions {
		appendQuestion(question)
		if len(result) >= maxRetrieveSubQuestions {
			break
		}
	}
	if len(result) == 0 && original != "" {
		return []string{original}
	}
	return result
}

func subQuestionMergeWeight(index int) float32 {
	if index == 0 {
		return originalSubQuestionRRFWeight
	}
	return 1.0
}

func (e *SubQuestionExecutor) RetrieveMerged(
	ctx context.Context,
	request Request,
	subQuestions []string,
	mergeTopK int,
) (Result, string, []SubQuestionResult, error) {
	if e == nil || e.retrieve == nil {
		return Result{}, "", nil, errors.New("sub-question retrieve executor is required")
	}

	subQuestions = NormalizeSubQuestions(subQuestions, request.Query)
	if len(subQuestions) == 0 {
		return Result{}, ExecutionModeSerial, nil, nil
	}

	executionMode := e.determineExecutionMode(subQuestions)
	var subResults []SubQuestionResult
	switch executionMode {
	case ExecutionModeParallel:
		subResults = e.retrieveParallel(ctx, request, subQuestions)
	default:
		subResults = e.retrieveSerial(ctx, request, subQuestions)
	}

	successResults := collectSuccessfulSubQuestionResults(subResults)
	if len(successResults) > 0 {
		if mergeTopK <= 0 {
			mergeTopK = DefaultTopK
		}
		return MergeResults(successResults, mergeTopK), executionMode, subResults, nil
	}

	fallback, err := e.retrieve.Retrieve(ctx, request)
	return fallback, executionMode, subResults, err
}

func (e *SubQuestionExecutor) determineExecutionMode(subQuestions []string) string {
	if len(subQuestions) <= 1 || !e.parallelEnabled || e.effectiveMaxConcurrency() <= 1 {
		return ExecutionModeSerial
	}
	if shouldSerializeSubQuestions(subQuestions) {
		return ExecutionModeSerialDependencyRisk
	}
	return ExecutionModeParallel
}

func (e *SubQuestionExecutor) effectiveMaxConcurrency() int {
	if e == nil || e.maxConcurrency <= 0 {
		return defaultSubQuestionConcurrency
	}
	return e.maxConcurrency
}

func (e *SubQuestionExecutor) retrieveSerial(ctx context.Context, request Request, subQuestions []string) []SubQuestionResult {
	results := make([]SubQuestionResult, 0, len(subQuestions))
	for _, q := range subQuestions {
		results = append(results, e.executeSingle(ctx, request, q))
	}
	return results
}

func (e *SubQuestionExecutor) retrieveParallel(ctx context.Context, request Request, subQuestions []string) []SubQuestionResult {
	results := make([]SubQuestionResult, len(subQuestions))
	maxConcurrency := e.effectiveMaxConcurrency()
	if maxConcurrency > len(subQuestions) {
		maxConcurrency = len(subQuestions)
	}
	if maxConcurrency <= 1 {
		return e.retrieveSerial(ctx, request, subQuestions)
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
				results[job.index] = e.executeSingle(ctx, request, job.query)
			}
		}()
	}

	for idx, q := range subQuestions {
		select {
		case <-ctx.Done():
			for remaining := idx; remaining < len(subQuestions); remaining++ {
				results[remaining] = SubQuestionResult{
					Query:  strings.TrimSpace(subQuestions[remaining]),
					Status: SubQuestionStatusCancel,
					Error:  ctx.Err().Error(),
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

func (e *SubQuestionExecutor) executeSingle(ctx context.Context, request Request, question string) SubQuestionResult {
	question = strings.TrimSpace(question)
	startedAt := time.Now()
	subRequest := request
	subRequest.Query = question

	result, err := e.retrieve.Retrieve(ctx, subRequest)
	durationMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		status := SubQuestionStatusFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = SubQuestionStatusCancel
		}
		return SubQuestionResult{
			Query:      question,
			Status:     status,
			DurationMs: durationMs,
			Error:      err.Error(),
		}
	}

	status := SubQuestionStatusSuccess
	if len(result.Chunks) == 0 {
		status = SubQuestionStatusEmpty
	}
	return SubQuestionResult{
		Query:      question,
		Status:     status,
		DurationMs: durationMs,
		ChunkCount: len(result.Chunks),
		Result:     result,
	}
}

func collectSuccessfulSubQuestionResults(subResults []SubQuestionResult) []Result {
	results := make([]Result, 0, len(subResults))
	for _, item := range subResults {
		if item.Status != SubQuestionStatusSuccess {
			continue
		}
		results = append(results, item.Result)
	}
	return results
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
