package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
)

var nextActionRegistry *Registry

// SetNextActionRegistry provides a Registry for nextAction lookups.
// Set once during assembly; nextAction uses it to resolve tool behaviors
// instead of the old hardcoded switch.
func SetNextActionRegistry(r *Registry) {
	nextActionRegistry = r
}

// nextAction returns the next tool to call based on the latest tool result.
func nextAction(result Result) (hintCall *HintCall, done bool, reason string) {
	if nextActionRegistry != nil {
		if behavior, ok := nextActionRegistry.GetBehavior(result.Name); ok && behavior.Next != nil {
			return nextActionFromDecision(behavior.Next(result, WorkflowInput{}))
		}
	}
	return nil, true, "terminal"
}

func nextActionFromDecision(decision NextDecision) (hintCall *HintCall, done bool, reason string) {
	hints := NormalizeHintCalls(decision.HintCalls)
	if len(hints) > 0 {
		return &hints[0], false, strings.TrimSpace(decision.Reason)
	}
	if decision.Done || decision.Terminal || strings.TrimSpace(decision.Reason) != "" {
		return nil, decision.Done || decision.Terminal, strings.TrimSpace(decision.Reason)
	}
	return nil, true, "terminal"
}
