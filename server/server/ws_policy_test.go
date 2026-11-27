package server

import (
	"errors"
	"testing"

	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/policy"
	"GOSpeak/internal/protocol"
	"GOSpeak/internal/ws"
)

type fakeWSClient struct {
	id     string
	claims *pkg.Claims
}

func (c *fakeWSClient) ID() string                                        { return c.id }
func (c *fakeWSClient) Claims() *pkg.Claims                               { return c.claims }
func (c *fakeWSClient) Send(v interface{}) bool                           { return true }
func (c *fakeWSClient) SendACK(id, event string, data interface{})        {}
func (c *fakeWSClient) SendErrorACK(id, event string, code int, m string) {}
func (c *fakeWSClient) Close()                                            {}

// 回归「策略契约 ↔ 传输通道」：/ws 绕过 Gin 的 BanCheck，
// 逐事件判决必须在每条上行消息上重新执行，不能只在建连时判一次。
func TestNewWSPolicyGuard_BanRoleRejectedOnEveryEvent(t *testing.T) {
	guard := newWSPolicyGuard(policy.NewEngine(), func(clientID string) string { return "dom-1" })
	client := &fakeWSClient{
		id: "conn-1",
		claims: &pkg.Claims{
			UserUUID:  "banned-1",
			Username:  "banned",
			Role:      "ban",
			TokenType: pkg.AccessTokenType,
		},
	}

	for _, event := range []string{protocol.EventMessageSend, protocol.EventPrivateSend, protocol.EventTyping} {
		d := guard(client, ws.Message{Event: event})
		if d.Code != pkg.USER_BANNED || !d.Terminate {
			t.Errorf("event %q for ban role must be USER_BANNED + terminate, got %+v", event, d)
		}
	}
}

func TestNewWSPolicyGuard_NormalUserUnaffected(t *testing.T) {
	guard := newWSPolicyGuard(policy.NewEngine(), func(string) string { return "" })
	client := &fakeWSClient{id: "conn-1", claims: &pkg.Claims{UserUUID: "user-1", Role: "user"}}

	for _, event := range []string{protocol.EventPrivateSend, protocol.EventMessageSend, protocol.EventTyping} {
		if d := guard(client, ws.Message{Event: event}); d.Code != pkg.SUCCESS {
			t.Errorf("event %q must pass for normal user, got %+v", event, d)
		}
	}
}

func TestNewWSPolicyGuard_MissingClaimsRejected(t *testing.T) {
	guard := newWSPolicyGuard(policy.NewEngine(), func(string) string { return "" })
	if d := guard(&fakeWSClient{id: "conn-1"}, ws.Message{Event: protocol.EventTyping}); d.Code != pkg.TOKEN_NOT_EXIST {
		t.Fatalf("connection without claims must be rejected, got %+v", d)
	}
}

func TestConnectPolicy_DefaultPasswordGate(t *testing.T) {
	engine := policy.NewEngine()
	admin := &pkg.Claims{UserUUID: "admin-1", Username: "admin", Role: "admin", TokenType: pkg.AccessTokenType}

	// 情形一：checker 报错必须 fail-closed，不能让默认口令会话建连。
	middleware.Configure(middleware.Dependencies{
		DefaultPasswordChecker: func(string) (bool, error) { return false, errors.New("db down") },
	})
	cfg := &ws.UpgraderConfig{}
	installWSPolicy(engine, ws.NewHandlerRegistry(), cfg, nil)
	if code, _ := cfg.ConnectPolicy(admin); code != pkg.INTERNAL_ERROR {
		t.Errorf("checker error must fail-closed with INTERNAL_ERROR, got %d", code)
	}

	// 情形二：仍需改密 → FORBIDDEN。
	middleware.Configure(middleware.Dependencies{
		DefaultPasswordChecker: func(string) (bool, error) { return true, nil },
	})
	if code, _ := cfg.ConnectPolicy(admin); code != pkg.FORBIDDEN {
		t.Errorf("default password admin must be FORBIDDEN, got %d", code)
	}

	// 情形三：已改密 → 放行到 ban 判决（admin 非 ban，SUCCESS）。
	middleware.Configure(middleware.Dependencies{
		DefaultPasswordChecker: func(string) (bool, error) { return false, nil },
	})
	if code, _ := cfg.ConnectPolicy(admin); code != pkg.SUCCESS {
		t.Errorf("changed-password admin must pass, got %d", code)
	}

	// 非 admin 不查 checker。
	user := &pkg.Claims{UserUUID: "u1", Username: "u", Role: "user", TokenType: pkg.AccessTokenType}
	if code, _ := cfg.ConnectPolicy(user); code != pkg.SUCCESS {
		t.Errorf("non-admin must pass regardless of password gate, got %d", code)
	}
}
