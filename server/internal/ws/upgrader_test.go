package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	nhooyrws "nhooyr.io/websocket"

	"GOSpeak/internal/pkg"
)

func TestAccessTokenFromCookie(t *testing.T) {
	upgrader := NewUpgrader(UpgraderConfig{})
	r := httptest.NewRequest("GET", "/ws", nil)
	r.AddCookie(&http.Cookie{Name: "gospeak_token", Value: "access-token-123"})

	if got := upgrader.accessTokenFromCookie(r); got != "access-token-123" {
		t.Fatalf("expected cookie access token, got %q", got)
	}
}

func TestAccessTokenFromCookie_CustomName(t *testing.T) {
	upgrader := NewUpgrader(UpgraderConfig{AuthCookieName: "gospeak_auth"})
	r := httptest.NewRequest("GET", "/ws", nil)
	r.AddCookie(&http.Cookie{Name: "gospeak_auth", Value: "access-token-456"})

	if got := upgrader.accessTokenFromCookie(r); got != "access-token-456" {
		t.Fatalf("expected custom cookie access token, got %q", got)
	}
}

func TestAccessTokenFromCookie_Empty(t *testing.T) {
	upgrader := NewUpgrader(UpgraderConfig{})
	r := httptest.NewRequest("GET", "/ws", nil)

	if got := upgrader.accessTokenFromCookie(r); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}

func TestAccessTokenFromCookie_RefreshCookieIgnored(t *testing.T) {
	upgrader := NewUpgrader(UpgraderConfig{})
	r := httptest.NewRequest("GET", "/ws", nil)
	r.AddCookie(&http.Cookie{Name: "gospeak_refresh_token", Value: "refresh-token"})

	if got := upgrader.accessTokenFromCookie(r); got != "" {
		t.Fatalf("refresh cookie must not be used for WS auth, got %q", got)
	}
}

func TestUpgrader_ServeHTTP_RejectsMissingCookie(t *testing.T) {
	upgrader := NewUpgrader(UpgraderConfig{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)

	upgrader.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie, got %d", w.Code)
	}
}

func TestUpgrader_ServeHTTP_RejectsHeaderAccessToken(t *testing.T) {
	access, err := pkg.GenerateToken("alice", "Alice", "uuid-alice", "user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	upgrader := NewUpgrader(UpgraderConfig{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Bearer "+access)

	upgrader.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for header access token, got %d", w.Code)
	}
}

func TestUpgrader_ServeHTTP_RejectsSubprotocolAccessToken(t *testing.T) {
	access, err := pkg.GenerateToken("alice", "Alice", "uuid-alice", "user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	upgrader := NewUpgrader(UpgraderConfig{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Sec-WebSocket-Protocol", "gospeak, "+access)

	upgrader.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for subprotocol access token, got %d", w.Code)
	}
}

func TestUpgrader_ServeHTTP_RejectsRefreshTokenCookie(t *testing.T) {
	refresh, err := pkg.GenerateRefreshToken("alice", "Alice", "uuid-alice", "user", 1)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	upgrader := NewUpgrader(UpgraderConfig{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.AddCookie(&http.Cookie{Name: "gospeak_token", Value: refresh})

	upgrader.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for refresh token cookie, got %d", w.Code)
	}
}

func TestUpgrader_OriginAllowed(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		origin  string
		want    bool
	}{
		{name: "no origin header", allowed: nil, origin: "", want: true},
		{name: "same origin default", allowed: nil, origin: "https://app.example.com", want: true},
		{name: "cross origin default", allowed: nil, origin: "https://evil.example.com", want: false},
		{name: "whitelisted origin", allowed: []string{"https://app.example.com"}, origin: "https://app.example.com", want: true},
		{name: "whitelisted origin trailing slash", allowed: []string{"https://app.example.com/"}, origin: "https://app.example.com", want: true},
		{name: "not whitelisted origin", allowed: []string{"https://app.example.com"}, origin: "https://evil.example.com", want: false},
		{name: "wildcard origin", allowed: []string{"*"}, origin: "https://evil.example.com", want: true},
		{name: "malformed origin", allowed: nil, origin: "http://[::1", want: false},
		{name: "null origin", allowed: nil, origin: "null", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upgrader := NewUpgrader(UpgraderConfig{AllowedOrigins: tc.allowed})
			r := httptest.NewRequest(http.MethodGet, "https://app.example.com/ws", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := upgrader.originAllowed(r); got != tc.want {
				t.Fatalf("originAllowed(origin=%q, allowed=%v) = %v, want %v", tc.origin, tc.allowed, got, tc.want)
			}
		})
	}
}

func TestUpgrader_ServeHTTP_ConnectPolicyAllows(t *testing.T) {
	token, err := pkg.GenerateToken("alice", "Alice", "uuid-alice", "user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	policyCalled := false
	u := NewUpgrader(UpgraderConfig{
		Fanout: NewFanout(),
		ConnectPolicy: func(claims *pkg.Claims) (pkg.ErrCode, string) {
			policyCalled = true
			if claims.UserUUID != "uuid-alice" {
				t.Errorf("ConnectPolicy received claims for %q", claims.UserUUID)
			}
			return pkg.SUCCESS, ""
		},
	})

	server := httptest.NewServer(http.HandlerFunc(u.ServeHTTP))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := nhooyrws.Dial(ctx, "ws://"+server.Listener.Addr().String()+"/ws", wsDialOptionsWithCookie(token))
	if err != nil {
		t.Fatalf("allowed ConnectPolicy must complete the upgrade: %v", err)
	}
	conn.CloseNow()

	if !policyCalled {
		t.Fatal("ConnectPolicy must run for allowed handshakes")
	}
}
