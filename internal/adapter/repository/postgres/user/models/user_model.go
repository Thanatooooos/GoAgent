package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type UserModel struct {
	ID           string                `gorm:"column:id;type:varchar(20);primaryKey"`
	Username     string                `gorm:"column:username;type:varchar(32);not null"`
	PasswordHash string                `gorm:"column:password_hash;type:varchar(128);not null"`
	Role         string                `gorm:"column:role;type:varchar(16);not null"`
	Avatar       string                `gorm:"column:avatar;type:varchar(1024)"`
	CreatedBy    string                `gorm:"column:created_by;type:varchar(32);not null"`
	UpdatedBy    string                `gorm:"column:updated_by;type:varchar(32)"`
	CreateTime   time.Time             `gorm:"column:create_time;not null"`
	UpdateTime   time.Time             `gorm:"column:update_time;not null"`
	Deleted      soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (UserModel) TableName() string {
	return "t_user"
}
