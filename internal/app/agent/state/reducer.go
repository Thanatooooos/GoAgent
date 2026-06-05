package state

// Reducer is the only runtime component allowed to merge a StateDelta back
// into a StateSnapshot.
type Reducer interface {
	Apply(snapshot StateSnapshot, delta StateDelta) (StateSnapshot, error)
}

// DefaultReducer is the M0-M1 baseline reducer with deterministic merge rules.
type DefaultReducer struct{}

// Apply merges the supplied delta into the snapshot and returns a new snapshot.
func (r DefaultReducer) Apply(snapshot StateSnapshot, delta StateDelta) (StateSnapshot, error) {
	next := snapshot

	if delta.Request != nil {
		applyRequestDelta(&next.Request, *delta.Request)
	}
	if delta.Context != nil {
		applyContextDelta(&next.Context, *delta.Context)
	}
	if delta.Plan != nil {
		applyPlanDelta(&next.Plan, *delta.Plan)
	}
	if delta.Evidence != nil {
		applyEvidenceDelta(&next.Evidence, *delta.Evidence)
	}
	if delta.Approval != nil {
		applyApprovalDelta(&next.Approval, *delta.Approval)
	}
	if delta.Execution != nil {
		applyExecutionDelta(&next.Execution, *delta.Execution)
	}
	if delta.Answer != nil {
		applyAnswerDelta(&next.Answer, *delta.Answer)
	}

	return next, nil
}

func applyRequestDelta(target *RequestState, delta RequestDelta) {
	if target == nil {
		return
	}
	if delta.ConversationID != nil && target.ConversationID == "" {
		target.ConversationID = *delta.ConversationID
	}
	if len(delta.KnowledgeBaseIDs) > 0 {
		target.KnowledgeBaseIDs = append(target.KnowledgeBaseIDs, delta.KnowledgeBaseIDs...)
	}
	if delta.RuntimeOptions != nil {
		target.RuntimeOptions = *delta.RuntimeOptions
	}
}

func applyContextDelta(target *ContextState, delta ContextDelta) {
	if target == nil {
		return
	}
	if delta.RewrittenQuery != nil {
		target.RewrittenQuery = *delta.RewrittenQuery
	}
	if delta.SearchQuery != nil {
		target.SearchQuery = *delta.SearchQuery
	}
	if delta.SearchProvider != nil {
		target.SearchProvider = *delta.SearchProvider
	}
	if delta.SearchProviderActual != nil {
		target.SearchProviderActual = *delta.SearchProviderActual
	}
	if delta.SearchErrorClass != nil {
		target.SearchErrorClass = *delta.SearchErrorClass
	}
	if delta.FetchErrorClass != nil {
		target.FetchErrorClass = *delta.FetchErrorClass
	}
	if delta.ResetSearchResults {
		target.SearchResults = nil
	}
	if delta.ResetFetchResults {
		target.FetchResults = nil
	}
	if len(delta.SearchResults) > 0 {
		target.SearchResults = append(target.SearchResults, delta.SearchResults...)
	}
	if len(delta.FetchResults) > 0 {
		target.FetchResults = append(target.FetchResults, delta.FetchResults...)
	}
	if delta.PreferredURLs != nil {
		target.PreferredURLs = append([]string(nil), (*delta.PreferredURLs)...)
	}
	if delta.AvoidURLs != nil {
		target.AvoidURLs = append([]string(nil), (*delta.AvoidURLs)...)
	}
	if len(delta.SeenURLs) > 0 {
		target.SeenURLs = appendUniqueStrings(target.SeenURLs, delta.SeenURLs...)
	}
	if len(delta.MemoryRefs) > 0 {
		target.MemoryRefs = append(target.MemoryRefs, delta.MemoryRefs...)
	}
	if len(delta.Notes) > 0 {
		target.Notes = append(target.Notes, delta.Notes...)
	}
}

func applyPlanDelta(target *PlanState, delta PlanDelta) {
	if target == nil {
		return
	}
	if delta.Replace != nil {
		*target = ClonePlanState(*delta.Replace)
	}
}

func applyEvidenceDelta(target *EvidenceState, delta EvidenceDelta) {
	if target == nil {
		return
	}
	if len(delta.AddItems) > 0 {
		target.Items = append(target.Items, delta.AddItems...)
	}
	if delta.Sufficient != nil {
		target.Sufficient = *delta.Sufficient
	}
	if delta.SufficiencyReason != nil {
		target.SufficiencyReason = *delta.SufficiencyReason
	}
	if delta.NewItemsThisRound != nil {
		target.NewItemsThisRound = *delta.NewItemsThisRound
	}
	if len(delta.OpenQuestions) > 0 {
		target.OpenQuestions = append(target.OpenQuestions, delta.OpenQuestions...)
	}
}

func applyApprovalDelta(target *ApprovalState, delta ApprovalDelta) {
	if target == nil {
		return
	}
	if delta.Status != nil {
		target.Status = *delta.Status
	}
	if delta.Reason != nil {
		target.Reason = *delta.Reason
	}
	if delta.Node != nil {
		target.Node = *delta.Node
	}
	if delta.Capability != nil {
		target.Capability = *delta.Capability
	}
	if delta.CheckpointID != nil {
		target.CheckpointID = *delta.CheckpointID
	}
	if delta.RerunNode != nil {
		target.RerunNode = *delta.RerunNode
	}
	if delta.RequestedAt != nil {
		target.RequestedAt = *delta.RequestedAt
	}
	if delta.ReviewedAt != nil {
		target.ReviewedAt = *delta.ReviewedAt
	}
	if delta.DecisionNote != nil {
		target.DecisionNote = *delta.DecisionNote
	}
}

func applyExecutionDelta(target *ExecutionState, delta ExecutionDelta) {
	if target == nil {
		return
	}
	if delta.CurrentNode != nil {
		target.CurrentNode = *delta.CurrentNode
	}
	if delta.IterationIncrement != 0 {
		target.Iteration += delta.IterationIncrement
	}
	if delta.ContinueCountIncrement != 0 {
		target.ContinueCount += delta.ContinueCountIncrement
	}
	if delta.LastBranchTarget != nil {
		target.LastBranchTarget = *delta.LastBranchTarget
	}
	if delta.LastBranchReason != nil {
		target.LastBranchReason = *delta.LastBranchReason
	}
	if delta.LastProgressKind != nil {
		target.LastProgressKind = *delta.LastProgressKind
	}
	if delta.LastNewURLCount != nil {
		target.LastNewURLCount = *delta.LastNewURLCount
	}
	if delta.LastNewEvidenceCount != nil {
		target.LastNewEvidenceCount = *delta.LastNewEvidenceCount
	}
	if delta.ConsecutiveNoProgressRounds != nil {
		target.ConsecutiveNoProgressRounds = *delta.ConsecutiveNoProgressRounds
	}
	if len(delta.ScheduledActions) > 0 {
		target.ScheduledActions = appendUniqueStrings(target.ScheduledActions, delta.ScheduledActions...)
	}
	if len(delta.CompletedActions) > 0 {
		target.CompletedActions = appendUniqueStrings(target.CompletedActions, delta.CompletedActions...)
	}
	if len(delta.FailedActions) > 0 {
		target.FailedActions = appendUniqueStrings(target.FailedActions, delta.FailedActions...)
	}
	if delta.Interrupted != nil {
		target.Interrupted = *delta.Interrupted
	}
	if delta.InterruptReason != nil {
		target.InterruptReason = *delta.InterruptReason
	}
}

func appendUniqueStrings(existing []string, values ...string) []string {
	if len(values) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing)+len(values))
	result := append([]string(nil), existing...)
	for _, value := range existing {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func applyAnswerDelta(target *AnswerState, delta AnswerDelta) {
	if target == nil {
		return
	}
	if delta.Draft != nil {
		target.Draft = *delta.Draft
	}
	if delta.DegradeReason != nil {
		target.DegradeReason = *delta.DegradeReason
	}
	if delta.Final != nil {
		target.Final = *delta.Final
	}
}
