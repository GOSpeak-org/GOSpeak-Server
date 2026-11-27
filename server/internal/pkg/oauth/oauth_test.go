package oauth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient_TimeoutAndBodyLimit(t *testing.T) {
	t.Run("body limit", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(bytes.Repeat([]byte("x"), 3*1024*1024))
		}))
		defer ts.Close()

		if _, err := httpGet(ts.URL, ""); err == nil {
			t.Fatal("expected body too large error")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		}))
		defer ts.Close()

		prev := oauthHTTPClient
		oauthHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
		t.Cleanup(func() { oauthHTTPClient = prev })

		if _, err := httpGet(ts.URL, ""); err == nil {
			t.Fatal("expected timeout error")
		}
	})
}

func TestHTTPClient_RejectsUnsafeURL(t *testing.T) {
	if _, err := httpGet("file:///etc/passwd", ""); err == nil {
		t.Fatal("expected unsafe scheme error")
	}
}
