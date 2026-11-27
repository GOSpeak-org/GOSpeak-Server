package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func securityHeadersRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/normal", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/swagger/index.html", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestSecurityHeaders_Global(t *testing.T) {
	r := securityHeadersRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/normal", nil))

	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("missing Content-Security-Policy")
	}
	// 现有功能依赖的来源必须放行，否则头像裁切/背景/发言检测会坏。
	for _, need := range []string{"blob:", "worker-src", "wss:", "https:", "http:", "unsafe-inline"} {
		if !strings.Contains(csp, need) {
			t.Errorf("CSP missing %q: %s", need, csp)
		}
	}
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("global CSP must not allow unsafe-eval: %s", csp)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got == "" {
		t.Error("missing Referrer-Policy")
	}
}

func TestSecurityHeaders_SwaggerRelaxed(t *testing.T) {
	r := securityHeadersRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "unsafe-eval") {
		t.Errorf("swagger CSP should allow unsafe-eval: %s", csp)
	}
}

func TestSecurityHeaders_OAuthOverrideWins(t *testing.T) {
	// OAuth 回调页在 handler 内 c.Header 覆盖全局 CSP（Set 语义，后写生效）。
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/oauth/callback", func(c *gin.Context) {
		c.Header("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'")
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oauth/callback", nil))

	csp := w.Header().Get("Content-Security-Policy")
	if csp != "default-src 'none'; script-src 'unsafe-inline'" {
		t.Errorf("OAuth override lost: %s", csp)
	}
}
