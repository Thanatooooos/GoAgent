package runtime

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

// PendingApprovalCompat carries the minimum shared approval state needed to
// normalize a legacy interrupted session into the runtime approval contract.
type PendingApprovalCompat struct {
	Reason     string
	Capability string
	RerunNode  string
}

// PendingApprovalCompatResolver maps a legacy interrupted session onto the
// shared approval contract without exposing node-specific logic to service.
type PendingApprovalCompatResolver func(session *RuntimeSession) (PendingApprovalCompat, bool)

// NewLegacyPendingApprovalCompatResolver returns the runtime-owned legacy
// approval resolver used to normalize older checkpoint/session shapes.
func NewLegacyPendingApprovalCompatResolver(registry *agentcapability.Registry, bindings agentcapability.RoleBindings) PendingApprovalCompatResolver {
	return func(session *RuntimeSession) (PendingApprovalCompat, bool) {
		if session == nil {
			return PendingApprovalCompat{}, false
		}
		spec, capabilityName, rerunNode, ok := ResolveLegacyApprovalCapability(registry, bindings, session.Snapshot.Execution.CurrentNode)
		if !ok || !spec.RequiresApproval {
			return PendingApprovalCompat{}, false
		}
		return PendingApprovalCompat{
			Reason:     approvalRequiredReasonForNode(session.Snapshot.Execution.CurrentNode),
			Capability: capabilityName,
			RerunNode:  rerunNode,
		}, true
	}
}

// ResolveLegacyApprovalCapability keeps legacy node-to-capability knowledge in
// runtime so service orchestration does not need to special-case internal nodes.
func ResolveLegacyApprovalCapability(registry *agentcapability.Registry, bindings agentcapability.RoleBindings, node string) (agentcapability.Spec, string, string, bool) {
	switch strings.TrimSpace(node) {
	case "search":
		return resolveLegacyApprovalCapabilityForRole(registry, bindings, agentcapability.RoleSearch, "search")
	case "fetch":
		return resolveLegacyApprovalCapabilityForRole(registry, bindings, agentcapability.RoleFetch, "fetch")
	case "external_evidence":
		return resolveLegacyApprovalCapabilityForRole(registry, bindings, agentcapability.RoleCollectExternalEvidence, "external_evidence")
	default:
		return agentcapability.Spec{}, "", "", false
	}
}

func resolveLegacyApprovalCapabilityForRole(registry *agentcapability.Registry, bindings agentcapability.RoleBindings, role string, rerunNode string) (agentcapability.Spec, string, string, bool) {
	name := strings.TrimSpace(bindings.Resolve(role))
	if name == "" && registry != nil {
		resolved, err := agentcapability.ResolveBinding(registry, bindings, role)
		if err == nil {
			name = strings.TrimSpace(resolved)
		}
	}
	if name == "" || registry == nil {
		return agentcapability.Spec{}, "", "", false
	}
	spec, ok := registry.Spec(name)
	return spec, name, rerunNode, ok
}

func approvalRequiredReasonForNode(node string) string {
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
