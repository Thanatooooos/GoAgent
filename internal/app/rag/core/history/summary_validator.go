package history

import (
	"fmt"
	"regexp"
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

type SummaryValidationResult struct {
	Accepted bool
	Reason   string
}

var criticalEntityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:doc|task|trace)_[a-z0-9_-]+\b`),
	regexp.MustCompile(`(?i)\b[a-z0-9_.-]+=[^\s,;，。；、]+`),
	regexp.MustCompile(`(?i)\bv\d+(?:\.\d+){1,2}\b`),
}

func ValidateStructuredSummary(summary StructuredSummary, source []domain.ConversationMessage) SummaryValidationResult {
	summary.Normalize()

	if strings.TrimSpace(summary.Goal) == "" {
		return SummaryValidationResult{Reason: "missing goal"}
	}
	if len(summary.ActivePriorities) == 0 && len(summary.Constraints) == 0 && len(summary.EstablishedFacts) == 0 && len(summary.RecentProgress) == 0 {
		return SummaryValidationResult{Reason: "missing high-value sections"}
	}
	if sourceSetsCurrentFocus(source) && len(summary.ActivePriorities) == 0 {
		return SummaryValidationResult{Reason: "missing active priority"}
	}
	if activePrioritiesContainBackgroundOnly(summary.ActivePriorities) {
		return SummaryValidationResult{Reason: "background issue promoted"}
	}
	if err := ensureCriticalEntitiesPreserved(summary, source); err != nil {
		return SummaryValidationResult{Reason: err.Error()}
	}
	return SummaryValidationResult{Accepted: true}
}

func ensureCriticalEntitiesPreserved(summary StructuredSummary, source []domain.ConversationMessage) error {
	entities := extractCriticalEntities(source)
	if len(entities) == 0 {
		return nil
	}

	summaryText := strings.ToLower(renderSummaryValidationText(summary))
	for _, entity := range entities {
		if strings.Contains(summaryText, strings.ToLower(entity)) {
			return nil
		}
	}

	return fmt.Errorf("missing critical entities: %s", strings.Join(entities, ", "))
}

func extractCriticalEntities(source []domain.ConversationMessage) []string {
	if len(source) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	var entities []string
	for _, message := range source {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		lower := strings.ToLower(content)

		for _, pattern := range criticalEntityPatterns {
			for _, match := range pattern.FindAllString(content, -1) {
				entities = appendUniqueEntity(entities, seen, match)
			}
		}

		for _, phrase := range []string{
			"indexer failed",
			"vector store unavailable",
			"connection refused",
			"request timeout",
			"summary-max-chars",
		} {
			if strings.Contains(lower, phrase) {
				entities = appendUniqueEntity(entities, seen, phrase)
			}
		}
	}

	return entities
}

func renderSummaryValidationText(summary StructuredSummary) string {
	parts := []string{summary.Goal}
	parts = append(parts, summary.ActivePriorities...)
	parts = append(parts, summary.UserPreferences...)
	parts = append(parts, summary.Constraints...)
	parts = append(parts, summary.EstablishedFacts...)
	parts = append(parts, summary.RecentProgress...)
	parts = append(parts, summary.OpenQuestions...)
	parts = append(parts, summary.BackgroundIssues...)
	return strings.ToLower(strings.Join(parts, "\n"))
}

func sourceSetsCurrentFocus(source []domain.ConversationMessage) bool {
	for _, message := range source {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if containsAnySummaryRepairMarker(content, []string{"当前真正活跃的目标", "当前活跃目标", "当前重点", "当前主线", "下一周优先级"}) {
			return true
		}
	}
	return false
}

func activePrioritiesContainBackgroundOnly(items []string) bool {
	for _, item := range items {
		if isSummaryRepairBackgroundOnlyItem(item) {
			return true
		}
	}
	return false
}

func appendUniqueEntity(entities []string, seen map[string]struct{}, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return entities
	}
	key := strings.ToLower(trimmed)
	if _, exists := seen[key]; exists {
		return entities
	}
	seen[key] = struct{}{}
	return append(entities, trimmed)
}


