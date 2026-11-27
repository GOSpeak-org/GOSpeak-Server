package model

import "time"

// PluginRegistration 记录已安装插件的注册信息与持久化开关状态。
type PluginRegistration struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	PluginID           string    `gorm:"size:128;uniqueIndex" json:"plugin_id"`
	Source             string    `gorm:"size:32" json:"source"`
	Version            string    `gorm:"size:64" json:"version"`
	Enabled            bool      `gorm:"index" json:"enabled"`
	AllowedRoutePrefix string    `gorm:"size:255" json:"allowed_route_prefix"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (PluginRegistration) TableName() string { return "plugin_registrations" }

func NewPluginRegistration(pluginID, source, version string) PluginRegistration {
	return PluginRegistration{
		PluginID: pluginID,
		Source:   source,
		Version:  version,
		Enabled:  true,
	}
}
