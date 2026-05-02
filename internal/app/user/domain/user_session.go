package domain

import "time"

type UserSession struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewUserSession(token, userID string, expiresAt time.Time) UserSession {
	now := time.Now()
	return UserSession{
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
