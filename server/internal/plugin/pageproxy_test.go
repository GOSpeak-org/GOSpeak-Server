package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
)

func newProxyTestEngine(t *testing.T, pp *PageProxy) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pp.SetFallback(func(c *gin.Context) {
		c.String(http.StatusOK, "SPA-FALLBACK")
	})
	pp.Register(r)
	return r
}

func TestPageProxyWildcardBeforeSPA(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "sid=x; Domain=evil.example; Path=/")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("PLUGIN-PAGE:" + r.URL.Path))
	}))
	defer upstream.Close()

	pp := NewPageProxy()
	pp.SetAllowLoopbackForTest(true)
	require.NoError(t, pp.Confirm("com.example.panel", "spa", "/plugins/com.example.panel/", upstream.URL))
	r := newProxyTestEngine(t, pp)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/com.example.panel/dashboard?x=1", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "PLUGIN-PAGE:")
	require.NotContains(t, rec.Body.String(), "SPA-FALLBACK")
	require.NotContains(t, rec.Header().Get("Set-Cookie"), "Domain=")
}

func TestPageProxyRejectsInvalidPrefix(t *testing.T) {
	pp := NewPageProxy()
	err := pp.Confirm("com.example.panel", "spa", "/admin/com.example.panel/", "http://127.0.0.1:1")
	require.Error(t, err)
	err = pp.Confirm("com.example.panel", "spa", "/plugins/other/", "http://127.0.0.1:1")
	require.Error(t, err)
}

func TestPageProxyRejectsPrivateEntryURL(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8998", "http://[::1]:8998", "http://localhost:3000", "http://10.0.0.5", "http://192.168.1.1", "http://169.254.1.1"} {
		pp := NewPageProxy()
		err := pp.Confirm("com.example.panel", "spa", "/plugins/com.example.panel/", raw)
		require.Error(t, err, raw)
		require.Contains(t, err.Error(), "not publicly routable", raw)
	}
}

func TestPageProxyUnconfirmedNotMounted(t *testing.T) {
	pp := NewPageProxy()
	r := newProxyTestEngine(t, pp)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/com.example.panel/dashboard", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "SPA-FALLBACK", rec.Body.String())
}

func TestPageProxyUnknownPluginNotProxied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SHOULD-NOT-BE-REACHED"))
	}))
	defer upstream.Close()
	pp := NewPageProxy()
	pp.SetAllowLoopbackForTest(true)
	require.NoError(t, pp.Confirm("com.example.panel", "spa", "/plugins/com.example.panel/", upstream.URL))
	r := newProxyTestEngine(t, pp)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins/com.example.other/dashboard", nil))
	require.Equal(t, "SPA-FALLBACK", rec.Body.String())
	require.NotContains(t, rec.Body.String(), "SHOULD-NOT-BE-REACHED")
}

func TestBrowserProxyJWTAndRoutePrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]string{"user_uuid": r.Header.Get("X-Gospeak-User-UUID")})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	pp := NewPageProxy()
	pp.SetAllowLoopbackForTest(true)
	require.NoError(t, pp.Confirm("com.example.panel", "spa", "/plugins/com.example.panel/", upstream.URL))
	bp := NewBrowserProxy(pp, nil)

	r := gin.New()
	r.POST("/api/v1/plugin/browser/call", bp.Call)
	r.POST("/api/v1/plugin/browser/refresh", bp.Refresh)

	body := strings.NewReader(`{"plugin_id":"com.example.panel","route_path":"/plugins/com.example.panel/dashboard","kind":"call"}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/plugin/browser/call", body))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	r2 := gin.New()
	bpAuth := NewBrowserProxy(pp, func(*gin.Context) string { return "user-uuid-1" })
	r2.Use(func(c *gin.Context) { bpAuth.auth(c); c.Next() })
	r2.POST("/api/v1/plugin/browser/call", bpAuth.Call)
	rec2 := httptest.NewRecorder()
	r2.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/plugin/browser/call", strings.NewReader(`{"plugin_id":"com.example.panel","route_path":"/plugins/evil/dashboard"}`)))
	require.Equal(t, http.StatusForbidden, rec2.Code)
}

func TestBrowserProxyWireWithJWTAndOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seen := make(chan string, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("X-Gospeak-User-UUID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"ok\":true}"))
	}))
	defer upstream.Close()

	pp := NewPageProxy()
	pp.SetAllowLoopbackForTest(true)
	require.NoError(t, pp.Confirm("com.example.panel", "spa", "/plugins/com.example.panel/", upstream.URL))
	bp := NewBrowserProxy(pp, middleware.CurrentUserUUID)

	r := gin.New()
	r.POST("/api/v1/plugin/browser/call", middleware.JWTAuth(), bp.Call)

	token, err := pkg.GenerateToken("alice", "Alice", "jwt-user-uuid", "user", 1)
	require.NoError(t, err)
	payload := `{"plugin_id":"com.example.panel","route_path":"/plugins/com.example.panel/dashboard","kind":"call"}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugin/browser/call", strings.NewReader(payload))
	req.Host = "gospeak.example.com"
	req.AddCookie(&http.Cookie{Name: "gospeak_token", Value: token})
	req.Header.Set("Origin", "https://gospeak.example.com")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "jwt-user-uuid", <-seen)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/plugin/browser/call", strings.NewReader(payload))
	req.Host = "gospeak.example.com"
	req.AddCookie(&http.Cookie{Name: "gospeak_token", Value: token})
	req.Header.Set("Origin", "https://evil.example")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	select {
	case v := <-seen:
		t.Fatalf("cross-site request must not reach upstream, got user=%s", v)
	default:
	}
}

func TestBrowserProxyRejectsForgedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pp := NewPageProxy()
	bp := NewBrowserProxy(pp, func(*gin.Context) string { return "user-uuid-1" })
	r := gin.New()
	r.Use(func(c *gin.Context) { bp.auth(c); c.Next() })
	r.POST("/api/v1/plugin/browser/call", bp.Call)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugin/browser/call", strings.NewReader(`{"plugin_id":"com.example.panel","route_path":"/plugins/com.example.panel/dashboard"}`))
	req.Header.Set("Origin", "https://evil.example")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
