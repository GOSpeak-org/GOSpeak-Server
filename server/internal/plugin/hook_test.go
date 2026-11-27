package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"GOSpeak/internal/plugin/gen"
	"GOSpeak/internal/protocol"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type hookTestServer struct {
	gen.UnimplementedPluginHookServiceServer
	mu     sync.Mutex
	events []*gen.HandleEventRequest
	block  bool
	hold   <-chan struct{}
	calls  int32
}

func (s *hookTestServer) HandleEvent(ctx context.Context, req *gen.HandleEventRequest) (*gen.HandleEventResponse, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.hold != nil {
		select {
		case <-s.hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.block {
		time.Sleep(20 * time.Millisecond)
		return nil, fmt.Errorf("hook failure")
	}
	s.mu.Lock()
	s.events = append(s.events, req)
	s.mu.Unlock()
	return &gen.HandleEventResponse{}, nil
}

func (s *hookTestServer) snapshot() []*gen.HandleEventRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*gen.HandleEventRequest(nil), s.events...)
}

func newHookTestRegistry(t *testing.T, srv *hookTestServer) (*HookRegistry, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	g := grpc.NewServer()
	gen.RegisterPluginHookServiceServer(g, srv)
	go func() { _ = g.Serve(lis) }()
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	reg := NewHookRegistry()
	conn.Connect()
	reg.dialer = func(string) (gen.PluginHookServiceClient, func() error, error) {
		return gen.NewPluginHookServiceClient(conn), conn.Close, nil
	}
	reg.Register("com.example.audit", "passthrough:///bufnet", []string{protocol.EventMessageCreated, protocol.EventMemberJoined})
	return reg, func() { _ = conn.Close(); g.Stop(); _ = lis.Close() }
}
func TestHook_DispatchAllowlistedEvent(t *testing.T) {
	srv := &hookTestServer{}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	d := NewHookDispatcher(reg, nil)

	d.Dispatch(context.Background(), protocol.EventMessageCreated, map[string]interface{}{
		"room_uuid": "room-1", "author_uuid": "user-1", "content": "secret body",
	})

	events := srv.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 hook event, got %d", len(events))
	}
	if events[0].GetPluginId() != "com.example.audit" || events[0].GetEventType() != protocol.EventMessageCreated {
		t.Fatalf("unexpected request: %+v", events[0])
	}
	if events[0].GetRequestId() == "" {
		t.Fatal("request_id must be set")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(events[0].GetPayloadJson()), &payload); err != nil {
		t.Fatalf("payload_json invalid: %v", err)
	}
	if payload["room_uuid"] != "room-1" || payload["author_uuid"] != "user-1" {
		t.Fatalf("payload lost identity fields: %v", payload)
	}
	if _, ok := payload["content"]; ok {
		t.Fatal("message body must be stripped from hook payload")
	}
}

func TestHook_SensitiveEventNotDispatched(t *testing.T) {
	srv := &hookTestServer{}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	d := NewHookDispatcher(reg, nil)

	for _, event := range []string{"auth:token-rotated", "audit:recorded", "jwt:rotated"} {
		d.Dispatch(context.Background(), event, map[string]interface{}{"x": "y"})
	}
	if got := len(srv.snapshot()); got != 0 {
		t.Fatalf("sensitive events must not be dispatched, got %d", got)
	}
}

func TestHook_UnsubscribedEventNotDispatched(t *testing.T) {
	srv := &hookTestServer{}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	d := NewHookDispatcher(reg, nil)

	d.Dispatch(context.Background(), protocol.EventMemberLeft, map[string]interface{}{"room": "r"})
	if got := len(srv.snapshot()); got != 0 {
		t.Fatalf("unsubscribed event must not be dispatched, got %d", got)
	}
}

func TestHook_CircuitBreakerAfterTenFailures(t *testing.T) {
	srv := &hookTestServer{block: true}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	var disabled []string
	var mu sync.Mutex
	d := NewHookDispatcher(reg, func(pluginID string) {
		mu.Lock()
		disabled = append(disabled, pluginID)
		mu.Unlock()
	})
	d.timeout = 200 * time.Millisecond
	d.failureLimit = 10

	for i := 0; i < 10; i++ {
		d.Dispatch(context.Background(), protocol.EventMemberJoined, map[string]interface{}{"room": "r"})
	}
	mu.Lock()
	defer mu.Unlock()
	if len(disabled) != 1 || disabled[0] != "com.example.audit" {
		t.Fatalf("expected one disable callback for plugin, got %v", disabled)
	}
}

func TestHook_SuccessResetsFailureCount(t *testing.T) {
	srv := &hookTestServer{}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	var disabled []string
	d := NewHookDispatcher(reg, func(pluginID string) { disabled = append(disabled, pluginID) })

	for i := 0; i < 9; i++ {
		srv.block = true
		d.Dispatch(context.Background(), protocol.EventMemberJoined, map[string]interface{}{"room": "r"})
	}
	srv.block = false
	d.Dispatch(context.Background(), protocol.EventMemberJoined, map[string]interface{}{"room": "r"})
	srv.block = true
	for i := 0; i < 9; i++ {
		d.Dispatch(context.Background(), protocol.EventMemberJoined, map[string]interface{}{"room": "r"})
	}
	if len(disabled) != 0 {
		t.Fatalf("success should reset consecutive failure count, disabled=%v", disabled)
	}
}

func TestHook_TimeoutUsesFiveSecondsDefault(t *testing.T) {
	if got := NewHookDispatcher(NewHookRegistry(), nil).timeout; got != 5*time.Second {
		t.Fatalf("default hook timeout = %v, want 5s", got)
	}
}

func TestHook_DispatchHonoursConfiguredTimeout(t *testing.T) {
	hold := make(chan struct{})
	defer close(hold)
	srv := &hookTestServer{hold: hold}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	d := NewHookDispatcher(reg, nil)
	d.timeout = 120 * time.Millisecond
	d.failureLimit = 100

	start := time.Now()
	d.Dispatch(context.Background(), protocol.EventMessageCreated, map[string]interface{}{"room_uuid": "room-1"})
	elapsed := time.Since(start)
	if elapsed < d.timeout {
		t.Fatalf("Dispatch returned before deadline: elapsed=%v timeout=%v", elapsed, d.timeout)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Dispatch blocked too long: elapsed=%v", elapsed)
	}
	if got := atomic.LoadInt32(&srv.calls); got != 1 {
		t.Fatalf("hook server calls = %d, want 1", got)
	}
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	target := reg.targets["com.example.audit"]
	if target == nil || target.failures != 1 {
		t.Fatalf("expected failure count 1 after deadline, target=%+v", target)
	}
}

func TestHook_DispatchTimeoutIsDeadlineExceeded(t *testing.T) {
	srv := &blockingHookClient{}
	reg := NewHookRegistry()
	reg.dialer = func(string) (gen.PluginHookServiceClient, func() error, error) {
		return srv, func() error { return nil }, nil
	}
	reg.Register("com.example.plugin", "unused", []string{protocol.EventMessageCreated})
	d := NewHookDispatcher(reg, nil)
	d.timeout = 80 * time.Millisecond

	d.Dispatch(context.Background(), protocol.EventMessageCreated, map[string]interface{}{"room_uuid": "r"})
	if !errors.Is(srv.err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded from HandleEvent, got %v", srv.err)
	}
}

type blockingHookClient struct {
	err error
}

func (c *blockingHookClient) HandleEvent(ctx context.Context, _ *gen.HandleEventRequest, _ ...grpc.CallOption) (*gen.HandleEventResponse, error) {
	<-ctx.Done()
	c.err = ctx.Err()
	return nil, c.err
}

func TestHook_CloseClosesCachedConnection(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	g := grpc.NewServer()
	gen.RegisterPluginHookServiceServer(g, &hookTestServer{})
	go func() { _ = g.Serve(lis) }()
	defer func() { g.Stop(); _ = lis.Close() }()
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	conn.Connect()
	closed := 0
	reg := NewHookRegistry()
	reg.dialer = func(string) (gen.PluginHookServiceClient, func() error, error) {
		return gen.NewPluginHookServiceClient(conn), func() error { closed++; return conn.Close() }, nil
	}
	reg.Register("com.example.audit", "passthrough:///bufnet", []string{protocol.EventMessageCreated})
	d := NewHookDispatcher(reg, nil)
	d.Dispatch(context.Background(), protocol.EventMessageCreated, map[string]interface{}{"room_uuid": "r"})
	reg.mu.RLock()
	target := reg.targets["com.example.audit"]
	reg.mu.RUnlock()
	if target == nil || target.client == nil {
		t.Fatal("target client must be cached before Close")
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if closed != 1 {
		t.Fatalf("cached conn close count = %d, want 1", closed)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close must be idempotent: %v", err)
	}
	if closed != 1 {
		t.Fatalf("idempotent Close must not close twice, got %d", closed)
	}
}

func TestHook_OversizedPayloadRejected(t *testing.T) {
	srv := &hookTestServer{}
	reg, cleanup := newHookTestRegistry(t, srv)
	defer cleanup()
	d := NewHookDispatcher(reg, nil)

	d.Dispatch(context.Background(), protocol.EventMessageCreated, map[string]interface{}{
		"room_uuid": strings.Repeat("x", maxHookPayloadBytes+1),
	})
	if got := len(srv.snapshot()); got != 0 {
		t.Fatalf("oversized payload must not be dispatched, got %d", got)
	}
	if _, err := trimHookPayload(map[string]interface{}{"room_uuid": strings.Repeat("x", maxHookPayloadBytes+1)}); err == nil {
		t.Fatal("oversized payload must return error")
	}
}
