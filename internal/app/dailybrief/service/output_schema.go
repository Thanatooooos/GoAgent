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

func AlignBriefArtifactWithCandidates(artifact domain.BriefArtifact, candidates []domain.Candidate) domain.BriefArtifact {
	byURL := make(map[string]domain.Candidate, len(candidates))
	for _, candidate := range candidates {
		key := normalizeSourceURL(candidate.URL)
		if key == "" {
			continue
		}
		byURL[key] = candidate
	}

	sections := make([]domain.BriefSection, len(artifact.Sections))
	for index, section := range artifact.Sections {
		items := make([]domain.BriefItemDraft, len(section.Items))
		for itemIndex, item := range section.Items {
			items[itemIndex] = item
			candidate, ok := byURL[normalizeSourceURL(item.URL)]
			if !ok {
				continue
			}
			items[itemIndex].URL = candidate.URL
			items[itemIndex].Source = candidate.Source
			items[itemIndex].Topic = candidate.Topic
		}
		section.Items = items
		sections[index] = section
	}
	artifact.Sections = sections
	return artifact
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
	if !domain.IsSourceKeySupported(item.Source) {
		return fmt.Errorf("item source %q is not supported", item.Source)
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
