package domain

import (
	"fmt"
	"strings"
	"time"
)

func ResolveBriefDate(now time.Time, timezone string) (string, error) {
	locationName := strings.TrimSpace(timezone)
	if locationName == "" {
		locationName = "UTC"
	}
	loc, err := time.LoadLocation(locationName)
	if err != nil {
		return "", fmt.Errorf("resolve brief date location: %w", err)
	}
	return now.In(loc).Format("2006-01-02"), nil
}

func DeliveryWindowOpen(subscription Subscription, now time.Time) (bool, error) {
	if !subscription.Enabled {
		return false, nil
	}
	loc, err := time.LoadLocation(strings.TrimSpace(subscription.Timezone))
	if err != nil {
		return false, fmt.Errorf("load subscription timezone: %w", err)
	}
	localNow := now.In(loc)
	deliveryTime, err := time.Parse("15:04", strings.TrimSpace(subscription.DeliveryTimeLocal))
	if err != nil {
		return false, fmt.Errorf("parse delivery time: %w", err)
	}
	scheduled := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		deliveryTime.Hour(), deliveryTime.Minute(), 0, 0, loc,
	)
	return !localNow.Before(scheduled), nil
}

func ShouldScheduleGeneration(subscription Subscription, issue Issue, now time.Time) (bool, error) {
	if !subscription.Enabled {
		return false, nil
	}
	if strings.TrimSpace(issue.ID) != "" {
		return false, nil
	}
	return DeliveryWindowOpen(subscription, now)
}
