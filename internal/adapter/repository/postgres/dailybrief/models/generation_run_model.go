package models

import "time"

type GenerationRunModel struct {
	ID              string     `gorm:"column:id;type:varchar(20);primaryKey"`
	UserID          string     `gorm:"column:user_id;type:varchar(20);not null;index:idx_daily_brief_generation_run_user_date_started"`
	BriefDate       string     `gorm:"column:brief_date;type:varchar(10);not null;index:idx_daily_brief_generation_run_user_date_started"`
	TriggerType     string     `gorm:"column:trigger_type;type:varchar(16);not null"`
	Status          string     `gorm:"column:status;type:varchar(16);not null;index:idx_daily_brief_generation_run_status_finished"`
	StartedAt       time.Time  `gorm:"column:started_at;not null;index:idx_daily_brief_generation_run_user_date_started,sort:desc"`
	FinishedAt      *time.Time `gorm:"column:finished_at;index:idx_daily_brief_generation_run_status_finished"`
	ErrorMessage    string     `gorm:"column:error_message;type:varchar(1024)"`
	SourceStatsJSON string     `gorm:"column:source_stats_json;type:jsonb;not null"`
	Model           string     `gorm:"column:model;type:varchar(128)"`
	PromptVersion   string     `gorm:"column:prompt_version;type:varchar(64)"`
	TokenUsageJSON  string     `gorm:"column:token_usage_json;type:jsonb;not null"`
	CreateTime      time.Time  `gorm:"column:create_time;not null"`
	UpdateTime      time.Time  `gorm:"column:update_time;not null"`
}

func (GenerationRunModel) TableName() string {
	return "t_daily_brief_generation_run"
}
