package evaluation

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRunnerRunSingleSuite(t *testing.T) {
	reg := NewRegistry()
	evaluator := &testEvaluator{
		suite:       SuiteSummary,
		loadSamples: []byte(`[{"name":"sample-1"}]`),
		runResult: SuiteResult{
			Suite: "summary",
			Samples: []SharedSampleResult{
				{Name: "sample-1", Passed: true},
			},
		},
	}
	if err := reg.Register(evaluator); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	runner := NewRunner(reg)
	results, err := runner.Run(context.Background(), RunRequest{
		Suite:     SuiteSummary,
		InputPath: "testdata/evals/summary/samples.json",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Run() result len = %d, want 1", len(results))
	}
	if evaluator.loadCalls != 1 || evaluator.runCalls != 1 {
		t.Fatalf("expected one load and one run call, got load=%d run=%d", evaluator.loadCalls, evaluator.runCalls)
	}
	if evaluator.lastRunInput.Suite != SuiteSummary {
		t.Fatalf("lastRunInput.Suite = %q, want %q", evaluator.lastRunInput.Suite, SuiteSummary)
	}
	if len(evaluator.lastRunInput.RawSamples) != 1 {
		t.Fatalf("lastRunInput.RawSamples len = %d, want 1", len(evaluator.lastRunInput.RawSamples))
	}
}

func TestRunnerRunAllSuites(t *testing.T) {
	reg := NewRegistry()
	summaryEvaluator := &testEvaluator{
		suite:       SuiteSummary,
		loadSamples: []byte(`[{"name":"summary-1"}]`),
		runResult:   SuiteResult{Suite: "summary"},
	}
	rewriteEvaluator := &testEvaluator{
		suite:       SuiteRewrite,
		loadSamples: []byte(`[{"name":"rewrite-1"}]`),
		runResult:   SuiteResult{Suite: "rewrite"},
	}
	if err := reg.Register(summaryEvaluator); err != nil {
		t.Fatalf("Register(summary) error = %v", err)
	}
	if err := reg.Register(rewriteEvaluator); err != nil {
		t.Fatalf("Register(rewrite) error = %v", err)
	}

	runner := NewRunner(reg)
	results, err := runner.Run(context.Background(), RunRequest{
		Suite: SuiteAll,
		InputPaths: map[SuiteName]string{
			SuiteSummary: "summary.json",
			SuiteRewrite: "rewrite.json",
		},
	})
	if err != nil {
		t.Fatalf("Run(all) error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Run(all) result len = %d, want 2", len(results))
	}
	if summaryEvaluator.lastLoadedPath != "summary.json" {
		t.Fatalf("summary path = %q, want summary.json", summaryEvaluator.lastLoadedPath)
	}
	if rewriteEvaluator.lastLoadedPath != "rewrite.json" {
		t.Fatalf("rewrite path = %q, want rewrite.json", rewriteEvaluator.lastLoadedPath)
	}
}

func TestRunnerRejectsUnsupportedSuite(t *testing.T) {
	runner := NewRunner(NewRegistry())
	_, err := runner.Run(context.Background(), RunRequest{Suite: SuiteName("tool")})
	if err == nil {
		t.Fatal("Run() expected unsupported suite error")
	}
}

func TestRunInputExposesRawPayload(t *testing.T) {
	reg := NewRegistry()
	evaluator := &testEvaluator{
		suite:       SuiteSummary,
		loadSamples: []byte(`{"samples":[{"name":"sample-1"}]}`),
		runResult:   SuiteResult{Suite: "summary"},
	}
	if err := reg.Register(evaluator); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	runner := NewRunner(reg)
	if _, err := runner.Run(context.Background(), RunRequest{
		Suite:     SuiteSummary,
		InputPath: "summary.json",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !json.Valid(evaluator.lastRunInput.RawPayload) {
		t.Fatal("RawPayload should contain valid JSON")
	}
}
