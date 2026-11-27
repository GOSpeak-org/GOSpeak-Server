package handler

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/service"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type domainLayoutResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Groups []service.LayoutGroup `json:"groups"`
		Items  []service.LayoutItem  `json:"items"`
	} `json:"data"`
}

func setupDomainLayoutHandler(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:layout_handler_%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatalf("db pool: %v", err)
	}
	pool.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.UserDomainGroup{}, &model.UserDomainLayoutItem{}, &model.DomainMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	h := NewDomainLayoutHandler(service.NewDomainLayoutService(repository.NewUserDomainLayoutRepository(db)))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_uuid", c.GetHeader("X-User-UUID"))
		c.Next()
	})
	r.POST("/api/v1/domain/layout/get", h.Get)
	r.POST("/api/v1/domain/layout/save", h.Save)
	return r, db
}

func postDomainLayout(t *testing.T, router *gin.Engine, path, body, userUUID string) domainLayoutResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userUUID != "" {
		req.Header.Set("X-User-UUID", userUUID)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp domainLayoutResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response %q: %v", w.Body.String(), err)
	}
	return resp
}

func TestDomainLayoutHandler_AuthAndBinding(t *testing.T) {
	router, _ := setupDomainLayoutHandler(t)

	resp := postDomainLayout(t, router, "/api/v1/domain/layout/get", "{}", "")
	if resp.Code != 2001 {
		t.Fatalf("unauthenticated get should be INVALID_PARAMS, got %d (%s)", resp.Code, resp.Msg)
	}
	resp = postDomainLayout(t, router, "/api/v1/domain/layout/save", "{}", "")
	if resp.Code != 2001 {
		t.Fatalf("unauthenticated save should be INVALID_PARAMS, got %d (%s)", resp.Code, resp.Msg)
	}
	resp = postDomainLayout(t, router, "/api/v1/domain/layout/save", "{invalid", "user-a")
	if resp.Code != 2001 {
		t.Fatalf("invalid JSON should be INVALID_PARAMS, got %d (%s)", resp.Code, resp.Msg)
	}
}

func TestDomainLayoutHandler_AcceptsNullGroupUUID(t *testing.T) {
	router, db := setupDomainLayoutHandler(t)
	if err := db.Create(&model.DomainMember{DomainUUID: "d1", UserUUID: "user-a"}).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
	resp := postDomainLayout(t, router, "/api/v1/domain/layout/save",
		`{"groups":[],"items":[{"domain_uuid":"d1","group_uuid":null,"sort_order":0}]}`, "user-a")
	if resp.Code != 0 {
		t.Fatalf("save with null group_uuid failed: %d (%s)", resp.Code, resp.Msg)
	}
	if len(resp.Data.Items) != 1 || resp.Data.Items[0].DomainUUID != "d1" || resp.Data.Items[0].GroupUUID != "" {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}
}

func TestDomainLayoutHandler_UserIsolation(t *testing.T) {
	router, db := setupDomainLayoutHandler(t)
	for _, member := range []model.DomainMember{
		{DomainUUID: "d1", UserUUID: "user-a"},
		{DomainUUID: "d2", UserUUID: "user-b"},
	} {
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create member: %v", err)
		}
	}

	body := `{"groups":[{"uuid":"g1","name":"Group A","color":"lime","sort_order":0}],"items":[{"domain_uuid":"d1","group_uuid":"g1","sort_order":0}]}`
	saved := postDomainLayout(t, router, "/api/v1/domain/layout/save", body, "user-a")
	if saved.Code != 0 {
		t.Fatalf("save failed: %d (%s)", saved.Code, saved.Msg)
	}
	if len(saved.Data.Groups) != 1 || saved.Data.Groups[0].UUID != "g1" || len(saved.Data.Items) != 1 || saved.Data.Items[0].DomainUUID != "d1" {
		t.Fatalf("unexpected saved layout: %+v", saved.Data)
	}

	gotA := postDomainLayout(t, router, "/api/v1/domain/layout/get", "{}", "user-a")
	if gotA.Code != 0 || len(gotA.Data.Groups) != 1 || gotA.Data.Groups[0].Name != "Group A" {
		t.Fatalf("get user-a failed: %+v", gotA)
	}
	gotB := postDomainLayout(t, router, "/api/v1/domain/layout/get", "{}", "user-b")
	if gotB.Code != 0 || len(gotB.Data.Groups) != 0 || len(gotB.Data.Items) != 1 || gotB.Data.Items[0].DomainUUID != "d2" {
		t.Fatalf("user-b should only see its own joined domain: %+v", gotB)
	}

	crossUser := postDomainLayout(t, router, "/api/v1/domain/layout/save",
		`{"groups":[],"items":[{"domain_uuid":"d2","group_uuid":"g1"}]}`, "user-b")
	if crossUser.Code != 2001 {
		t.Fatalf("user-b must not reference user-a's group, got %d (%s)", crossUser.Code, crossUser.Msg)
	}
}
