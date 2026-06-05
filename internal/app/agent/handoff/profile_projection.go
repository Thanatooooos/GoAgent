package handoff

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

// NodeCapabilityBinding associates a runtime node name with a registered
// capability name for handoff/profile projection.
type NodeCapabilityBinding struct {
	Node       string
	Capability string
}

// NewBuilderFromRegistry constructs a handoff builder from capability metadata
// instead of hand-built profiles.
func NewBuilderFromRegistry(registry *agentcapability.Registry, bindings []NodeCapabilityBinding) *Builder {
	return NewBuilder(BuildCapabilityProfiles(registry, bindings))
}

// BuildCapabilityProfiles projects registry specs into handoff profiles using
// explicit node-to-capability bindings.
func BuildCapabilityProfiles(registry *agentcapability.Registry, bindings []NodeCapabilityBinding) []CapabilityProfile {
	if registry == nil || len(bindings) == 0 {
		return nil
	}

	profiles := make([]CapabilityProfile, 0, len(bindings))
	for _, binding := range bindings {
		node := strings.TrimSpace(binding.Node)
		capabilityName := strings.TrimSpace(binding.Capability)
		if node == "" || capabilityName == "" {
			continue
		}
		spec, ok := registry.Spec(capabilityName)
		if !ok {
			continue
		}
		profiles = append(profiles, capabilityProfileForNode(node, spec))
	}
	return profiles
}

func capabilityProfileForNode(node string, spec agentcapability.Spec) CapabilityProfile {
	return CapabilityProfile{
		Node:               strings.TrimSpace(node),
		Name:               strings.TrimSpace(spec.Name),
		WorkflowCapability: workflowCapabilityForSpec(spec),
		Kind:               strings.TrimSpace(spec.Kind),
		RiskLevel:          strings.TrimSpace(spec.RiskLevel),
		RequiresApproval:   spec.RequiresApproval,
		SupportsParallel:   spec.SupportsParallel,
		SupportsResume:     spec.SupportsResume,
	}
}

func workflowCapabilityForSpec(spec agentcapability.Spec) string {
	switch strings.TrimSpace(spec.Family) {
	case agentcapability.FamilyExternalEvidence:
		return "search"
	case agentcapability.FamilyDocumentInvestigation, agentcapability.FamilyTraceInvestigation:
		return "diagnosis"
	case agentcapability.FamilyDiscovery:
		return "knowledge"
	default:
		return "general"
	}
}
