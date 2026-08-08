package domain

import "time"

type Subscription struct {
	UserID            string
	Enabled           bool
	Timezone          string
	DeliveryTimeLocal string
	Topics            []string
	Sources           []string
	LockOwner         string
	LockUntil         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewSubscription(userID, timezone, deliveryTimeLocal string, topics []string, sources []string) Subscription {
	now := time.Now()
	return Subscription{
		UserID:            userID,
		Enabled:           true,
		Timezone:          timezone,
		DeliveryTimeLocal: deliveryTimeLocal,
		Topics:            append([]string(nil), topics...),
		Sources:           append([]string(nil), sources...),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}
