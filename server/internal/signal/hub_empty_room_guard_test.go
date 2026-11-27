package signal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"GOSpeak/internal/bus"
	"GOSpeak/internal/sfu"

	"github.com/nats-io/nats.go"
)

type guardProvider struct {
	sfu.BaseProvider
	mu        sync.Mutex
	deleted   []string
	err       error
	rooms     []sfu.RoomSummary
	listErr   error
	listCalls atomic.Int64
	caps      *sfu.Capabilities
}

func (p *guardProvider) listCallCount() int64 { return p.listCalls.Load() }

func (p *guardProvider) deletedRooms() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.deleted...)
}

func (p *guardProvider) setRooms(rooms ...sfu.RoomSummary) {
	p.mu.Lock()
	p.rooms = rooms
	p.mu.Unlock()
}

func (p *guardProvider) ProviderName() string { return "guard" }
func (p *guardProvider) Capabilities() sfu.Capabilities {
	if p.caps != nil {
		return *p.caps
	}
	return sfu.CapabilitiesFor("livekit")
}
func (p *guardProvider) GenerateToken(string, string) (string, error) { return "t", nil }
func (p *guardProvider) ListRooms() ([]sfu.RoomSummary, error) {
	p.listCalls.Add(1)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rooms, p.listErr
}
func (p *guardProvider) ListParticipants(string) ([]sfu.ParticipantSummary, error) {
	return nil, nil
}
func (p *guardProvider) MuteParticipant(string, string, string, bool) error { return nil }
func (p *guardProvider) RemoveParticipant(string, string) error             { return nil }
func (p *guardProvider) DeleteRoom(room string) error {
	p.mu.Lock()
	p.deleted = append(p.deleted, room)
	p.mu.Unlock()
	return p.err
}
func (p *guardProvider) GetHost() string { return "host" }

type guardStore struct {
	*memStateStore
	getErr error
}

func (s *guardStore) GetRoomMembers(ctx context.Context, room string) (bus.RoomMembersSnapshot, error) {
	if s.getErr != nil {
		return bus.RoomMembersSnapshot{}, s.getErr
	}
	return s.memStateStore.GetRoomMembers(ctx, room)
}

func guardHub(store membershipStore, instanceID string, provider *guardProvider) *Hub {
	h := newTestHub()
	h.SetSFU(provider)
	h.SetMembershipStore(store, instanceID)
	return h
}

func putGuardSnapshot(t *testing.T, store *guardStore, room string, members ...bus.MemberRecord) {
	t.Helper()
	if err := store.PutRoomMembers(context.Background(), bus.RoomMembersSnapshot{
		Room: room, Members: members,
	}); err != nil {
		t.Fatalf("seed membership snapshot: %v", err)
	}
}

func remoteMember(room, instanceID string, expiresAtMS int64) bus.MemberRecord {
	return bus.MemberRecord{
		Room: room, Identity: "bob", InstanceID: instanceID, ExpiresAtMS: expiresAtMS,
	}
}

func TestDeleteRoomSafe_SkipsWhenRemoteMemberAlive(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore()}
	now := time.Now().UnixMilli()
	putGuardSnapshot(t, store, "lobby", remoteMember("lobby", "remote-instance", now+60000))
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.deleteRoomSafe("lobby")

	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("live remote member must block room deletion, got DeleteRoom calls %v", provider.deletedRooms())
	}
}

func TestDeleteRoomSafe_DeletesWhenRemoteMemberExpired(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore()}
	now := time.Now().UnixMilli()
	putGuardSnapshot(t, store, "lobby", remoteMember("lobby", "remote-instance", now-1))
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.deleteRoomSafe("lobby")

	if len(provider.deletedRooms()) != 1 || provider.deletedRooms()[0] != "lobby" {
		t.Fatalf("expired remote member must not block deletion, got %v", provider.deletedRooms())
	}
}

func TestDeleteRoomSafe_DeletesWhenOnlySameInstanceMembers(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore()}
	now := time.Now().UnixMilli()
	putGuardSnapshot(t, store, "lobby", remoteMember("lobby", "local-instance", now+60000))
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.deleteRoomSafe("lobby")

	if len(provider.deletedRooms()) != 1 || provider.deletedRooms()[0] != "lobby" {
		t.Fatalf("same-instance members must not block deletion, got %v", provider.deletedRooms())
	}
}

func TestDeleteRoomSafe_DeletesWhenNoSharedStore(t *testing.T) {
	provider := &guardProvider{}
	h := guardHub(nil, "local-instance", provider)

	h.deleteRoomSafe("lobby")

	if len(provider.deletedRooms()) != 1 || provider.deletedRooms()[0] != "lobby" {
		t.Fatalf("local-only mode must keep deleting, got %v", provider.deletedRooms())
	}
}

func TestDeleteRoomSafe_DeletesWhenMembershipKeyNotFound(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore(), getErr: nats.ErrKeyNotFound}
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.deleteRoomSafe("lobby")

	if len(provider.deletedRooms()) != 1 || provider.deletedRooms()[0] != "lobby" {
		t.Fatalf("missing membership key must not block deletion, got %v", provider.deletedRooms())
	}
}

func TestDeleteRoomSafe_SkipsWhenSharedStoreReadFails(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore(), getErr: errors.New("kv unavailable")}
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.deleteRoomSafe("lobby")

	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("KV read failure must fail-closed, got DeleteRoom calls %v", provider.deletedRooms())
	}
}

func TestForceDeleteRoom_BypassesGuard(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore()}
	now := time.Now().UnixMilli()
	putGuardSnapshot(t, store, "lobby", remoteMember("lobby", "remote-instance", now+60000))
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.forceDeleteRoom("lobby")

	if len(provider.deletedRooms()) != 1 || provider.deletedRooms()[0] != "lobby" {
		t.Fatalf("forced delete must ignore live remote members, got %v", provider.deletedRooms())
	}
}

func TestCleanupParticipant_DeleteRoomUsesGuard(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore()}
	now := time.Now().UnixMilli()
	putGuardSnapshot(t, store, "lobby", remoteMember("lobby", "remote-instance", now+60000))
	provider := &guardProvider{}
	h := guardHub(store, "local-instance", provider)

	h.CleanupParticipant("lobby", "", true)

	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("async cleanup must respect the guard, got DeleteRoom calls %v", provider.deletedRooms())
	}
}
