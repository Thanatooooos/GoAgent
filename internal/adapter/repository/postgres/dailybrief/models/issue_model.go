package models

import "time"

type IssueModel struct {
	ID             string     `gorm:"column:id;type:varchar(20);primaryKey"`
	UserID         string     `gorm:"column:user_id;type:varchar(20);not null;index:idx_daily_brief_issue_user_status"`
	BriefDate      string     `gorm:"column:brief_date;type:varchar(10);not null;uniqueIndex:uk_daily_brief_issue_user_date"`
	Status         string     `gorm:"column:status;type:varchar(16);not null;index:idx_daily_brief_issue_user_status"`
	Headline       string     `gorm:"column:headline;type:varchar(512)"`
	TopSummary     string     `gorm:"column:top_summary;type:text"`
	SectionsJSON   string     `gorm:"column:sections_json;type:jsonb;not null"`
	ItemCount      int        `gorm:"column:item_count;not null"`
	PublishedRunID string     `gorm:"column:published_run_id;type:varchar(20)"`
	GeneratedAt    *time.Time `gorm:"column:generated_at"`
	PublishedAt    *time.Time `gorm:"column:published_at"`
	CreateTime     time.Time  `gorm:"column:create_time;not null"`
	UpdateTime     time.Time  `gorm:"column:update_time;not null"`
}

func (IssueModel) TableName() string {
	return "t_daily_brief_issue"
}
