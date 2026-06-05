package reactive

import (
	agentcapability "local/rag-project/internal/app/agent/capability"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
)

// HandoffBindings projects reactive role bindings into the node-to-capability
// bindings the generic handoff builder needs.
func HandoffBindings(bindings agentcapability.RoleBindings) []agenthandoff.NodeCapabilityBinding {
	projected := make([]agenthandoff.NodeCapabilityBinding, 0, 3)
	if search := bindings.Resolve(agentcapability.RoleSearch); search != "" {
		projected = append(projected, agenthandoff.NodeCapabilityBinding{
			Node:       "search",
			Capability: search,
		})
	}
	if fetch := bindings.Resolve(agentcapability.RoleFetch); fetch != "" {
		projected = append(projected, agenthandoff.NodeCapabilityBinding{
			Node:       "fetch",
			Capability: fetch,
		})
	}
	if workflow := bindings.Resolve(agentcapability.RoleCollectExternalEvidence); workflow != "" {
		projected = append(projected, agenthandoff.NodeCapabilityBinding{
			Node:       "external_evidence",
			Capability: workflow,
		})
	}
	return projected
}
