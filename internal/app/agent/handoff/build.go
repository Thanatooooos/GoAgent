package handoff

import (
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func NewBuilder(profiles []CapabilityProfile) *Builder {
	indexed := make(map[string]CapabilityProfile, len(profiles))
	for _, profile := range profiles {
		node := strings.TrimSpace(profile.Node)
		if node == "" {
			continue
		}
		indexed[node] = profile
	}
	return &Builder{profiles: indexed}
}

func Build(session *agentruntime.RuntimeSession) Result {
	return NewBuilder(nil).Build(session)
}

func (b *Builder) Build(session *agentruntime.RuntimeSession) Result {
	if session == nil {
		return Result{}
	}

	searchResults := toSearchResults(session.Snapshot.Context.SearchResults)
	pages := toPages(session.Snapshot.Context.FetchResults)
	evidence := toAcceptedEvidence(session.Snapshot.Evidence.Items)
	replay := agentruntime.BuildReplayView(session)
	degradeReason := strings.TrimSpace(session.Snapshot.Answer.DegradeReason)

	return Result{
		Used:           len(searchResults) > 0 || len(pages) > 0 || len(evidence) > 0,
		ToolContext:    BuildToolContext(session, searchResults, pages, evidence),
		AnswerGuidance: BuildAnswerGuidance(session),
		WorkflowPolicy: b.BuildWorkflowPolicy(session),
		EvidenceBundle: EvidenceBundle{
			Question:          firstNonEmpty(session.Request.Question, session.Snapshot.Request.Question),
			SearchQuery:       firstNonEmpty(session.Snapshot.Context.SearchQuery, session.Snapshot.Context.RewrittenQuery),
			Provider:          firstNonEmpty(session.Snapshot.Context.SearchProviderActual, session.Snapshot.Context.SearchProvider),
			SearchResults:     searchResults,
			Pages:             pages,
			AcceptedEvidence:  evidence,
			Sufficient:        session.Snapshot.Evidence.Sufficient,
			SufficiencyReason: session.Snapshot.Evidence.SufficiencyReason,
			OpenQuestions:     append([]string(nil), session.Snapshot.Evidence.OpenQuestions...),
			NewItemsThisRound: session.Snapshot.Evidence.NewItemsThisRound,
		},
		DecisionSummary: buildDecisionSummary(session, replay, degradeReason),
		Replay:          replay,
		Degraded:        degradeReason != "",
		DegradeReason:   degradeReason,
	}
}

func buildDecisionSummary(session *agentruntime.RuntimeSession, replay agentruntime.ReplayView, degradeReason string) DecisionSummary {
	return DecisionSummary{
		FinalAction:                 finalAction(session),
		Reason:                      firstNonEmpty(session.Snapshot.Execution.LastBranchReason, degradeReason, session.Snapshot.Evidence.SufficiencyReason),
		Confidence:                  lastDecisionConfidence(replay),
		Iteration:                   session.Snapshot.Execution.Iteration,
		MaxIterations:               session.Snapshot.Execution.MaxIterations,
		ContinueCount:               session.Snapshot.Execution.ContinueCount,
		LastProgressKind:            session.Snapshot.Execution.LastProgressKind,
		LastNewURLCount:             session.Snapshot.Execution.LastNewURLCount,
		LastNewEvidenceCount:        session.Snapshot.Execution.LastNewEvidenceCount,
		ConsecutiveNoProgressRounds: session.Snapshot.Execution.ConsecutiveNoProgressRounds,
	}
}

func finalAction(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	if strings.TrimSpace(session.Snapshot.Answer.DegradeReason) != "" {
		return ActionDegrade
	}
	if strings.TrimSpace(session.Snapshot.Execution.LastBranchTarget) == "answer" {
		return ActionFinalAnswer
	}
	return ActionHandoffToRAG
}

func lastDecisionConfidence(replay agentruntime.ReplayView) float64 {
	if replay.LastDecision == nil {
		return 0
	}
	return replay.LastDecision.Confidence
}

func toAcceptedEvidence(items []agentstate.EvidenceItem) []AcceptedEvidenceItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]AcceptedEvidenceItem, 0, len(items))
	for _, item := range items {
		result = append(result, AcceptedEvidenceItem{
			ID:        item.ID,
			Source:    item.Source,
			Content:   item.Content,
			Level:     item.Level,
			SourceRef: item.SourceRef,
		})
	}
	return result
}

func toSearchResults(refs []agentstate.SearchResultRef) []agentsearch.SearchResultItem {
	if len(refs) == 0 {
		return nil
	}
	results := make([]agentsearch.SearchResultItem, 0, len(refs))
	for _, ref := range refs {
		results = append(results, agentsearch.SearchResultItem{
			Title:      ref.Title,
			URL:        ref.URL,
			Snippet:    ref.Snippet,
			Domain:     ref.Domain,
			SourceType: ref.SourceType,
			Policy:     ref.Policy,
			RiskFlags:  append([]string(nil), ref.RiskFlags...),
			Reasons:    append([]string(nil), ref.Reasons...),
		})
	}
	return results
}

func toPages(refs []agentstate.FetchResultRef) []agentfetch.PageResult {
	if len(refs) == 0 {
		return nil
	}
	pages := make([]agentfetch.PageResult, 0, len(refs))
	for _, ref := range refs {
		pages = append(pages, agentfetch.PageResult{
			URL:            ref.URL,
			Text:           ref.Text,
			ErrorMessage:   ref.ErrorReason,
			OriginalLength: ref.OriginalLength,
			WasTruncated:   ref.WasTruncated,
		})
	}
	return pages
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
