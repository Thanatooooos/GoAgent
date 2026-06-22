package evaluation

import (
	"context"
	"encoding/json"
	"testing"
)

type testEvaluator struct {
	suite          SuiteName
	loadSamples    []byte
	runResult      SuiteResult
	loadCalls      int
	runCalls       int
	lastLoadedPath string
	lastRunInput   RunInput
}

func (e *testEvaluator) Suite() SuiteName { return e.suite }

func (e *testEvaluator) LoadSamples(_ context.Context, path string) (json.RawMessage, error) {
	e.loadCalls++
	e.lastLoadedPath = path
	return append([]byte(nil), e.loadSamples...), nil
}

func (e *testEvaluator) Run(_ context.Context, input RunInput) (SuiteResult, error) {
	e.runCalls++
	e.lastRunInput = input
	return e.runResult, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	evaluator := &testEvaluator{suite: SuiteSummary}

	if err := reg.Register(evaluator); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := reg.Get(SuiteSummary)
	if !ok {
		t.Fatal("Get(summary) expected evaluator")
	}
	if got != evaluator {
		t.Fatal("Get(summary) returned unexpected evaluator")
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	reg := NewRegistry()
	first := &testEvaluator{suite: SuiteSummary}
	second := &testEvaluator{suite: SuiteSummary}

	if err := reg.Register(first); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := reg.Register(second); err == nil {
		t.Fatal("Register(second) expected duplicate error")
	}
}

func TestRegistryRejectsAggregateSuite(t *testing.T) {
	reg := NewRegistry()

	if err := reg.Register(&testEvaluator{suite: SuiteAll}); err == nil {
		t.Fatal("Register(all) expected error")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&testEvaluator{suite: SuiteRewrite}); err != nil {
		t.Fatalf("Register(rewrite) error = %v", err)
	}
	if err := reg.Register(&testEvaluator{suite: SuiteSummary}); err != nil {
		t.Fatalf("Register(summary) error = %v", err)
	}

	got := reg.List()
	want := []SuiteName{SuiteRewrite, SuiteSummary}
	if len(got) != len(want) {
		t.Fatalf("List() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
