package repository

import (
	"GOSpeak/internal/model"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openRoomLayoutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Room{}, &model.DomainRoomGroup{}, &model.DomainRoomLayout{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRoomLayoutRepository_SaveLayout(t *testing.T) {
	db := openRoomLayoutTestDB(t)
	if err := db.Create([]model.Room{
		{Name: "a", DomainUUID: "domain-a", UUID: "room-a"},
		{Name: "b", DomainUUID: "domain-a", UUID: "room-b"},
		{Name: "c", DomainUUID: "domain-b", UUID: "room-c"},
	}).Error; err != nil {
		t.Fatalf("seed rooms: %v", err)
	}

	repo := NewRoomLayoutRepository(db)
	err := repo.SaveLayout("domain-a", []GroupInput{
		{UUID: "", Name: "常用", SortOrder: 0, RoomUUIDs: []string{"room-a"}},
	}, []string{"room-b"}, 1)
	if err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	groups, err := repo.ListGroups("domain-a")
	if err != nil || len(groups) != 1 || groups[0].Name != "常用" {
		t.Fatalf("groups: %+v err=%v", groups, err)
	}
	var gotA, gotB, gotC model.Room
	db.First(&gotA, "uuid = ?", "room-a")
	db.First(&gotB, "uuid = ?", "room-b")
	db.First(&gotC, "uuid = ?", "room-c")
	if gotA.GroupUUID != groups[0].UUID || gotA.SortOrder != 0 {
		t.Fatalf("room-a not assigned: %+v", gotA)
	}
	if gotB.GroupUUID != "" || gotB.SortOrder != 0 {
		t.Fatalf("room-b not ungrouped first: %+v", gotB)
	}
	if gotC.GroupUUID != "" || gotC.SortOrder != 0 {
		t.Fatalf("domain-b room touched: %+v", gotC)
	}
	layout, err := repo.GetLayout("domain-a")
	if err != nil || layout.UngroupedPosition != 1 {
		t.Fatalf("layout: %+v err=%v", layout, err)
	}
}
