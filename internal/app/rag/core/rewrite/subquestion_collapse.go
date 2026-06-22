package rewrite

import "strings"

func collapseSingleIntentSubQuestions(originalQuestion string, result Result) Result {
	rewritten := strings.TrimSpace(result.RewrittenQuestion)
	if rewritten == "" || len(result.SubQuestions) <= 1 {
		return result
	}
	if looksLikeMultiIntentQuery(originalQuestion) {
		return result
	}
	result.SubQuestions = []string{rewritten}
	return result
}

func looksLikeMultiIntentQuery(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	lower := strings.ToLower(question)

	if strings.Contains(question, "以及") {
		return true
	}

	for _, marker := range []string{"分别", "各自", "一方面", "另一方面", "同时告警"} {
		if strings.Contains(question, marker) {
			return true
		}
	}

	if strings.ContainsAny(question, "，,") {
		if (strings.Contains(lower, "http") && strings.Contains(lower, "https")) ||
			(strings.Contains(question, "窃听") && strings.Contains(question, "篡改")) {
			return true
		}
	}

	if !strings.Contains(question, "和") {
		return false
	}

	if strings.Contains(question, "相比") ||
		strings.Contains(question, "区别") ||
		strings.Contains(question, "差异") ||
		strings.Contains(question, "异同") {
		return false
	}

	return true
}
