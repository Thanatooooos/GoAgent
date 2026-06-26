package agent

import (
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
	"local/rag-project/internal/framework/log"
)

func (s *Service) approvalPendingFromSession(session *agentruntime.RuntimeSession, fallbackCheckpointID string) *ApprovalPending {
	if session == nil {
		return nil
	}

	approval := session.Snapshot.Approval
	node := firstNonEmpty(approval.Node, session.Snapshot.Execution.CurrentNode)
	sourceNode := approvalSourceNode(session)
	spec, capabilityName, ok := s.approvalSpecForSession(session, firstNonEmpty(sourceNode, node))
	schedule := agentruntime.CapabilityScheduleResult{}
	if ok {
		schedule = agentruntime.EvaluateCapabilitySchedule(agentruntime.CapabilityScheduleInput{
			Session:             session,
			RuntimeOptions:      sessionApprovalRuntimeOptions(session),
			Snapshot:            session.Snapshot,
			PatternAction:       "approval_projection",
			Spec:                spec,
			SkipInputValidation: true,
		})
	}
	step, hasStep := approvalPlanStep(session, capabilityName)

	searchQuery, err := approvalSearchQuery(session, step, hasStep)
	if err != nil {
		log.Warnf("agent approval: failed to extract search query from capability input: %v", err)
		searchQuery = ""
	}
	candidateURLs, err := approvalCandidateURLs(session, step, hasStep)
	if err != nil {
		log.Warnf("agent approval: failed to extract candidate URLs from capability input: %v", err)
		candidateURLs = nil
	}

	return &ApprovalPending{
		Required:              true,
		Status:                firstNonEmpty(approval.Status, agentstate.ApprovalStatusPending),
		Reason:                approval.Reason,
		ReasonCode:            approval.Reason,
		ReasonMessage:         approvalReasonMessage(approval.Reason, capabilityName),
		Trigger:               approvalTrigger(session, sourceNode, node, capabilityName),
		Node:                  node,
		RerunNode:             approval.RerunNode,
		Capability:            capabilityName,
		CapabilityName:        capabilityName,
		CapabilityKind:        approvalSpecField(ok, spec.Kind),
		CapabilityFamily:      approvalSpecField(ok, spec.Family),
		CapabilityDescription: approvalSpecField(ok, spec.Description),
		RiskLevel:             strings.TrimSpace(schedule.RiskLevel),
		SupportsParallel:      schedule.SupportsParallel,
		SupportsResume:        schedule.SupportsResume,
		Idempotency:           strings.TrimSpace(schedule.Idempotency),
		CheckpointID:          firstNonEmpty(approval.CheckpointID, fallbackCheckpointID),
		SessionID:             strings.TrimSpace(session.SessionID),
		RequestedAt:           approval.RequestedAt,
		ResumeCount:           session.Metadata.ResumeCount,
		Question:              approvalQuestion(session),
		SearchQuery:           searchQuery,
		CurrentStepID:         approvalPlanStepField(hasStep, step.StepID),
		CurrentStepTitle:      approvalPlanStepField(hasStep, step.Title),
		CandidateURLs:         candidateURLs,
		CanApprove:            true,
		CanReject:             true,
		RejectOutcome:         RunStatusDegraded,
	}
}

func approvalSourceNode(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	if session.Checkpoint != nil {
		if node := strings.TrimSpace(session.Checkpoint.Node); node != "" {
			return node
		}
	}
	if node := strings.TrimSpace(session.Snapshot.Execution.CurrentNode); node != "" {
		return node
	}
	return strings.TrimSpace(session.Snapshot.Approval.RerunNode)
}

func sessionApprovalRuntimeOptions(session *agentruntime.RuntimeSession) agentstate.RuntimeOptions {
	if session == nil {
		return agentstate.RuntimeOptions{}
	}
	if session.Snapshot.Request.RuntimeOptions != (agentstate.RuntimeOptions{}) {
		return session.Snapshot.Request.RuntimeOptions
	}
	return session.Request.Options
}

func (s *Service) approvalSpecForSession(session *agentruntime.RuntimeSession, node string) (agentcapability.Spec, string, bool) {
	if session == nil {
		return agentcapability.Spec{}, "", false
	}

	capabilityName := strings.TrimSpace(session.Snapshot.Approval.Capability)
	if capabilityName != "" && s != nil && s.registry != nil {
		spec, ok := s.registry.Spec(capabilityName)
		if ok {
			return spec, capabilityName, true
		}
	}

	spec, resolvedName, _, ok := s.approvalCapabilityForNode(node)
	if ok {
		return spec, firstNonEmpty(capabilityName, resolvedName), true
	}
	return agentcapability.Spec{}, capabilityName, false
}

func approvalReasonMessage(reasonCode string, capabilityName string) string {
	switch strings.TrimSpace(reasonCode) {
	case "search_approval_required":
		return "Approval is required before searching external sources."
	case "fetch_approval_required":
		return "Approval is required before fetching external page content."
	case "external_evidence_approval_required":
		return "Approval is required before collecting external evidence."
	case "approval_rejected":
		return "The required approval was rejected, so the run cannot continue."
	case "approval_required":
		return "Approval is required before the runtime can continue."
	default:
		if trimmed := strings.TrimSpace(capabilityName); trimmed != "" {
			return "Approval is required before the runtime can continue with " + trimmed + "."
		}
		return "Approval is required before the runtime can continue."
	}
}

func approvalTrigger(session *agentruntime.RuntimeSession, sourceNode string, node string, capabilityName string) string {
	if session == nil {
		return ""
	}

	switch strings.TrimSpace(capabilityName) {
	case agentcapability.NameWebSearch:
		if session.Snapshot.Context.SearchErrorClass == agentcapability.ErrorClassPermission {
			return "capability_permission_error"
		}
	case agentcapability.NameWebFetch, agentcapability.NameExternalEvidenceCollect:
		if session.Snapshot.Context.FetchErrorClass == agentcapability.ErrorClassPermission {
			return "capability_permission_error"
		}
	}

	if strings.TrimSpace(sourceNode) == "approval" || (strings.TrimSpace(sourceNode) == "" && strings.TrimSpace(node) == "approval") {
		return "approval_gate"
	}
	if strings.TrimSpace(sourceNode) != "" {
		return "interrupt_before_node"
	}
	return ""
}

func approvalSpecField(ok bool, value string) string {
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func approvalPlanStepField(hasStep bool, value string) string {
	if !hasStep {
		return ""
	}
	return strings.TrimSpace(value)
}

func approvalPlanStep(session *agentruntime.RuntimeSession, capabilityName string) (agentstate.PlanStep, bool) {
	if session == nil {
		return agentstate.PlanStep{}, false
	}

	plan := session.Snapshot.Plan
	if plan.CurrentStepIndex >= 0 && plan.CurrentStepIndex < len(plan.Steps) {
		return plan.Steps[plan.CurrentStepIndex], true
	}

	trimmedCapability := strings.TrimSpace(capabilityName)
	if trimmedCapability == "" {
		return agentstate.PlanStep{}, false
	}
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.CapabilityName) != trimmedCapability {
			continue
		}
		if step.Status == agentstate.PlanStepStatusPending || step.Status == agentstate.PlanStepStatusRunning {
			return step, true
		}
	}
	return agentstate.PlanStep{}, false
}

func approvalQuestion(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	return firstNonEmpty(
		session.Snapshot.Request.Question,
		session.Request.Question,
	)
}

func approvalSearchQuery(session *agentruntime.RuntimeSession, step agentstate.PlanStep, hasStep bool) (string, error) {
	if hasStep {
		capInput, err := stringCapabilityInput(step.CapabilityInput, "query")
		if err != nil {
			return "", err
		}
		if query := firstNonEmpty(step.Query, capInput); query != "" {
			return query, nil
		}
	}
	if session == nil {
		return "", nil
	}
	return firstNonEmpty(
		session.Snapshot.Context.SearchQuery,
		session.Snapshot.Context.RewrittenQuery,
		session.Snapshot.Request.Question,
		session.Request.Question,
	), nil
}

func approvalCandidateURLs(session *agentruntime.RuntimeSession, step agentstate.PlanStep, hasStep bool) ([]string, error) {
	candidates := make([]string, 0, 4)
	if hasStep {
		candidates = append(candidates, step.URLs...)
		urls, err := stringSliceCapabilityInput(step.CapabilityInput, "urls")
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, urls...)
	}
	if session != nil {
		candidates = append(candidates, session.Snapshot.Context.PreferredURLs...)
		for _, result := range session.Snapshot.Context.SearchResults {
			candidates = append(candidates, result.URL)
		}
	}
	return uniqueNonEmptyStrings(candidates...), nil
}

func stringCapabilityInput(values map[string]any, key string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	value, ok := values[key]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("capability input key %q has unexpected type %T, expected string", key, value)
	}
	return strings.TrimSpace(text), nil
}

func stringSliceCapabilityInput(values map[string]any, key string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	value, ok := values[key]
	if !ok {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return uniqueNonEmptyStrings(typed...), nil
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("capability input key %q contains element of unexpected type %T, expected string", key, item)
			}
			items = append(items, text)
		}
		return uniqueNonEmptyStrings(items...), nil
	default:
		return nil, fmt.Errorf("capability input key %q has unexpected type %T, expected []string or []any", key, value)
	}
}

func uniqueNonEmptyStrings(values ...string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
