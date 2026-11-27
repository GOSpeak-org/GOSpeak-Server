package repository

import (
	"log"

	"GOSpeak/internal/model"

	"gorm.io/gorm"
)

type GroupInput struct {
	UUID      string   `json:"uuid"`
	Name      string   `json:"name"`
	SortOrder int      `json:"sort_order"`
	RoomUUIDs []string `json:"room_uuids"`
}

type RoomLayoutRepository struct {
	db *gorm.DB
}

func NewRoomLayoutRepository(db *gorm.DB) *RoomLayoutRepository {
	return &RoomLayoutRepository{db: db}
}

func (r *RoomLayoutRepository) ListGroups(domainUUID string) ([]model.DomainRoomGroup, error) {
	var groups []model.DomainRoomGroup
	err := r.db.Where("domain_uuid = ?", domainUUID).Order("sort_order ASC, id ASC").Find(&groups).Error
	return groups, err
}

func (r *RoomLayoutRepository) GetLayout(domainUUID string) (model.DomainRoomLayout, error) {
	var layout model.DomainRoomLayout
	err := r.db.Where("domain_uuid = ?", domainUUID).First(&layout).Error
	if err == gorm.ErrRecordNotFound {
		groups, listErr := r.ListGroups(domainUUID)
		if listErr != nil {
			log.Printf("[RoomLayout] list groups for %s failed while building default layout: %v", domainUUID, listErr)
		}
		return model.DomainRoomLayout{DomainUUID: domainUUID, UngroupedPosition: len(groups)}, nil
	}
	return layout, err
}

func (r *RoomLayoutRepository) Groups(domainUUID string) ([]model.DomainRoomGroup, error) {
	return r.ListGroups(domainUUID)
}

func (r *RoomLayoutRepository) Layout(domainUUID string) (model.DomainRoomLayout, error) {
	return r.GetLayout(domainUUID)
}

func (r *RoomLayoutRepository) DomainRooms(domainUUID string) ([]model.Room, error) {
	var rooms []model.Room
	err := r.db.Where("domain_uuid = ?", domainUUID).Order("id ASC").Find(&rooms).Error
	return rooms, err
}

func (r *RoomLayoutRepository) SaveLayout(domainUUID string, groups []GroupInput, ungroupedUUIDs []string, ungroupedPosition int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing []model.DomainRoomGroup
		if err := tx.Where("domain_uuid = ?", domainUUID).Find(&existing).Error; err != nil {
			return err
		}
		keep := make(map[string]bool, len(groups))
		for _, g := range groups {
			if g.UUID != "" {
				keep[g.UUID] = true
			}
		}
		for _, old := range existing {
			if keep[old.UUID] {
				continue
			}
			if err := tx.Model(&model.Room{}).
				Where("domain_uuid = ? AND group_uuid = ?", domainUUID, old.UUID).
				Updates(map[string]interface{}{"group_uuid": "", "sort_order": 0}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}

		byUUID := make(map[string]model.DomainRoomGroup, len(existing))
		for _, g := range existing {
			byUUID[g.UUID] = g
		}
		for _, g := range groups {
			row := byUUID[g.UUID]
			row.DomainUUID = domainUUID
			row.Name = g.Name
			row.SortOrder = g.SortOrder
			if row.UUID == "" {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&row).Error; err != nil {
				return err
			}
			for i, roomUUID := range g.RoomUUIDs {
				if err := tx.Model(&model.Room{}).
					Where("domain_uuid = ? AND uuid = ?", domainUUID, roomUUID).
					Updates(map[string]interface{}{"group_uuid": row.UUID, "sort_order": i}).Error; err != nil {
					return err
				}
			}
		}
		for i, roomUUID := range ungroupedUUIDs {
			if err := tx.Model(&model.Room{}).
				Where("domain_uuid = ? AND uuid = ?", domainUUID, roomUUID).
				Updates(map[string]interface{}{"group_uuid": "", "sort_order": i}).Error; err != nil {
				return err
			}
		}

		layout := model.DomainRoomLayout{DomainUUID: domainUUID, UngroupedPosition: ungroupedPosition}
		return tx.Save(&layout).Error
	})
}
