package repository

import (
	"GOSpeak/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PluginRegistrationStore struct {
	db *gorm.DB
}

func NewPluginRegistrationStore(db *gorm.DB) *PluginRegistrationStore {
	return &PluginRegistrationStore{db: db}
}

func (s *PluginRegistrationStore) Upsert(reg *model.PluginRegistration) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"source", "version", "enabled", "updated_at"}),
	}).Create(reg).Error
}

func (s *PluginRegistrationStore) SetEnabled(pluginID string, enabled bool) error {
	return s.db.Model(&model.PluginRegistration{}).
		Where("plugin_id = ?", pluginID).
		Update("enabled", enabled).Error
}

func (s *PluginRegistrationStore) Delete(pluginID string) error {
	return s.db.Where("plugin_id = ?", pluginID).Delete(&model.PluginRegistration{}).Error
}
