package server

import (
	"testing"

	"GOSpeak/internal/handler"
)

func TestToMetricsSnapshotCarriesPluginFields(t *testing.T) {
	got := toMetricsSnapshot(handler.HealthSnapshot{
		PluginRegistered: 5,
		PluginActive:     4,
		PluginRestarts:   3,
		PluginAPICalls:   2,
		PluginAPIErrors:  1,
	})
	if got.PluginRegistered != 5 || got.PluginActive != 4 || got.PluginRestarts != 3 ||
		got.PluginAPICalls != 2 || got.PluginAPIErrors != 1 {
		t.Fatalf("plugin fields not transferred: %+v", got)
	}
}
