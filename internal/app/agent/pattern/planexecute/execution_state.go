package planexecute

import agentstate "local/rag-project/internal/app/agent/state"

func executionNodeDelta(nodeName string) *agentstate.ExecutionDelta {
	return &agentstate.ExecutionDelta{
		CurrentNode:      stringPtr(nodeName),
		ScheduledActions: []string{nodeName},
		CompletedActions: []string{nodeName},
	}
}

func executionAssessDelta(branch, reason, progress string) *agentstate.ExecutionDelta {
	delta := executionNodeDelta("assess_step")
	delta.IterationIncrement = 1
	delta.LastBranchTarget = stringPtr(branch)
	delta.LastBranchReason = stringPtr(reason)
	delta.LastProgressKind = stringPtr(progress)
	if branch == branchContinue {
		delta.ContinueCountIncrement = 1
	}
	return delta
}

func executionTerminalDelta() *agentstate.ExecutionDelta {
	interrupted := false
	reason := ""
	delta := executionNodeDelta("finalize")
	delta.Interrupted = &interrupted
	delta.InterruptReason = &reason
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
	delta.LastProgressKind = stringPtr(progressPlanDegraded)
	return delta
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}
