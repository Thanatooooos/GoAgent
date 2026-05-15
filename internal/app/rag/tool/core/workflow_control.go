package core

import (
	"fmt"
	"strings"
)

const (
	ExecutionModeReadOnly     = "read_only"
	ExecutionModeProposalOnly = "proposal_only"
	ExecutionModeGuardedWrite = "guarded_write"

	RiskLevelLow    = "low"
	RiskLevelMedium = "medium"
	RiskLevelHigh   = "high"

	ApprovalRequirementNone        = "none"
	ApprovalRequirementRecommended = "recommended"
	ApprovalRequirementRequired    = "required"

	CapabilityKnowledge = "knowledge"
	CapabilityDiagnosis = "diagnosis"
	CapabilitySearch    = "search"
	CapabilityGeneral   = "general"

	EvidenceSourceKnowledgeBase = "knowledge_base"
	EvidenceSourceSystemRecords = "system_records"
	EvidenceSourceRAGTrace      = "rag_trace"
	EvidenceSourceExternalWeb   = "external_web"
)

type WorkflowControl struct {
	Capability          string
	ExecutionMode       string
	RiskLevel           string
	ApprovalRequirement string
}

func (c WorkflowControl) Normalize() WorkflowControl {
	c.Capability = strings.TrimSpace(strings.ToLower(c.Capability))
	if c.Capability == "" {
		c.Capability = CapabilityGeneral
	}
	switch strings.TrimSpace(strings.ToLower(c.ExecutionMode)) {
	case ExecutionModeProposalOnly, ExecutionModeGuardedWrite:
		c.ExecutionMode = strings.TrimSpace(strings.ToLower(c.ExecutionMode))
	default:
		c.ExecutionMode = ExecutionModeReadOnly
	}
	switch strings.TrimSpace(strings.ToLower(c.RiskLevel)) {
	case RiskLevelMedium, RiskLevelHigh:
		c.RiskLevel = strings.TrimSpace(strings.ToLower(c.RiskLevel))
	default:
		c.RiskLevel = RiskLevelLow
	}
	switch strings.TrimSpace(strings.ToLower(c.ApprovalRequirement)) {
	case ApprovalRequirementRecommended, ApprovalRequirementRequired:
		c.ApprovalRequirement = strings.TrimSpace(strings.ToLower(c.ApprovalRequirement))
	default:
		c.ApprovalRequirement = ApprovalRequirementNone
	}
	return c
}

func (c WorkflowControl) PromptString() string {
	c = c.Normalize()
	return fmt.Sprintf("能力域：%s\n执行模式：%s\n风险等级：%s\n审批要求：%s", c.Capability, c.ExecutionMode, c.RiskLevel, c.ApprovalRequirement)
}

type WorkflowTraceMeta struct {
	Capability          string   `json:"capability"`
	ExecutionMode       string   `json:"executionMode"`
	RiskLevel           string   `json:"riskLevel"`
	ApprovalRequirement string   `json:"approvalRequirement"`
	EvidenceSources     []string `json:"evidenceSources,omitempty"`
}

func (m WorkflowTraceMeta) Normalize() WorkflowTraceMeta {
	control := WorkflowControl{
		Capability:          m.Capability,
		ExecutionMode:       m.ExecutionMode,
		RiskLevel:           m.RiskLevel,
		ApprovalRequirement: m.ApprovalRequirement,
	}.Normalize()
	m.Capability = control.Capability
	m.ExecutionMode = control.ExecutionMode
	m.RiskLevel = control.RiskLevel
	m.ApprovalRequirement = control.ApprovalRequirement
	m.EvidenceSources = UniqueTrimmedStrings(m.EvidenceSources)
	return m
}
