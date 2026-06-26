package main

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
)

func TestRunWithDepsPrefersSummaryTokenThresholds(t *testing.T) {
	var captured summaryEvalOptions
	exitCode := runWithDeps(
		[]string{
			"-suite", "summary",
			"-input", "summary.json",
			"-summary-mode", "strategy",
			"-summary-thresholds", "4,8",
			"-summary-token-thresholds", "800,1200",
		},
		io.Discard,
		io.Discard,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{}, nil
			},
			buildRegistry: func(
				_ *ragbootstrap.Runtime,
				_ rageval.SuiteName,
				_ []string,
				options summaryEvalOptions,
			) (*rageval.Registry, error) {
				captured = options
				return summaryStrategyFlagTestRegistry(t), nil
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if captured.thresholdUnit != rageval.SummaryStrategyThresholdTokens {
		t.Fatalf("thresholdUnit = %q, want tokens", captured.thresholdUnit)
	}
	if !reflect.DeepEqual(captured.thresholds, []int{800, 1200}) {
		t.Fatalf("thresholds = %#v, want [800 1200]", captured.thresholds)
	}
}

func TestRunWithDepsKeepsLegacySummaryTurnThresholds(t *testing.T) {
	var captured summaryEvalOptions
	exitCode := runWithDeps(
		[]string{
			"-suite", "summary",
			"-input", "summary.json",
			"-summary-mode", "strategy",
			"-summary-thresholds", "4,8",
		},
		io.Discard,
		io.Discard,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{}, nil
			},
			buildRegistry: func(
				_ *ragbootstrap.Runtime,
				_ rageval.SuiteName,
				_ []string,
				options summaryEvalOptions,
			) (*rageval.Registry, error) {
				captured = options
				return summaryStrategyFlagTestRegistry(t), nil
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if captured.thresholdUnit != rageval.SummaryStrategyThresholdTurns {
		t.Fatalf("thresholdUnit = %q, want turns", captured.thresholdUnit)
	}
	if !reflect.DeepEqual(captured.thresholds, []int{4, 8}) {
		t.Fatalf("thresholds = %#v, want [4 8]", captured.thresholds)
	}
}

func TestRunRejectsSummaryStrategyWithoutThresholdsBeforeRuntime(t *testing.T) {
	var stderr bytes.Buffer
	runtimeCalled := false
	exitCode := runWithDeps(
		[]string{"-suite", "summary", "-summary-mode", "strategy"},
		io.Discard,
		&stderr,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				runtimeCalled = true
				return &ragbootstrap.Runtime{}, nil
			},
		},
	)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
	if runtimeCalled {
		t.Fatal("runtime should not be built for invalid strategy options")
	}
	if !strings.Contains(stderr.String(), "summary strategy thresholds are required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsInvalidSummaryTokenThresholdBeforeRuntime(t *testing.T) {
	var stderr bytes.Buffer
	runtimeCalled := false
	exitCode := runWithDeps(
		[]string{
			"-suite", "summary",
			"-summary-mode", "strategy",
			"-summary-token-thresholds", "800,bad",
		},
		io.Discard,
		&stderr,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				runtimeCalled = true
				return &ragbootstrap.Runtime{}, nil
			},
		},
	)
	if exitCode != 2 {
		t.Fatalf("exitCode = %d, want 2", exitCode)
	}
	if runtimeCalled {
		t.Fatal("runtime should not be built for invalid token thresholds")
	}
	if !strings.Contains(stderr.String(), `invalid summary token threshold "bad"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func summaryStrategyFlagTestRegistry(t *testing.T) *rageval.Registry {
	t.Helper()
	registry := rageval.NewRegistry()
	if err := registry.Register(&evalRunnerTestEvaluator{
		suite:       rageval.SuiteSummary,
		loadSamples: []byte(`[{"name":"sample-1"}]`),
		runResult: rageval.SuiteResult{
			Suite:       string(rageval.SuiteSummary),
			RunMetadata: rageval.RunMetadata{Suite: string(rageval.SuiteSummary)},
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return registry
}
