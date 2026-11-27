package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DomainRoomGroup struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UUID       string    `gorm:"type:uuid;uniqueIndex" json:"uuid"`
	DomainUUID string    `gorm:"size:32;not null;uniqueIndex:idx_domain_room_group_name,priority:1" json:"domain_uuid"`
	Name       string    `gorm:"size:64;not null;uniqueIndex:idx_domain_room_group_name,priority:2" json:"name"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (g *DomainRoomGroup) BeforeCreate(_ *gorm.DB) error {
	if g.UUID == "" {
		g.UUID = uuid.New().String()
	}
	return nil
}

func (DomainRoomGroup) TableName() string { return "domain_room_groups" }
