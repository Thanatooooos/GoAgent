package models

import "time"

type UserMemoryProfileModel struct {
	UserID          string     `gorm:"column:user_id;primaryKey"`
	ContentMarkdown string     `gorm:"column:content_markdown"`
	Version         int64      `gorm:"column:version"`
	LastObservedAt  *time.Time `gorm:"column:last_observed_at"`
	CreateTime      time.Time  `gorm:"column:create_time"`
	UpdateTime      time.Time  `gorm:"column:update_time"`
}

func (UserMemoryProfileModel) TableName() string { return "t_user_memory_profile" }
