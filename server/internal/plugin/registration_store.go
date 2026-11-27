package plugin

import "GOSpeak/internal/model"

type RegistrationStore interface {
	Upsert(*model.PluginRegistration) error
	SetEnabled(pluginID string, enabled bool) error
	Delete(pluginID string) error
}
