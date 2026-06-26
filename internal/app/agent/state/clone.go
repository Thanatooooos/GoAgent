package state

import "time"

// CloneSnapshot deep-copies a state snapshot so replay/projection can safely
// reuse it without sharing slice backing arrays.
func CloneSnapshot(snapshot StateSnapshot) StateSnapshot {
	cloned := NormalizeSnapshot(snapshot)
	cloned.Request.KnowledgeBaseIDs = append([]string(nil), snapshot.Request.KnowledgeBaseIDs...)
	cloned.Context.SearchResults = cloneSearchResultRefs(snapshot.Context.SearchResults)
	cloned.Context.FetchResults = cloneFetchResultRefs(snapshot.Context.FetchResults)
	cloned.Context.PreferredURLs = append([]string(nil), snapshot.Context.PreferredURLs...)
	cloned.Context.AvoidURLs = append([]string(nil), snapshot.Context.AvoidURLs...)
	cloned.Context.SeenURLs = append([]string(nil), snapshot.Context.SeenURLs...)
	cloned.Context.MemoryRefs = cloneMemoryRefs(snapshot.Context.MemoryRefs)
	cloned.Context.Notes = append([]string(nil), snapshot.Context.Notes...)
	cloned.Plan = ClonePlanState(snapshot.Plan)
	cloned.Evidence.Items = cloneEvidenceItems(snapshot.Evidence.Items)
	cloned.Evidence.OpenQuestions = append([]string(nil), snapshot.Evidence.OpenQuestions...)
	cloned.Execution.ScheduledActions = append([]string(nil), snapshot.Execution.ScheduledActions...)
	cloned.Execution.CompletedActions = append([]string(nil), snapshot.Execution.CompletedActions...)
	cloned.Execution.FailedActions = append([]string(nil), snapshot.Execution.FailedActions...)
	cloned.Pattern.Data = cloneStringAnyMap(snapshot.Pattern.Data)
	return cloned
}

// CloneDelta deep-copies a state delta so journaled projections remain stable.
func CloneDelta(delta StateDelta) StateDelta {
	cloned := StateDelta{}
	if delta.Request != nil {
		cloned.Request = &RequestDelta{
			ConversationID:   cloneStringPtr(delta.Request.ConversationID),
			KnowledgeBaseIDs: append([]string(nil), delta.Request.KnowledgeBaseIDs...),
			RuntimeOptions:   cloneRuntimeOptionsPtr(delta.Request.RuntimeOptions),
		}
	}
	if delta.Context != nil {
		cloned.Context = &ContextDelta{
			RewrittenQuery:       cloneStringPtr(delta.Context.RewrittenQuery),
			SearchQuery:          cloneStringPtr(delta.Context.SearchQuery),
			SearchProvider:       cloneStringPtr(delta.Context.SearchProvider),
			SearchProviderActual: cloneStringPtr(delta.Context.SearchProviderActual),
			SearchErrorClass:     cloneStringPtr(delta.Context.SearchErrorClass),
			FetchErrorClass:      cloneStringPtr(delta.Context.FetchErrorClass),
			ResetSearchResults:   delta.Context.ResetSearchResults,
			ResetFetchResults:    delta.Context.ResetFetchResults,
			SearchResults:        cloneSearchResultRefs(delta.Context.SearchResults),
			FetchResults:         cloneFetchResultRefs(delta.Context.FetchResults),
			PreferredURLs:        cloneStringSlicePtr(delta.Context.PreferredURLs),
			AvoidURLs:            cloneStringSlicePtr(delta.Context.AvoidURLs),
			SeenURLs:             append([]string(nil), delta.Context.SeenURLs...),
			MemoryRefs:           cloneMemoryRefs(delta.Context.MemoryRefs),
			Notes:                append([]string(nil), delta.Context.Notes...),
		}
	}
	if delta.Plan != nil {
		cloned.Plan = &PlanDelta{
			Replace: clonePlanStatePtr(delta.Plan.Replace),
		}
	}
	if delta.Evidence != nil {
		cloned.Evidence = &EvidenceDelta{
			AddItems:          cloneEvidenceItems(delta.Evidence.AddItems),
			Sufficient:        cloneBoolPtr(delta.Evidence.Sufficient),
			SufficiencyReason: cloneStringPtr(delta.Evidence.SufficiencyReason),
			NewItemsThisRound: cloneIntPtr(delta.Evidence.NewItemsThisRound),
			OpenQuestions:     append([]string(nil), delta.Evidence.OpenQuestions...),
		}
	}
	if delta.Approval != nil {
		cloned.Approval = &ApprovalDelta{
			Status:       cloneStringPtr(delta.Approval.Status),
			Reason:       cloneStringPtr(delta.Approval.Reason),
			Node:         cloneStringPtr(delta.Approval.Node),
			Capability:   cloneStringPtr(delta.Approval.Capability),
			CheckpointID: cloneStringPtr(delta.Approval.CheckpointID),
			RerunNode:    cloneStringPtr(delta.Approval.RerunNode),
			RequestedAt:  cloneTimePtr(delta.Approval.RequestedAt),
			ReviewedAt:   cloneTimePtr(delta.Approval.ReviewedAt),
			DecisionNote: cloneStringPtr(delta.Approval.DecisionNote),
		}
	}
	if delta.Execution != nil {
		cloned.Execution = &ExecutionDelta{
			Status:                      cloneStringPtr(delta.Execution.Status),
			CurrentNode:                 cloneStringPtr(delta.Execution.CurrentNode),
			IterationIncrement:          delta.Execution.IterationIncrement,
			ContinueCountIncrement:      delta.Execution.ContinueCountIncrement,
			LastBranchTarget:            cloneStringPtr(delta.Execution.LastBranchTarget),
			LastBranchReason:            cloneStringPtr(delta.Execution.LastBranchReason),
			LastProgressKind:            cloneStringPtr(delta.Execution.LastProgressKind),
			LastNewURLCount:             cloneIntPtr(delta.Execution.LastNewURLCount),
			LastNewEvidenceCount:        cloneIntPtr(delta.Execution.LastNewEvidenceCount),
			ConsecutiveNoProgressRounds: cloneIntPtr(delta.Execution.ConsecutiveNoProgressRounds),
			ScheduledActions:            append([]string(nil), delta.Execution.ScheduledActions...),
			CompletedActions:            append([]string(nil), delta.Execution.CompletedActions...),
			FailedActions:               append([]string(nil), delta.Execution.FailedActions...),
			Interrupted:                 cloneBoolPtr(delta.Execution.Interrupted),
			InterruptReason:             cloneStringPtr(delta.Execution.InterruptReason),
		}
	}
	if delta.Answer != nil {
		cloned.Answer = &AnswerDelta{
			Draft:         cloneStringPtr(delta.Answer.Draft),
			DegradeReason: cloneStringPtr(delta.Answer.DegradeReason),
			Final:         cloneStringPtr(delta.Answer.Final),
		}
	}
	return cloned
}

// ClonePlanState deep-copies plan state so plan-execute reducers and replay
// logic can safely retain prior snapshots.
func ClonePlanState(plan PlanState) PlanState {
	cloned := plan
	cloned.Steps = clonePlanSteps(plan.Steps)
	cloned.CompletionCriteria = append([]string(nil), plan.CompletionCriteria...)
	cloned.LastStepResult = clonePlanStepResult(plan.LastStepResult)
	return cloned
}

func cloneSearchResultRefs(items []SearchResultRef) []SearchResultRef {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]SearchResultRef, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].RiskFlags = append([]string(nil), item.RiskFlags...)
		cloned[i].Reasons = append([]string(nil), item.Reasons...)
	}
	return cloned
}

func cloneFetchResultRefs(items []FetchResultRef) []FetchResultRef {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]FetchResultRef, len(items))
	copy(cloned, items)
	return cloned
}

func cloneMemoryRefs(items []MemoryRef) []MemoryRef {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]MemoryRef, len(items))
	copy(cloned, items)
	return cloned
}

func cloneEvidenceItems(items []EvidenceItem) []EvidenceItem {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]EvidenceItem, len(items))
	copy(cloned, items)
	return cloned
}

func clonePlanSteps(items []PlanStep) []PlanStep {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]PlanStep, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].CapabilityInput = cloneStringAnyMap(item.CapabilityInput)
		cloned[i].Consumes = append([]string(nil), item.Consumes...)
		cloned[i].Produces = append([]string(nil), item.Produces...)
		cloned[i].URLs = append([]string(nil), item.URLs...)
		cloned[i].DependsOn = append([]string(nil), item.DependsOn...)
		cloned[i].ExpectedEvidence = append([]string(nil), item.ExpectedEvidence...)
	}
	return cloned
}

func clonePlanStepResult(item PlanStepResult) PlanStepResult {
	cloned := item
	cloned.URLs = append([]string(nil), item.URLs...)
	cloned.Artifacts = clonePlanStepArtifacts(item.Artifacts)
	return cloned
}

func clonePlanStepArtifacts(items []PlanStepArtifact) []PlanStepArtifact {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]PlanStepArtifact, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].StringValues = append([]string(nil), item.StringValues...)
		cloned[i].Refs = append([]string(nil), item.Refs...)
		cloned[i].Metadata = cloneStringStringMap(item.Metadata)
	}
	return cloned
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRuntimeOptionsPtr(value *RuntimeOptions) *RuntimeOptions {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringSlicePtr(value *[]string) *[]string {
	if value == nil {
		return nil
	}
	cloned := append([]string(nil), (*value)...)
	return &cloned
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = cloneJSONLikeValue(item)
	}
	return cloned
}

func cloneStringStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneJSONLikeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneJSONLikeValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func clonePlanStatePtr(value *PlanState) *PlanState {
	if value == nil {
		return nil
	}
	cloned := ClonePlanState(*value)
	return &cloned
}
