package reactive

import agentstate "local/rag-project/internal/app/agent/state"

func executionNodeDelta(nodeName string) *agentstate.ExecutionDelta {
	return &agentstate.ExecutionDelta{
		CurrentNode:      stringPtr(nodeName),
		ScheduledActions: []string{nodeName},
		CompletedActions: []string{nodeName},
	}
}

func executionObserveDelta() *agentstate.ExecutionDelta {
	delta := executionNodeDelta("observe")
	delta.IterationIncrement = 1
	return delta
}

func executionContinueDelta() *agentstate.ExecutionDelta {
	delta := executionNodeDelta("continue")
	delta.ContinueCountIncrement = 1
	return delta
}

func executionTerminalDelta(nodeName string) *agentstate.ExecutionDelta {
	interrupted := false
	interruptReason := ""
	delta := executionNodeDelta(nodeName)
	delta.Interrupted = &interrupted
	delta.InterruptReason = &interruptReason
	return delta
}

func executionApprovalDelta(reason string) *agentstate.ExecutionDelta {
	interrupted := true
	delta := executionNodeDelta("approval")
	delta.Interrupted = &interrupted
	delta.InterruptReason = stringPtr(reason)
	return delta
}

func executionApprovalResolutionDelta(target string, reason string) *agentstate.ExecutionDelta {
	interrupted := false
	delta := executionNodeDelta("approval")
	delta.Interrupted = &interrupted
	delta.InterruptReason = stringPtr("")
	delta.LastBranchTarget = stringPtr(target)
	delta.LastBranchReason = stringPtr(reason)
	delta.LastProgressKind = stringPtr(progressNone)
	return delta
}
