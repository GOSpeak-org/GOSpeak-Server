package model

import "time"

// PluginRequestLog 记录插件写请求的 request_id，用于幂等去重。
type PluginRequestLog struct {
	PluginID  string    `gorm:"uniqueIndex:idx_plugin_request_log_plugin_request;size:128" json:"plugin_id"`
	RequestID string    `gorm:"uniqueIndex:idx_plugin_request_log_plugin_request;size:128" json:"request_id"`
	Status    string    `gorm:"size:16;default:pending" json:"status"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (PluginRequestLog) TableName() string { return "plugin_request_logs" }
