package model

import (
	"database/sql"
	"time"
)

// UserDomainGroup 是用户自定义的域分组，属于个人 UI 状态。
type UserDomainGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UUID      string    `gorm:"size:36;uniqueIndex;not null" json:"uuid"`
	UserUUID  string    `gorm:"type:uuid;index;not null" json:"user_uuid"`
	Name      string    `gorm:"size:32;not null" json:"name"`
	Color     string    `gorm:"size:16;not null" json:"color"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserDomainGroup) TableName() string {
	return "user_domain_groups"
}

// UserDomainLayoutItem 记录单个域在个人布局中的位置，group_uuid 为空表示未分组。
type UserDomainLayoutItem struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	UserUUID   string         `gorm:"type:uuid;index;uniqueIndex:idx_user_domain_layout_item,priority:1;not null" json:"user_uuid"`
	DomainUUID string         `gorm:"size:32;uniqueIndex:idx_user_domain_layout_item,priority:2;not null" json:"domain_uuid"`
	GroupUUID  sql.NullString `gorm:"size:36;index" json:"group_uuid"`
	SortOrder  int            `gorm:"not null;default:0" json:"sort_order"`
}

func (UserDomainLayoutItem) TableName() string {
	return "user_domain_layout_items"
}
