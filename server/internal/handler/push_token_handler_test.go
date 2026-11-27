package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPushTokenHandlerEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:push_token_handler_%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.PushToken{}); err != nil {
		t.Fatal(err)
	}
	svc := service.NewPushTokenService(repository.NewPushTokenRepository(db))
	h := NewPushTokenHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		if uuid := c.GetHeader("X-User-UUID"); uuid != "" {
			c.Set("user_uuid", uuid)
		}
		c.Next()
	})
	r.POST("/api/v1/user/push-token", h.Register)
	return r, db
}

func postPushToken(t *testing.T, r *gin.Engine, userUUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/push-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userUUID != "" {
		req.Header.Set("X-User-UUID", userUUID)
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestPushTokenHandler_RegisterPersists(t *testing.T) {
	r, db := newPushTokenHandlerEnv(t)
	rec := postPushToken(t, r, "user-1", `{"token":"tok-1","platform":"android"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d; body=%s", resp.Code, rec.Body.String())
	}
	var row model.PushToken
	if err := db.Where("token = ?", "tok-1").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.UserUUID != "user-1" || row.Platform != "android" {
		t.Fatalf("got user=%q platform=%q", row.UserUUID, row.Platform)
	}
}

func TestPushTokenHandler_RegisterRejectsMissingFields(t *testing.T) {
	r, db := newPushTokenHandlerEnv(t)

	rec := postPushToken(t, r, "user-1", `{}`)
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 2001 {
		t.Fatalf("missing token code = %d, want 2001; body=%s", resp.Code, rec.Body.String())
	}

	rec = postPushToken(t, r, "", `{"token":"tok-x"}`)
	resp = struct {
		Code int `json:"code"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 1001 {
		t.Fatalf("missing identity code = %d, want 1001; body=%s", resp.Code, rec.Body.String())
	}
	var count int64
	if err := db.Model(&model.PushToken{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid requests must not persist rows, got %d", count)
	}
}
