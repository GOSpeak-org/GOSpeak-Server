package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
)

func TestRoomService_LockDomainIsScoped(t *testing.T) {
	svc := NewRoomService(nil)
	unlockA := svc.lockDomain("dom-a")

	done := make(chan struct{})
	go func() {
		unlockB := svc.lockDomain("dom-b")
		unlockB()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("different domains share the same lock")
	}
	unlockA()
}

func setupRoomServiceTestDB(t *testing.T) (*RoomService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Room{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRoomService(repository.NewRoomRepository(db)), db
}

func seedRoomServiceRooms(t *testing.T, db *gorm.DB, domainUUID, roomType, prefix string, count int) []model.Room {
	t.Helper()
	base := time.Unix(1700000000, 0)
	rooms := make([]model.Room, 0, count)
	for i := 0; i < count; i++ {
		rooms = append(rooms, model.Room{
			Name:       fmt.Sprintf("%s-%03d", prefix, i),
			DomainUUID: domainUUID,
			Type:       roomType,
			CreatedAt:  base.Add(time.Duration(i) * time.Minute),
		})
	}
	if err := db.Create(&rooms).Error; err != nil {
		t.Fatalf("seed rooms: %v", err)
	}
	return rooms
}

func assertRoomServiceErrCode(t *testing.T, err error, code pkg.ErrCode) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *pkg.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *pkg.AppError, got %T", err)
	}
	if appErr.Code != code {
		t.Fatalf("expected code=%d, got code=%d (msg=%q)", code, appErr.Code, appErr.Message)
	}
}

func TestRoomService_Create_PersistsFields(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	room := &model.Room{
		Name:          "lobby",
		Description:   "main lobby",
		Limit:         10,
		AudioOnly:     true,
		AllowAudience: true,
		Type:          model.RoomTypeVoice,
		CreatedBy:     "user-1",
		DomainUUID:    "domain-a",
	}
	if err := svc.Create(room); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if room.ID == 0 {
		t.Fatal("expected Create to assign primary key")
	}
	if room.UUID == "" {
		t.Fatal("expected Create to assign uuid")
	}

	saved, err := svc.GetByUUID(room.UUID)
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if saved.ID != room.ID || saved.Name != "lobby" || saved.DomainUUID != "domain-a" {
		t.Fatalf("unexpected persisted room: %+v", saved)
	}
	if saved.Description != "main lobby" || saved.Limit != 10 || saved.CreatedBy != "user-1" {
		t.Fatalf("unexpected persisted room: %+v", saved)
	}
	if saved.Type != model.RoomTypeVoice || !saved.AudioOnly || !saved.AllowAudience {
		t.Fatalf("unexpected persisted room: %+v", saved)
	}

	var count int64
	if err := db.Model(&model.Room{}).Count(&count).Error; err != nil {
		t.Fatalf("count rooms: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 persisted room, got %d", count)
	}
}

func TestRoomService_CreateRoom_VoiceRoomLimitClampedToMin(t *testing.T) {
	for _, requested := range []uint{0, 1, 2} {
		svc, _ := setupRoomServiceTestDB(t)
		room, err := svc.CreateRoom("lobby", "", "voice channel", requested, true, true, "user-1", model.RoomTypeVoice, "domain-a")
		if err != nil {
			t.Fatalf("CreateRoom with limit=%d: %v", requested, err)
		}
		want := uint(model.MinVoiceRoomLimit)
		if requested > want {
			want = requested
		}
		if room.Limit != want {
			t.Fatalf("limit=%d: voice room limit = %d, want %d", requested, room.Limit, want)
		}
		saved, err := svc.GetByID(room.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if saved.Limit != want {
			t.Fatalf("limit=%d: persisted limit = %d, want %d", requested, saved.Limit, want)
		}
	}
}

func TestRoomService_Update_VoiceRoomLimitClampedToMin(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	room, err := svc.CreateRoom("lobby", "", "", 10, true, true, "user-1", model.RoomTypeVoice, "domain-a")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room.Limit = 0
	if err := svc.Update(room); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if room.Limit != model.MinVoiceRoomLimit {
		t.Fatalf("voice room limit = %d, want %d", room.Limit, model.MinVoiceRoomLimit)
	}
	var saved model.Room
	if err := db.First(&saved, room.ID).Error; err != nil {
		t.Fatalf("load saved room: %v", err)
	}
	if saved.Limit != model.MinVoiceRoomLimit {
		t.Fatalf("persisted limit = %d, want %d", saved.Limit, model.MinVoiceRoomLimit)
	}
}

func TestRoomService_CreateRoom_TextRoomHasNoLimit(t *testing.T) {
	svc, _ := setupRoomServiceTestDB(t)
	room, err := svc.CreateRoom("general", "", "text channel", 50, false, false, "user-1", model.RoomTypeText, "domain-a")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.Limit != 0 {
		t.Fatalf("text room limit = %d, want 0 (unlimited)", room.Limit)
	}
	if !room.AudioOnly {
		t.Fatal("text room should be audio_only")
	}
	saved, err := svc.GetByUUID(room.UUID)
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if saved.Limit != 0 {
		t.Fatalf("persisted text room limit = %d, want 0", saved.Limit)
	}
}

func TestRoomService_Update_TextRoomHasNoLimit(t *testing.T) {
	svc, _ := setupRoomServiceTestDB(t)
	room, err := svc.CreateRoom("general", "", "text channel", 0, false, false, "user-1", model.RoomTypeText, "domain-a")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	room.Limit = 50
	if err := svc.Update(room); err != nil {
		t.Fatalf("Update: %v", err)
	}
	saved, err := svc.GetByUUID(room.UUID)
	if err != nil {
		t.Fatalf("GetByUUID: %v", err)
	}
	if saved.Limit != 0 {
		t.Fatalf("updated text room limit = %d, want 0", saved.Limit)
	}
}

func TestRoomService_List_ClampsPageBelowOne(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeVoice, "room", 5)

	for _, page := range []int{0, -3} {
		t.Run(fmt.Sprintf("page=%d", page), func(t *testing.T) {
			got, total, err := svc.List(page, 2, "", "domain-a")
			if err != nil {
				t.Fatalf("List(page=%d): %v", page, err)
			}
			if total != 5 || len(got) != 2 {
				t.Fatalf("expected total=5 rooms=2, got total=%d rooms=%d", total, len(got))
			}
			if got[0].Name != "room-000" || got[1].Name != "room-001" {
				t.Fatalf("expected first page rooms, got %q,%q", got[0].Name, got[1].Name)
			}
		})
	}
}

func TestRoomService_List_ClampsPageSizeBelowOne(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeVoice, "room", 25)

	for _, pageSize := range []int{0, -5} {
		t.Run(fmt.Sprintf("page_size=%d", pageSize), func(t *testing.T) {
			got, total, err := svc.List(1, pageSize, "", "domain-a")
			if err != nil {
				t.Fatalf("List(pageSize=%d): %v", pageSize, err)
			}
			if total != 25 || len(got) != 20 {
				t.Fatalf("expected total=25 rooms=20, got total=%d rooms=%d", total, len(got))
			}
			if got[0].Name != "room-000" || got[19].Name != "room-019" {
				t.Fatalf("expected 20 room window, got %q..%q", got[0].Name, got[19].Name)
			}
		})
	}
}

func TestRoomService_List_ClampsPageSizeAboveHundred(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeVoice, "room", 120)

	got, total, err := svc.List(1, 200, "", "domain-a")
	if err != nil {
		t.Fatalf("List(pageSize=200): %v", err)
	}
	if total != 120 || len(got) != 100 {
		t.Fatalf("expected total=120 rooms=100, got total=%d rooms=%d", total, len(got))
	}
	if got[0].Name != "room-000" || got[99].Name != "room-099" {
		t.Fatalf("expected capped 100 room window, got %q..%q", got[0].Name, got[99].Name)
	}

	second, secondTotal, err := svc.List(2, 200, "", "domain-a")
	if err != nil {
		t.Fatalf("List(page=2): %v", err)
	}
	if secondTotal != 120 || len(second) != 20 {
		t.Fatalf("expected total=120 rooms=20, got total=%d rooms=%d", secondTotal, len(second))
	}
	if second[0].Name != "room-100" {
		t.Fatalf("expected offset to use capped page size, got first room %q", second[0].Name)
	}
}

func TestRoomService_List_FiltersRoomType(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeVoice, "voice", 3)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeText, "text", 2)
	seedRoomServiceRooms(t, db, "domain-b", model.RoomTypeText, "other", 1)

	textRooms, textTotal, err := svc.List(1, 100, model.RoomTypeText, "domain-a")
	if err != nil {
		t.Fatalf("List text: %v", err)
	}
	if textTotal != 2 || len(textRooms) != 2 {
		t.Fatalf("expected total=2 text rooms, got total=%d rooms=%d", textTotal, len(textRooms))
	}
	for _, room := range textRooms {
		if room.Type != model.RoomTypeText || room.DomainUUID != "domain-a" {
			t.Fatalf("unexpected room in text filter: %+v", room)
		}
	}

	voiceRooms, voiceTotal, err := svc.List(1, 100, model.RoomTypeVoice, "domain-a")
	if err != nil {
		t.Fatalf("List voice: %v", err)
	}
	if voiceTotal != 3 || len(voiceRooms) != 3 {
		t.Fatalf("expected total=3 voice rooms, got total=%d rooms=%d", voiceTotal, len(voiceRooms))
	}
	for _, room := range voiceRooms {
		if room.Type != model.RoomTypeVoice {
			t.Fatalf("unexpected room in voice filter: %+v", room)
		}
	}

	allRooms, allTotal, err := svc.List(1, 100, "", "domain-a")
	if err != nil {
		t.Fatalf("List all types: %v", err)
	}
	if allTotal != 5 || len(allRooms) != 5 {
		t.Fatalf("expected total=5 rooms=5, got total=%d rooms=%d", allTotal, len(allRooms))
	}
}

func TestRoomService_List_RejectsUnknownRoomType(t *testing.T) {
	svc, _ := setupRoomServiceTestDB(t)

	_, _, err := svc.List(1, 20, "video", "domain-a")
	assertRoomServiceErrCode(t, err, pkg.INVALID_PARAMS)
}

func TestRoomService_GetByID_NotFound(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeVoice, "room", 1)

	room, err := svc.GetByID(9999)
	if room != nil {
		t.Fatalf("expected nil room, got %+v", room)
	}
	assertRoomServiceErrCode(t, err, pkg.NOT_FOUND)
	if !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound sentinel, got %v", err)
	}
}

func TestRoomService_GetByUUID_NotFound(t *testing.T) {
	svc, db := setupRoomServiceTestDB(t)
	seedRoomServiceRooms(t, db, "domain-a", model.RoomTypeVoice, "room", 1)

	room, err := svc.GetByUUID("00000000-0000-7000-8000-000000000000")
	if room != nil {
		t.Fatalf("expected nil room, got %+v", room)
	}
	assertRoomServiceErrCode(t, err, pkg.NOT_FOUND)
	if !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound sentinel, got %v", err)
	}
}
