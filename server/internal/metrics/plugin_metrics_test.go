package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPluginMetricsExposeSnapshotValues(t *testing.T) {
	srv := New(func() Snapshot {
		return Snapshot{
			PluginRegistered: 2,
			PluginActive:     1,
			PluginRestarts:   3,
			PluginAPICalls:   9,
			PluginAPIErrors:  4,
		}
	})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, metric := range []string{
		"gospeak_plugin_installed 2",
		"gospeak_plugin_active 1",
		"gospeak_plugin_restarts 3",
		"gospeak_plugin_api_calls 9",
		"gospeak_plugin_api_errors 4",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics missing %q in body: %s", metric, body)
		}
	}
}
