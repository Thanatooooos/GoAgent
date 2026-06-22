package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestRunRequiresSuite(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatal("run() expected non-zero exit code when suite flag is missing")
	}
	if !strings.Contains(stderr.String(), "suite is required") {
		t.Fatalf("stderr = %q, want suite error", stderr.String())
	}
}

func TestRunRejectsUnsupportedSuite(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"-suite", "tool"}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatal("run() expected non-zero exit code for unsupported suite")
	}
	if !strings.Contains(stderr.String(), "unsupported suite") {
		t.Fatalf("stderr = %q, want unsupported suite error", stderr.String())
	}
}

func TestRunUsesInjectedRegistryAndExecutesSuite(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	expectedResult := rageval.SuiteResult{
		Suite: "rewrite",
		RunMetadata: rageval.RunMetadata{
			RunAt:       "2026-06-19T12:00:00Z",
			Suite:       "rewrite",
			SampleSetID: "rewrite-sample-set",
		},
		Samples: []rageval.SharedSampleResult{{
			Name:   "sample-1",
			Passed: true,
			Tags:   []string{"baseline"},
		}},
		Aggregate: rageval.SharedAggregateResult{
			PassRate:            1,
			CriticalFailureRate: 0,
			Metrics:             map[string]any{"candidate_mrr": 1.0},
		},
		Artifacts: map[string]any{"raw_output": map[string]any{"status": "ok"}},
	}

	registry := rageval.NewRegistry()
	if err := registry.Register(&evalRunnerTestEvaluator{
		suite:       rageval.SuiteRewrite,
		loadSamples: []byte(`[{"name":"sample-1"}]`),
		runResult:   expectedResult,
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	exitCode := runWithDeps(
		[]string{"-suite", "rewrite", "-input", "rewrite.json"},
		&stdout,
		&stderr,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{}, nil
			},
			buildRegistry: func(*ragbootstrap.Runtime, rageval.SuiteName, []string) (*rageval.Registry, error) {
				return registry, nil
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("runWithDeps() exitCode = %d, want 0, stderr=%q", exitCode, stderr.String())
	}

	var got rageval.SuiteResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout should be suite result JSON, got %q, err=%v", stdout.String(), err)
	}
	if got.Suite != expectedResult.Suite {
		t.Fatalf("Suite = %q, want %q", got.Suite, expectedResult.Suite)
	}
	if got.RunMetadata.Suite != expectedResult.RunMetadata.Suite {
		t.Fatalf("RunMetadata.Suite = %q, want %q", got.RunMetadata.Suite, expectedResult.RunMetadata.Suite)
	}
	if len(got.Samples) != 1 || got.Samples[0].Name != "sample-1" {
		t.Fatalf("Samples = %#v, want one sample named sample-1", got.Samples)
	}
	if got.Aggregate.Metrics["candidate_mrr"] != expectedResult.Aggregate.Metrics["candidate_mrr"] {
		t.Fatalf("Aggregate.Metrics[candidate_mrr] = %v, want %v", got.Aggregate.Metrics["candidate_mrr"], expectedResult.Aggregate.Metrics["candidate_mrr"])
	}
	if _, ok := got.Artifacts["raw_output"]; !ok {
		t.Fatalf("Artifacts = %#v, want raw_output", got.Artifacts)
	}
}

func TestBuildPhase1RegistrySummaryDoesNotRequireRewriteService(t *testing.T) {
	registry, err := buildPhase1Registry(&ragbootstrap.Runtime{LLMChat: summaryOnlyLLMService{}}, rageval.SuiteSummary, nil, rewriteEvalOptions{})
	if err != nil {
		t.Fatalf("buildPhase1Registry(summary) error = %v", err)
	}
	if _, ok := registry.Get(rageval.SuiteSummary); !ok {
		t.Fatal("summary evaluator expected registered")
	}
	if _, ok := registry.Get(rageval.SuiteRewrite); ok {
		t.Fatal("rewrite evaluator should not be registered for summary-only registry")
	}
}

func TestBuildPhase1RegistryRewriteDoesNotRequireLLMChat(t *testing.T) {
	registry, err := buildPhase1Registry(&ragbootstrap.Runtime{
		Rewrite:  rewriteOnlyService{},
		Retrieve: rewriteOnlyRetrieveService{},
	}, rageval.SuiteRewrite, nil, rewriteEvalOptions{})
	if err != nil {
		t.Fatalf("buildPhase1Registry(rewrite) error = %v", err)
	}
	if _, ok := registry.Get(rageval.SuiteRewrite); !ok {
		t.Fatal("rewrite evaluator expected registered")
	}
	if _, ok := registry.Get(rageval.SuiteSummary); ok {
		t.Fatal("summary evaluator should not be registered for rewrite-only registry")
	}
}

func TestBuildPhase1RegistrySummaryWiresJudgeAndEquivalence(t *testing.T) {
	chat := &sequencedLLMService{responses: []string{
		`{"schema_version":1,"goal":"draft spec first","constraints":["no implementation yet"],"recent_progress":["collecting eval requirements"],"open_questions":["what task should come next"]}`,
		`{"passed":true,"score":1,"details":{"fields":{"goal":{"fidelity":1,"usefulness":1},"constraints":{"fidelity":1,"usefulness":1},"established_facts":{"fidelity":1,"usefulness":1},"recent_progress":{"fidelity":1,"usefulness":1},"open_questions":{"fidelity":1,"usefulness":1}}}}`,
		"draft spec first",
		"draft spec first",
		`{"passed":true,"score":1,"details":{"dangerous_drift":false}}`,
	}}
	registry, err := buildPhase1Registry(&ragbootstrap.Runtime{LLMChat: chat}, rageval.SuiteSummary, nil, rewriteEvalOptions{})
	if err != nil {
		t.Fatalf("buildPhase1Registry(summary) error = %v", err)
	}
	evaluator, ok := registry.Get(rageval.SuiteSummary)
	if !ok {
		t.Fatal("summary evaluator expected registered")
	}

	rawPayload := json.RawMessage(`[
		{
			"name":"summary-sample-1",
			"tags":["goal_drift"],
			"input":{"source_messages":[{"role":"user","content":"Draft the spec first. Do not implement yet."}]},
			"expected_summary":{
				"goal":{"must_cover":["draft spec first"]},
				"constraints":{"must_cover":["no implementation yet"]}
			},
			"critical_contract":{
				"critical_constraints":["no implementation yet"]
			},
			"next_turn_eval":{"queries":[{"id":"q1","query":"What should happen next?","equivalence_expectations":["draft spec first"]}]}
		}
	]`)
	rawSamples, err := rageval.ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	result, err := evaluator.Run(context.Background(), rageval.RunInput{
		Suite:      rageval.SuiteSummary,
		InputPath:  "testdata/evals/summary/samples.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("summary evaluator Run() error = %v", err)
	}
	if len(chat.requests) != 5 {
		t.Fatalf("chat request count = %d, want 5", len(chat.requests))
	}
	if len(result.Samples) != 1 {
		t.Fatalf("samples len = %d, want 1", len(result.Samples))
	}
	judgeChecks := result.Samples[0].JudgeChecks
	if _, ok := judgeChecks["field_level"]; !ok {
		t.Fatalf("JudgeChecks = %#v, want field_level", judgeChecks)
	}
	if _, ok := judgeChecks["downstream_equivalence"]; !ok {
		t.Fatalf("JudgeChecks = %#v, want downstream_equivalence", judgeChecks)
	}
}

func TestRunSummarySuiteEndToEndEmitsSharedContract(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	samplePath := filepath.Join(t.TempDir(), "summary-e2e.json")
	if err := os.WriteFile(samplePath, []byte(`[
		{
			"name":"summary-e2e-1",
			"tags":["e2e"],
			"input":{"source_messages":[{"role":"user","content":"Draft the spec first. Do not implement yet."}]},
			"expected_summary":{
				"goal":{"must_cover":["draft spec first"]},
				"constraints":{"must_cover":["no implementation yet"]}
			},
			"critical_contract":{"critical_constraints":["no implementation yet"]},
			"next_turn_eval":{"queries":[{"id":"q1","query":"What should happen next?","equivalence_expectations":["draft spec first"]}]}
		}
	]`), 0o644); err != nil {
		t.Fatalf("WriteFile(summary-e2e.json) error = %v", err)
	}

	llm := &smartEvalLLMService{}
	exitCode := runWithDeps(
		[]string{"-suite", "summary", "-input", samplePath},
		&stdout,
		&stderr,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{LLMChat: llm}, nil
			},
			buildRegistry: func(runtime *ragbootstrap.Runtime, suite rageval.SuiteName, kbIDs []string) (*rageval.Registry, error) {
				return buildPhase1Registry(runtime, suite, kbIDs, rewriteEvalOptions{})
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("runWithDeps(summary e2e) exitCode = %d, want 0, stderr=%q", exitCode, stderr.String())
	}

	var got rageval.SuiteResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout should be summary suite JSON, got %q, err=%v", stdout.String(), err)
	}
	if got.Suite != string(rageval.SuiteSummary) {
		t.Fatalf("Suite = %q, want %q", got.Suite, rageval.SuiteSummary)
	}
	if got.RunMetadata.Suite != string(rageval.SuiteSummary) {
		t.Fatalf("RunMetadata.Suite = %q, want %q", got.RunMetadata.Suite, rageval.SuiteSummary)
	}
	if len(got.Samples) != 1 {
		t.Fatalf("Samples len = %d, want 1", len(got.Samples))
	}
	if _, ok := got.Samples[0].JudgeChecks["field_level"]; !ok {
		t.Fatalf("JudgeChecks = %#v, want field_level", got.Samples[0].JudgeChecks)
	}
	if _, ok := got.Samples[0].JudgeChecks["downstream_equivalence"]; !ok {
		t.Fatalf("JudgeChecks = %#v, want downstream_equivalence", got.Samples[0].JudgeChecks)
	}
	executions, ok := got.Artifacts["executions"].(map[string]any)
	if !ok {
		t.Fatalf("Artifacts = %#v, want executions map", got.Artifacts)
	}
	if _, ok := executions["summary-e2e-1"]; !ok {
		t.Fatalf("executions = %#v, want summary-e2e-1 artifact", executions)
	}
}

func TestRunRewriteSuiteEndToEndUsesDefaultAssetPath(t *testing.T) {
	withRepoRoot(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runWithDeps(
		[]string{"-suite", "rewrite"},
		&stdout,
		&stderr,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{
					Rewrite:  rewriteOnlyService{},
					Retrieve: rewriteOnlyRetrieveService{},
				}, nil
			},
			buildRegistry: func(runtime *ragbootstrap.Runtime, suite rageval.SuiteName, kbIDs []string) (*rageval.Registry, error) {
				return buildPhase1Registry(runtime, suite, kbIDs, rewriteEvalOptions{})
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("runWithDeps(rewrite e2e) exitCode = %d, want 0, stderr=%q", exitCode, stderr.String())
	}

	var got rageval.SuiteResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout should be rewrite suite JSON, got %q, err=%v", stdout.String(), err)
	}
	if got.Suite != string(rageval.SuiteRewrite) {
		t.Fatalf("Suite = %q, want %q", got.Suite, rageval.SuiteRewrite)
	}
	if got.RunMetadata.SampleSetID != "testdata/evals/rewrite/samples.json" {
		t.Fatalf("RunMetadata.SampleSetID = %q, want default rewrite path", got.RunMetadata.SampleSetID)
	}
	if len(got.Samples) != 48 {
		t.Fatalf("Samples len = %d, want 48 default rewrite samples", len(got.Samples))
	}
	executions, ok := got.Artifacts["executions"].(map[string]any)
	if !ok {
		t.Fatalf("Artifacts = %#v, want executions map", got.Artifacts)
	}
	if _, ok := executions["coref_go_slice_followup"]; !ok {
		t.Fatalf("executions = %#v, want coref_go_slice_followup artifact", executions)
	}
}

func TestRunAllSuitesEndToEndEmitsSuiteResultArray(t *testing.T) {
	withRepoRoot(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	llm := &smartEvalLLMService{}
	exitCode := runWithDeps(
		[]string{"-suite", "all"},
		&stdout,
		&stderr,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{
					LLMChat:  llm,
					Rewrite:  rewriteOnlyService{},
					Retrieve: rewriteOnlyRetrieveService{},
				}, nil
			},
			buildRegistry: func(runtime *ragbootstrap.Runtime, suite rageval.SuiteName, kbIDs []string) (*rageval.Registry, error) {
				return buildPhase1Registry(runtime, suite, kbIDs, rewriteEvalOptions{})
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("runWithDeps(all e2e) exitCode = %d, want 0, stderr=%q", exitCode, stderr.String())
	}

	var got []rageval.SuiteResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout should be suite result array JSON, got %q, err=%v", stdout.String(), err)
	}
	if len(got) != 2 {
		t.Fatalf("SuiteResult array len = %d, want 2", len(got))
	}
	bySuite := map[string]rageval.SuiteResult{}
	for _, result := range got {
		bySuite[result.Suite] = result
	}
	if _, ok := bySuite[string(rageval.SuiteSummary)]; !ok {
		t.Fatalf("suite results = %#v, want summary", bySuite)
	}
	if _, ok := bySuite[string(rageval.SuiteRewrite)]; !ok {
		t.Fatalf("suite results = %#v, want rewrite", bySuite)
	}
}

func withRepoRoot(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%q) error = %v", root, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

type evalRunnerTestEvaluator struct {
	suite       rageval.SuiteName
	loadSamples []byte
	runResult   rageval.SuiteResult
}

func (e *evalRunnerTestEvaluator) Suite() rageval.SuiteName { return e.suite }

func (e *evalRunnerTestEvaluator) LoadSamples(_ context.Context, _ string) (json.RawMessage, error) {
	return append([]byte(nil), e.loadSamples...), nil
}

func (e *evalRunnerTestEvaluator) Run(_ context.Context, _ rageval.RunInput) (rageval.SuiteResult, error) {
	return e.runResult, nil
}

type summaryOnlyLLMService struct{}

func (summaryOnlyLLMService) Chat(string) (string, error) { return "", nil }
func (summaryOnlyLLMService) ChatWithRequest(convention.ChatRequest) (string, error) {
	return "", nil
}
func (summaryOnlyLLMService) ChatWithModel(convention.ChatRequest, string) (string, error) {
	return "", nil
}
func (summaryOnlyLLMService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (summaryOnlyLLMService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

type sequencedLLMService struct {
	responses []string
	requests  []convention.ChatRequest
}

func (s *sequencedLLMService) Chat(string) (string, error) { return s.nextResponse() }
func (s *sequencedLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.nextResponse()
}
func (s *sequencedLLMService) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	s.requests = append(s.requests, request)
	return s.nextResponse()
}
func (s *sequencedLLMService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *sequencedLLMService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *sequencedLLMService) nextResponse() (string, error) {
	if len(s.responses) == 0 {
		return "", nil
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

type smartEvalLLMService struct {
	requests []convention.ChatRequest
}

func (s *smartEvalLLMService) Chat(string) (string, error) { return "draft spec first", nil }
func (s *smartEvalLLMService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.requests = append(s.requests, request)
	return s.respond(request), nil
}
func (s *smartEvalLLMService) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	s.requests = append(s.requests, request)
	return s.respond(request), nil
}
func (s *smartEvalLLMService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *smartEvalLLMService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}
func (s *smartEvalLLMService) respond(request convention.ChatRequest) string {
	lastContent := ""
	if len(request.Messages) > 0 {
		lastContent = request.Messages[len(request.Messages)-1].Content
	}
	if strings.HasPrefix(lastContent, "Judge payload:") {
		return `{"passed":true,"score":1,"reason":"ok","details":{"dangerous_drift":false,"fields":{"goal":{"fidelity":1,"usefulness":1},"constraints":{"fidelity":1,"usefulness":1},"established_facts":{"fidelity":1,"usefulness":1},"recent_progress":{"fidelity":1,"usefulness":1},"open_questions":{"fidelity":1,"usefulness":1}}}}`
	}
	if request.JSONMode != nil && *request.JSONMode {
		return `{"schema_version":1,"goal":"draft spec first","constraints":["no implementation yet"],"established_facts":["phase one covers summary and rewrite"],"recent_progress":["e2e verification in progress"],"open_questions":["what should be verified next"]}`
	}
	return "draft spec first"
}

type rewriteOnlyService struct{}

func (rewriteOnlyService) Rewrite(question string) string { return question }
func (rewriteOnlyService) RewriteWithSplit(question string) ragrewrite.Result {
	return ragrewrite.Result{
		RewrittenQuestion: question,
		SubQuestions:      []string{question},
		NeedRetrieval:     true,
	}
}
func (rewriteOnlyService) RewriteWithHistory(question string, _ []convention.ChatMessage) ragrewrite.Result {
	return rewriteOnlyService{}.RewriteWithSplit(question)
}

type rewriteOnlyRetrieveService struct{}

func (rewriteOnlyRetrieveService) Retrieve(context.Context, ragretrieve.Request) (ragretrieve.Result, error) {
	return ragretrieve.Result{}, nil
}
func (rewriteOnlyRetrieveService) RetrieveByVector(context.Context, []float32, ragretrieve.Request) (ragretrieve.Result, error) {
	return ragretrieve.Result{}, nil
}
