package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"GOSpeak/internal/plugin/gen"
	"GOSpeak/internal/protocol"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultHookTimeout   = 5 * time.Second
	defaultHookFailLimit = 10
	maxHookPayloadBytes  = 8 * 1024
)

var hookPayloadFields = map[string]bool{
	"room": true, "room_uuid": true, "domain_uuid": true,
	"author_uuid": true, "author_id": true, "author_name": true,
	"identity": true, "id": true, "user_uuid": true,
	"is_muted": true, "is_mic_muted": true, "joined_at": true,
	"at": true, "name": true, "display_name": true,
}

type hookTarget struct {
	pluginID  string
	endpoint  string
	hookTypes map[string]bool
	client    gen.PluginHookServiceClient
	close     func() error
	failures  int
	disabled  bool
}

// HookRegistry 保存插件 Register 上报的 hook_endpoint 与订阅事件。
type HookRegistry struct {
	mu      sync.RWMutex
	targets map[string]*hookTarget
	dialer  func(endpoint string) (gen.PluginHookServiceClient, func() error, error)
}

func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		targets: make(map[string]*hookTarget),
		dialer:  dialHookEndpoint,
	}
}

func (r *HookRegistry) Register(pluginID, endpoint string, hookTypes []string) {
	types := make(map[string]bool, len(hookTypes))
	for _, event := range hookTypes {
		types[event] = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.targets[pluginID]; old != nil && old.close != nil {
		_ = old.close()
	}
	r.targets[pluginID] = &hookTarget{pluginID: pluginID, endpoint: endpoint, hookTypes: types}
}

func dialHookEndpoint(endpoint string) (gen.PluginHookServiceClient, func() error, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return gen.NewPluginHookServiceClient(conn), conn.Close, nil
}

// HookDispatcher 订阅白名单事件并推送给订阅插件；连续失败达阈值时熔断 disable。
type HookDispatcher struct {
	registry      *HookRegistry
	disable       func(pluginID string)
	allowedEvents map[string]bool
	timeout       time.Duration
	failureLimit  int
}

func NewHookDispatcher(registry *HookRegistry, disable func(pluginID string)) *HookDispatcher {
	return &HookDispatcher{
		registry: registry,
		disable:  disable,
		allowedEvents: map[string]bool{
			protocol.EventMessageCreated: true,
			protocol.EventMemberJoined:   true,
			protocol.EventMemberLeft:     true,
		},
		timeout:      defaultHookTimeout,
		failureLimit: defaultHookFailLimit,
	}
}

func (d *HookDispatcher) Dispatch(ctx context.Context, event string, payload interface{}) {
	if !d.allowedEvents[event] {
		return
	}
	raw, err := trimHookPayload(payload)
	if err != nil {
		return
	}
	d.registry.mu.RLock()
	targets := make([]*hookTarget, 0, len(d.registry.targets))
	for _, t := range d.registry.targets {
		if t.hookTypes[event] && !t.disabled {
			targets = append(targets, t)
		}
	}
	d.registry.mu.RUnlock()
	for _, target := range targets {
		d.push(ctx, target, event, raw)
	}
}

func (d *HookDispatcher) push(ctx context.Context, target *hookTarget, event string, payload []byte) {
	if target.client == nil {
		client, closeFn, err := d.registry.dialer(target.endpoint)
		if err != nil {
			d.recordFailure(target)
			return
		}
		d.registry.mu.Lock()
		target.client = client
		target.close = closeFn
		d.registry.mu.Unlock()
	}
	callCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	req := &gen.HandleEventRequest{
		PluginId:    target.pluginID,
		EventType:   event,
		PayloadJson: string(payload),
		RequestId:   newHookRequestID(),
	}
	if _, err := target.client.HandleEvent(callCtx, req); err != nil {
		d.recordFailure(target)
		return
	}
	d.registry.mu.Lock()
	target.failures = 0
	d.registry.mu.Unlock()
}

func (d *HookDispatcher) recordFailure(target *hookTarget) {
	d.registry.mu.Lock()
	target.failures++
	tripped := target.failures >= d.failureLimit && !target.disabled
	if tripped {
		target.disabled = true
	}
	d.registry.mu.Unlock()
	if tripped && d.disable != nil {
		d.disable(target.pluginID)
	}
}

func trimHookPayload(payload interface{}) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var src map[string]interface{}
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, err
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		if hookPayloadFields[k] {
			dst[k] = v
		}
	}
	out, err := json.Marshal(dst)
	if err != nil {
		return nil, err
	}
	if len(out) > maxHookPayloadBytes {
		return nil, fmt.Errorf("hook payload too large")
	}
	return out, nil
}

var hookSeq uint64

func newHookRequestID() string {
	return fmt.Sprintf("hook-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&hookSeq, 1))
}

func (r *HookRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for _, t := range r.targets {
		if t != nil && t.close != nil {
			if err := t.close(); err != nil && firstErr == nil {
				firstErr = err
			}
			t.close = nil
			t.client = nil
		}
	}
	return firstErr
}

func (d *HookDispatcher) Close() error {
	return d.registry.Close()
}
