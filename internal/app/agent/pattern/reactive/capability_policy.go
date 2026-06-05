package reactive

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentruntime "local/rag-project/internal/app/agent/runtime"
)

const (
	progressRetryableSearchFailed = "progress_retryable_search_failure"
)

type capabilityRuntimePolicy struct {
	searchSpec        agentcapability.Spec
	fetchSpec         agentcapability.Spec
	workflowSpec      agentcapability.Spec
	preferWorkflowRun bool
}

func buildCapabilityRuntimePolicy(searchHandle agentcapability.Handle, fetchHandle agentcapability.Handle, workflowHandle agentcapability.Handle, preferWorkflow bool) capabilityRuntimePolicy {
	policy := capabilityRuntimePolicy{
		preferWorkflowRun: preferWorkflow,
	}
	if searchHandle != nil {
		policy.searchSpec = searchHandle.Spec()
	}
	if fetchHandle != nil {
		policy.fetchSpec = fetchHandle.Spec()
	}
	if workflowHandle != nil {
		policy.workflowSpec = workflowHandle.Spec()
	}
	return policy
}

func (p capabilityRuntimePolicy) retryDirective(session *agentruntime.RuntimeSession) (branch string, reason string, confidence float64, progressKind string, capabilityName string, rerunNode string, applied bool) {
	errorClass, spec, reasonPrefix := p.activeFailure(session)
	switch errorClass {
	case agentcapability.ErrorClassValidation:
		return branchDegrade, reasonPrefix + "_validation_failed", 0.60, progressNone, "", "", true
	case agentcapability.ErrorClassDependency:
		return branchDegrade, reasonPrefix + "_dependency_failed", 0.60, progressNone, "", "", true
	case agentcapability.ErrorClassPermission:
		if requiresRuntimeApproval(session, spec) {
			return branchApproval, reasonPrefix + "_approval_required", 0.70, progressNone, spec.Name, rerunNodeForReasonPrefix(reasonPrefix), true
		}
		return branchDegrade, reasonPrefix + "_permission_required", 0.60, progressNone, "", "", true
	case agentcapability.ErrorClassExternal:
		progressKind = progressKindForRetry(reasonPrefix)
		if strings.TrimSpace(spec.Idempotency) == agentcapability.IdempotencyUnknown {
			return branchDegrade, "retry_blocked_unknown_idempotency", 0.55, progressNone, "", "", true
		}
		if session != nil && session.Metadata.ResumeCount > 0 && !spec.SupportsResume {
			return branchDegrade, "resume_retry_not_supported", 0.55, progressNone, "", "", true
		}
		if nextNoProgressRounds(session, progressKind) >= 2 {
			return "", "", 0, "", "", "", false
		}
		if withinIterationBudget(session) {
			return branchContinue, reasonPrefix + "_failed_retryable", 0.58, progressKind, "", "", true
		}
		return branchDegrade, "iteration_budget_exhausted", 0.45, progressKind, "", "", true
	default:
		return "", "", 0, "", "", "", false
	}
}

func requiresRuntimeApproval(session *agentruntime.RuntimeSession, spec agentcapability.Spec) bool {
	if spec.RequiresApproval {
		return true
	}
	if session == nil {
		return false
	}
	return session.Snapshot.Request.RuntimeOptions.RequireApproval || session.Request.Options.RequireApproval
}

func (p capabilityRuntimePolicy) activeFailure(session *agentruntime.RuntimeSession) (errorClass string, spec agentcapability.Spec, reasonPrefix string) {
	if session == nil {
		return "", agentcapability.Spec{}, ""
	}
	if p.preferWorkflowRun && session.Snapshot.Context.FetchErrorClass != "" {
		return session.Snapshot.Context.FetchErrorClass, p.workflowSpec, "external_evidence"
	}
	if session.Snapshot.Context.FetchErrorClass != "" {
		return session.Snapshot.Context.FetchErrorClass, p.fetchSpec, "fetch"
	}
	if session.Snapshot.Context.SearchErrorClass != "" {
		return session.Snapshot.Context.SearchErrorClass, p.searchSpec, "search"
	}
	return "", agentcapability.Spec{}, ""
}

func progressKindForRetry(reasonPrefix string) string {
	switch reasonPrefix {
	case "search":
		return progressRetryableSearchFailed
	case "fetch", "external_evidence":
		return progressRetryableFetchFailed
	default:
		return progressNone
	}
}

func rerunNodeForReasonPrefix(reasonPrefix string) string {
	switch reasonPrefix {
	case "search":
		return "search"
	case "fetch":
		return "fetch"
	case "external_evidence":
		return "external_evidence"
	default:
		return ""
	}
}
