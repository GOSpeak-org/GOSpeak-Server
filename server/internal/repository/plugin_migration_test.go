package repository

import (
	"reflect"
	"testing"

	"GOSpeak/internal/model"
)

func TestPluginModelsRegisteredInMigration(t *testing.T) {
	want := []reflect.Type{
		reflect.TypeOf(&model.PluginRegistration{}),
		reflect.TypeOf(&model.PluginConfig{}),
		reflect.TypeOf(&model.PluginKV{}),
		reflect.TypeOf(&model.PluginRequestLog{}),
	}
	registered := map[reflect.Type]bool{}
	for _, m := range migrationModels() {
		registered[reflect.TypeOf(m)] = true
	}
	for _, typ := range want {
		if !registered[typ] {
			t.Fatalf("%s not registered in AutoMigrate", typ)
		}
	}
}
