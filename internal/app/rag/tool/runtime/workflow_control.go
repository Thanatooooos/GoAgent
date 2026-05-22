package runtime

import (
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	. "local/rag-project/internal/app/rag/tool/core"
)

func DeriveWorkflowControlWithRegistry(input WorkflowInput, results []Result, registry *Registry) WorkflowControl {
	return deriveWorkflowControlWithRegistry(input, results, registry)
}

func deriveWorkflowControlWithRegistry(input WorkflowInput, results []Result, registry *Registry) WorkflowControl {
	control := input.Control.Normalize()
	if control.Capability == CapabilityGeneral {
		control.Capability = inferWorkflowCapability(results, input.RetrieveResult, registry)
	}
	if control.ExecutionMode == ExecutionModeReadOnly {
		if mode := inferWorkflowExecutionMode(results, registry); mode != "" {
			control.ExecutionMode = mode
		}
	}
	if control.RiskLevel == RiskLevelLow {
		if level := inferWorkflowRiskLevel(results, registry); level != "" {
			control.RiskLevel = level
		}
	}
	if control.ApprovalRequirement == ApprovalRequirementNone {
		if requirement := inferWorkflowApprovalRequirement(results, registry); requirement != "" {
			control.ApprovalRequirement = requirement
		}
	}
	return control.Normalize()
}

func BuildWorkflowTraceMetaWithRegistry(control WorkflowControl, retrieve ragretrieve.Result, results []Result, registry *Registry) WorkflowTraceMeta {
	return buildWorkflowTraceMetaWithRegistry(control, retrieve, results, registry)
}

func buildWorkflowTraceMetaWithRegistry(control WorkflowControl, retrieve ragretrieve.Result, results []Result, registry *Registry) WorkflowTraceMeta {
	return WorkflowTraceMeta{
		Capability:          control.Capability,
		ExecutionMode:       control.ExecutionMode,
		RiskLevel:           control.RiskLevel,
		ApprovalRequirement: control.ApprovalRequirement,
		EvidenceSources:     inferWorkflowEvidenceSources(retrieve, results, registry),
	}.Normalize()
}

func inferWorkflowCapability(results []Result, retrieve ragretrieve.Result, registry *Registry) string {
	hasSearch := false
	hasDiagnosis := false
	hasKnowledge := false
	hasGeneral := false
	for _, result := range results {
		switch inferCapabilityFromResult(result, registry) {
		case CapabilitySearch:
			hasSearch = true
		case CapabilityDiagnosis:
			hasDiagnosis = true
		case CapabilityKnowledge:
			hasKnowledge = true
		case CapabilityGeneral:
			hasGeneral = true
		}
	}
	switch {
	case hasSearch:
		return CapabilitySearch
	case hasDiagnosis:
		return CapabilityDiagnosis
	case hasKnowledge:
		return CapabilityKnowledge
	case hasGeneral:
		return CapabilityGeneral
	case len(retrieve.Chunks) > 0:
		return CapabilityKnowledge
	default:
		return CapabilityGeneral
	}
}

func inferWorkflowEvidenceSources(retrieve ragretrieve.Result, results []Result, registry *Registry) []string {
	sources := make([]string, 0, 4)
	appendSource := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range sources {
			if existing == value {
				return
			}
		}
		sources = append(sources, value)
	}
	if len(retrieve.Chunks) > 0 {
		appendSource(EvidenceSourceKnowledgeBase)
	}
	for _, result := range results {
		if len(result.Meta.EvidenceSources) > 0 {
			for _, source := range result.Meta.EvidenceSources {
				appendSource(source)
			}
			continue
		}
		if registry != nil {
			if spec, ok := registry.GetSpec(result.Name); ok && len(spec.EvidenceSources) > 0 {
				for _, source := range spec.EvidenceSources {
					appendSource(source)
				}
			}
		}
	}
	return sources
}

func inferWorkflowExecutionMode(results []Result, registry *Registry) string {
	for _, result := range results {
		if mode := strings.TrimSpace(result.Meta.ExecutionMode); mode != "" {
			return mode
		}
		if registry != nil {
			if spec, ok := registry.GetSpec(result.Name); ok {
				if mode := strings.TrimSpace(spec.ExecutionMode); mode != "" {
					return mode
				}
			}
		}
	}
	return ""
}

func inferWorkflowRiskLevel(results []Result, registry *Registry) string {
	for _, result := range results {
		if level := strings.TrimSpace(result.Meta.RiskLevel); level != "" {
			return level
		}
		if registry != nil {
			if spec, ok := registry.GetSpec(result.Name); ok {
				if level := strings.TrimSpace(spec.RiskLevel); level != "" {
					return level
				}
			}
		}
	}
	return ""
}

func inferWorkflowApprovalRequirement(results []Result, registry *Registry) string {
	for _, result := range results {
		if requirement := strings.TrimSpace(result.Meta.ApprovalRequirement); requirement != "" {
			return requirement
		}
		if registry != nil {
			if spec, ok := registry.GetSpec(result.Name); ok {
				if requirement := strings.TrimSpace(spec.ApprovalRequirement); requirement != "" {
					return requirement
				}
			}
		}
	}
	return ""
}

func inferCapabilityFromResult(result Result, registry *Registry) string {
	if capability := strings.TrimSpace(result.Meta.Capability); capability != "" {
		return capability
	}
	if registry != nil {
		if spec, ok := registry.GetSpec(result.Name); ok {
			return strings.TrimSpace(spec.Capability)
		}
	}
	return ""
}
