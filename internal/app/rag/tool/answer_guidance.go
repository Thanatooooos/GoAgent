package tool

import (
	"fmt"
	"strings"
)

func BuildAnswerGuidance(results []Result) string {
	for _, result := range results {
		switch strings.TrimSpace(result.Name) {
		case "document_ingestion_diagnose":
			return buildDiagnosisGuidance(result)
		case "task_ingestion_diagnose":
			return buildDiagnosisGuidance(result)
		case "trace_retrieval_diagnose":
			return buildDiagnosisGuidance(result)
		}
	}
	return ""
}

func buildDiagnosisGuidance(result Result) string {
	conclusion := strings.TrimSpace(readDataString(result.Data, "conclusion"))
	confidence := strings.TrimSpace(readDataString(result.Data, "confidence"))
	evidence := readDataStringSlice(result.Data, "evidence")
	suggestions := readDataStringSlice(result.Data, "suggestions")

	var builder strings.Builder
	builder.WriteString("这是一次诊断类回答。请优先使用中文，并按“结论 / 证据 / 建议”的顺序回答。")
	if conclusion != "" {
		builder.WriteString("\n结论需要直接说明当前最可能的问题：")
		builder.WriteString(conclusion)
		builder.WriteString("。")
	}
	if confidence != "" {
		builder.WriteString("\n明确给出判断置信度：")
		builder.WriteString(confidence)
		builder.WriteString("。")
	}
	if len(evidence) > 0 {
		builder.WriteString("\n证据部分只挑最关键的 3-5 条，优先引用以下信息：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(evidence, "\n- "))
	}
	if len(suggestions) > 0 {
		builder.WriteString("\n建议部分给出可执行的下一步检查项，优先包含：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(suggestions, "\n- "))
	}
	builder.WriteString("\n不要只复述工具调用过程，也不要把回答写成泛泛的知识解释。")
	return builder.String()
}

func readDataString(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func readDataStringSlice(data map[string]any, key string) []string {
	if len(data) == 0 {
		return []string{}
	}
	value, ok := data[key]
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := fmt.Sprintf("%v", item)
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	default:
		return []string{}
	}
}
