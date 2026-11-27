package invite_join

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GOSpeak/internal/handler"

	"github.com/gin-gonic/gin"
)

func TestRoutes_Registered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pub := r.Group("/api/v1/invite")
	RegisterPublic(pub, nil)
	prot := r.Group("/api/v1/invite")
	RegisterProtected(prot, nil)
	admin := r.Group("/api/v1/domain")
	RegisterDomainAdmin(admin, nil)

	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/invite/register"},
		{http.MethodPost, "/api/v1/invite/join"},
		{http.MethodPost, "/api/v1/invite/my-request"},
		{http.MethodPost, "/api/v1/invite/cancel-request"},
		{http.MethodPost, "/api/v1/domain/join-requests/list"},
		{http.MethodPost, "/api/v1/domain/join-requests/review"},
		{http.MethodPost, "/api/v1/domain/join-approval/config"},
	}
	registered := map[string]bool{}
	for _, route := range r.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, want := range cases {
		if !registered[want.method+" "+want.path] {
			t.Fatalf("route %s %s is not registered", want.method, want.path)
		}
	}
}

func TestRoutes_RegisterIsRateLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewInviteJoinHandler(nil, nil)
	RegisterPublic(r.Group("/api/v1/invite"), h)
	// 畸形 JSON 在 handler 绑定阶段即返回，不会触及 nil service。
	for i := 0; i < 21; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/invite/register", strings.NewReader("not-json"))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.1.2.3:1234"
		r.ServeHTTP(rec, req)
		if i < 20 && rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d must pass rate limit, got 429", i+1)
		}
		if i == 20 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("21st request status = %d, want 429", rec.Code)
		}
	}
}
