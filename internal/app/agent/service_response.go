package agent

import (
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func responseFromSession(session *agentruntime.RuntimeSession) Response {
	if session == nil {
		return Response{}
	}
	results := toSearchResults(session.Snapshot.Context.SearchResults)
	pages := toPages(session.Snapshot.Context.FetchResults)
	combinedText := buildCombinedText(pages)
	summary := firstNonEmpty(
		session.Snapshot.Answer.Final,
		latestNote(session.Snapshot.Context.Notes),
		session.Snapshot.Approval.Reason,
		session.Snapshot.Evidence.SufficiencyReason,
	)
	provider := strings.TrimSpace(firstNonEmpty(
		session.Snapshot.Context.SearchProviderActual,
		session.Snapshot.Context.SearchProvider,
	))
	degradeReason := strings.TrimSpace(session.Snapshot.Answer.DegradeReason)

	return Response{
		Query:         firstNonEmpty(session.Snapshot.Context.SearchQuery, session.Request.Question, session.Snapshot.Request.Question),
		Results:       results,
		Pages:         pages,
		CombinedText:  combinedText,
		Summary:       summary,
		Provider:      provider,
		Degraded:      degradeReason != "",
		DegradeReason: degradeReason,
	}
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

func buildCombinedText(pages []agentfetch.PageResult) string {
	if len(pages) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, page := range pages {
		if strings.TrimSpace(page.Text) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n---\n\n")
		}
		builder.WriteString("[")
		builder.WriteString(page.URL)
		builder.WriteString("]\n")
		builder.WriteString(page.Text)
	}
	return builder.String()
}

func latestNote(notes []string) string {
	for i := len(notes) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(notes[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneJournal(events []agentstate.RuntimeEvent) []agentstate.RuntimeEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]agentstate.RuntimeEvent, len(events))
	copy(cloned, events)
	return cloned
}
