package signal

import (
	"testing"

	"GOSpeak/internal/bus"
	"GOSpeak/internal/model"
	"GOSpeak/internal/permcode"
	"GOSpeak/internal/pkg"
)

func TestOnRoomJoinSFU_DowngradedMemberCannotCreateTemporaryRoom(t *testing.T) {
	store := newMemStateStore()
	hub := NewHub(nil, nil, nil, nil)
	hub.fanout = newMockBroadcaster()
	hub.SetMembershipStore(store, "inst-a")
	hub.SetDomainChecker(func(domainUUID, userUUID string) bool {
		return domainUUID == "domain-a" && userUUID == "member-1"
	})
	var checkedCode string
	hub.SetDomainPermissionChecker(func(domainUUID, userUUID, permCode string) bool {
		checkedCode = permCode
		return false
	})

	conn := newAuthedMockClient("sock-1", "member-1")
	ackRaw, err := hub.OnRoomJoinSFU(conn, `{"room":"ghost-room","domain_uuid":"domain-a","identity":"member-1","password":"secret"}`)
	if err != nil {
		t.Fatalf("OnRoomJoinSFU error: %v", err)
	}
	ack := decodeAck(t, ackRaw)
	if ack["error"] == nil {
		t.Fatalf("expected error ack for downgraded member, got %s", ackRaw)
	}
	if checkedCode != permcode.PermRoomCreate {
		t.Fatalf("expected PermRoomCreate check, got %q", checkedCode)
	}

	key := roomKey("domain-a", "ghost-room")
	hub.mu.RLock()
	_, created := hub.rooms[key]
	hub.mu.RUnlock()
	if created {
		t.Fatal("room must not be materialized for member without PermRoomCreate")
	}

	store.mu.Lock()
	_, metaWritten := store.metas[key]
	_, membersWritten := store.rooms[key]
	store.mu.Unlock()
	if metaWritten || membersWritten {
		t.Fatalf("no KV state should be written: room_meta=%v membership=%v", metaWritten, membersWritten)
	}
}

func TestOnRoomJoinSFU_MemberWithCreatePermissionCanCreateTemporaryRoom(t *testing.T) {
	store := newMemStateStore()
	hub := NewHub(nil, nil, nil, nil)
	hub.fanout = newMockBroadcaster()
	hub.SetMembershipStore(store, "inst-a")
	hub.SetDomainChecker(func(domainUUID, userUUID string) bool { return true })
	hub.SetDomainPermissionChecker(func(domainUUID, userUUID, permCode string) bool {
		return permCode == permcode.PermRoomCreate
	})

	conn := newAuthedMockClient("sock-2", "creator-1")
	ackRaw, err := hub.OnRoomJoinSFU(conn, `{"room":"temp-room","domain_uuid":"domain-a","identity":"creator-1","password":"secret"}`)
	if err != nil {
		t.Fatalf("OnRoomJoinSFU error: %v", err)
	}
	if ack := decodeAck(t, ackRaw); ack["ok"] != true {
		t.Fatalf("expected ok ack, got %s", ackRaw)
	}

	key := roomKey("domain-a", "temp-room")
	hub.mu.RLock()
	room := hub.rooms[key]
	hub.mu.RUnlock()
	if room == nil {
		t.Fatal("expected temporary room to be created")
	}
	if room.Password == "" || !pkg.VerifyPassword(room.Password, "secret") {
		t.Fatalf("room password should be stored hashed, got %q", room.Password)
	}

	store.mu.Lock()
	meta, metaWritten := store.metas[key]
	store.mu.Unlock()
	if !metaWritten || meta.Name != "temp-room" || meta.OwnerNodeID != "inst-a" {
		t.Fatalf("expected room meta with owner, got %+v written=%v", meta, metaWritten)
	}
	if meta.Password != room.Password {
		t.Fatal("room meta should carry the same hashed password as the room")
	}
}

func TestOnRoomJoinSFU_ExistingDBRoomDoesNotRequireCreatePermission(t *testing.T) {
	store := &mockRoomStore{rooms: []model.Room{
		{Name: "existing-room", DomainUUID: "domain-a", Type: model.RoomTypeVoice},
	}}
	hub := NewHub(store, nil, nil, nil)
	hub.fanout = newMockBroadcaster()
	hub.SetDomainChecker(func(domainUUID, userUUID string) bool { return true })
	hub.SetDomainPermissionChecker(func(domainUUID, userUUID, permCode string) bool {
		return false
	})

	conn := newAuthedMockClient("sock-3", "member-3")
	ackRaw, err := hub.OnRoomJoinSFU(conn, `{"room":"existing-room","domain_uuid":"domain-a","identity":"member-3"}`)
	if err != nil {
		t.Fatalf("OnRoomJoinSFU error: %v", err)
	}
	if ack := decodeAck(t, ackRaw); ack["ok"] != true {
		t.Fatalf("joining an existing room must not require PermRoomCreate, got %s", ackRaw)
	}
}

func TestOnRoomJoinSFU_RemoteMetaRoomDoesNotRequireCreatePermission(t *testing.T) {
	store := newMemStateStore()
	hub := NewHub(nil, nil, nil, nil)
	hub.fanout = newMockBroadcaster()
	hub.SetMembershipStore(store, "inst-a")
	hub.SetDomainChecker(func(domainUUID, userUUID string) bool { return true })
	hub.SetDomainPermissionChecker(func(domainUUID, userUUID, permCode string) bool {
		return false
	})

	key := roomKey("domain-a", "remote-room")
	store.metas[key] = bus.RoomMeta{Name: "remote-room", DomainUUID: "domain-a", Type: model.RoomTypeVoice}

	conn := newAuthedMockClient("sock-4", "member-4")
	ackRaw, err := hub.OnRoomJoinSFU(conn, `{"room":"remote-room","domain_uuid":"domain-a","identity":"member-4"}`)
	if err != nil {
		t.Fatalf("OnRoomJoinSFU error: %v", err)
	}
	if ack := decodeAck(t, ackRaw); ack["ok"] != true {
		t.Fatalf("joining a room known via shared meta must not require PermRoomCreate, got %s", ackRaw)
	}
}

func TestOnRoomJoinSFU_RemoteMetaRoomKeepsPassword(t *testing.T) {
	store := newMemStateStore()
	hub := NewHub(nil, nil, nil, nil)
	hub.fanout = newMockBroadcaster()
	hub.SetMembershipStore(store, "inst-a")
	hub.SetDomainChecker(func(domainUUID, userUUID string) bool { return true })
	hub.SetDomainPermissionChecker(func(domainUUID, userUUID, permCode string) bool { return false })

	hash, err := pkg.HashPassword("secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	key := roomKey("domain-a", "meta-secret-room")
	store.metas[key] = bus.RoomMeta{
		Name: "meta-secret-room", DomainUUID: "domain-a", Type: model.RoomTypeVoice, Password: hash,
	}

	// 无密码加入仅存在于 KV 的带密码房间：必须被拒，不能因本地 Room.Password 为空而放行。
	conn := newAuthedMockClient("sock-5", "member-5")
	ackRaw, err := hub.OnRoomJoinSFU(conn, `{"room":"meta-secret-room","domain_uuid":"domain-a","identity":"member-5"}`)
	if err != nil {
		t.Fatalf("OnRoomJoinSFU error: %v", err)
	}
	if ack := decodeAck(t, ackRaw); ack["ok"] == true {
		t.Fatalf("joining a password-protected meta room without password must fail, got %s", ackRaw)
	}
}
