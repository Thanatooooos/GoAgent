package reactive

import (
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func scheduledCapabilityNodeResult(node string, capabilityName string, execution agentruntime.CapabilityExecutionResult) (agentruntime.NodeResult, bool) {
	if execution.Schedule.Decision == agentruntime.ScheduleDecisionExecute {
		return agentruntime.NodeResult{}, false
	}

	delta := withExecutionNodeDelta(execution.Invocation.Delta, node)
	if delta.Context == nil {
		delta.Context = &agentstate.ContextDelta{}
	}
	if summary := strings.TrimSpace(execution.Invocation.Observation.Summary); summary != "" {
		delta.Context.Notes = append(delta.Context.Notes, summary)
	}
	if execution.Schedule.Decision != agentruntime.ScheduleDecisionWaitApproval &&
		execution.Schedule.Decision != agentruntime.ScheduleDecisionFail {
		if errorClass := strings.TrimSpace(execution.Invocation.ErrorClass); errorClass != "" {
			applyCapabilityErrorClass(node, delta.Context, errorClass)
		}
	}
	if execution.Schedule.Decision == agentruntime.ScheduleDecisionWaitApproval {
		delta.Approval = agentruntime.BuildPendingApprovalDelta(
			approvalRequiredReason(node),
			capabilityName,
			node,
			"",
			time.Now(),
		)
	}
	return agentruntime.NodeResult{
		Events: execution.Events,
		Delta:  delta,
	}, true
}

func applyCapabilityErrorClass(node string, delta *agentstate.ContextDelta, errorClass string) {
	if delta == nil {
		return
	}
	switch strings.TrimSpace(node) {
	case "search":
		delta.SearchErrorClass = stringPtr(errorClass)
	case "fetch", "external_evidence":
		delta.FetchErrorClass = stringPtr(errorClass)
	}
}

func approvalRequiredReason(node string) string {
	switch strings.TrimSpace(node) {
	case "search":
		return "search_approval_required"
	case "fetch":
		return "fetch_approval_required"
	case "external_evidence":
		return "external_evidence_approval_required"
	default:
		return "approval_required"
	}
}

func capabilityPermissionErrorClass(errorClass string) string {
	if strings.TrimSpace(errorClass) != "" {
		return errorClass
	}
	return agentcapability.ErrorClassPermission
}
