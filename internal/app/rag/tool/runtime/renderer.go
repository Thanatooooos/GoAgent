package runtime

import (
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	. "local/rag-project/internal/app/rag/tool/core"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

const maxRenderedToolContextLen = 12000

// RenderContext 把多次 tool 结果渲染成可注入 prompt 的文本。
func RenderContext(results []Result) string {
	if len(results) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, result := range results {
		name := strings.TrimSpace(result.Name)
		if name == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("### ")
		builder.WriteString(name)
		builder.WriteString("\n")
		if summary := strings.TrimSpace(result.Summary); summary != "" {
			builder.WriteString(summary)
		} else if result.Successful() {
			builder.WriteString("tool executed successfully")
		} else {
			builder.WriteString(strings.TrimSpace(result.ErrorMessage))
		}
		if detail := renderResultContextDetail(result); detail != "" {
			builder.WriteString("\n")
			builder.WriteString(detail)
		}
	}
	return strings.TrimSpace(builder.String())
}

func RenderContextWithinBudget(
	results []Result,
	budget int,
	estimator tokenbudget.Estimator,
) (string, tokenbudget.TruncationStats) {
	sections := make([]tokenbudget.Section, 0, len(results)*2)
	for _, result := range results {
		name := strings.TrimSpace(result.Name)
		if name == "" {
			continue
		}
		summary := strings.TrimSpace(result.Summary)
		if summary == "" {
			if result.Successful() {
				summary = "tool executed successfully"
			} else {
				summary = strings.TrimSpace(result.ErrorMessage)
			}
		}
		sections = append(sections, tokenbudget.Section{
			Name:     name + ".summary",
			Text:     "### " + name + "\n" + summary,
			Priority: 100,
			Required: true,
		})
		if detail := renderResultContextDetail(result); detail != "" {
			priority := 30
			required := false
			if name == "web_search" || name == "external_evidence_workflow" {
				priority = 90
				required = true
			}
			sections = append(sections, tokenbudget.Section{
				Name:     name + ".detail",
				Text:     detail,
				Priority: priority,
				Required: required,
			})
		}
	}
	return tokenbudget.JoinSectionsWithinBudget(sections, budget, estimator, maxRenderedToolContextLen)
}

func renderResultContextDetail(result Result) string {
	switch strings.TrimSpace(result.Name) {
	case "web_search":
		return renderWebSearchContext(result)
	case "web_fetch":
		return renderWebFetchContext(result)
	case "external_evidence_workflow":
		return renderExternalEvidenceContext(result)
	default:
		return ""
	}
}

func renderWebSearchContext(result Result) string {
	view, ok := webmod.ViewWebSearchResult(result)
	if !ok || len(view.Results) == 0 {
		return ""
	}

	lines := make([]string, 0, len(view.Results))
	for idx, item := range view.Results {
		title := strings.TrimSpace(item.Title)
		u := strings.TrimSpace(item.URL)
		snippet := strings.TrimSpace(item.Snippet)
		domain := strings.TrimSpace(item.Domain)
		policy := strings.TrimSpace(item.Policy)
		sourceType := strings.TrimSpace(item.SourceType)
		if title == "" && u == "" && snippet == "" {
			continue
		}
		meta := make([]string, 0, 3)
		if domain != "" {
			meta = append(meta, domain)
		}
		if policy != "" {
			meta = append(meta, "policy="+policy)
		}
		if sourceType != "" {
			meta = append(meta, "type="+sourceType)
		}
		if len(meta) > 0 {
			lines = append(lines, fmt.Sprintf("%d. %s (%s) [%s]: %s", idx+1, title, u, strings.Join(meta, ", "), snippet))
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s): %s", idx+1, title, u, snippet))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Search results:\n" + strings.Join(lines, "\n")
}

func renderWebFetchContext(result Result) string {
	view, ok := webmod.ViewWebFetchResult(result)
	if !ok {
		return ""
	}
	combined := strings.TrimSpace(view.ReadableText())
	if combined == "" {
		return ""
	}
	return "Fetched web content:\n" + truncateRenderedContext(combined)
}

func renderExternalEvidenceContext(result Result) string {
	view, ok := webmod.ViewExternalEvidenceWorkflowResult(result)
	if !ok {
		return ""
	}
	parts := make([]string, 0, 4)
	if len(view.Search.Results) > 0 {
		parts = append(parts, renderWebSearchContext(Result{Name: "web_search", Data: result.Data}))
	}
	if combined := strings.TrimSpace(view.Fetch.ReadableText()); combined != "" {
		parts = append(parts, "Fetched web content:\n"+truncateRenderedContext(combined))
	}
	if len(view.SourceReview.SelectedSources) > 0 {
		lines := make([]string, 0, len(view.SourceReview.SelectedSources))
		for _, item := range view.SourceReview.SelectedSources {
			meta := make([]string, 0, 2)
			if item.Policy != "" {
				meta = append(meta, "policy="+item.Policy)
			}
			if item.SourceType != "" {
				meta = append(meta, "type="+item.SourceType)
			}
			if len(meta) > 0 {
				lines = append(lines, fmt.Sprintf("- %s (%s) [%s]", item.Title, item.URL, strings.Join(meta, ", ")))
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s (%s)", item.Title, item.URL))
		}
		parts = append(parts, "Selected sources:\n"+strings.Join(lines, "\n"))
	}
	if readiness := strings.TrimSpace(view.Readiness); readiness != "" {
		line := fmt.Sprintf("Readiness: %s (confidence=%.2f)", readiness, view.ReadinessConfidence)
		if reasoning := strings.TrimSpace(view.ReadinessReasoning); reasoning != "" {
			line += "\n" + reasoning
		}
		if quality := strings.TrimSpace(view.Quality.Quality); quality != "" {
			line += fmt.Sprintf("\nQuality: %s (confidence=%.2f)", quality, view.Quality.Confidence)
		}
		if qualityReasoning := strings.TrimSpace(view.Quality.Reasoning); qualityReasoning != "" {
			line += "\n" + qualityReasoning
		}
		if strategy := strings.TrimSpace(view.AnswerStrategy); strategy != "" {
			line += "\nStrategy: " + strategy
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n\n")
}

func truncateRenderedContext(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= maxRenderedToolContextLen {
		return raw
	}
	return strings.TrimSpace(raw[:maxRenderedToolContextLen-3]) + "..."
}

// ToCallSummaries 把执行结果转换成 workflow 可直接复用的摘要结构。
func ToCallSummaries(results []Result) []CallSummary {
	if len(results) == 0 {
		return []CallSummary{}
	}
	items := make([]CallSummary, 0, len(results))
	for _, result := range results {
		items = append(items, CallSummary{
			Name:    strings.TrimSpace(result.Name),
			Status:  strings.TrimSpace(result.Status),
			Summary: strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage)),
		})
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
