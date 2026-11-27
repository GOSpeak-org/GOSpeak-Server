package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"GOSpeak/internal/protocol"
)

func TestFanout_Add_Remove(t *testing.T) {
	f := NewFanout()
	c := NewTestClient("client-1", nil)

	f.Add(c)
	// Check via internal map (same package)
	if _, ok := f.clients["client-1"]; !ok {
		t.Fatal("expected client to be registered")
	}

	f.Remove("client-1")
	if _, ok := f.clients["client-1"]; ok {
		t.Fatal("expected client to be removed")
	}
}

func TestFanout_Join_Leave(t *testing.T) {
	f := NewFanout()
	c := NewTestClient("client-1", nil)
	f.Add(c)

	f.Join("room-a", "client-1")
	if !f.RoomExists("room-a") {
		t.Fatal("expected room-a to exist after join")
	}

	f.Leave("room-a", "client-1")
	if f.RoomExists("room-a") {
		t.Fatal("expected room-a to be removed after leave")
	}
}

func TestFanout_BroadcastToRoom(t *testing.T) {
	f := NewFanout()
	c1 := NewTestClient("c1", nil)
	c2 := NewTestClient("c2", nil)
	f.Add(c1)
	f.Add(c2)
	f.Join("room-x", "c1")
	f.Join("room-x", "c2")

	f.BroadcastToRoom("room-x", protocol.EventRoomUpdated, "hello")

	// c1 and c2 should have received the message in their writeCh
	select {
	case <-c1.writeCh:
	default:
		t.Fatal("expected c1 to receive broadcast")
	}
	select {
	case <-c2.writeCh:
	default:
		t.Fatal("expected c2 to receive broadcast")
	}
}

func TestFanout_BroadcastToNamespace(t *testing.T) {
	f := NewFanout()
	c1 := NewTestClient("c1", nil)
	c2 := NewTestClient("c2", nil)
	f.Add(c1)
	f.Add(c2)

	f.BroadcastToNamespace(protocol.EventRoomCreated, "payload")

	select {
	case <-c1.writeCh:
	default:
		t.Fatal("expected c1 to receive namespace broadcast")
	}
	select {
	case <-c2.writeCh:
	default:
		t.Fatal("expected c2 to receive namespace broadcast")
	}
}

func TestFanout_BroadcastManyClients(t *testing.T) {
	f := NewFanout()
	const n = 20
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("c%d", i)
		c := NewTestClient(id, nil)
		f.Add(c)
		f.Join("room-x", id)
	}

	f.BroadcastToRoom("room-x", protocol.EventRoomUpdated, "hello")

	for i := 0; i < n; i++ {
		select {
		case <-f.clients[fmt.Sprintf("c%d", i)].writeCh:
		default:
			t.Fatalf("expected client c%d to receive broadcast", i)
		}
	}
}

func TestFanout_ForEach(t *testing.T) {
	f := NewFanout()
	for i := 0; i < 3; i++ {
		c := NewTestClient(fmt.Sprintf("c%d", i), nil)
		f.Add(c)
		f.Join("room-y", c.ID())
	}

	var count int
	f.ForEach("room-y", func(cm ClientMessenger) bool {
		count++
		return true
	})
	if count != 3 {
		t.Fatalf("expected 3 iterations, got %d", count)
	}
}

func TestFanout_ForEach_StopEarly(t *testing.T) {
	f := NewFanout()
	c1 := NewTestClient("c1", nil)
	c2 := NewTestClient("c2", nil)
	f.Add(c1)
	f.Add(c2)
	f.Join("room-z", "c1")
	f.Join("room-z", "c2")

	var count int
	f.ForEach("room-z", func(cm ClientMessenger) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Fatalf("expected 1 iteration, got %d", count)
	}
}

func TestFanout_DroppedCount(t *testing.T) {
	f := NewFanout()
	c1 := NewTestClient("c1", nil)
	c2 := NewTestClient("c2", nil)
	f.Add(c1)
	f.Add(c2)

	fill := func(c *Client) {
		for i := 0; i < cap(c.writeCh); i++ {
			if ok := c.Send(map[string]int{"fill": i}); !ok {
				t.Fatalf("%s: fill send %d unexpectedly failed", c.ID(), i)
			}
		}
	}
	fill(c1)
	fill(c2)

	dropped := make([]bool, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		dropped[0] = !c1.Send(map[string]string{"overflow": "c1"})
	}()
	go func() {
		defer wg.Done()
		dropped[1] = !c2.Send(map[string]string{"overflow": "c2"})
	}()
	wg.Wait()

	if !dropped[0] || !dropped[1] {
		t.Fatalf("expected both overflowing sends to be dropped, c1=%v c2=%v", dropped[0], dropped[1])
	}
	if got := c1.DroppedCount(); got != 1 {
		t.Fatalf("c1 DroppedCount = %d, want 1", got)
	}
	if got := c2.DroppedCount(); got != 1 {
		t.Fatalf("c2 DroppedCount = %d, want 1", got)
	}
	if got := f.DroppedCount(); got != 2 {
		t.Fatalf("expected fanout dropped count 2, got %d", got)
	}
}

func TestFanout_RoomCount(t *testing.T) {
	f := NewFanout()
	c := NewTestClient("c1", nil)
	f.Add(c)
	f.Join("room-a", "c1")

	if !f.RoomExists("room-a") {
		t.Fatal("expected room-a to exist")
	}

	// Remove client should clean up the room
	f.Remove("c1")
	if f.RoomExists("room-a") {
		t.Fatal("expected room-a to be cleaned up after client removal")
	}
}

func TestFanout_EmptyRoom_AutoDelete(t *testing.T) {
	f := NewFanout()
	c := NewTestClient("c1", nil)
	f.Add(c)
	f.Join("room-empty", "c1")

	f.Leave("room-empty", "c1")
	if f.RoomExists("room-empty") {
		t.Fatal("expected room to be deleted after last member leaves")
	}
}

func TestFanout_Remove_CleansRooms(t *testing.T) {
	f := NewFanout()
	c := NewTestClient("c1", nil)
	f.Add(c)
	f.Join("room-1", "c1")
	f.Join("room-2", "c1")

	rooms := f.Remove("c1")
	if len(rooms) != 0 {
		t.Fatalf("empty rooms should not be returned, got %v", rooms)
	}
	if f.RoomExists("room-1") || f.RoomExists("room-2") {
		t.Fatal("expected both rooms to be cleaned up")
	}
}

func TestFanout_Remove_ReturnsOnlyNonEmptyRooms(t *testing.T) {
	f := NewFanout()
	c1 := NewTestClient("c1", nil)
	c2 := NewTestClient("c2", nil)
	f.Add(c1)
	f.Add(c2)
	f.Join("room-a", "c1")
	f.Join("room-a", "c2")
	f.Join("room-b", "c1")

	rooms := f.Remove("c1")
	if len(rooms) != 1 || rooms[0] != "room-a" {
		t.Fatalf("expected only non-empty room-a, got %v", rooms)
	}
	if f.RoomExists("room-b") {
		t.Fatal("room-b should be cleaned up")
	}
	if !f.RoomExists("room-a") {
		t.Fatal("room-a should still exist with c2")
	}
}

func TestFanout_ConcurrentAccess(t *testing.T) {
	f := NewFanout()

	ids := make([]string, 10)
	for i := 0; i < 10; i++ {
		ids[i] = string(rune('a' + i))
		f.Add(NewTestClient(ids[i], nil))
		f.Join("room-concurrent", ids[i])
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		f.BroadcastToRoom("room-concurrent", protocol.EventRoomUpdated, map[string]string{"data": "data"})
	}()
	go func() {
		defer wg.Done()
		f.BroadcastToNamespace(protocol.EventRoomCreated, map[string]string{"data": "data"})
	}()
	go func() {
		defer wg.Done()
		f.ForEach("room-concurrent", func(cm ClientMessenger) bool { return true })
	}()
	wg.Wait()

	for _, id := range ids {
		c, ok := f.clients[id]
		if !ok {
			t.Fatalf("client %s missing after concurrent access", id)
		}
		if got := len(c.writeCh); got != 2 {
			t.Fatalf("client %s queued %d messages, want 2", id, got)
		}
	}
}

func newTestClient(id string) *Client {
	return NewTestClient(id, nil)
}

type countingPayload struct {
	marshals int32
}

func (p *countingPayload) MarshalJSON() ([]byte, error) {
	atomic.AddInt32(&p.marshals, 1)
	return []byte(`{"k":"v"}`), nil
}

func receiveOne(t *testing.T, c *Client) []byte {
	t.Helper()
	select {
	case data := <-c.writeCh:
		return data
	default:
		t.Fatalf("client %s did not receive broadcast", c.ID())
		return nil
	}
}

func TestBroadcastToRoom_MarshalsOnce(t *testing.T) {
	f := NewFanout()
	c1 := newTestClient("c1")
	c2 := newTestClient("c2")
	f.Add(c1)
	f.Add(c2)
	f.Join("r1", "c1")
	f.Join("r1", "c2")

	payload := &countingPayload{}
	f.BroadcastToRoom("r1", protocol.EventRoomUpdated, payload)

	if got := atomic.LoadInt32(&payload.marshals); got != 1 {
		t.Fatalf("broadcast must marshal the payload exactly once, got %d", got)
	}

	first := receiveOne(t, c1)
	second := receiveOne(t, c2)
	if string(first) != string(second) {
		t.Fatalf("room members received different bytes: %q vs %q", first, second)
	}

	var envelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(first, &envelope); err != nil {
		t.Fatalf("unmarshal broadcast envelope: %v", err)
	}
	if envelope.Event != protocol.EventRoomUpdated || string(envelope.Data) != `{"k":"v"}` {
		t.Fatalf("unexpected envelope event=%q data=%s", envelope.Event, envelope.Data)
	}
}

func TestFanout_CloseAll(t *testing.T) {
	f := NewFanout()
	c1 := newTestClient("c1")
	c2 := newTestClient("c2")
	f.Add(c1)
	f.Add(c2)
	f.CloseAll()
	if atomic.LoadUint64(&c1.closedCount) != 1 || atomic.LoadUint64(&c2.closedCount) != 1 {
		t.Fatalf("expected both closed, c1=%d c2=%d", c1.closedCount, c2.closedCount)
	}
}
