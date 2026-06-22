package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

type Evaluator interface {
	Suite() SuiteName
	LoadSamples(ctx context.Context, path string) (json.RawMessage, error)
	Run(ctx context.Context, input RunInput) (SuiteResult, error)
}

type Registry struct {
	evaluators map[SuiteName]Evaluator
}

func NewRegistry() *Registry {
	return &Registry{evaluators: map[SuiteName]Evaluator{}}
}

func (r *Registry) Register(evaluator Evaluator) error {
	if evaluator == nil {
		return fmt.Errorf("evaluator is required")
	}
	suite := evaluator.Suite()
	if !suite.IsExecutableSuite() {
		return fmt.Errorf("cannot register non-executable suite %q", suite)
	}
	if _, exists := r.evaluators[suite]; exists {
		return fmt.Errorf("evaluator already registered for suite %q", suite)
	}
	r.evaluators[suite] = evaluator
	return nil
}

func (r *Registry) Get(suite SuiteName) (Evaluator, bool) {
	if r == nil {
		return nil, false
	}
	evaluator, ok := r.evaluators[suite]
	return evaluator, ok
}

func (r *Registry) List() []SuiteName {
	if r == nil || len(r.evaluators) == 0 {
		return nil
	}
	suites := make([]SuiteName, 0, len(r.evaluators))
	for suite := range r.evaluators {
		suites = append(suites, suite)
	}
	sort.Slice(suites, func(i, j int) bool {
		return suites[i] < suites[j]
	})
	return suites
}
