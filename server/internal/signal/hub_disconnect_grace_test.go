package signal

import (
	"context"
	"sync"
	"testing"
	"time"

	"GOSpeak/internal/bus"
	"GOSpeak/internal/sfu"
)

// graceProvider 记录 SFU 层的 RemoveParticipant / DeleteRoom 调用。
type graceProvider struct {
	sfu.BaseProvider
	mu           sync.Mutex
	removed      []string
	deleted      []string
	participants []sfu.ParticipantSummary
	listErr      error
}

func (p *graceProvider) ProviderName() string { return "grace" }

func (p *graceProvider) Capabilities() sfu.Capabilities { return sfu.CapabilitiesFor("livekit") }

func (p *graceProvider) GenerateToken(string, string) (string, error) { return "t", nil }

func (p *graceProvider) ListRooms() ([]sfu.RoomSummary, error) { return nil, nil }

func (p *graceProvider) ListParticipants(string) ([]sfu.ParticipantSummary, error) {
	return p.participants, p.listErr
}

func (p *graceProvider) MuteParticipant(string, string, string, bool) error { return nil }

func (p *graceProvider) GetHost() string { return "host" }

func (p *graceProvider) RemoveParticipant(room, identity string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removed = append(p.removed, room+"/"+identity)
	return nil
}

func (p *graceProvider) DeleteRoom(room string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deleted = append(p.deleted, room)
	return nil
}

func (p *graceProvider) removedSnapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.removed...)
}

func (p *graceProvider) deletedSnapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.deleted...)
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

// waitGraceProbe 在同一 Hub 上挂一个必定触发的清理作为时钟探针：探针进入
// removed 即说明宽限期已过，用可观察事件替代固定 sleep 的时长猜测。
func waitGraceProbe(t *testing.T, h *Hub, p *graceProvider) {
	t.Helper()
	const room, identity = "__grace_probe__", "__probe__"
	h.scheduleDisconnectCleanup(disconnectCleanup{room: room, identity: identity})
	want := room + "/" + identity
	if !waitUntil(t, 5*time.Second, func() bool {
		for _, r := range p.removedSnapshot() {
			if r == want {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("grace probe %q never fired, removed=%v", want, p.removedSnapshot())
	}
}

func removedContains(p *graceProvider, target string) bool {
	for _, r := range p.removedSnapshot() {
		if r == target {
			return true
		}
	}
	return false
}

func newGraceHub(grace time.Duration, provider *graceProvider) *Hub {
	h := newTestHub()
	h.SetSFU(provider)
	h.disconnectGrace = grace
	return h
}

func addRoomMember(h *Hub, room, sid, identity string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[room]
	if !ok {
		r = &Room{
			Name:       room,
			Members:    make(map[string]*MemberInfo),
			ByIdentity: make(map[string]*MemberInfo),
			MicMuted:   make(map[string]bool),
			Speaking:   make(map[string]bool),
			CreatedAt:  time.Now(),
		}
		h.rooms[room] = r
	}
	roomSetMember(r, sid, identity, &MemberInfo{ID: sid, Identity: identity})
}

// 宽限期内不应立即移除媒体 participant，逾期才移除。
func TestDisconnectGrace_DelaysRemoval(t *testing.T) {
	p := &graceProvider{}
	h := newGraceHub(DefaultDisconnectGracePeriod, p)

	h.scheduleDisconnectCleanup(disconnectCleanup{room: "meeting-room", identity: "alice"})

	if got := p.removedSnapshot(); len(got) != 0 {
		t.Fatalf("participant removed before grace elapsed: %v", got)
	}
	key := pendingCleanupKey("meeting-room", "alice")
	h.cleanupMu.Lock()
	pc := h.pendingCleanups[key]
	h.cleanupMu.Unlock()
	if pc == nil {
		t.Fatal("expected cleanup to be pending during grace window")
	}
	// 手动触发到期清理，避免用固定 sleep 猜测宽限期时序。
	pc.timer.Stop()
	h.runPendingCleanup(key, pc)

	if !waitUntil(t, time.Second, func() bool { return len(p.removedSnapshot()) == 1 }) {
		t.Fatalf("participant not removed after grace: %v", p.removedSnapshot())
	}
	if got := p.removedSnapshot()[0]; got != "meeting-room/alice" {
		t.Fatalf("unexpected removal target: %s", got)
	}
}

// 宽限期内取消（重新加入）后，不应移除。
func TestDisconnectGrace_CancelOnRejoin(t *testing.T) {
	p := &graceProvider{}
	h := newGraceHub(80*time.Millisecond, p)

	h.scheduleDisconnectCleanup(disconnectCleanup{room: "meeting-room", identity: "alice"})
	h.cancelPendingCleanup("meeting-room", "alice")

	waitGraceProbe(t, h, p)
	if removedContains(p, "meeting-room/alice") {
		t.Fatalf("cancelled cleanup still removed participant: %v", p.removedSnapshot())
	}
}

// 即使没有显式取消，只要到期时房间内已有同 identity 存活成员，也应跳过清理。
func TestDisconnectGrace_SkipsWhenIdentityRejoined(t *testing.T) {
	p := &graceProvider{}
	h := newGraceHub(50*time.Millisecond, p)

	h.scheduleDisconnectCleanup(disconnectCleanup{room: "meeting-room", identity: "alice"})
	addRoomMember(h, "meeting-room", "socket-new", "alice")

	waitGraceProbe(t, h, p)
	if removedContains(p, "meeting-room/alice") {
		t.Fatalf("rejoined identity should not be removed, got %v", p.removedSnapshot())
	}
}

// 到期执行时若房间已有他人（不再是空房），应仅移除断连者，不删除房间。
func TestDisconnectGrace_DoesNotDeleteRoomIfRefilled(t *testing.T) {
	p := &graceProvider{}
	h := newGraceHub(50*time.Millisecond, p)

	h.scheduleDisconnectCleanup(disconnectCleanup{room: "meeting-room", identity: "alice", deleted: true})
	// 宽限期内另一个人进房。
	addRoomMember(h, "meeting-room", "socket-bob", "bob")

	if !waitUntil(t, time.Second, func() bool { return len(p.removedSnapshot()) == 1 }) {
		t.Fatalf("participant not removed after grace: %v", p.removedSnapshot())
	}
	waitGraceProbe(t, h, p)
	if got := p.deletedSnapshot(); len(got) != 0 {
		t.Fatalf("room with live member must not be deleted, got %v", got)
	}
}

// grace <= 0 时保持原有即时清理语义（测试/兼容路径）。
func TestDisconnectGrace_ZeroIsImmediate(t *testing.T) {
	p := &graceProvider{}
	h := newGraceHub(0, p)

	h.scheduleDisconnectCleanup(disconnectCleanup{room: "meeting-room", identity: "alice"})
	if !waitUntil(t, time.Second, func() bool { return len(p.removedSnapshot()) == 1 }) {
		t.Fatalf("zero-grace cleanup did not run: %v", p.removedSnapshot())
	}
}

// 语音房 WS 重连只走 room:join（媒体仍在），也应取消待执行清理。
func TestDisconnectGrace_OnRoomJoinCancels(t *testing.T) {
	p := &graceProvider{}
	h := newGraceHub(80*time.Millisecond, p)

	h.scheduleDisconnectCleanup(disconnectCleanup{room: "meeting-room", identity: "alice"})

	conn := newAuthedMockClient("socket-reconnect", "alice")
	if _, err := h.OnRoomJoin(conn, `{"room":"meeting-room","identity":"alice"}`); err != nil {
		t.Fatalf("OnRoomJoin: %v", err)
	}

	waitGraceProbe(t, h, p)
	if removedContains(p, "meeting-room/alice") {
		t.Fatalf("room:join during grace should cancel cleanup, got %v", p.removedSnapshot())
	}
}

// 多实例下：即便共享 KV 里因 lease 过期看不到远端成员，只要 SFU 仍有参与者就绝不删房。
func TestDeleteRoomSafe_SkipsWhenSFUParticipantsRemain(t *testing.T) {
	p := &graceProvider{participants: []sfu.ParticipantSummary{{Identity: "alice"}}}
	h := seededHub(t, p)

	h.deleteRoomSafe("lobby")
	if got := p.deletedSnapshot(); len(got) != 0 {
		t.Fatalf("must not delete room with live SFU participants, got %v", got)
	}
}

// SFU 确已空时仍应删除（否则空房永远不清理）。
func TestDeleteRoomSafe_DeletesWhenSFUEmpty(t *testing.T) {
	p := &graceProvider{}
	h := seededHub(t, p)

	h.deleteRoomSafe("lobby")
	if got := p.deletedSnapshot(); len(got) != 1 || got[0] != "lobby" {
		t.Fatalf("empty SFU room should be deleted, got %v", got)
	}
}

// seededHub 构造一个带共享 KV 的多实例 Hub，房间快照已存在但成员为空，
// 用于验证 deleteRoomSafe 的 SFU 参与者兜底逻辑。
func seededHub(t *testing.T, p *graceProvider) *Hub {
	t.Helper()
	store := newMemStateStore()
	if err := store.PutRoomMembers(context.Background(), bus.RoomMembersSnapshot{Room: "lobby"}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	h := NewHub(nil, nil, nil, nil)
	h.SetMembershipStore(store, "test-instance")
	h.SetSFU(p)
	return h
}
