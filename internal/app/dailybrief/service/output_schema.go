package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
)

func ValidateBriefArtifact(artifact domain.BriefArtifact, maxItems int) error {
	if strings.TrimSpace(artifact.Headline) == "" {
		return fmt.Errorf("brief headline is required")
	}
	if strings.TrimSpace(artifact.TopSummary) == "" {
		return fmt.Errorf("brief top summary is required")
	}
	if len(artifact.Sections) == 0 {
		return fmt.Errorf("brief must contain at least one section")
	}

	itemCount := 0
	for _, section := range artifact.Sections {
		if strings.TrimSpace(section.Key) == "" {
			return fmt.Errorf("brief section key is required")
		}
		if strings.TrimSpace(section.Title) == "" {
			return fmt.Errorf("brief section title is required")
		}
		for _, item := range section.Items {
			if err := validateBriefItemDraft(item); err != nil {
				return fmt.Errorf("section %q: %w", section.Key, err)
			}
			itemCount++
		}
	}
	if itemCount == 0 {
		return fmt.Errorf("brief must contain at least one item")
	}
	if maxItems > 0 && itemCount > maxItems {
		return fmt.Errorf("brief item count %d exceeds max %d", itemCount, maxItems)
	}
	return nil
}

func TruncateBriefArtifact(artifact domain.BriefArtifact, maxItems int) domain.BriefArtifact {
	if maxItems <= 0 {
		return artifact
	}
	remaining := maxItems
	sections := make([]domain.BriefSection, 0, len(artifact.Sections))
	for _, section := range artifact.Sections {
		if remaining <= 0 {
			break
		}
		if len(section.Items) <= remaining {
			sections = append(sections, section)
			remaining -= len(section.Items)
			continue
		}
		trimmed := section
		trimmed.Items = append([]domain.BriefItemDraft(nil), section.Items[:remaining]...)
		sections = append(sections, trimmed)
		remaining = 0
	}
	artifact.Sections = sections
	return artifact
}

// TruncateBriefArtifactPerTopic caps items within each section/topic, then applies a global cap.
func TruncateBriefArtifactPerTopic(artifact domain.BriefArtifact, perTopicLimit int, globalMax int) domain.BriefArtifact {
	if perTopicLimit <= 0 {
		return TruncateBriefArtifact(artifact, globalMax)
	}

	sections := make([]domain.BriefSection, 0, len(artifact.Sections))
	for _, section := range artifact.Sections {
		if len(section.Items) <= perTopicLimit {
			sections = append(sections, section)
			continue
		}
		trimmed := section
		trimmed.Items = append([]domain.BriefItemDraft(nil), section.Items[:perTopicLimit]...)
		sections = append(sections, trimmed)
	}
	artifact.Sections = sections

	total := countBriefItems(artifact)
	if globalMax <= 0 || total <= globalMax {
		return artifact
	}

	remove := total - globalMax
	for remove > 0 {
		removed := false
		for index := len(artifact.Sections) - 1; index >= 0 && remove > 0; index-- {
			section := &artifact.Sections[index]
			if len(section.Items) == 0 {
				continue
			}
			section.Items = section.Items[:len(section.Items)-1]
			remove--
			removed = true
		}
		if !removed {
			break
		}
	}

	compact := make([]domain.BriefSection, 0, len(artifact.Sections))
	for _, section := range artifact.Sections {
		if len(section.Items) == 0 {
			continue
		}
		compact = append(compact, section)
	}
	artifact.Sections = compact
	return artifact
}

func countBriefItems(artifact domain.BriefArtifact) int {
	total := 0
	for _, section := range artifact.Sections {
		total += len(section.Items)
	}
	return total
}

const (
	fallbackItemSummary      = "该条目暂未生成摘要，可点击原文查看详情。"
	fallbackItemWhyItMatters = "该动态对关注本领域的读者具有参考价值。"
)

func AlignBriefArtifactWithCandidates(artifact domain.BriefArtifact, candidates []domain.Candidate) domain.BriefArtifact {
	byURL := make(map[string]domain.Candidate, len(candidates))
	byTitle := make(map[string]domain.Candidate, len(candidates))
	for _, candidate := range candidates {
		urlKey := normalizeSourceURL(candidate.URL)
		if urlKey != "" {
			byURL[urlKey] = candidate
		}
		titleKey := normalizeItemText(candidate.Title)
		if titleKey != "" {
			byTitle[titleKey] = candidate
		}
	}

	sections := make([]domain.BriefSection, len(artifact.Sections))
	for index := range artifact.Sections {
		section := artifact.Sections[index]
		items := make([]domain.BriefItemDraft, len(section.Items))
		for itemIndex, item := range section.Items {
			candidate, ok := byURL[normalizeSourceURL(item.URL)]
			if !ok {
				candidate, ok = byTitle[normalizeItemText(item.Title)]
			}
			items[itemIndex] = repairBriefItemDraft(item, candidate, ok)
		}
		section.Items = items
		section.Title = firstNonEmpty(section.Title, topicTitleForSectionKey(section.Key))
		sections[index] = section
	}
	artifact.Sections = sections
	return artifact
}

func repairBriefItemDraft(item domain.BriefItemDraft, candidate domain.Candidate, matched bool) domain.BriefItemDraft {
	if matched {
		item.URL = candidate.URL
		item.Source = candidate.Source
		item.Topic = candidate.Topic
		item.Title = firstNonEmpty(item.Title, candidate.Title)
		item.Summary = firstNonEmpty(item.Summary, candidate.SummarySnippet, fallbackItemSummary)
		item.WhyItMatters = firstNonEmpty(item.WhyItMatters, fallbackItemWhyItMatters)
		return item
	}
	item.Title = firstNonEmpty(item.Title, candidate.Title)
	item.URL = firstNonEmpty(item.URL, candidate.URL)
	item.Source = firstNonEmpty(item.Source, candidate.Source)
	item.Topic = firstNonEmpty(item.Topic, candidate.Topic)
	item.Summary = firstNonEmpty(item.Summary, fallbackItemSummary)
	item.WhyItMatters = firstNonEmpty(item.WhyItMatters, fallbackItemWhyItMatters)
	return item
}

func normalizeItemText(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(raw)), " "))
}

func topicTitleForSectionKey(key string) string {
	if key == "" {
		return ""
	}
	return domain.TopicDisplayName(strings.TrimSpace(key))
}

func validateBriefItemDraft(item domain.BriefItemDraft) error {
	if strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("item title is required")
	}
	if strings.TrimSpace(item.Summary) == "" {
		return fmt.Errorf("item summary is required")
	}
	if strings.TrimSpace(item.WhyItMatters) == "" {
		return fmt.Errorf("item whyItMatters is required")
	}
	if strings.TrimSpace(item.URL) == "" {
		return fmt.Errorf("item url is required")
	}
	if strings.TrimSpace(item.Source) == "" {
		return fmt.Errorf("item source is required")
	}
	if strings.TrimSpace(item.Topic) == "" {
		return fmt.Errorf("item topic is required")
	}
	if !domain.IsTopicKeySupported(item.Topic) {
		return fmt.Errorf("item topic %q is not supported", item.Topic)
	}
	return nil
}

func ParseBriefArtifact(raw string) (domain.BriefArtifact, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return domain.BriefArtifact{}, fmt.Errorf("brief generation output is empty")
	}
	if extracted := extractJSONObject(raw); extracted != "" {
		raw = extracted
	}

	var artifact domain.BriefArtifact
	if err := json.Unmarshal([]byte(raw), &artifact); err != nil {
		return domain.BriefArtifact{}, fmt.Errorf("parse brief artifact json: %w", err)
	}
	return artifact, nil
}

func MarshalBriefSections(sections []domain.BriefSection) (string, error) {
	payload, err := json.Marshal(sections)
	if err != nil {
		return "", fmt.Errorf("marshal brief sections: %w", err)
	}
	return string(payload), nil
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	return raw[start : end+1]
}
