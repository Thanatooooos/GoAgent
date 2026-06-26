package reactive

import (
	"context"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentplanner "local/rag-project/internal/app/agent/planner"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newObserveNode(planner agentplanner.Planner, outputMode string, capabilityPolicy capabilityRuntimePolicy) (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("observe", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		policy := evaluateObservePolicy(session, outputMode, capabilityPolicy)
		policy = maybeOverrideObservePolicy(ctx, planner, session, policy, outputMode)
		evidenceItems := buildNewEvidence(session)
		sufficient := policy.Answerable

		openQuestions := []string{"No readable fetched content was available."}
		if sufficient || policy.Branch == branchContinue {
			openQuestions = nil
		}

		contextDelta := &agentstate.ContextDelta{
			SeenURLs: fetchURLs(session),
		}
		if policy.NextQuery != "" {
			contextDelta.SearchQuery = stringPtr(policy.NextQuery)
		}
		contextDelta.PreferredURLs = stringSlicePtr(policy.PreferredURLs)
		contextDelta.AvoidURLs = stringSlicePtr(policy.AvoidURLs)

		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Evidence: &agentstate.EvidenceDelta{
					AddItems:          evidenceItems,
					Sufficient:        &sufficient,
					SufficiencyReason: &policy.Reason,
					NewItemsThisRound: intPtr(policy.NewEvidenceCount),
					OpenQuestions:     openQuestions,
				},
				Context:   contextDelta,
				Approval:  approvalDeltaForPolicy(session, policy),
				Execution: executionObserveDeltaForDecision(policy),
			},
			Decision: &agentruntime.DecisionArtifact{
				Kind:       "branch",
				Target:     policy.Branch,
				Confidence: policy.Confidence,
				Reasoning:  policy.Reason,
			},
		}, nil
	})
}

func approvalDeltaForPolicy(session *agentruntime.RuntimeSession, policy observePolicyResult) *agentstate.ApprovalDelta {
	if policy.Branch != branchApproval {
		return nil
	}
	now := time.Now()
	checkpointID := ""
	if session != nil && session.Checkpoint != nil {
		checkpointID = session.Checkpoint.ID
	}
	return agentruntime.BuildPendingApprovalDelta(
		policy.Reason,
		policy.ApprovalCapability,
		policy.ApprovalRerunNode,
		checkpointID,
		now,
	)
}

func maybeOverrideObservePolicy(ctx context.Context, planner agentplanner.Planner, session *agentruntime.RuntimeSession, baseline observePolicyResult, outputMode string) observePolicyResult {
	if planner == nil || session == nil {
		return baseline
	}

	result, err := planner.Plan(ctx, agentplanner.PlanInput{
		Session:          session,
		BaselineDecision: baseline.Branch,
		BaselineReason:   baseline.Reason,
	})
	if err != nil {
		return baseline
	}
	if result.Decision == "" {
		return baseline
	}

	updated := baseline
	updated.Branch = result.Decision
	updated.Reason = result.Reason
	updated.Confidence = result.Confidence
	updated.NextQuery = result.NextQuery
	updated.PreferredURLs = append([]string(nil), result.PreferredURLs...)
	updated.AvoidURLs = append([]string(nil), result.AvoidURLs...)
	if isTerminalAnswerLike(result.Decision) {
		updated.Answerable = true
	}
	if updated.Answerable && updated.Branch == branchAnswer && effectiveOutputMode(session, outputMode) == agentstate.OutputModeHandoff {
		updated.Branch = branchHandoff
	}
	return updated
}

func branchOnEvidence(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
	_ = ctx
	if session == nil {
		return branchDegrade, nil
	}
	if session.Snapshot.Execution.LastBranchTarget != "" {
		return session.Snapshot.Execution.LastBranchTarget, nil
	}
	if session.Snapshot.Evidence.Sufficient {
		if effectiveOutputMode(session, agentstate.OutputModeFinalAnswer) == agentstate.OutputModeHandoff {
			return branchHandoff, nil
		}
		return branchAnswer, nil
	}
	return branchDegrade, nil
}

const (
	branchHandoff  = "handoff"
	branchAnswer   = "answer"
	branchContinue = "continue"
	branchApproval = "approval"
	branchDegrade  = "degrade"
)

func effectiveOutputMode(session *agentruntime.RuntimeSession, configured string) string {
	if session != nil {
		if mode := session.Snapshot.Request.RuntimeOptions.OutputMode; mode != "" {
			return mode
		}
		if mode := session.Request.Options.OutputMode; mode != "" {
			return mode
		}
	}
	if configured != "" {
		return configured
	}
	return agentstate.OutputModeFinalAnswer
}

func isTerminalAnswerLike(decision string) bool {
	return decision == branchAnswer || decision == branchHandoff
}

func effectiveMaxIterations(session *agentruntime.RuntimeSession) int {
	if session == nil {
		return defaultMaxIterations
	}
	if session.Snapshot.Execution.MaxIterations > 0 {
		return session.Snapshot.Execution.MaxIterations
	}
	if session.Snapshot.Request.RuntimeOptions.MaxIterations > 0 {
		return session.Snapshot.Request.RuntimeOptions.MaxIterations
	}
	if session.Request.Options.MaxIterations > 0 {
		return session.Request.Options.MaxIterations
	}
	return defaultMaxIterations
}

func withinIterationBudget(session *agentruntime.RuntimeSession) bool {
	if session == nil {
		return false
	}
	nextIteration := session.Snapshot.Execution.Iteration + 1
	return nextIteration < effectiveMaxIterations(session)
}

func hasFetchableURLs(session *agentruntime.RuntimeSession) bool {
	return len(fetchURLs(session)) > 0
}

func hasRetryableFetchFailure(session *agentruntime.RuntimeSession) bool {
	if session == nil {
		return false
	}
	if session.Snapshot.Context.FetchErrorClass != "" && session.Snapshot.Context.FetchErrorClass != agentcapability.ErrorClassExternal {
		return false
	}
	for _, result := range session.Snapshot.Context.FetchResults {
		if result.Degraded {
			return true
		}
	}
	return false
}

func hasRetryableSearchFailure(session *agentruntime.RuntimeSession) bool {
	if session == nil {
		return false
	}
	return session.Snapshot.Context.SearchErrorClass == agentcapability.ErrorClassExternal
}

func countNewURLs(session *agentruntime.RuntimeSession) int {
	if session == nil {
		return 0
	}
	return countStringsNotSeen(fetchURLs(session), session.Snapshot.Context.SeenURLs)
}

func nextNoProgressRounds(session *agentruntime.RuntimeSession, progressKind string) int {
	if progressKind == progressEvidenceGained || progressKind == progressNewSourcesFound {
		return 0
	}
	if session == nil {
		return 1
	}
	return session.Snapshot.Execution.ConsecutiveNoProgressRounds + 1
}

func executionObserveDeltaForDecision(policy observePolicyResult) *agentstate.ExecutionDelta {
	delta := executionObserveDelta()
	delta.LastBranchTarget = stringPtr(policy.Branch)
	delta.LastBranchReason = stringPtr(policy.Reason)
	delta.LastProgressKind = stringPtr(policy.ProgressKind)
	delta.LastNewURLCount = intPtr(policy.NewURLCount)
	delta.LastNewEvidenceCount = intPtr(policy.NewEvidenceCount)
	delta.ConsecutiveNoProgressRounds = intPtr(policy.NoProgressRounds)
	return delta
}

func buildNewEvidence(session *agentruntime.RuntimeSession) []agentstate.EvidenceItem {
	if session == nil {
		return nil
	}
	existing := make(map[string]struct{}, len(session.Snapshot.Evidence.Items))
	for _, item := range session.Snapshot.Evidence.Items {
		key := evidenceKey(item.ID, item.SourceRef, item.Content)
		existing[key] = struct{}{}
	}
	items := make([]agentstate.EvidenceItem, 0, len(session.Snapshot.Context.FetchResults))
	for idx, result := range session.Snapshot.Context.FetchResults {
		if result.Degraded || result.Summary == "" {
			continue
		}
		item := agentstate.EvidenceItem{
			ID:        result.ID,
			Source:    "fetch",
			Content:   result.Summary,
			Level:     "high",
			SourceRef: result.ContentRef,
		}
		key := evidenceKey(item.ID, item.SourceRef, item.Content)
		if _, ok := existing[key]; ok {
			continue
		}
		existing[key] = struct{}{}
		items = append(items, item)
		if idx >= 1 {
			break
		}
	}
	return items
}

func evidenceKey(id, sourceRef, content string) string {
	return id + "|" + sourceRef + "|" + content
}

func countStringsNotSeen(values []string, seen []string) int {
	if len(values) == 0 {
		return 0
	}
	seenSet := make(map[string]struct{}, len(seen))
	for _, value := range seen {
		if value == "" {
			continue
		}
		seenSet[value] = struct{}{}
	}
	count := 0
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seenSet[value]; ok {
			continue
		}
		count++
	}
	return count
}

func intPtr(value int) *int {
	return &value
}

func stringSlicePtr(values []string) *[]string {
	cloned := append([]string(nil), values...)
	return &cloned
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
