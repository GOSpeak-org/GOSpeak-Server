package repository

import (
	"GOSpeak/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserDomainLayoutRepository struct {
	db *gorm.DB
}

func NewUserDomainLayoutRepository(db *gorm.DB) *UserDomainLayoutRepository {
	return &UserDomainLayoutRepository{db: db}
}

func (r *UserDomainLayoutRepository) ListGroups(userUUID string) ([]model.UserDomainGroup, error) {
	var groups []model.UserDomainGroup
	err := r.db.Where("user_uuid = ?", userUUID).Order("sort_order ASC").Order("uuid ASC").Find(&groups).Error
	return groups, err
}

func (r *UserDomainLayoutRepository) ListItems(userUUID string) ([]model.UserDomainLayoutItem, error) {
	var items []model.UserDomainLayoutItem
	err := r.db.Where("user_uuid = ?", userUUID).Order("group_uuid ASC").Order("sort_order ASC").Order("domain_uuid ASC").Find(&items).Error
	return items, err
}

func (r *UserDomainLayoutRepository) GroupsByUUIDs(uuids []string) ([]model.UserDomainGroup, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	var groups []model.UserDomainGroup
	err := r.db.Where("uuid IN ?", uuids).Find(&groups).Error
	return groups, err
}

func (r *UserDomainLayoutRepository) JoinedDomainUUIDs(userUUID string) ([]string, error) {
	var domainUUIDs []string
	err := r.db.Model(&model.DomainMember{}).Where("user_uuid = ?", userUUID).Pluck("domain_uuid", &domainUUIDs).Error
	return domainUUIDs, err
}

// Replace 在单事务内以整份布局覆盖用户的旧数据：按 uuid  upsert 分组、删除本次未出现的分组，
// 再清空并重建该用户的布局条目。user_uuid 始终由调用方强制指定，避免越权写入。
func (r *UserDomainLayoutRepository) Replace(userUUID string, groups []model.UserDomainGroup, items []model.UserDomainLayoutItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		keep := make([]string, 0, len(groups))
		for i := range groups {
			group := groups[i]
			group.UserUUID = userUUID
			keep = append(keep, group.UUID)
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "uuid"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "color", "sort_order", "updated_at"}),
			}).Create(&group).Error; err != nil {
				return err
			}
		}

		deleteGroups := tx.Where("user_uuid = ?", userUUID)
		if len(keep) > 0 {
			deleteGroups = deleteGroups.Where("uuid NOT IN ?", keep)
		}
		if err := deleteGroups.Delete(&model.UserDomainGroup{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_uuid = ?", userUUID).Delete(&model.UserDomainLayoutItem{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		rows := make([]model.UserDomainLayoutItem, 0, len(items))
		for _, item := range items {
			item.UserUUID = userUUID
			rows = append(rows, item)
		}
		return tx.Create(&rows).Error
	})
}
