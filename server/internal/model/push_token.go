package model

import "time"

type PushToken struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserUUID  string    `gorm:"index;size:64" json:"user_uuid"`
	Token     string    `gorm:"uniqueIndex;size:512" json:"token"`
	Platform  string    `gorm:"size:32" json:"platform"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PushToken) TableName() string {
	return "push_tokens"
}
