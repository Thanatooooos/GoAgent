package domain

import "time"

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string
	Avatar       string
	CreatedBy    string
	UpdatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewUser(id, username, passwordHash, role, avatar, createdBy string) User {
	now := time.Now()
	return User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         role,
		Avatar:       avatar,
		CreatedBy:    createdBy,
		UpdatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
