package repository

import (
	"GOSpeak/internal/model"

	"gorm.io/gorm"
)

type UserGroupRepository struct {
	db *gorm.DB
}

func NewUserGroupRepository(db *gorm.DB) *UserGroupRepository {
	return &UserGroupRepository{db: db}
}

func (r *UserGroupRepository) ListByUser(userUUID string) ([]model.UserGroup, error) {
	var groups []model.UserGroup
	err := r.db.Where("user_uuid = ?", userUUID).Order("group_name ASC").Find(&groups).Error
	return groups, err
}

func (r *UserGroupRepository) GetByID(id uint) (*model.UserGroup, error) {
	var group model.UserGroup
	err := r.db.First(&group, id).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *UserGroupRepository) FindByName(userUUID, groupName string) (*model.UserGroup, error) {
	var group model.UserGroup
	err := r.db.Where("user_uuid = ? AND group_name = ?", userUUID, groupName).First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *UserGroupRepository) Create(group *model.UserGroup) error {
	return r.db.Create(group).Error
}

func (r *UserGroupRepository) Rename(id uint, groupName string) error {
	return r.db.Model(&model.UserGroup{}).Where("id = ?", id).
		Update("group_name", groupName).Error
}

func (r *UserGroupRepository) Delete(id uint) error {
	return r.db.Delete(&model.UserGroup{}, id).Error
}
