package models

import "time"

type UserSessionModel struct {
	Token      string    `gorm:"column:token;type:varchar(64);primaryKey"`
	UserID     string    `gorm:"column:user_id;type:varchar(20);not null"`
	ExpireTime time.Time `gorm:"column:expire_time;not null"`
	CreateTime time.Time `gorm:"column:create_time;not null"`
	UpdateTime time.Time `gorm:"column:update_time;not null"`
}

func (UserSessionModel) TableName() string {
	return "t_user_session"
}
