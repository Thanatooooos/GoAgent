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

func deriveWorkflowControl(input WorkflowInput, results []Result, registry *Registry) WorkflowControl {
	return ragruntime.DeriveWorkflowControlWithRegistry(input, results, registry)
}

func buildWorkflowTraceMeta(control WorkflowControl, retrieve ragretrieve.Result, results []Result, registry *Registry) WorkflowTraceMeta {
	return ragruntime.BuildWorkflowTraceMetaWithRegistry(control, retrieve, results, registry)
}
