package service

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/repository"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRoomLayoutService_GetAndSave(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{}); err != nil {
		t.Fatal(err)
	}
	db.Create([]model.Room{
		{Name: "a", DomainUUID: "domain-a", UUID: "room-a"},
		{Name: "b", DomainUUID: "domain-a", UUID: "room-b"},
	})
	svc := NewRoomLayoutService(repository.NewRoomLayoutRepository(db))

	view, appErr := svc.GetLayout("domain-a", true)
	if appErr != nil || view.UngroupedPosition != 0 || len(view.Ungrouped) != 2 {
		t.Fatalf("initial view: %+v err=%v", view, appErr)
	}
	if _, appErr = svc.SaveLayout("domain-a", []RoomGroupInput{
		{Name: "语音", RoomUUIDs: []string{"room-a"}},
	}, []string{"room-b"}, 1); appErr != nil {
		t.Fatalf("SaveLayout: %v", appErr)
	}
	view, appErr = svc.GetLayout("domain-a", false)
	if appErr != nil {
		t.Fatal(appErr)
	}
	if len(view.Groups) != 1 || view.Groups[0].Name != "语音" || len(view.Groups[0].Rooms) != 1 {
		t.Fatalf("groups: %+v", view.Groups)
	}
	if len(view.Ungrouped) != 1 || view.Ungrouped[0].UUID != "room-b" {
		t.Fatalf("ungrouped: %+v", view.Ungrouped)
	}
	if view.UngroupedPosition != 1 || view.CanManage {
		t.Fatalf("meta: pos=%d canManage=%v", view.UngroupedPosition, view.CanManage)
	}
}

func TestRoomLayoutService_RejectsForeignRoom(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{})
	db.Create(&model.Room{Name: "x", DomainUUID: "domain-b", UUID: "room-x"})
	svc := NewRoomLayoutService(repository.NewRoomLayoutRepository(db))
	if _, appErr := svc.SaveLayout("domain-a", nil, []string{"room-x"}, 0); appErr == nil {
		t.Fatal("expected error for foreign room")
	}
}
