package evaluation

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

type captureRewriteService struct {
	historyCalls int
	splitCalls   int
	lastQuestion string
	lastHistory  []convention.ChatMessage
	result       ragrewrite.Result
}

type queryRetrieveService struct {
	mu       sync.Mutex
	requests []ragretrieve.Request
	results  map[string][]ragretrieve.Result
}

func (s *queryRetrieveService) Retrieve(_ context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	results := s.results[request.Query]
	if len(results) > 1 {
		result := results[0]
		s.results[request.Query] = results[1:]
		s.mu.Unlock()
		return result, nil
	}
	if len(results) == 1 {
		result := results[0]
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	return ragretrieve.Result{}, nil
}

func (s *queryRetrieveService) RetrieveByVector(_ context.Context, _ []float32, request ragretrieve.Request) (ragretrieve.Result, error) {
	return s.Retrieve(context.Background(), request)
}

func (s *captureRewriteService) Rewrite(question string) string {
	return s.result.RewrittenQuestion
}

func (s *captureRewriteService) RewriteWithSplit(question string) ragrewrite.Result {
	s.splitCalls++
	s.lastQuestion = question
	return s.result
}

func (s *captureRewriteService) RewriteWithHistory(question string, history []convention.ChatMessage) ragrewrite.Result {
	s.historyCalls++
	s.lastQuestion = question
	s.lastHistory = append([]convention.ChatMessage(nil), history...)
	return s.result
}

func TestRewriteEvaluatorRunUsesHistoryAwareRewritePath(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"rewrite-sample-1",
			"tags":["alias_normalization"],
			"input":{
				"query":"它有哪些应用场景",
				"history":[
					{"role":"user","content":"什么是向量数据库"},
					{"role":"assistant","content":"向量数据库用于语义检索"}
				]
			},
			"rewrite_expectation":{
				"need_retrieval":true,
				"must_keep_terms":["向量数据库"],
				"critical_terms":["向量数据库"],
				"alias_groups":[["向量数据库","vector database"]],
				"forbidden_rewrites":["它有哪些应用场景"],
				"sub_question_count":{"min":1,"max":2}
			},
			"retrieval_expectation":{
				"target":"knowledge_base",
				"expected_ids":["doc-1"],
				"top_k":5,
				"search_mode":"hybrid"
			}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	rewrite := &captureRewriteService{
		result: ragrewrite.Result{
			RewrittenQuestion: "向量数据库有哪些应用场景",
			SubQuestions:      []string{"向量数据库应用场景", "向量数据库使用案例"},
			NeedRetrieval:     true,
		},
	}
	evaluator := NewRewriteEvaluator(rewrite)

	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteRewrite,
		InputPath:  "testdata/evals/rewrite/samples.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rewrite.historyCalls != 1 {
		t.Fatalf("history calls = %d, want 1", rewrite.historyCalls)
	}
	if rewrite.splitCalls != 0 {
		t.Fatalf("split calls = %d, want 0", rewrite.splitCalls)
	}
	if len(rewrite.lastHistory) != 2 {
		t.Fatalf("last history len = %d, want 2", len(rewrite.lastHistory))
	}
	if len(result.Samples) != 1 {
		t.Fatalf("samples len = %d, want 1", len(result.Samples))
	}
	if !result.Samples[0].Passed {
		t.Fatal("sample expected passed")
	}
	if result.Samples[0].RuleChecks["critical_terms_ok"] != true {
		t.Fatalf("critical_terms_ok = %v, want true", result.Samples[0].RuleChecks["critical_terms_ok"])
	}
}

func TestRewriteEvaluatorRunReportsRetrievalComparison(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"rewrite-retrieval-1",
			"tags":["retrieval_uplift"],
			"input":{"query":"Go SSE 长连接扛不住怎么办"},
			"rewrite_expectation":{
				"need_retrieval":true,
				"critical_terms":["Go"],
				"sub_question_count":{"min":2,"max":2}
			},
			"retrieval_expectation":{
				"target":"document",
				"expected_ids":["doc-1"],
				"critical_expected_ids":["doc-1"],
				"top_k":3,
				"search_mode":"hybrid"
			}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	rewrite := &captureRewriteService{
		result: ragrewrite.Result{
			RewrittenQuestion: "Go 调度模型和 netpoller 如何支撑 SSE 长连接",
			SubQuestions:      []string{"Go GMP 调度模型", "Go netpoller 多路复用"},
			NeedRetrieval:     true,
		},
	}
	retrieve := &queryRetrieveService{
		results: map[string][]ragretrieve.Result{
			"Go SSE 长连接扛不住怎么办": {{
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-x", DocumentID: "doc-x", Score: 0.9},
				},
			}},
			"Go GMP 调度模型": {{
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-1", DocumentID: "doc-1", Score: 0.95},
				},
			}},
			"Go netpoller 多路复用": {{
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-2", DocumentID: "doc-2", Score: 0.8},
				},
			}},
		},
	}

	evaluator := NewRewriteEvaluator(
		rewrite,
		WithRewriteRetrieveService(retrieve),
		WithRewriteRetrievalKs([]int{1, 3}),
		WithRewriteSubQuestionOptions(ragretrieve.SubQuestionOptions{
			ParallelEnabled: true,
			MaxConcurrency:  2,
		}),
	)
	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteRewrite,
		InputPath:  "testdata/evals/rewrite/samples.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(retrieve.requests) != 4 {
		t.Fatalf("retrieve requests = %d, want 4", len(retrieve.requests))
	}
	if result.Samples[0].Passed != true {
		t.Fatal("sample expected passed after retrieval uplift")
	}
	retrievalImpact, ok := result.Samples[0].Scores["retrieval_impact"].(float64)
	if !ok || retrievalImpact <= 0 {
		t.Fatalf("retrieval_impact = %v, want positive uplift score", result.Samples[0].Scores["retrieval_impact"])
	}
	rewriteQuality, rewriteQualityOK := result.Samples[0].Scores["rewrite_quality"].(float64)
	diagnosticScore, diagnosticOK := result.Samples[0].Scores["diagnostic_score"].(float64)
	if !rewriteQualityOK || !diagnosticOK {
		t.Fatalf("rewrite_quality = %v, diagnostic_score = %v, want numeric scores", result.Samples[0].Scores["rewrite_quality"], result.Samples[0].Scores["diagnostic_score"])
	}
	wantDiagnostic := roundSummaryScore((rewriteQuality * 0.45) + (retrievalImpact * 0.55))
	if diagnosticScore != wantDiagnostic {
		t.Fatalf("diagnostic_score = %v, want %v", diagnosticScore, wantDiagnostic)
	}
	candidateMRR, candidateOK := result.Aggregate.Metrics["candidate_mrr"].(float64)
	baselineMRR, baselineOK := result.Aggregate.Metrics["baseline_mrr"].(float64)
	if !candidateOK || !baselineOK || candidateMRR <= baselineMRR {
		t.Fatalf("candidate_mrr = %v, baseline_mrr = %v, want candidate > baseline", result.Aggregate.Metrics["candidate_mrr"], result.Aggregate.Metrics["baseline_mrr"])
	}
}

func TestRewriteEvaluatorRunFlagsMustNotRegress(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"rewrite-regression-1",
			"tags":["must_not_regress"],
			"input":{"query":"trace_123 为什么失败"},
			"rewrite_expectation":{
				"need_retrieval":true,
				"critical_terms":["trace_123"]
			},
			"retrieval_expectation":{
				"target":"document",
				"expected_ids":["doc-1"],
				"critical_expected_ids":["doc-1"],
				"top_k":1,
				"search_mode":"hybrid",
				"must_not_regress":true
			}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	rewrite := &captureRewriteService{
		result: ragrewrite.Result{
			RewrittenQuestion: "trace_123 调试",
			SubQuestions:      []string{"trace_123 调试"},
			NeedRetrieval:     true,
		},
	}
	retrieve := &queryRetrieveService{
		results: map[string][]ragretrieve.Result{
			"trace_123 为什么失败": {{
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-1", DocumentID: "doc-1", Score: 0.9},
				},
			}, {
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-x", DocumentID: "doc-x", Score: 0.9},
				},
			}},
			"trace_123 调试": {{
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-y", DocumentID: "doc-y", Score: 0.95},
				},
			}},
		},
	}

	evaluator := NewRewriteEvaluator(
		rewrite,
		WithRewriteRetrieveService(retrieve),
		WithRewriteRetrievalKs([]int{1, 3}),
	)
	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteRewrite,
		InputPath:  "testdata/evals/rewrite/samples.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Samples[0].Passed {
		t.Fatal("sample should fail on must_not_regress retrieval regression")
	}
	if len(result.Samples[0].CriticalFailures) == 0 || result.Samples[0].CriticalFailures[0] != "retrieval_must_not_regress" {
		t.Fatalf("critical failures = %+v, want retrieval_must_not_regress", result.Samples[0].CriticalFailures)
	}
}

func TestRewriteEvaluatorRunEmitsSemanticAndJudgeArtifacts(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"rewrite-semantic-judge-1",
			"tags":["coreference","followup"],
			"input":{"query":"那持久化呢","history":[{"role":"user","content":"Redis 为什么能那么快"}]},
			"rewrite_expectation":{"need_retrieval":true,"sub_question_count":{"min":1,"max":1}},
			"retrieval_expectation":{"target":"chunk","expected_ids":["chunk-1"],"top_k":1,"search_mode":"hybrid"}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	rewrite := &captureRewriteService{
		result: ragrewrite.Result{
			RewrittenQuestion: "Redis 持久化 AOF 和 RDB 机制",
			SubQuestions:      []string{"Redis 持久化 AOF 和 RDB 机制"},
			NeedRetrieval:     true,
		},
	}
	retrieve := &queryRetrieveService{
		results: map[string][]ragretrieve.Result{
			"那持久化呢": {{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-1", DocumentID: "doc-1", Score: 0.5}},
			}},
			"Redis 持久化 AOF 和 RDB 机制": {{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-1", DocumentID: "doc-1", Score: 0.95}},
			}},
		},
	}
	embedder := &stubQueryEmbedder{
		vectors: map[string][]float32{
			"user: Redis 为什么能那么快\nuser: 那持久化呢": {1, 0},
			"Redis 持久化 AOF 和 RDB 机制":              {0.95, 0.31},
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{{
			Passed: true,
			Score:  0.88,
			Details: map[string]any{
				"dimensions": map[string]any{
					"intent_preservation":   0.9,
					"standalone_clarity":    0.85,
					"term_preservation":     0.8,
					"split_appropriateness": 1.0,
					"retrieval_usefulness":  0.85,
				},
			},
		}},
	}

	evaluator := NewRewriteEvaluator(
		rewrite,
		WithRewriteRetrieveService(retrieve),
		WithRewriteQueryEmbedder(embedder),
		WithRewriteJudge(judge),
		WithRewriteRetrievalKs([]int{1}),
	)
	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteRewrite,
		InputPath:  "rewrite-semantic-judge.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, ok := result.Samples[0].JudgeChecks["semantic"]; !ok {
		t.Fatalf("JudgeChecks = %#v, want semantic", result.Samples[0].JudgeChecks)
	}
	if _, ok := result.Samples[0].JudgeChecks["quality"]; !ok {
		t.Fatalf("JudgeChecks = %#v, want quality", result.Samples[0].JudgeChecks)
	}
	if result.Samples[0].Scores["semantic_score"] == nil {
		t.Fatalf("Scores = %#v, want semantic_score", result.Samples[0].Scores)
	}
	if result.Samples[0].Scores["judge_score"] == nil {
		t.Fatalf("Scores = %#v, want judge_score", result.Samples[0].Scores)
	}
	executions, ok := result.Artifacts["executions"].(map[string]any)
	if !ok {
		t.Fatalf("Artifacts = %#v, want executions map", result.Artifacts)
	}
	artifact, ok := executions["rewrite-semantic-judge-1"].(map[string]any)
	if !ok {
		t.Fatalf("executions = %#v, want sample artifact", executions)
	}
	if artifact["semantic_evaluation"] == nil {
		t.Fatalf("artifact = %#v, want semantic_evaluation", artifact)
	}
	if artifact["judge_evaluation"] == nil {
		t.Fatalf("artifact = %#v, want judge_evaluation", artifact)
	}
	if result.Aggregate.Metrics["avg_semantic_score"] == nil {
		t.Fatalf("Aggregate.Metrics = %#v, want avg_semantic_score", result.Aggregate.Metrics)
	}
	if result.Aggregate.Metrics["avg_judge_score"] == nil {
		t.Fatalf("Aggregate.Metrics = %#v, want avg_judge_score", result.Aggregate.Metrics)
	}
}

func TestRewriteEvaluatorSoftGateOverridesMustContainAnyWhenRetrievalPasses(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"rewrite-soft-gate-1",
			"tags":["coreference","followup"],
			"input":{"query":"那持久化呢","history":[{"role":"user","content":"Redis 为什么能那么快"}]},
			"rewrite_expectation":{
				"need_retrieval":true,
				"must_contain_any":[["性能"]],
				"sub_question_count":{"min":1,"max":1}
			},
			"retrieval_expectation":{"target":"chunk","expected_ids":["chunk-1"],"critical_expected_ids":["chunk-1"],"top_k":1,"search_mode":"hybrid"}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	rewrite := &captureRewriteService{
		result: ragrewrite.Result{
			RewrittenQuestion: "Redis 持久化 AOF 和 RDB 机制",
			SubQuestions:      []string{"Redis 持久化 AOF 和 RDB 机制"},
			NeedRetrieval:     true,
		},
	}
	retrieve := &queryRetrieveService{
		results: map[string][]ragretrieve.Result{
			"那持久化呢": {{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-x", DocumentID: "doc-x", Score: 0.2}},
			}},
			"Redis 持久化 AOF 和 RDB 机制": {{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-1", DocumentID: "doc-1", Score: 0.95}},
			}},
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{{Passed: true, Score: 0.9}},
	}

	evaluator := NewRewriteEvaluator(
		rewrite,
		WithRewriteRetrieveService(retrieve),
		WithRewriteQueryEmbedder(&stubQueryEmbedder{
			vectors: map[string][]float32{
				"user: Redis 为什么能那么快\nuser: 那持久化呢": {1, 0},
				"Redis 持久化 AOF 和 RDB 机制":              {0.95, 0.31},
			},
		}),
		WithRewriteJudge(judge),
		WithRewriteRetrievalKs([]int{1}),
	)
	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteRewrite,
		InputPath:  "rewrite-soft-gate.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Samples[0].Passed {
		t.Fatalf("sample = %#v, want soft gate pass", result.Samples[0])
	}
	if result.Samples[0].Scores["pass_path"] != "semantic_judge" {
		t.Fatalf("pass_path = %v, want semantic_judge", result.Samples[0].Scores["pass_path"])
	}
}
