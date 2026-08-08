package schedule

import (
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

func ResolveBriefDate(now time.Time, timezone string) (string, error) {
	return domain.ResolveBriefDate(now, timezone)
}

func DeliveryWindowOpen(subscription domain.Subscription, now time.Time) (bool, error) {
	return domain.DeliveryWindowOpen(subscription, now)
}

func ShouldScheduleGeneration(subscription domain.Subscription, issue domain.Issue, now time.Time) (bool, error) {
	return domain.ShouldScheduleGeneration(subscription, issue, now)
}
