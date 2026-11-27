package model

import "time"

// PluginConfig 保存插件配置 JSON，由管理 API 写入、插件按需拉取。
type PluginConfig struct {
	PluginID   string    `gorm:"primaryKey;size:128" json:"plugin_id"`
	ConfigJSON string    `gorm:"type:text" json:"config_json"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (PluginConfig) TableName() string { return "plugin_configs" }
