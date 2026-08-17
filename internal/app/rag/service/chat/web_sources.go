package chat

import (
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	ragcitation "local/rag-project/internal/app/rag/core/citation"
	ragtool "local/rag-project/internal/app/rag/tool/core"
)

// registerLegacyWebSources registers external web sources collected by the
// legacy tool workflow (external_evidence_workflow) into the citation
// registry and returns the handle-carrying source list for the tool context.
func registerLegacyWebSources(registry *ragcitation.Registry, result ragtool.WorkflowResult) string {
	if registry == nil {
		return ""
	}
	refs := make([]ragcitation.WebReference, 0, 8)
	seen := make(map[string]struct{}, 8)
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
	for _, call := range result.Calls {
		if !isWebRelatedCall(call.Name) {
			continue
		}
		collectURLsFromCallData(call.Data, appendRef)
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

func isWebRelatedCall(name string) bool {
	switch strings.TrimSpace(name) {
	case "web_search", "web_fetch", "external_evidence_workflow":
		return true
	default:
		return false
	}
}

func collectURLsFromCallData(data map[string]any, appendRef func(title, url string)) {
	if len(data) == 0 {
		return
	}
	var walk func(value any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			urlText, _ := typed["url"].(string)
			titleText, _ := typed["title"].(string)
			if strings.TrimSpace(urlText) != "" {
				appendRef(titleText, urlText)
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case []string:
			for _, text := range typed {
				appendRef("", text)
			}
		}
	}
	for _, value := range data {
		walk(value)
	}
}

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
