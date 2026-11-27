package oauth

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeOAuthURL(t *testing.T) {
	ok := []string{
		"https://example.com/userinfo",
		"http://localhost:8080/userinfo",
		"http://127.0.0.1:8080/userinfo",
	}
	for _, raw := range ok {
		if _, err := safeOAuthURL(raw); err != nil {
			t.Fatalf("safeOAuthURL(%q) unexpected error: %v", raw, err)
		}
	}
	bad := []string{
		"file:///etc/passwd",
		"http://example.com/userinfo",
		"http://169.254.169.254/latest/meta-data",
		"",
	}
	for _, raw := range bad {
		if _, err := safeOAuthURL(raw); err == nil {
			t.Fatalf("safeOAuthURL(%q) expected error", raw)
		}
	}
}

func TestBlockedIP(t *testing.T) {
	blocked := []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254", "0.0.0.0", "fe80::1"}
	for _, raw := range blocked {
		if !blockedIP(net.ParseIP(raw)) {
			t.Fatalf("blockedIP(%q) should be blocked", raw)
		}
	}
	allowed := []string{"8.8.8.8", "192.168.1.10", "127.0.0.1"}
	for _, raw := range allowed {
		if blockedIP(net.ParseIP(raw)) {
			t.Fatalf("blockedIP(%q) should be allowed", raw)
		}
	}
}

func TestGenericProvider_RejectsMissingUID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"username":"alice"}`))
	}))
	defer ts.Close()

	p := NewGenericProvider("custom", &ProviderConfig{UserInfoURL: ts.URL}, FieldMapping{UIDField: "id"})
	if _, err := p.GetUserInfo("token"); err == nil {
		t.Fatal("expected missing uid error")
	}
}
