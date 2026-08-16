package chat

import (
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	ragcitation "local/rag-project/internal/app/rag/core/citation"
)

// registerAgentWebSources registers external web sources collected by the
// agent tool stage into the citation registry and returns a handle-carrying
// source list that is appended to the tool context, so the model can cite
// external pages with <ref id="wN"/>.
func registerAgentWebSources(registry *ragcitation.Registry, run agentapp.RunResponse) string {
	if registry == nil {
		return ""
	}
	seen := make(map[string]struct{}, len(run.Response.Results)+len(run.Response.Pages))
	refs := make([]ragcitation.WebReference, 0, len(run.Response.Results)+len(run.Response.Pages))
	appendRef := func(title, rawURL string) {
		title = strings.TrimSpace(title)
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return
		}
		if _, ok := seen[rawURL]; ok {
			return
		}
		seen[rawURL] = struct{}{}
		refs = append(refs, ragcitation.WebReference{URL: rawURL, Title: title})
	}
	for _, item := range run.Response.Results {
		appendRef(item.Title, item.URL)
	}
	for _, page := range run.Response.Pages {
		appendRef("", page.URL)
	}
	if len(refs) == 0 {
		return ""
	}
	handles := registry.RegisterWebs(refs)
	if len(handles) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("外部来源句柄：")
	for index, handle := range handles {
		builder.WriteString("\n- [")
		builder.WriteString(handle)
		builder.WriteString("] ")
		if title := strings.TrimSpace(refs[index].Title); title != "" {
			builder.WriteString(title)
			builder.WriteString(" - ")
		}
		builder.WriteString(refs[index].URL)
	}
	return builder.String()
}
