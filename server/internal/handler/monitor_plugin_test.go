package handler

import (
	"testing"

	"GOSpeak/internal/config"
)

type fakePluginStats struct{ stats PluginStats }

func (f fakePluginStats) PluginStats() PluginStats { return f.stats }

func TestMonitorCollectIncludesPluginStats(t *testing.T) {
	s := fakePluginStats{stats: PluginStats{
		Registered: 6, Active: 5, Restarts: 4, APIRequests: 3, APIErrors: 2,
	}}
	h := NewMonitorHandler(nil, &config.Config{}, nil)
	h.SetPluginStats(s)
	snap := h.Collect()
	if snap.PluginRegistered != 6 || snap.PluginActive != 5 || snap.PluginRestarts != 4 ||
		snap.PluginAPICalls != 3 || snap.PluginAPIErrors != 2 {
		t.Fatalf("collect did not include plugin stats: %+v", snap)
	}
}
