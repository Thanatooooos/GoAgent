package models

import "time"

type ItemModel struct {
	ID           string     `gorm:"column:id;type:varchar(20);primaryKey"`
	IssueID      string     `gorm:"column:issue_id;type:varchar(20);not null;index:idx_daily_brief_item_issue_section_rank"`
	SectionKey   string     `gorm:"column:section_key;type:varchar(64);not null;index:idx_daily_brief_item_issue_section_rank"`
	Rank         int        `gorm:"column:rank;not null;index:idx_daily_brief_item_issue_section_rank"`
	Title        string     `gorm:"column:title;type:varchar(512);not null"`
	Summary      string     `gorm:"column:summary;type:text"`
	WhyItMatters string     `gorm:"column:why_it_matters;type:text"`
	URL          string     `gorm:"column:url;type:varchar(2048)"`
	Source       string     `gorm:"column:source;type:varchar(64)"`
	Topic        string     `gorm:"column:topic;type:varchar(64)"`
	PublishedAt  *time.Time `gorm:"column:published_at"`
	MetadataJSON string     `gorm:"column:metadata_json;type:jsonb;not null"`
	CreateTime   time.Time  `gorm:"column:create_time;not null"`
	UpdateTime   time.Time  `gorm:"column:update_time;not null"`
}

func (ItemModel) TableName() string {
	return "t_daily_brief_item"
}
