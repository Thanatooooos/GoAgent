package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type SubscriptionService struct {
	repo port.SubscriptionRepository
}

func NewSubscriptionService(repo port.SubscriptionRepository) *SubscriptionService {
	return &SubscriptionService{repo: repo}
}

func (s *SubscriptionService) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	normalized := normalizeSubscription(subscription)
	derived, err := deriveSubscriptionSnapshot(normalized)
	if err != nil {
		return domain.Subscription{}, err
	}
	if err := validateSubscription(derived); err != nil {
		return domain.Subscription{}, err
	}
	return s.repo.Upsert(ctx, derived)
}

func (s *SubscriptionService) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	return s.repo.GetByUserID(ctx, strings.TrimSpace(userID))
}

func deriveSubscriptionSnapshot(subscription domain.Subscription) (domain.Subscription, error) {
	derived := subscription
	for _, topic := range derived.Topics {
		if !domain.IsTopicKeySelectable(topic) {
			return domain.Subscription{}, fmt.Errorf("subscription topic %q must be an enabled selectable leaf", topic)
		}
	}
	derived.Sources = domain.SourceKeysForTopics(derived.Topics)
	if derived.Enabled && len(derived.Topics) > 0 && len(derived.Sources) == 0 {
		return domain.Subscription{}, fmt.Errorf("subscription topics %v do not currently resolve to any sources", derived.Topics)
	}
	return derived, nil
}

func validateSubscription(subscription domain.Subscription) error {
	if strings.TrimSpace(subscription.UserID) == "" {
		return fmt.Errorf("subscription user id is required")
	}
	if strings.TrimSpace(subscription.Timezone) == "" {
		return fmt.Errorf("subscription timezone is required")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(subscription.Timezone)); err != nil {
		return fmt.Errorf("subscription timezone is invalid: %w", err)
	}
	if strings.TrimSpace(subscription.DeliveryTimeLocal) == "" {
		return fmt.Errorf("subscription delivery time is required")
	}
	if _, err := time.Parse("15:04", strings.TrimSpace(subscription.DeliveryTimeLocal)); err != nil {
		return fmt.Errorf("subscription delivery time must use HH:MM format: %w", err)
	}
	for _, topic := range subscription.Topics {
		key := strings.TrimSpace(topic)
		if key == "" || !domain.IsTopicKeySelectable(key) {
			return fmt.Errorf("subscription topic %q is not supported", topic)
		}
	}
	for _, source := range subscription.Sources {
		key := strings.TrimSpace(source)
		if key == "" || !domain.IsSourceKeySupported(key) {
			return fmt.Errorf("subscription source %q is not supported", source)
		}
	}
	return nil
}

func normalizeSubscription(subscription domain.Subscription) domain.Subscription {
	normalized := subscription
	normalized.UserID = strings.TrimSpace(subscription.UserID)
	normalized.Timezone = strings.TrimSpace(subscription.Timezone)
	normalized.DeliveryTimeLocal = strings.TrimSpace(subscription.DeliveryTimeLocal)
	normalized.Topics = uniqueTrimmed(subscription.Topics)
	return normalized
}

func uniqueTrimmed(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

type SubscriptionSnapshotRecomputeInput struct {
	UserIDs []string
}

type SubscriptionSnapshotRecomputeResult struct {
	ScannedCount int `json:"scannedCount"`
	UpdatedCount int `json:"updatedCount"`
}

type SubscriptionSnapshotService struct {
	repo port.SubscriptionRepository
}

type SubscriptionSnapshotRefresher interface {
	Recompute(ctx context.Context, input SubscriptionSnapshotRecomputeInput) (SubscriptionSnapshotRecomputeResult, error)
}

func NewSubscriptionSnapshotService(repo port.SubscriptionRepository) *SubscriptionSnapshotService {
	return &SubscriptionSnapshotService{repo: repo}
}

func (s *SubscriptionSnapshotService) Recompute(ctx context.Context, input SubscriptionSnapshotRecomputeInput) (SubscriptionSnapshotRecomputeResult, error) {
	subs, err := s.repo.List(ctx, port.SubscriptionListFilter{})
	if err != nil {
		return SubscriptionSnapshotRecomputeResult{}, err
	}
	result := SubscriptionSnapshotRecomputeResult{ScannedCount: len(subs)}
	filter := make(map[string]struct{}, len(input.UserIDs))
	for _, userID := range input.UserIDs {
		filter[strings.TrimSpace(userID)] = struct{}{}
	}
	for _, subscription := range subs {
		if len(filter) > 0 {
			if _, ok := filter[subscription.UserID]; !ok {
				continue
			}
		}
		if strings.TrimSpace(subscription.UserID) == "" {
			continue
		}
		updated, err := deriveSubscriptionSnapshot(normalizeSubscription(subscription))
		if err != nil {
			return SubscriptionSnapshotRecomputeResult{}, err
		}
		if reflect.DeepEqual(updated.Sources, subscription.Sources) {
			continue
		}
		if _, err := s.repo.Upsert(ctx, updated); err != nil {
			return SubscriptionSnapshotRecomputeResult{}, err
		}
		result.UpdatedCount++
	}
	return result, nil
}
