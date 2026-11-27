package plugin

import "testing"

func TestManagerTracksAPICalls(t *testing.T) {
	m := NewManager(t.TempDir(), nil)
	m.RecordAPI(true)
	m.RecordAPI(false)
	s := m.PluginStats()
	if s.APIRequests != 2 || s.APIErrors != 1 {
		t.Fatalf("unexpected API counters: %+v", s)
	}
}
