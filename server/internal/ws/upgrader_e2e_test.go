package ws

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	nhooyrws "nhooyr.io/websocket"

	"GOSpeak/internal/pkg"
)

func wsDialOptionsWithCookie(token string) *nhooyrws.DialOptions {
	return &nhooyrws.DialOptions{
		Subprotocols: []string{"gospeak"},
		HTTPHeader: http.Header{
			"Cookie": {accessCookieName + "=" + token},
		},
	}
}

const accessCookieName = "gospeak_token"

func TestNewConnID_UniquePerConnection(t *testing.T) {
	first := newConnID("uuid-1", "user-1")
	second := newConnID("uuid-1", "user-1")
	if first == second {
		t.Fatalf("expected unique connection IDs, got %q", first)
	}
	if !strings.HasPrefix(first, "uuid-1-") || !strings.HasPrefix(second, "uuid-1-") {
		t.Fatalf("expected user prefix, got %q and %q", first, second)
	}
	if got := newConnID("", "legacy-user"); !strings.HasPrefix(got, "legacy-user-") {
		t.Fatalf("expected username fallback prefix, got %q", got)
	}
}

// TestUpgrader_E2E_Lifecycle verifies the full ws.Client lifecycle through Upgrader:
// upgrade → auth → fanout.Add → OnConnect → read loop → disconnect → fanout.Remove.
func TestUpgrader_E2E_Lifecycle(t *testing.T) {
	token, err := pkg.GenerateToken("e2e-user", "E2E User", "e2e-uuid", "user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	connected := make(chan *Client, 1)
	disconnected := make(chan *Client, 1)
	var mu sync.Mutex
	var receivedData string
	handlerCalled := make(chan struct{})

	handler := newPermissiveRegistry()
	handler.Handle("ping", func(c ClientMessenger, data string) {
		mu.Lock()
		receivedData = data
		mu.Unlock()
		c.SendACK("ack-1", "pong", data)
		close(handlerCalled)
	})

	fanout := NewFanout()
	upgrader := NewUpgrader(UpgraderConfig{
		Fanout:  fanout,
		Handler: handler,
		OnConnect: func(c *Client) {
			connected <- c
		},
		OnDisconnect: func(c *Client) {
			disconnected <- c
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.ServeHTTP(w, r)
	}))
	defer server.Close()

	wsURL := "ws://" + server.Listener.Addr().String() + "/ws"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, resp, err := nhooyrws.Dial(ctx, wsURL, wsDialOptionsWithCookie(token))
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("Dial: %v (status=%d)", err, status)
	}
	defer conn.Close(nhooyrws.StatusNormalClosure, "test done")

	// Wait for OnConnect
	var connectedID string
	select {
	case client := <-connected:
		connectedID = client.ID()
		if !strings.HasPrefix(connectedID, "e2e-uuid-") {
			t.Fatalf("expected client ID prefix 'e2e-uuid-', got %q", connectedID)
		}
		if fanout.GetClient(connectedID) == nil {
			t.Fatal("client should be registered in fanout")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnConnect")
	}

	// Send a message
	err = conn.Write(ctx, nhooyrws.MessageText, []byte(`{"id":"req-1","event":"ping","data":"hello"}`))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Read ACK response
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read ACK: %v", err)
	}
	if len(msg) == 0 {
		t.Fatal("expected non-empty ACK response")
	}

	// Verify handler was called
	select {
	case <-handlerCalled:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for handler")
	}

	mu.Lock()
	if receivedData != `"hello"` {
		t.Fatalf("expected handler to receive 'hello', got %q", receivedData)
	}
	mu.Unlock()

	// Close connection from client side to trigger server cleanup
	conn.Close(nhooyrws.StatusNormalClosure, "test done")

	// Verify OnDisconnect lifecycle hook fired；事件到达即说明服务端已处理
	// close frame 并完成清理，用可观察事件替代固定 sleep。
	select {
	case c := <-disconnected:
		if !strings.HasPrefix(c.ID(), "e2e-uuid-") {
			t.Fatalf("expected disconnect client ID prefix 'e2e-uuid-', got %q", c.ID())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for OnDisconnect")
	}

	removalDeadline := time.Now().Add(2 * time.Second)
	for fanout.GetClient(connectedID) != nil {
		if !time.Now().Before(removalDeadline) {
			t.Fatal("client must be removed from fanout after disconnect")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestUpgrader_E2E_Unauthenticated verifies that requests without token get 401.
func TestUpgrader_E2E_Unauthenticated(t *testing.T) {
	fanout := NewFanout()
	upgrader := NewUpgrader(UpgraderConfig{Fanout: fanout})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.ServeHTTP(w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, resp, err := nhooyrws.Dial(ctx, "ws://"+server.Listener.Addr().String()+"/ws", &nhooyrws.DialOptions{})
	if conn != nil {
		conn.CloseNow()
		t.Fatal("unauthenticated dial must not complete the upgrade")
	}
	if err == nil {
		t.Fatal("expected unauthenticated dial to be rejected")
	}

	if resp != nil {
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected handshake rejection with 401, got %d", resp.StatusCode)
		}
		return
	}

	var netErr net.Error
	if !errors.As(err, &netErr) {
		t.Fatalf("expected transport-level dial error, got %T: %v", err, err)
	}
}

// TestUpgrader_E2E_OnConnectPanicClosesClient verifies an accepted connection is
// explicitly closed when OnConnect panics before the read loop starts.
func TestUpgrader_E2E_OnConnectPanicClosesClient(t *testing.T) {
	token, err := pkg.GenerateToken("e2e-user", "E2E User", "e2e-uuid", "user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	connected := make(chan *Client, 1)
	fanout := NewFanout()
	upgrader := NewUpgrader(UpgraderConfig{
		Fanout: fanout,
		OnConnect: func(c *Client) {
			connected <- c
			panic("test panic")
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.ServeHTTP(w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := nhooyrws.Dial(ctx, "ws://"+server.Listener.Addr().String()+"/ws", wsDialOptionsWithCookie(token))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close(nhooyrws.StatusNormalClosure, "test done")

	var client *Client
	select {
	case client = <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnConnect")
	}

	select {
	case <-client.ctx.Done():
		// connection closed by upgrader defer
	case <-time.After(2 * time.Second):
		t.Fatal("expected client to be closed after OnConnect panic")
	}
	if client != nil && fanout.clients[client.ID()] != nil {
		t.Fatal("expected client removed from fanout after OnConnect panic")
	}
}

// TestUpgrader_E2E_LargeMessageWithin64K verifies a message between nhooyr's default
// 32KB limit and the configured 64KB limit is accepted and does not kill the loop.
func TestUpgrader_E2E_LargeMessageWithin64K(t *testing.T) {
	token, err := pkg.GenerateToken("e2e-user", "E2E User", "e2e-uuid", "user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	handler := newPermissiveRegistry()
	handler.Handle("ping", func(c ClientMessenger, data string) {
		c.SendACK("ack-1", "pong", data)
	})

	upgrader := NewUpgrader(UpgraderConfig{
		Fanout:  NewFanout(),
		Handler: handler,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.ServeHTTP(w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := nhooyrws.Dial(ctx, "ws://"+server.Listener.Addr().String()+"/ws", wsDialOptionsWithCookie(token))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close(nhooyrws.StatusNormalClosure, "test done")

	payload := strings.Repeat("x", 48*1024)
	msg := `{"id":"req-1","event":"ping","data":"` + payload + `"}`
	if err := conn.Write(ctx, nhooyrws.MessageText, []byte(msg)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The server echoes the payload back; client must raise its own read limit too.
	conn.SetReadLimit(64 << 10)
	_, resp, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read ACK for 48KB message: %v", err)
	}
	if len(resp) == 0 {
		t.Fatal("expected non-empty ACK response")
	}
}
