package dailybrief

import (
	"strings"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
	"local/rag-project/internal/app/dailybrief/domain"
)

func toSubscriptionModel(item domain.Subscription) models.SubscriptionModel {
	return models.SubscriptionModel{
		UserID:            item.UserID,
		Enabled:           boolToFlag(item.Enabled),
		Timezone:          item.Timezone,
		DeliveryTimeLocal: item.DeliveryTimeLocal,
		TopicsJSON:        item.Topics,
		SourcesJSON:       item.Sources,
		LockOwner:         stringToPointer(item.LockOwner),
		LockUntil:         item.LockUntil,
		CreateTime:        item.CreatedAt,
		UpdateTime:        item.UpdatedAt,
	}
}

func toSubscriptionDomain(item models.SubscriptionModel) domain.Subscription {
	lockOwner := ""
	if item.LockOwner != nil {
		lockOwner = *item.LockOwner
	}
	return domain.Subscription{
		UserID:            item.UserID,
		Enabled:           item.Enabled == 1,
		Timezone:          item.Timezone,
		DeliveryTimeLocal: item.DeliveryTimeLocal,
		Topics:            append([]string(nil), item.TopicsJSON...),
		Sources:           append([]string(nil), item.SourcesJSON...),
		LockOwner:         lockOwner,
		LockUntil:         item.LockUntil,
		CreatedAt:         item.CreateTime,
		UpdatedAt:         item.UpdateTime,
	}
}

func toIssueModel(item domain.Issue) models.IssueModel {
	return models.IssueModel{
		ID:             item.ID,
		UserID:         item.UserID,
		BriefDate:      item.BriefDate,
		Status:         item.Status,
		Headline:       item.Headline,
		TopSummary:     item.TopSummary,
		SectionsJSON:   normalizeJSONObject(item.SectionsJSON),
		ItemCount:      item.ItemCount,
		PublishedRunID: item.PublishedRunID,
		GeneratedAt:    item.GeneratedAt,
		PublishedAt:    item.PublishedAt,
		CreateTime:     item.CreatedAt,
		UpdateTime:     item.UpdatedAt,
	}
}

func toIssueDomain(item models.IssueModel) domain.Issue {
	return domain.Issue{
		ID:             item.ID,
		UserID:         item.UserID,
		BriefDate:      item.BriefDate,
		Status:         item.Status,
		Headline:       item.Headline,
		TopSummary:     item.TopSummary,
		SectionsJSON:   item.SectionsJSON,
		ItemCount:      item.ItemCount,
		PublishedRunID: item.PublishedRunID,
		GeneratedAt:    item.GeneratedAt,
		PublishedAt:    item.PublishedAt,
		CreatedAt:      item.CreateTime,
		UpdatedAt:      item.UpdateTime,
	}
}

func toItemModel(item domain.Item) models.ItemModel {
	return models.ItemModel{
		ID:           item.ID,
		IssueID:      item.IssueID,
		SectionKey:   item.SectionKey,
		Rank:         item.Rank,
		Title:        item.Title,
		Summary:      item.Summary,
		WhyItMatters: item.WhyItMatters,
		URL:          item.URL,
		Source:       item.Source,
		Topic:        item.Topic,
		PublishedAt:  item.PublishedAt,
		MetadataJSON: normalizeJSONObject(item.MetadataJSON),
		CreateTime:   item.CreatedAt,
		UpdateTime:   item.UpdatedAt,
	}
}

func toItemDomain(item models.ItemModel) domain.Item {
	return domain.Item{
		ID:           item.ID,
		IssueID:      item.IssueID,
		SectionKey:   item.SectionKey,
		Rank:         item.Rank,
		Title:        item.Title,
		Summary:      item.Summary,
		WhyItMatters: item.WhyItMatters,
		URL:          item.URL,
		Source:       item.Source,
		Topic:        item.Topic,
		PublishedAt:  item.PublishedAt,
		MetadataJSON: item.MetadataJSON,
		CreatedAt:    item.CreateTime,
		UpdatedAt:    item.UpdateTime,
	}
}

func toGenerationRunModel(item domain.GenerationRun) models.GenerationRunModel {
	return models.GenerationRunModel{
		ID:              item.ID,
		UserID:          item.UserID,
		BriefDate:       item.BriefDate,
		TriggerType:     item.TriggerType,
		Status:          item.Status,
		StartedAt:       item.StartedAt,
		FinishedAt:      item.FinishedAt,
		ErrorMessage:    item.ErrorMessage,
		SourceStatsJSON: normalizeJSONObject(item.SourceStatsJSON),
		Model:           item.Model,
		PromptVersion:   item.PromptVersion,
		TokenUsageJSON:  normalizeJSONObject(item.TokenUsageJSON),
		CreateTime:      item.CreatedAt,
		UpdateTime:      item.UpdatedAt,
	}
}

func toGenerationRunDomain(item models.GenerationRunModel) domain.GenerationRun {
	return domain.GenerationRun{
		ID:              item.ID,
		UserID:          item.UserID,
		BriefDate:       item.BriefDate,
		TriggerType:     item.TriggerType,
		Status:          item.Status,
		StartedAt:       item.StartedAt,
		FinishedAt:      item.FinishedAt,
		ErrorMessage:    item.ErrorMessage,
		SourceStatsJSON: item.SourceStatsJSON,
		Model:           item.Model,
		PromptVersion:   item.PromptVersion,
		TokenUsageJSON:  item.TokenUsageJSON,
		CreatedAt:       item.CreateTime,
		UpdatedAt:       item.UpdateTime,
	}
}

func boolToFlag(value bool) int16 {
	if value {
		return 1
	}
	return 0
}

func normalizeJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

func stringToPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
