package runtime

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	ErrorClassValidation       = "validation"
	ErrorClassPermission       = "permission"
	ErrorClassApprovalRejected = "approval_rejected"
	ErrorClassExternal         = "external"
	ErrorClassTimeout          = "timeout"
	ErrorClassBudget           = "budget"
	ErrorClassNoProgress       = "no_progress"
	ErrorClassModelOutput      = "model_output"
	ErrorClassDependency       = "dependency"
	ErrorClassUnknown          = "unknown"
)

// NormalizeErrorClass maps capability- or pattern-originated error names onto
// the runtime-owned canonical error taxonomy.
func NormalizeErrorClass(value string) string {
	switch strings.TrimSpace(value) {
	case "", ErrorClassUnknown:
		return ErrorClassUnknown
	case ErrorClassValidation, agentcapability.ErrorClassValidation:
		return ErrorClassValidation
	case ErrorClassPermission, agentcapability.ErrorClassPermission:
		return ErrorClassPermission
	case ErrorClassApprovalRejected:
		return ErrorClassApprovalRejected
	case ErrorClassExternal, agentcapability.ErrorClassExternal:
		return ErrorClassExternal
	case ErrorClassTimeout:
		return ErrorClassTimeout
	case ErrorClassBudget:
		return ErrorClassBudget
	case ErrorClassNoProgress:
		return ErrorClassNoProgress
	case ErrorClassModelOutput:
		return ErrorClassModelOutput
	case ErrorClassDependency, agentcapability.ErrorClassDependency:
		return ErrorClassDependency
	default:
		return ErrorClassUnknown
	}
}

// ErrorClassForSession inspects shared runtime state and determines the best
// canonical runtime error class for the current session.
func ErrorClassForSession(session *RuntimeSession) string {
	if session == nil {
		return ErrorClassUnknown
	}

	if class := NormalizeErrorClass(session.Snapshot.Context.FetchErrorClass); class != ErrorClassUnknown {
		return class
	}
	if class := NormalizeErrorClass(session.Snapshot.Context.SearchErrorClass); class != ErrorClassUnknown {
		return class
	}
	if class := ErrorClassForReason(session.Snapshot.Answer.DegradeReason); class != ErrorClassUnknown {
		return class
	}
	if strings.TrimSpace(session.Snapshot.Approval.Status) == agentstate.ApprovalStatusRejected {
		return ErrorClassApprovalRejected
	}
	return ErrorClassUnknown
}

// ErrorClassForReason normalizes runtime-visible degrade or interruption
// reasons onto canonical error classes when possible.
func ErrorClassForReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case "approval_rejected":
		return ErrorClassApprovalRejected
	case "iteration_budget_exhausted":
		return ErrorClassBudget
	case "no_progress_across_rounds":
		return ErrorClassNoProgress
	default:
		return ErrorClassUnknown
	}
}
