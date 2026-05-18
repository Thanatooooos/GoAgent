package runtime

import (
	"context"
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

func newObserveResult(done bool, reasoning string, state AgentState) ObserveResult {
	return ObserveResult{
		Done:      done,
		Reasoning: strings.TrimSpace(reasoning),
		State:     state.Normalize(),
	}
}

func observeState(phase string, hypothesis string, confidence float64, nextHintCalls []HintCall, checkedTools []string, openQuestions ...string) AgentState {
	return AgentState{
		Phase:         phase,
		Hypothesis:    hypothesis,
		Confidence:    confidence,
		OpenQuestions: openQuestions,
		CheckedTools:  checkedTools,
		NextHintCalls: nextHintCalls,
	}.Normalize()
}

// RuleObserver is the lightweight V1 observer built on top of tool results.
type RuleObserver struct{}

func NewRuleObserver() *RuleObserver {
	return &RuleObserver{}
}

func (o *RuleObserver) Observe(_ context.Context, input ObserveInput) (ObserveResult, error) {
	if len(input.RoundResults) == 0 {
		return newObserveResult(true, "No new tool results were produced in this round, so the agent loop stops here.", AgentState{
			Phase:        "complete",
			CheckedTools: input.PreviousState.CheckedTools,
		}), nil
	}
	if input.ReachedMaxLoop {
		return newObserveResult(true, fmt.Sprintf("The agent loop already reached the maximum of %d iterations, so it must answer with the current evidence.", input.MaxIterations), AgentState{
			Phase:         "complete",
			Confidence:    input.PreviousState.Confidence,
			Hypothesis:    input.PreviousState.Hypothesis,
			CheckedTools:  input.PreviousState.CheckedTools,
			NextHintCalls: input.PreviousState.NextHintCalls,
		}), nil
	}

	latest, ok := lastNonThinkResult(input.RoundResults)
	if !ok {
		return newObserveResult(true, "All tool results in this round were think calls; no diagnostic evidence to evaluate.", AgentState{
			Phase:        "complete",
			CheckedTools: input.PreviousState.CheckedTools,
		}), nil
	}
	if observation, handled := observeWithRegistry(latest, input); handled {
		return observation, nil
	}

	switch latest.GetString("diagnosisDepth") {
	case "node_level":
		return newObserveResult(true, "The diagnosis chain reached node-level evidence. The agent can answer with high confidence.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.95,
			nil,
			[]string{latest.Name},
		)), nil
	case "task_level":
		return newObserveResult(true, "The diagnosis chain reached task-level evidence but not a specific node. The agent can answer with moderate confidence.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.75,
			nil,
			[]string{latest.Name},
		)), nil
	default:
		log.Infof("[observer] no module behavior for %q, falling back to generic completion", strings.TrimSpace(latest.Name))
		return newObserveResult(true, "The current tool result is already sufficient as supporting context, so the agent loop stops here.", observeState(
			"complete",
			strings.TrimSpace(firstNonEmpty(latest.Summary, latest.ErrorMessage)),
			0.6,
			nil,
			[]string{latest.Name},
		)), nil
	}
}

func lastNonThinkResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(results[idx].Name) != "think" {
			return results[idx], true
		}
	}
	return Result{}, false
}
