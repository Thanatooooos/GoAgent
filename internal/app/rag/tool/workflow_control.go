package tool

import (
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
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

var legacySpecRegistry *ragcore.Registry

func deriveWorkflowControl(input WorkflowInput, results []Result) WorkflowControl {
	return ragruntime.DeriveWorkflowControlWithRegistry(input, results, legacySpecRegistry)
}

func buildWorkflowTraceMeta(control WorkflowControl, retrieve ragretrieve.Result, results []Result) WorkflowTraceMeta {
	return ragruntime.BuildWorkflowTraceMetaWithRegistry(control, retrieve, results, legacySpecRegistry)
}

// SetLegacySpecRegistry keeps the legacy facade tests and fallback helpers working.
// The production runtime no longer relies on this global registry.
func SetLegacySpecRegistry(r *ragcore.Registry) {
	legacySpecRegistry = r
}
