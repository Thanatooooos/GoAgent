package tool

import "strings"

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
			continue
		}
		if result.Successful() {
			builder.WriteString("tool executed successfully")
			continue
		}
		builder.WriteString(strings.TrimSpace(result.ErrorMessage))
	}
	return strings.TrimSpace(builder.String())
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
