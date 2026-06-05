package state

// MergeStateDeltas combines multiple deltas using shared state-domain merge
// rules so workflow capabilities do not need their own ad hoc delta semantics.
func MergeStateDeltas(deltas ...StateDelta) StateDelta {
	merged := StateDelta{}
	for _, delta := range deltas {
		mergeStateDeltaInto(&merged, delta)
	}
	return merged
}

func mergeStateDeltaInto(target *StateDelta, delta StateDelta) {
	if target == nil {
		return
	}
	if delta.Request != nil {
		target.Request = mergeRequestDelta(target.Request, delta.Request)
	}
	if delta.Context != nil {
		target.Context = mergeContextDelta(target.Context, delta.Context)
	}
	if delta.Plan != nil {
		target.Plan = mergePlanDelta(target.Plan, delta.Plan)
	}
	if delta.Evidence != nil {
		target.Evidence = mergeEvidenceDelta(target.Evidence, delta.Evidence)
	}
	if delta.Approval != nil {
		target.Approval = mergeApprovalDelta(target.Approval, delta.Approval)
	}
	if delta.Execution != nil {
		target.Execution = mergeExecutionDelta(target.Execution, delta.Execution)
	}
	if delta.Answer != nil {
		target.Answer = mergeAnswerDelta(target.Answer, delta.Answer)
	}
}

func mergeRequestDelta(left, right *RequestDelta) *RequestDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Request: right})
		return clone.Request
	case right == nil:
		clone := CloneDelta(StateDelta{Request: left})
		return clone.Request
	}

	merged := &RequestDelta{
		ConversationID:   cloneStringPtr(left.ConversationID),
		KnowledgeBaseIDs: append([]string(nil), left.KnowledgeBaseIDs...),
		RuntimeOptions:   cloneRuntimeOptionsPtr(left.RuntimeOptions),
	}
	if merged.ConversationID == nil && right.ConversationID != nil {
		merged.ConversationID = cloneStringPtr(right.ConversationID)
	}
	if len(right.KnowledgeBaseIDs) > 0 {
		merged.KnowledgeBaseIDs = append(merged.KnowledgeBaseIDs, right.KnowledgeBaseIDs...)
	}
	if right.RuntimeOptions != nil {
		merged.RuntimeOptions = cloneRuntimeOptionsPtr(right.RuntimeOptions)
	}
	return merged
}

func mergeContextDelta(left, right *ContextDelta) *ContextDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Context: right})
		return clone.Context
	case right == nil:
		clone := CloneDelta(StateDelta{Context: left})
		return clone.Context
	}

	merged := &ContextDelta{
		RewrittenQuery:       cloneStringPtr(left.RewrittenQuery),
		SearchQuery:          cloneStringPtr(left.SearchQuery),
		SearchProvider:       cloneStringPtr(left.SearchProvider),
		SearchProviderActual: cloneStringPtr(left.SearchProviderActual),
		SearchErrorClass:     cloneStringPtr(left.SearchErrorClass),
		FetchErrorClass:      cloneStringPtr(left.FetchErrorClass),
		ResetSearchResults:   left.ResetSearchResults,
		ResetFetchResults:    left.ResetFetchResults,
		SearchResults:        cloneSearchResultRefs(left.SearchResults),
		FetchResults:         cloneFetchResultRefs(left.FetchResults),
		PreferredURLs:        cloneStringSlicePtr(left.PreferredURLs),
		AvoidURLs:            cloneStringSlicePtr(left.AvoidURLs),
		SeenURLs:             append([]string(nil), left.SeenURLs...),
		MemoryRefs:           cloneMemoryRefs(left.MemoryRefs),
		Notes:                append([]string(nil), left.Notes...),
	}

	if right.RewrittenQuery != nil {
		merged.RewrittenQuery = cloneStringPtr(right.RewrittenQuery)
	}
	if right.SearchQuery != nil {
		merged.SearchQuery = cloneStringPtr(right.SearchQuery)
	}
	if right.SearchProvider != nil {
		merged.SearchProvider = cloneStringPtr(right.SearchProvider)
	}
	if right.SearchProviderActual != nil {
		merged.SearchProviderActual = cloneStringPtr(right.SearchProviderActual)
	}
	if right.SearchErrorClass != nil {
		merged.SearchErrorClass = cloneStringPtr(right.SearchErrorClass)
	}
	if right.FetchErrorClass != nil {
		merged.FetchErrorClass = cloneStringPtr(right.FetchErrorClass)
	}
	if right.ResetSearchResults {
		merged.ResetSearchResults = true
		merged.SearchResults = nil
	}
	if right.ResetFetchResults {
		merged.ResetFetchResults = true
		merged.FetchResults = nil
	}
	if len(right.SearchResults) > 0 {
		merged.SearchResults = append(merged.SearchResults, cloneSearchResultRefs(right.SearchResults)...)
	}
	if len(right.FetchResults) > 0 {
		merged.FetchResults = append(merged.FetchResults, cloneFetchResultRefs(right.FetchResults)...)
	}
	if right.PreferredURLs != nil {
		merged.PreferredURLs = cloneStringSlicePtr(right.PreferredURLs)
	}
	if right.AvoidURLs != nil {
		merged.AvoidURLs = cloneStringSlicePtr(right.AvoidURLs)
	}
	if len(right.SeenURLs) > 0 {
		merged.SeenURLs = appendUniqueStrings(merged.SeenURLs, right.SeenURLs...)
	}
	if len(right.MemoryRefs) > 0 {
		merged.MemoryRefs = append(merged.MemoryRefs, cloneMemoryRefs(right.MemoryRefs)...)
	}
	if len(right.Notes) > 0 {
		merged.Notes = append(merged.Notes, right.Notes...)
	}
	return merged
}

func mergePlanDelta(left, right *PlanDelta) *PlanDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Plan: right})
		return clone.Plan
	case right == nil:
		clone := CloneDelta(StateDelta{Plan: left})
		return clone.Plan
	}

	merged := &PlanDelta{
		Replace: clonePlanStatePtr(left.Replace),
	}
	if right.Replace != nil {
		merged.Replace = clonePlanStatePtr(right.Replace)
	}
	return merged
}

func mergeEvidenceDelta(left, right *EvidenceDelta) *EvidenceDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Evidence: right})
		return clone.Evidence
	case right == nil:
		clone := CloneDelta(StateDelta{Evidence: left})
		return clone.Evidence
	}

	merged := &EvidenceDelta{
		AddItems:          cloneEvidenceItems(left.AddItems),
		Sufficient:        cloneBoolPtr(left.Sufficient),
		SufficiencyReason: cloneStringPtr(left.SufficiencyReason),
		NewItemsThisRound: cloneIntPtr(left.NewItemsThisRound),
		OpenQuestions:     append([]string(nil), left.OpenQuestions...),
	}
	if len(right.AddItems) > 0 {
		merged.AddItems = append(merged.AddItems, cloneEvidenceItems(right.AddItems)...)
	}
	if right.Sufficient != nil {
		merged.Sufficient = cloneBoolPtr(right.Sufficient)
	}
	if right.SufficiencyReason != nil {
		merged.SufficiencyReason = cloneStringPtr(right.SufficiencyReason)
	}
	if right.NewItemsThisRound != nil {
		merged.NewItemsThisRound = cloneIntPtr(right.NewItemsThisRound)
	}
	if len(right.OpenQuestions) > 0 {
		merged.OpenQuestions = append(merged.OpenQuestions, right.OpenQuestions...)
	}
	return merged
}

func mergeApprovalDelta(left, right *ApprovalDelta) *ApprovalDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Approval: right})
		return clone.Approval
	case right == nil:
		clone := CloneDelta(StateDelta{Approval: left})
		return clone.Approval
	}

	merged := &ApprovalDelta{
		Status:       cloneStringPtr(left.Status),
		Reason:       cloneStringPtr(left.Reason),
		Node:         cloneStringPtr(left.Node),
		Capability:   cloneStringPtr(left.Capability),
		CheckpointID: cloneStringPtr(left.CheckpointID),
		RerunNode:    cloneStringPtr(left.RerunNode),
		RequestedAt:  cloneTimePtr(left.RequestedAt),
		ReviewedAt:   cloneTimePtr(left.ReviewedAt),
		DecisionNote: cloneStringPtr(left.DecisionNote),
	}
	if right.Status != nil {
		merged.Status = cloneStringPtr(right.Status)
	}
	if right.Reason != nil {
		merged.Reason = cloneStringPtr(right.Reason)
	}
	if right.Node != nil {
		merged.Node = cloneStringPtr(right.Node)
	}
	if right.Capability != nil {
		merged.Capability = cloneStringPtr(right.Capability)
	}
	if right.CheckpointID != nil {
		merged.CheckpointID = cloneStringPtr(right.CheckpointID)
	}
	if right.RerunNode != nil {
		merged.RerunNode = cloneStringPtr(right.RerunNode)
	}
	if right.RequestedAt != nil {
		merged.RequestedAt = cloneTimePtr(right.RequestedAt)
	}
	if right.ReviewedAt != nil {
		merged.ReviewedAt = cloneTimePtr(right.ReviewedAt)
	}
	if right.DecisionNote != nil {
		merged.DecisionNote = cloneStringPtr(right.DecisionNote)
	}
	return merged
}

func mergeExecutionDelta(left, right *ExecutionDelta) *ExecutionDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Execution: right})
		return clone.Execution
	case right == nil:
		clone := CloneDelta(StateDelta{Execution: left})
		return clone.Execution
	}

	merged := &ExecutionDelta{
		CurrentNode:                 cloneStringPtr(left.CurrentNode),
		IterationIncrement:          left.IterationIncrement,
		ContinueCountIncrement:      left.ContinueCountIncrement,
		LastBranchTarget:            cloneStringPtr(left.LastBranchTarget),
		LastBranchReason:            cloneStringPtr(left.LastBranchReason),
		LastProgressKind:            cloneStringPtr(left.LastProgressKind),
		LastNewURLCount:             cloneIntPtr(left.LastNewURLCount),
		LastNewEvidenceCount:        cloneIntPtr(left.LastNewEvidenceCount),
		ConsecutiveNoProgressRounds: cloneIntPtr(left.ConsecutiveNoProgressRounds),
		ScheduledActions:            append([]string(nil), left.ScheduledActions...),
		CompletedActions:            append([]string(nil), left.CompletedActions...),
		FailedActions:               append([]string(nil), left.FailedActions...),
		Interrupted:                 cloneBoolPtr(left.Interrupted),
		InterruptReason:             cloneStringPtr(left.InterruptReason),
	}

	merged.IterationIncrement += right.IterationIncrement
	merged.ContinueCountIncrement += right.ContinueCountIncrement
	if right.CurrentNode != nil {
		merged.CurrentNode = cloneStringPtr(right.CurrentNode)
	}
	if right.LastBranchTarget != nil {
		merged.LastBranchTarget = cloneStringPtr(right.LastBranchTarget)
	}
	if right.LastBranchReason != nil {
		merged.LastBranchReason = cloneStringPtr(right.LastBranchReason)
	}
	if right.LastProgressKind != nil {
		merged.LastProgressKind = cloneStringPtr(right.LastProgressKind)
	}
	if right.LastNewURLCount != nil {
		merged.LastNewURLCount = cloneIntPtr(right.LastNewURLCount)
	}
	if right.LastNewEvidenceCount != nil {
		merged.LastNewEvidenceCount = cloneIntPtr(right.LastNewEvidenceCount)
	}
	if right.ConsecutiveNoProgressRounds != nil {
		merged.ConsecutiveNoProgressRounds = cloneIntPtr(right.ConsecutiveNoProgressRounds)
	}
	if len(right.ScheduledActions) > 0 {
		merged.ScheduledActions = appendUniqueStrings(merged.ScheduledActions, right.ScheduledActions...)
	}
	if len(right.CompletedActions) > 0 {
		merged.CompletedActions = appendUniqueStrings(merged.CompletedActions, right.CompletedActions...)
	}
	if len(right.FailedActions) > 0 {
		merged.FailedActions = appendUniqueStrings(merged.FailedActions, right.FailedActions...)
	}
	if right.Interrupted != nil {
		merged.Interrupted = cloneBoolPtr(right.Interrupted)
	}
	if right.InterruptReason != nil {
		merged.InterruptReason = cloneStringPtr(right.InterruptReason)
	}
	return merged
}

func mergeAnswerDelta(left, right *AnswerDelta) *AnswerDelta {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		clone := CloneDelta(StateDelta{Answer: right})
		return clone.Answer
	case right == nil:
		clone := CloneDelta(StateDelta{Answer: left})
		return clone.Answer
	}

	merged := &AnswerDelta{
		Draft:         cloneStringPtr(left.Draft),
		DegradeReason: cloneStringPtr(left.DegradeReason),
		Final:         cloneStringPtr(left.Final),
	}
	if right.Draft != nil {
		merged.Draft = cloneStringPtr(right.Draft)
	}
	if right.DegradeReason != nil {
		merged.DegradeReason = cloneStringPtr(right.DegradeReason)
	}
	if right.Final != nil {
		merged.Final = cloneStringPtr(right.Final)
	}
	return merged
}
