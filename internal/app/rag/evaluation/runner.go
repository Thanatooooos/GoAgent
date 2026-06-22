package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
)

type RunRequest struct {
	Suite      SuiteName
	InputPath  string
	InputPaths map[SuiteName]string
}

type RunInput struct {
	Suite     SuiteName
	InputPath string
	RawPayload json.RawMessage
	RawSamples []json.RawMessage
}

type Runner struct {
	registry *Registry
}

func NewRunner(registry *Registry) *Runner {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Runner{registry: registry}
}

func (r *Runner) Run(ctx context.Context, req RunRequest) ([]SuiteResult, error) {
	switch {
	case req.Suite.IsExecutableSuite():
		result, err := r.runSuite(ctx, req.Suite, inputPathForSuite(req, req.Suite))
		if err != nil {
			return nil, err
		}
		return []SuiteResult{result}, nil
	case req.Suite.IsAggregateSuite():
		suites := r.registry.List()
		results := make([]SuiteResult, 0, len(suites))
		for _, suite := range suites {
			result, err := r.runSuite(ctx, suite, inputPathForSuite(req, suite))
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}
		return results, nil
	default:
		return nil, fmt.Errorf("unsupported suite %q", req.Suite)
	}
}

func (r *Runner) runSuite(ctx context.Context, suite SuiteName, inputPath string) (SuiteResult, error) {
	evaluator, ok := r.registry.Get(suite)
	if !ok {
		return SuiteResult{}, fmt.Errorf("no evaluator registered for suite %q", suite)
	}

	rawPayload, err := evaluator.LoadSamples(ctx, inputPath)
	if err != nil {
		return SuiteResult{}, fmt.Errorf("load suite %q samples: %w", suite, err)
	}
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		return SuiteResult{}, fmt.Errorf("extract suite %q samples: %w", suite, err)
	}

	result, err := evaluator.Run(ctx, RunInput{
		Suite:      suite,
		InputPath:  inputPath,
		RawPayload: append(json.RawMessage(nil), rawPayload...),
		RawSamples: cloneRawMessages(rawSamples),
	})
	if err != nil {
		return SuiteResult{}, fmt.Errorf("run suite %q: %w", suite, err)
	}
	return result, nil
}

func inputPathForSuite(req RunRequest, suite SuiteName) string {
	if req.InputPaths != nil {
		if path := req.InputPaths[suite]; path != "" {
			return path
		}
	}
	if req.InputPath != "" {
		return req.InputPath
	}
	switch suite {
	case SuiteSummary:
		return "testdata/evals/summary/samples.json"
	case SuiteRewrite:
		return "testdata/evals/rewrite/samples.json"
	default:
		return ""
	}
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, append(json.RawMessage(nil), value...))
	}
	return cloned
}
