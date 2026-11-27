package handler

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"
	"GOSpeak/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRoomLayoutHandler_Get(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{})
	db.Create(&model.Room{Name: "a", DomainUUID: "domain-a", UUID: "room-a"})
	h := NewRoomLayoutHandler(service.NewRoomLayoutService(repository.NewRoomLayoutRepository(db)), nil, nil, nil)

	r := gin.New()
	r.POST("/get", h.Get)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/get", strings.NewReader(`{"domain_uuid":"domain-a"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "room-a") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
