package handoff

import (
	"fmt"
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
)

func BuildToolContext(
	session *agentruntime.RuntimeSession,
	searchResults []agentsearch.SearchResultItem,
	pages []agentfetch.PageResult,
	evidence []AcceptedEvidenceItem,
) string {
	if session == nil {
		return ""
	}

	var sections []string
	if query := firstNonEmpty(session.Snapshot.Context.SearchQuery, session.Snapshot.Context.RewrittenQuery); query != "" {
		sections = append(sections, "Search query:\n"+query)
	}
	if len(searchResults) > 0 {
		lines := make([]string, 0, len(searchResults))
		for idx, item := range searchResults {
			meta := make([]string, 0, 3)
			if item.Domain != "" {
				meta = append(meta, item.Domain)
			}
			if item.Policy != "" {
				meta = append(meta, "policy="+item.Policy)
			}
			if item.SourceType != "" {
				meta = append(meta, "type="+item.SourceType)
			}
			line := fmt.Sprintf("%d. %s (%s)", idx+1, strings.TrimSpace(item.Title), strings.TrimSpace(item.URL))
			if len(meta) > 0 {
				line += " [" + strings.Join(meta, ", ") + "]"
			}
			if snippet := strings.TrimSpace(item.Snippet); snippet != "" {
				line += ": " + snippet
			}
			lines = append(lines, line)
		}
		sections = append(sections, "Search results:\n"+strings.Join(lines, "\n"))
	}
	if len(pages) > 0 {
		lines := make([]string, 0, len(pages))
		for _, page := range pages {
			text := strings.TrimSpace(page.Text)
			if text == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("[%s]\n%s", strings.TrimSpace(page.URL), text))
		}
		if len(lines) > 0 {
			sections = append(sections, "Fetched web content:\n"+strings.Join(lines, "\n\n"))
		}
	}
	if len(evidence) > 0 {
		lines := make([]string, 0, len(evidence))
		for _, item := range evidence {
			line := "- " + strings.TrimSpace(item.Content)
			if item.SourceRef != "" {
				line += " (" + strings.TrimSpace(item.SourceRef) + ")"
			}
			lines = append(lines, line)
		}
		sections = append(sections, "Accepted evidence:\n"+strings.Join(lines, "\n"))
	}
	if reason := strings.TrimSpace(session.Snapshot.Evidence.SufficiencyReason); reason != "" {
		sections = append(sections, "Evidence assessment:\n"+reason)
	}
	if len(session.Snapshot.Evidence.OpenQuestions) > 0 {
		sections = append(sections, "Open questions:\n- "+strings.Join(session.Snapshot.Evidence.OpenQuestions, "\n- "))
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}
