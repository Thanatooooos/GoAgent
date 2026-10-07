package models

import "time"

type ConversationProfileStateModel struct {
	ConversationID        string     `gorm:"column:conversation_id;primaryKey"`
	UserID                string     `gorm:"column:user_id;index:idx_conversation_profile_state_due"`
	LastObservedMessageID string     `gorm:"column:last_observed_message_id"`
	ProcessingToMessageID string     `gorm:"column:processing_to_message_id"`
	LastObservedAt        *time.Time `gorm:"column:last_observed_at"`
	Status                string     `gorm:"column:status;index:idx_conversation_profile_state_due"`
	NextRunAt             time.Time  `gorm:"column:next_run_at;index:idx_conversation_profile_state_due"`
	Attempts              int        `gorm:"column:attempts"`
	CreateTime            time.Time  `gorm:"column:create_time"`
	UpdateTime            time.Time  `gorm:"column:update_time"`
}

func (ConversationProfileStateModel) TableName() string { return "t_conversation_profile_state" }
