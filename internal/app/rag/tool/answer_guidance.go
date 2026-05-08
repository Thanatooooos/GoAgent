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
	facts := preferDataStringSlice(result.Data, "facts", "evidence")
	inferences := readDataStringSlice(result.Data, "inferences")
	riskHints := readDataStringSlice(result.Data, "riskHints")
	nextActions := preferDataStringSlice(result.Data, "nextActions", "suggestions")

	var builder strings.Builder
	builder.WriteString("这是一次诊断类回答。请优先使用中文，并按“结论 / 证据 / 建议”的顺序组织回答。")
	builder.WriteString("\n结论部分只陈述当前最可能的问题，并明确写出置信度。")
	if conclusion != "" {
		builder.WriteString("\n当前建议结论：")
		builder.WriteString(conclusion)
		builder.WriteString("。")
	}
	if confidence != "" {
		builder.WriteString("\n当前置信度：")
		builder.WriteString(confidence)
		builder.WriteString("。")
	}
	if len(facts) > 0 {
		builder.WriteString("\n证据部分优先引用 3-5 条最关键事实，不要把推断写成事实：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(facts, "\n- "))
	}
	if len(inferences) > 0 {
		builder.WriteString("\n如果需要补充推断，请明确标注“推断”或“可能原因”，不要伪装成已确认事实：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(inferences, "\n- "))
	}
	if len(riskHints) > 0 {
		builder.WriteString("\n若证据存在边界或风险，请显式提醒用户：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(riskHints, "\n- "))
	}
	if len(nextActions) > 0 {
		builder.WriteString("\n建议部分给出可执行的下一步检查项：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(nextActions, "\n- "))
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

func preferDataStringSlice(data map[string]any, primary string, fallback string) []string {
	items := readDataStringSlice(data, primary)
	if len(items) > 0 {
		return items
	}
	return readDataStringSlice(data, fallback)
}
