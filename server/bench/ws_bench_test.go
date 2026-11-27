package bench

import (
	"fmt"
	"testing"

	"GOSpeak/internal/pkg"
	"GOSpeak/internal/protocol"
	"GOSpeak/internal/ws"
)

const (
	benchRoom        = "testroom"
	benchClientCount = 100
	benchRotateEvery = 32
)

type mockClient struct {
	id string
}

func (m *mockClient) ID() string                                              { return m.id }
func (m *mockClient) Claims() *pkg.Claims                                     { return nil }
func (m *mockClient) Send(v interface{}) bool                                 { return true }
func (m *mockClient) SendACK(id, event string, data interface{})              {}
func (m *mockClient) SendErrorACK(id, event string, code int, message string) {}
func (m *mockClient) Close()                                                  {}

func newBroadcastClients(fanout *ws.Fanout, generation int) []*ws.Client {
	clients := make([]*ws.Client, benchClientCount)
	for i := 0; i < benchClientCount; i++ {
		id := fmt.Sprintf("c%d-%d", generation, i)
		clients[i] = ws.NewTestClient(id, nil)
		fanout.Add(clients[i])
		fanout.Join(benchRoom, id)
	}
	return clients
}

func BenchmarkBroadcastToRoom(b *testing.B) {
	fanout := ws.NewFanout()
	generation := 0
	clients := newBroadcastClients(fanout, generation)
	var dropped uint64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i > 0 && i%benchRotateEvery == 0 {
			b.StopTimer()
			for _, c := range clients {
				dropped += c.DroppedCount()
				fanout.Remove(c.ID())
			}
			generation++
			clients = newBroadcastClients(fanout, generation)
			b.StartTimer()
		}
		fanout.BroadcastToRoom(benchRoom, protocol.EventRoomUpdated, map[string]string{"data": "payload"})
	}
	b.StopTimer()

	for _, c := range clients {
		dropped += c.DroppedCount()
	}
	if dropped != 0 {
		b.Fatalf("broadcast sink overflowed: %d messages dropped", dropped)
	}
}

func BenchmarkFanoutJoinLeave(b *testing.B) {
	fanout := ws.NewFanout()
	clients := make([]*ws.Client, 50)

	for i := 0; i < 50; i++ {
		id := string(rune('a' + i))
		clients[i] = ws.NewTestClient(id, nil)
		fanout.Add(clients[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room := "room-" + string(rune('A'+(i%26)))
		for _, c := range clients {
			fanout.Join(room, c.ID())
		}
		for _, c := range clients {
			fanout.Leave(room, c.ID())
		}
	}
}

func BenchmarkMessageDispatch(b *testing.B) {
	reg := ws.NewHandlerRegistry()
	reg.SetPolicyGuard(func(ws.ClientMessenger, ws.Message) ws.PolicyDecision {
		return ws.PolicyDecision{Code: pkg.SUCCESS}
	})
	reg.Handle(protocol.EventMessageSend, func(c ws.ClientMessenger, data string) {})

	msg := ws.Message{
		ID:    "bench-1",
		Event: protocol.EventMessageSend,
		Data:  []byte(`{"key":"value","nested":{"a":1}}`),
	}

	tc := &mockClient{id: "bench-client"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reg.Dispatch(tc, msg)
	}
}
