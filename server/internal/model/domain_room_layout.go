package model

import "time"

type DomainRoomLayout struct {
	DomainUUID        string    `gorm:"size:32;primaryKey" json:"domain_uuid"`
	UngroupedPosition int       `gorm:"not null;default:0" json:"ungrouped_position"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (DomainRoomLayout) TableName() string { return "domain_room_layouts" }
