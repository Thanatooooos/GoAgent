package dailybrief

import (
	"encoding/json"
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
	dailybriefservice "local/rag-project/internal/app/dailybrief/service"
)

type issueResponse struct {
	PageState       string    `json:"pageState"`
	Issue           *issueDTO `json:"issue,omitempty"`
	BriefDate       string    `json:"briefDate"`
	LastGeneratedAt *string   `json:"lastGeneratedAt,omitempty"`
}

type issueDTO struct {
	Headline   string            `json:"headline"`
	TopSummary string            `json:"topSummary"`
	Sections   []issueSectionDTO `json:"sections"`
	Items      []issueItemDTO    `json:"items"`
}

type issueSectionDTO struct {
	Key   string         `json:"key"`
	Title string         `json:"title"`
	Items []issueItemDTO `json:"items"`
}

type issueItemDTO struct {
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	WhyItMatters string `json:"whyItMatters"`
	URL          string `json:"url"`
	Source       string `json:"source"`
	Topic        string `json:"topic"`
}

type subscriptionResponse struct {
	Enabled           bool     `json:"enabled"`
	Timezone          string   `json:"timezone"`
	DeliveryTimeLocal string   `json:"deliveryTimeLocal"`
	Topics            []string `json:"topics"`
}

type updateSubscriptionRequest struct {
	Enabled           bool     `json:"enabled"`
	Timezone          string   `json:"timezone"`
	DeliveryTimeLocal string   `json:"deliveryTimeLocal"`
	Topics            []string `json:"topics"`
}

type topicCatalogResponse struct {
	Nodes []dailybriefservice.TopicCatalogNodeReadModel `json:"nodes"`
}

type recomputeSnapshotsRequest struct {
	UserIDs []string `json:"userIds"`
}

func toSubscriptionResponse(subscription domain.Subscription) subscriptionResponse {
	response := subscriptionResponse{
		Enabled:           subscription.Enabled,
		Timezone:          subscription.Timezone,
		DeliveryTimeLocal: subscription.DeliveryTimeLocal,
		Topics:            append([]string(nil), subscription.Topics...),
	}
	if response.Topics == nil {
		response.Topics = []string{}
	}
	if strings.TrimSpace(subscription.UserID) == "" {
		response.Enabled = false
		if response.Timezone == "" {
			response.Timezone = "UTC"
		}
		if response.DeliveryTimeLocal == "" {
			response.DeliveryTimeLocal = "08:00"
		}
	}
	return response
}

func fromSubscriptionRequest(userID string, req updateSubscriptionRequest) domain.Subscription {
	return domain.Subscription{
		UserID:            userID,
		Enabled:           req.Enabled,
		Timezone:          req.Timezone,
		DeliveryTimeLocal: req.DeliveryTimeLocal,
		Topics:            append([]string(nil), req.Topics...),
	}
}

func toIssueDTO(model dailybriefservice.IssueReadModel) *issueDTO {
	items := toIssueItemDTOs(model.Items)
	return &issueDTO{
		Headline:   model.Issue.Headline,
		TopSummary: model.Issue.TopSummary,
		Sections:   buildIssueSections(model.Issue.SectionsJSON, model.Items),
		Items:      items,
	}
}

func toIssueItemDTOs(items []domain.Item) []issueItemDTO {
	result := make([]issueItemDTO, 0, len(items))
	for _, item := range items {
		result = append(result, issueItemDTO{
			Title:        item.Title,
			Summary:      item.Summary,
			WhyItMatters: item.WhyItMatters,
			URL:          item.URL,
			Source:       item.Source,
			Topic:        item.Topic,
		})
	}
	return result
}

func buildIssueSections(sectionsJSON string, items []domain.Item) []issueSectionDTO {
	sectionTitles := map[string]string{}
	if strings.TrimSpace(sectionsJSON) != "" {
		var sections []domain.BriefSection
		if err := json.Unmarshal([]byte(sectionsJSON), &sections); err == nil {
			for _, section := range sections {
				sectionTitles[section.Key] = section.Title
			}
		}
	}

	grouped := make(map[string][]issueItemDTO)
	order := make([]string, 0)
	for _, item := range items {
		key := strings.TrimSpace(item.SectionKey)
		if key == "" {
			key = "highlights"
		}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], issueItemDTO{
			Title:        item.Title,
			Summary:      item.Summary,
			WhyItMatters: item.WhyItMatters,
			URL:          item.URL,
			Source:       item.Source,
			Topic:        item.Topic,
		})
	}

	result := make([]issueSectionDTO, 0, len(order))
	for _, key := range order {
		title := sectionTitles[key]
		if title == "" {
			title = key
		}
		result = append(result, issueSectionDTO{
			Key:   key,
			Title: title,
			Items: grouped[key],
		})
	}
	return result
}
