package signal

import (
	"errors"
	"testing"
	"time"

	"GOSpeak/internal/sfu"
)

var graceMS = emptyRoomGracePeriod.Milliseconds()

func reconcileHub(provider *guardProvider, store membershipStore) *Hub {
	h := newTestHub()
	h.SetSFU(provider)
	h.SetMembershipStore(store, "local-instance")
	return h
}

func TestReconcileEmptyRooms_DeletesOrphanAfterGracePeriod(t *testing.T) {
	provider := &guardProvider{rooms: []sfu.RoomSummary{{Name: "orphan", MemberCount: 0}}}
	h := reconcileHub(provider, nil)

	h.reconcileEmptyRooms(func() int64 { return 0 })
	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("first observation must not delete, got %v", provider.deletedRooms())
	}

	h.reconcileEmptyRooms(func() int64 { return graceMS + 1 })
	if len(provider.deletedRooms()) != 1 || provider.deletedRooms()[0] != "orphan" {
		t.Fatalf("orphan room must be deleted after grace period, got %v", provider.deletedRooms())
	}
}

func TestReconcileEmptyRooms_KeepsOccupiedRoom(t *testing.T) {
	provider := &guardProvider{rooms: []sfu.RoomSummary{{Name: "lobby", MemberCount: 2}}}
	h := reconcileHub(provider, nil)

	h.reconcileEmptyRooms(func() int64 { return 0 })
	h.reconcileEmptyRooms(func() int64 { return graceMS + 1 })

	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("occupied room must never be deleted, got %v", provider.deletedRooms())
	}
	if len(h.emptyRoomSeen) != 0 {
		t.Fatalf("occupied room must not be tracked, got %v", h.emptyRoomSeen)
	}
}

func TestReconcileEmptyRooms_ResetsTrackingWhenRoomRefills(t *testing.T) {
	provider := &guardProvider{rooms: []sfu.RoomSummary{{Name: "lobby", MemberCount: 0}}}
	h := reconcileHub(provider, nil)

	h.reconcileEmptyRooms(func() int64 { return 0 })
	provider.setRooms(sfu.RoomSummary{Name: "lobby", MemberCount: 1})
	h.reconcileEmptyRooms(func() int64 { return graceMS + 1 })
	provider.setRooms(sfu.RoomSummary{Name: "lobby", MemberCount: 0})
	h.reconcileEmptyRooms(func() int64 { return graceMS + 2 })
	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("refill must reset the grace window, got premature delete %v", provider.deletedRooms())
	}

	h.reconcileEmptyRooms(func() int64 { return 2*graceMS + 3 })
	if len(provider.deletedRooms()) != 1 {
		t.Fatalf("room must be deleted one full grace period after becoming empty again, got %v", provider.deletedRooms())
	}
}

func TestReconcileEmptyRooms_SkipsRoomWithLiveSharedMembers(t *testing.T) {
	store := &guardStore{memStateStore: newMemStateStore()}
	now := time.Now().UnixMilli()
	putGuardSnapshot(t, store, "lobby", remoteMember("lobby", "remote-instance", now+60000))
	provider := &guardProvider{rooms: []sfu.RoomSummary{{Name: "lobby", MemberCount: 0}}}
	h := reconcileHub(provider, store)

	h.reconcileEmptyRooms(func() int64 { return 0 })
	h.reconcileEmptyRooms(func() int64 { return graceMS + 1 })

	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("shared members must block reconcile delete, got %v", provider.deletedRooms())
	}
}

func TestReconcileEmptyRooms_NoDeleteWhenListRoomsFails(t *testing.T) {
	provider := &guardProvider{listErr: errors.New("sfu down")}
	h := reconcileHub(provider, nil)

	h.reconcileEmptyRooms(func() int64 { return 0 })

	if len(provider.deletedRooms()) != 0 {
		t.Fatalf("ListRooms failure must not delete anything, got %v", provider.deletedRooms())
	}
}

func TestReconcileEmptyRooms_SkipsWhenCapabilityMissing(t *testing.T) {
	caps := sfu.CapabilitiesFor("livekit")
	caps.ListRooms = false
	caps.ListLevel = sfu.EnforcementNone
	caps.DeleteRoom = false
	caps.DeleteLevel = sfu.EnforcementNone
	provider := &guardProvider{caps: &caps}
	h := reconcileHub(provider, nil)

	h.reconcileEmptyRooms(func() int64 { return 0 })

	if provider.listCallCount() != 0 {
		t.Fatalf("reconcile must skip when ListRooms capability is absent, got %d calls", provider.listCallCount())
	}
}

func waitForReconcileTicks(t *testing.T, provider *guardProvider, min int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if provider.listCallCount() >= min {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("reconciler ticked %d times, want at least %d", provider.listCallCount(), min)
}

func waitReconcileStable(t *testing.T, provider *guardProvider, interval time.Duration) {
	t.Helper()
	deadline := time.Now().Add(interval * 12)
	last := provider.listCallCount()
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		now := provider.listCallCount()
		if now != last {
			last = now
			stableSince = time.Now()
		}
		if time.Since(stableSince) >= interval*4 {
			return
		}
	}
}

func TestStartEmptyRoomReconciler_RunsAndStops(t *testing.T) {
	provider := &guardProvider{rooms: []sfu.RoomSummary{{Name: "orphan", MemberCount: 0}}}
	h := reconcileHub(provider, nil)

	stop := h.StartEmptyRoomReconciler(10 * time.Millisecond)
	waitForReconcileTicks(t, provider, 1)
	stop()

	if provider.listCallCount() == 0 {
		t.Fatal("expected reconciler to tick")
	}
	after := provider.listCallCount()
	waitReconcileStable(t, provider, 10*time.Millisecond)
	if provider.listCallCount() != after {
		t.Fatal("reconciler kept ticking after stop")
	}
}

func TestStartEmptyRoomReconciler_Idempotent(t *testing.T) {
	provider := &guardProvider{}
	h := reconcileHub(provider, nil)
	const interval = 20 * time.Millisecond

	stop1 := h.StartEmptyRoomReconciler(interval)
	stop2 := h.StartEmptyRoomReconciler(interval)
	waitForReconcileTicks(t, provider, 1)

	before := provider.listCallCount()
	windowStart := time.Now()
	time.Sleep(200 * time.Millisecond)
	elapsed := time.Since(windowStart)
	ticked := provider.listCallCount() - before
	if ticked < 1 {
		t.Fatalf("reconciler stopped ticking after duplicate Start: %d ticks", ticked)
	}
	if maxTicks := int64(elapsed/interval) + 4; ticked > maxTicks {
		t.Fatalf("duplicate Start spawned a second goroutine: %d ticks in %s, want at most %d", ticked, elapsed, maxTicks)
	}

	stop2()
	afterStop2 := provider.listCallCount()
	waitForReconcileTicks(t, provider, afterStop2+1)

	stop1()
	afterStop := provider.listCallCount()
	waitReconcileStable(t, provider, interval)
	if provider.listCallCount() != afterStop {
		t.Fatal("reconciler kept ticking after stop")
	}
}
