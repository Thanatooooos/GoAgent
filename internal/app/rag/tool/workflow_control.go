package tool

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

const (
	ExecutionModeReadOnly     = ragcore.ExecutionModeReadOnly
	ExecutionModeProposalOnly = ragcore.ExecutionModeProposalOnly
	ExecutionModeGuardedWrite = ragcore.ExecutionModeGuardedWrite

	RiskLevelLow    = ragcore.RiskLevelLow
	RiskLevelMedium = ragcore.RiskLevelMedium
	RiskLevelHigh   = ragcore.RiskLevelHigh

	ApprovalRequirementNone        = ragcore.ApprovalRequirementNone
	ApprovalRequirementRecommended = ragcore.ApprovalRequirementRecommended
	ApprovalRequirementRequired    = ragcore.ApprovalRequirementRequired

	CapabilityKnowledge = ragcore.CapabilityKnowledge
	CapabilityDiagnosis = ragcore.CapabilityDiagnosis
	CapabilitySearch    = ragcore.CapabilitySearch
	CapabilityGeneral   = ragcore.CapabilityGeneral

	EvidenceSourceKnowledgeBase = ragcore.EvidenceSourceKnowledgeBase
	EvidenceSourceSystemRecords = ragcore.EvidenceSourceSystemRecords
	EvidenceSourceRAGTrace      = ragcore.EvidenceSourceRAGTrace
	EvidenceSourceExternalWeb   = ragcore.EvidenceSourceExternalWeb
)

type WorkflowControl = ragcore.WorkflowControl
type WorkflowTraceMeta = ragcore.WorkflowTraceMeta

func deriveWorkflowControl(input WorkflowInput, results []Result) WorkflowControl {
	control := input.Control.Normalize()
	if control.Capability == CapabilityGeneral {
		control.Capability = inferWorkflowCapability(results, input.RetrieveResult)
	}
	if control.ExecutionMode == ExecutionModeReadOnly {
		if mode := inferWorkflowExecutionMode(results); mode != "" {
			control.ExecutionMode = mode
		}
	}
	if control.RiskLevel == RiskLevelLow {
		if level := inferWorkflowRiskLevel(results); level != "" {
			control.RiskLevel = level
		}
	}
	if control.ApprovalRequirement == ApprovalRequirementNone {
		if requirement := inferWorkflowApprovalRequirement(results); requirement != "" {
			control.ApprovalRequirement = requirement
		}
	}
	return control.Normalize()
}

func buildWorkflowTraceMeta(control WorkflowControl, retrieve ragretrieve.Result, results []Result) WorkflowTraceMeta {
	return WorkflowTraceMeta{
		Capability:          control.Capability,
		ExecutionMode:       control.ExecutionMode,
		RiskLevel:           control.RiskLevel,
		ApprovalRequirement: control.ApprovalRequirement,
		EvidenceSources:     inferWorkflowEvidenceSources(retrieve, results),
	}.Normalize()
}

func inferWorkflowCapability(results []Result, retrieve ragretrieve.Result) string {
	hasSearch := false
	hasDiagnosis := false
	hasKnowledge := false
	hasGeneral := false

	for _, result := range results {
		switch inferCapabilityFromResult(result) {
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

var legacySpecRegistry *ragcore.Registry

// SetLegacySpecRegistry makes the Registry available for legacy spec inference.
func SetLegacySpecRegistry(r *ragcore.Registry) {
	legacySpecRegistry = r
}

func inferWorkflowEvidenceSources(retrieve ragretrieve.Result, results []Result) []string {
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
		if legacySpecRegistry != nil {
			if spec, ok := legacySpecRegistry.GetSpec(result.Name); ok && len(spec.EvidenceSources) > 0 {
				for _, source := range spec.EvidenceSources {
					appendSource(source)
				}
			}
		}
	}
	return sources
}

func inferWorkflowExecutionMode(results []Result) string {
	for _, result := range results {
		if mode := strings.TrimSpace(result.Meta.ExecutionMode); mode != "" {
			return mode
		}
		if legacySpecRegistry != nil {
			if spec, ok := legacySpecRegistry.GetSpec(result.Name); ok {
				if mode := strings.TrimSpace(spec.ExecutionMode); mode != "" {
					return mode
				}
			}
		}
	}
	return ""
}

func inferWorkflowRiskLevel(results []Result) string {
	for _, result := range results {
		if level := strings.TrimSpace(result.Meta.RiskLevel); level != "" {
			return level
		}
		if legacySpecRegistry != nil {
			if spec, ok := legacySpecRegistry.GetSpec(result.Name); ok {
				if level := strings.TrimSpace(spec.RiskLevel); level != "" {
					return level
				}
			}
		}
	}
	return ""
}

func inferWorkflowApprovalRequirement(results []Result) string {
	for _, result := range results {
		if requirement := strings.TrimSpace(result.Meta.ApprovalRequirement); requirement != "" {
			return requirement
		}
		if legacySpecRegistry != nil {
			if spec, ok := legacySpecRegistry.GetSpec(result.Name); ok {
				if requirement := strings.TrimSpace(spec.ApprovalRequirement); requirement != "" {
					return requirement
				}
			}
		}
	}
	return ""
}

func inferCapabilityFromResult(result Result) string {
	if capability := strings.TrimSpace(result.Meta.Capability); capability != "" {
		return capability
	}
	if legacySpecRegistry != nil {
		if spec, ok := legacySpecRegistry.GetSpec(result.Name); ok {
			return strings.TrimSpace(spec.Capability)
		}
	}
	return ""
}
