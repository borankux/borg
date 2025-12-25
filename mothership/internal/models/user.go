package models

import (
	"time"
)

type User struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null;type:varchar(255)" json:"username"`
	PasswordHash string    `gorm:"not null;type:varchar(255)" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

