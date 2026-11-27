package model

// PluginKV 为插件提供通用键值存储，值为 JSON 文本。
type PluginKV struct {
	PluginID  string `gorm:"primaryKey;size:128" json:"plugin_id"`
	Key       string `gorm:"primaryKey;size:255" json:"key"`
	ValueJSON string `gorm:"type:text" json:"value_json"`
}

func (PluginKV) TableName() string { return "plugin_kvs" }
