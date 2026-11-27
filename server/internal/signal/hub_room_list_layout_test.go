package signal

import (
	"testing"
	"time"

	"GOSpeak/internal/model"
	"GOSpeak/internal/protocol"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakeLayoutStore struct {
	groups []model.DomainRoomGroup
	layout model.DomainRoomLayout
}

func (f *fakeLayoutStore) Groups(domainUUID string) ([]model.DomainRoomGroup, error) {
	return f.groups, nil
}
func (f *fakeLayoutStore) Layout(domainUUID string) (model.DomainRoomLayout, error) {
	return f.layout, nil
}

type fakeRoomStore struct {
	rooms []model.Room
}

func (f *fakeRoomStore) List(page, pageSize int, roomType, domainUUID string) ([]model.Room, int64, error) {
	return f.rooms, int64(len(f.rooms)), nil
}
func (f *fakeRoomStore) GetByName(name string) (*model.Room, error) { return nil, nil }
func (f *fakeRoomStore) GetByDomainAndName(domainUUID, name string) (*model.Room, error) {
	return nil, nil
}

func TestHubRoomListLayoutOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Room{}); err != nil {
		t.Fatal(err)
	}
	rooms := []model.Room{
		{UUID: "room-a", Name: "a", DomainUUID: "dom1", GroupUUID: "group-1", SortOrder: 0},
		{UUID: "room-b", Name: "b", DomainUUID: "dom1", GroupUUID: "group-1", SortOrder: 1},
		{UUID: "room-c", Name: "c", DomainUUID: "dom1", SortOrder: 0},
	}
	db.Create(&rooms)

	store := &fakeRoomStore{rooms: rooms}
	layoutStore := &fakeLayoutStore{
		groups: []model.DomainRoomGroup{{UUID: "group-1", Name: "常用", SortOrder: 0}},
		layout: model.DomainRoomLayout{DomainUUID: "dom1", UngroupedPosition: 0},
	}
	h := NewHubWithOptions(store, nil, nil, nil, HubOptions{LayoutStore: layoutStore})

	got := h.getMergedRooms("dom1")
	if len(got) != 3 {
		t.Fatalf("expected 3 rooms, got %d", len(got))
	}
	want := []string{"room-c", "room-a", "room-b"}
	for i, uuid := range want {
		if got[i].UUID != uuid {
			t.Fatalf("position %d: got %s, want %s (all: %+v)", i, got[i].UUID, uuid, got)
		}
	}
	if got[1].GroupUUID != "group-1" || got[1].SortOrder != 0 {
		t.Fatalf("room-a lost group fields: %+v", got[1])
	}
}
func TestHub_OnRoomList_IncludesLayout(t *testing.T) {
	store := &fakeRoomStore{rooms: []model.Room{
		{UUID: "room-a", Name: "a", DomainUUID: "dom1", GroupUUID: "group-1"},
		{UUID: "room-b", Name: "b", DomainUUID: "dom1"},
	}}
	layoutStore := &fakeLayoutStore{
		groups: []model.DomainRoomGroup{{UUID: "group-1", Name: "常用", SortOrder: 0}},
		layout: model.DomainRoomLayout{DomainUUID: "dom1", UngroupedPosition: 1},
	}
	h := NewHubWithOptions(store, nil, nil, nil, HubOptions{
		LayoutStore:   layoutStore,
		DomainChecker: func(domainUUID, userUUID string) bool { return true },
	})
	conn := newAuthedMockClient("socket-1", "user-1")

	h.OnRoomList(conn, `{"domain_uuid":"dom1"}`)

	data, ok := conn.lastEvent(EventRoomListResult).(map[string]interface{})
	if !ok {
		t.Fatal("expected room:list:result event")
	}
	summary, ok := data["layout"].(protocol.RoomListLayoutPayload)
	if !ok {
		t.Fatalf("room:list:result missing layout, got %v", data["layout"])
	}
	if len(summary.Groups) != 1 || summary.Groups[0].Name != "常用" {
		t.Fatalf("unexpected layout groups: %+v", summary.Groups)
	}
	if summary.UngroupedPosition != 1 {
		t.Fatalf("unexpected ungrouped position: %d", summary.UngroupedPosition)
	}
}

func TestHub_GetMergedRooms_ActiveRoomKeepsGroupAndSort(t *testing.T) {
	store := &mockRoomStore{rooms: []model.Room{
		{UUID: "room-a", Name: "大厅", DomainUUID: "dom1", GroupUUID: "group-1", SortOrder: 2},
	}}
	layoutStore := &fakeLayoutStore{
		groups: []model.DomainRoomGroup{{UUID: "group-1", Name: "常用", SortOrder: 0}},
		layout: model.DomainRoomLayout{DomainUUID: "dom1", UngroupedPosition: 1},
	}
	h := NewHubWithOptions(store, nil, nil, nil, HubOptions{LayoutStore: layoutStore})

	key := roomKey("dom1", "大厅")
	h.mu.Lock()
	h.rooms[key] = &Room{
		Name:      "大厅",
		Members:   map[string]*MemberInfo{"socket-1": {ID: "socket-1", Identity: "user-1"}},
		CreatedAt: time.Now(),
	}
	h.mu.Unlock()

	got := h.getMergedRooms("dom1")
	if len(got) != 1 {
		t.Fatalf("expected 1 room, got %d", len(got))
	}
	if got[0].GroupUUID != "group-1" || got[0].SortOrder != 2 {
		t.Fatalf("active room lost layout fields: group=%q sort=%d", got[0].GroupUUID, got[0].SortOrder)
	}

	info := h.roomInfoMerged(key)
	if info.GroupUUID != "group-1" || info.SortOrder != 2 {
		t.Fatalf("room:updated payload lost layout fields: group=%q sort=%d", info.GroupUUID, info.SortOrder)
	}
}
