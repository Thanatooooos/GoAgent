package handoff

import (
	"fmt"
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	executionModeReadOnly       = "read_only"
	approvalRequirementNone     = "none"
	approvalRequirementRequired = "required"
	workflowCapabilityGeneral   = "general"
	riskLevelLow                = "low"
	riskLevelMedium             = "medium"
	riskLevelHigh               = "high"
)

type WorkflowPolicySummary struct {
	Capability          string `json:"capability,omitempty"`
	ExecutionMode       string `json:"execution_mode,omitempty"`
	RiskLevel           string `json:"risk_level,omitempty"`
	ApprovalRequirement string `json:"approval_requirement,omitempty"`
	OutputMode          string `json:"output_mode,omitempty"`
	AllowWebSearch      bool   `json:"allow_web_search,omitempty"`
	MaxIterations       int    `json:"max_iterations,omitempty"`
}

func (b *Builder) BuildWorkflowPolicy(session *agentruntime.RuntimeSession) string {
	summary := b.BuildWorkflowPolicySummary(session)
	if summary == (WorkflowPolicySummary{}) {
		return ""
	}

	lines := []string{
		"capability: " + summary.Capability,
		"execution_mode: " + summary.ExecutionMode,
		"risk_level: " + summary.RiskLevel,
		"approval_requirement: " + summary.ApprovalRequirement,
		"output_mode: " + summary.OutputMode,
	}
	if summary.AllowWebSearch {
		lines = append(lines, "allow_web_search: true")
	}
	if summary.MaxIterations > 0 {
		lines = append(lines, fmt.Sprintf("max_iterations: %d", summary.MaxIterations))
	}
	return strings.Join(lines, "\n")
}

func (b *Builder) BuildWorkflowPolicySummary(session *agentruntime.RuntimeSession) WorkflowPolicySummary {
	if session == nil {
		return WorkflowPolicySummary{}
	}

	profiles := b.usedProfiles(session)
	return WorkflowPolicySummary{
		Capability:          inferWorkflowCapability(profiles),
		ExecutionMode:       inferExecutionMode(profiles),
		RiskLevel:           inferRiskLevel(profiles),
		ApprovalRequirement: inferApprovalRequirement(session, profiles),
		OutputMode:          firstNonEmpty(session.Snapshot.Request.RuntimeOptions.OutputMode, session.Request.Options.OutputMode, agentstate.OutputModeHandoff),
		AllowWebSearch:      session.Snapshot.Request.RuntimeOptions.AllowWebSearch || session.Request.Options.AllowWebSearch,
		MaxIterations:       maxIterations(session),
	}
}

func (b *Builder) usedProfiles(session *agentruntime.RuntimeSession) []CapabilityProfile {
	if b == nil || len(b.profiles) == 0 || session == nil {
		return nil
	}

	usedNodes := make(map[string]struct{})
	for _, event := range session.Journal {
		switch event.EventType {
		case agentstate.EventTypeCapabilityStart, agentstate.EventTypeCapabilityResult, agentstate.EventTypeCapabilitySkipped:
			node := strings.TrimSpace(event.Node)
			if node != "" {
				usedNodes[node] = struct{}{}
			}
		}
	}
	if len(usedNodes) == 0 {
		if len(session.Snapshot.Context.SearchResults) > 0 || strings.TrimSpace(session.Snapshot.Context.SearchQuery) != "" {
			usedNodes["search"] = struct{}{}
		}
		if len(session.Snapshot.Context.FetchResults) > 0 {
			usedNodes["fetch"] = struct{}{}
		}
	}
	if len(usedNodes) == 0 {
		return nil
	}

	profiles := make([]CapabilityProfile, 0, len(usedNodes))
	for node := range usedNodes {
		profile, ok := b.profiles[node]
		if !ok {
			continue
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func inferWorkflowCapability(profiles []CapabilityProfile) string {
	if len(profiles) == 0 {
		return workflowCapabilityGeneral
	}
	values := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		capability := strings.TrimSpace(profile.WorkflowCapability)
		if capability == "" {
			continue
		}
		values = append(values, capability)
	}
	if len(values) == 0 {
		return workflowCapabilityGeneral
	}
	return highestPriorityCapability(values)
}

func highestPriorityCapability(values []string) string {
	priority := map[string]int{
		"search":                  3,
		"diagnosis":               2,
		"knowledge":               1,
		workflowCapabilityGeneral: 0,
	}
	best := workflowCapabilityGeneral
	bestScore := -1
	for _, value := range values {
		score, ok := priority[value]
		if !ok {
			score = 0
		}
		if score > bestScore {
			best = value
			bestScore = score
		}
	}
	return best
}

func inferExecutionMode(profiles []CapabilityProfile) string {
	_ = profiles
	return executionModeReadOnly
}

func inferRiskLevel(profiles []CapabilityProfile) string {
	if len(profiles) == 0 {
		return riskLevelLow
	}
	best := riskLevelLow
	for _, profile := range profiles {
		switch strings.TrimSpace(profile.RiskLevel) {
		case riskLevelHigh:
			return riskLevelHigh
		case riskLevelMedium:
			best = riskLevelMedium
		}
	}
	return best
}

func inferApprovalRequirement(session *agentruntime.RuntimeSession, profiles []CapabilityProfile) string {
	if session != nil && session.Snapshot.Request.RuntimeOptions.RequireApproval {
		return approvalRequirementRequired
	}
	for _, profile := range profiles {
		if profile.RequiresApproval {
			return approvalRequirementRequired
		}
	}
	return approvalRequirementNone
}

func maxIterations(session *agentruntime.RuntimeSession) int {
	if session == nil {
		return 0
	}
	if session.Snapshot.Execution.MaxIterations > 0 {
		return session.Snapshot.Execution.MaxIterations
	}
	if session.Snapshot.Request.RuntimeOptions.MaxIterations > 0 {
		return session.Snapshot.Request.RuntimeOptions.MaxIterations
	}
	return session.Request.Options.MaxIterations
}
