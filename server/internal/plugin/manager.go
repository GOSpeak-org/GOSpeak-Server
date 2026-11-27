package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"GOSpeak/internal/logger"
	"GOSpeak/internal/model"
	"GOSpeak/internal/plugin/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	noNodePanic        = "plugin manager: no installed node runtime; install one in the admin page"
	credentialEnvName  = "GOSPEAK_PLUGIN_CREDENTIAL"
	stateEnvName       = "GOSPEAK_PLUGIN_STATE_DIR"
	defaultHeartbeat   = 30 * time.Second
	defaultStopTimeout = 10 * time.Second
	heartbeatFailWarn  = 3
	heartbeatFailStop  = 5
	maxBackoff         = 60 * time.Second
)

type instanceStatus int

const (
	instanceRunning instanceStatus = iota
	instanceBackoff
	instanceStopped
)

type RegisterFunc func(ctx context.Context, req *gen.RegisterRequest) (*gen.RegisterResponse, error)

type managerCmd interface {
	Run() error
	Start() error
	Wait() error
	ProcessKill() error
}

type execCmd struct{ cmd *exec.Cmd }

func (c *execCmd) Run() error   { return c.cmd.Run() }
func (c *execCmd) Start() error { return c.cmd.Start() }
func (c *execCmd) Wait() error  { return c.cmd.Wait() }
func (c *execCmd) ProcessKill() error {
	if c.cmd.Process == nil {
		return nil
	}
	return c.cmd.Process.Kill()
}

type managerDeps struct {
	runtimeBinary func() string
	command       func(name string, args ...string) managerCmd
	dial          func(endpoint string) (gen.RegistrationServiceClient, func() error, error)
	runtimeDial   func(endpoint string) (gen.RuntimeServiceClient, func() error, error)
	runtimeStart  func(ctx context.Context, binary, root string) (string, func() error, error)
}

type pluginInstance struct {
	pluginID  string
	metadata  PluginMetadata
	dir       string
	credPath  string
	client    gen.RegistrationServiceClient
	closeConn func() error
	server    *grpc.Server
	listener  net.Listener
	cmd       managerCmd
	cancel    context.CancelFunc
	status    instanceStatus
	failures  int
	backoff   time.Duration
	registry  *gen.RegisterRequest
	stopOnce  sync.Once
}

type registrationServer struct {
	gen.UnimplementedRegistrationServiceServer
	m           *Manager
	pluginID    string
	approved    bool
	heartbeat   chan struct{}
	apiEndpoint string
}

func (s *registrationServer) Register(_ context.Context, req *gen.RegisterRequest) (*gen.RegisterResponse, error) {
	if req.GetPluginId() != s.pluginID {
		return nil, fmt.Errorf("plugin id mismatch: %q", req.GetPluginId())
	}
	s.m.mu.Lock()
	inst := s.m.instances[s.pluginID]
	s.m.mu.Unlock()
	if inst == nil {
		return nil, fmt.Errorf("plugin %s is not running", s.pluginID)
	}
	resp := &gen.RegisterResponse{ApiEndpoint: s.apiEndpoint}
	confirmed := make([]string, 0, len(req.GetPages()))
	for _, page := range req.GetPages() {
		if err := s.m.confirmPage(req.GetPluginId(), page); err != nil {
			logger.WithComponent("plugin:"+s.pluginID).Warnf("reject page %s: %v", page.GetRoutePath(), err)
			continue
		}
		confirmed = append(confirmed, page.GetRoutePath())
	}
	s.m.mu.Lock()
	inst.registry = req
	md := inst.metadata
	s.m.mu.Unlock()
	s.m.upsertRegistration(s.pluginID, md, "")
	s.approved = true
	select {
	case s.heartbeat <- struct{}{}:
	default:
	}
	resp.ConfirmedPages = confirmed
	return resp, nil
}

func (s *registrationServer) Heartbeat(_ context.Context, _ *gen.HeartbeatRequest) (*gen.HeartbeatResponse, error) {
	select {
	case s.heartbeat <- struct{}{}:
	default:
	}
	return &gen.HeartbeatResponse{}, nil
}

type Manager struct {
	mu                sync.Mutex
	rootDir           string
	register          RegisterFunc
	instances         map[string]*pluginInstance
	registrations     map[string]bool
	started           bool
	stopTime          time.Time
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	shutdownTimeout   time.Duration
	deps              managerDeps
	lastInstall       map[string]InstallOptions
	cancel            context.CancelFunc
	confirmer         PageConfirmer
	runtime           *runtimeClient
	runtimeCancel     context.CancelFunc
	noRuntime         bool
	registrationStore RegistrationStore
	stats             statsCounters
}

type PageConfirmer interface {
	Confirm(pluginID, mode, routePath, entryURL string) error
}

func NewManager(rootDir string, register RegisterFunc) *Manager {
	if rootDir == "" {
		rootDir = "/app/plugins"
	}
	m := &Manager{
		rootDir:           rootDir,
		register:          register,
		instances:         make(map[string]*pluginInstance),
		registrations:     make(map[string]bool),
		lastInstall:       make(map[string]InstallOptions),
		heartbeatInterval: defaultHeartbeat,
		heartbeatTimeout:  5 * time.Second,
		shutdownTimeout:   defaultStopTimeout,
	}
	m.deps.runtimeBinary = func() string { return filepath.Join(m.binDir(), "gospeak.runtime") }
	m.deps.command = func(name string, args ...string) managerCmd { return &execCmd{cmd: exec.Command(name, args...)} }
	m.deps.dial = dialRegistration
	m.deps.runtimeDial = dialRuntime
	m.deps.runtimeStart = startRuntimeProcess
	m.loadRegistrations()
	return m
}

func (m *Manager) SetPageConfirmer(c PageConfirmer) { m.confirmer = c }

func (m *Manager) SetRegistrationStore(store RegistrationStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registrationStore = store
}

func (m *Manager) upsertRegistration(pluginID string, meta PluginMetadata, source string) {
	m.mu.Lock()
	store := m.registrationStore
	if source == "" {
		source = m.lastInstall[pluginID].Source
	}
	enabled, registered := m.registrations[pluginID]
	m.mu.Unlock()
	if store == nil {
		return
	}
	reg := model.NewPluginRegistration(pluginID, source, meta.Version)
	if registered {
		reg.Enabled = enabled
	}
	if err := store.Upsert(&reg); err != nil {
		logger.WithComponent("plugin:"+pluginID).Warnf("persist registration: %v", err)
	}
}

func (m *Manager) setRegistrationEnabled(pluginID string, enabled bool) {
	m.mu.Lock()
	store := m.registrationStore
	m.mu.Unlock()
	if store == nil {
		return
	}
	if err := store.SetEnabled(pluginID, enabled); err != nil {
		logger.WithComponent("plugin:"+pluginID).Warnf("persist enabled state: %v", err)
	}
}

func (m *Manager) deleteRegistration(pluginID string) {
	m.mu.Lock()
	store := m.registrationStore
	m.mu.Unlock()
	if store == nil {
		return
	}
	if err := store.Delete(pluginID); err != nil {
		logger.WithComponent("plugin:"+pluginID).Warnf("delete registration: %v", err)
	}
}

func dialRegistration(endpoint string) (gen.RegistrationServiceClient, func() error, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return gen.NewRegistrationServiceClient(conn), conn.Close, nil
}

func (m *Manager) binDir() string             { return filepath.Join(m.rootDir, "bin") }
func (m *Manager) stateDir() string           { return filepath.Join(m.rootDir, "runtime", "state") }
func (m *Manager) credDir() string            { return filepath.Join(m.rootDir, "runtime", "credentials") }
func (m *Manager) pluginDir(id string) string { return filepath.Join(m.binDir(), id) }

func (m *Manager) registrationPath() string { return registrationStatePath(m.rootDir) }

func (m *Manager) loadRegistrations() {
	data, err := os.ReadFile(m.registrationPath())
	if err != nil {
		return
	}
	entries, err := parseRegistrationState(data)
	if err != nil {
		logger.WithComponent("plugin").Warnf("load registrations: %v", err)
		return
	}
	for id, enabled := range entries {
		m.registrations[id] = enabled
	}
}

func (m *Manager) saveRegistrationsLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.registrationPath()), 0o755); err != nil {
		return err
	}
	data, err := marshalRegistrationState(m.registrations)
	if err != nil {
		return err
	}
	tmp := m.registrationPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.registrationPath())
}

func (m *Manager) Started() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *Manager) stoppedAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopTime
}

func (m *Manager) Start(ctx context.Context) error {
	if _, _, ok := resolveNode(m.rootDir); !ok {
		logger.WithComponent("plugin").Warn("no node runtime found; plugins will be skipped until one is installed")
		m.mu.Lock()
		m.noRuntime = true
		m.mu.Unlock()
		return nil
	}
	binary := m.deps.runtimeBinary()
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("plugin runtime: %w", err)
	}
	if err := m.startRuntime(ctx, binary); err != nil {
		panic(fmt.Sprintf("plugin manager: runtime handshake failed: %v", err))
	}
	m.mu.Lock()
	m.started = true
	m.noRuntime = false
	m.mu.Unlock()
	return nil
}

func (m *Manager) startRuntime(ctx context.Context, binary string) error {
	addr, stopFn, err := m.deps.runtimeStart(ctx, binary, m.rootDir)
	if err != nil {
		return fmt.Errorf("start runtime: %w", err)
	}
	client, closeFn, err := m.deps.runtimeDial(addr)
	if err != nil {
		_ = stopFn()
		return err
	}
	rt := newRuntimeClient(m)
	rt.mu.Lock()
	rt.client = client
	rt.closeFn = closeFn
	rt.stopProcess = stopFn
	rt.mu.Unlock()
	m.mu.Lock()
	m.runtime = rt
	m.runtimeCancel = func() {}
	m.mu.Unlock()
	if err := rt.verifyNode(ctx); err != nil {
		_ = closeFn()
		_ = stopFn()
		return err
	}
	return nil
}

func (m *Manager) confirmPage(pluginID string, page *gen.Page) error {
	if page == nil {
		return errors.New("page is nil")
	}
	if m.confirmer != nil {
		return m.confirmer.Confirm(pluginID, page.GetMode(), page.GetRoutePath(), page.GetEntryUrl())
	}
	return defaultConfirmPage(pluginID, page.GetMode(), page.GetRoutePath(), page.GetEntryUrl())
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.stopTime.IsZero() {
		m.mu.Unlock()
		return nil
	}
	m.stopTime = time.Now()
	instances := make([]*pluginInstance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	cancel := m.cancel
	runtimeCancel := m.runtimeCancel
	runtime := m.runtime
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.mu.Lock()
	for _, inst := range m.instances {
		if inst.cancel != nil {
			inst.cancel()
		}
	}
	m.mu.Unlock()
	if runtimeCancel != nil {
		runtimeCancel()
	}
	for _, inst := range instances {
		m.stopInstance(ctx, inst)
	}
	if runtime != nil {
		runtime.stop(m.shutdownTimeout)
	}
	if m.register != nil && len(instances) == 0 {
		logger.WithComponent("plugin").Debug("no plugin instances to unregister")
	}
	return nil
}

func (m *Manager) stopInstance(ctx context.Context, inst *pluginInstance) {
	inst.stopOnce.Do(func() {
		m.mu.Lock()
		inst.status = instanceStopped
		m.mu.Unlock()
		if inst.cmd != nil {
			_ = inst.cmd.ProcessKill()
		}
		if inst.server != nil {
			done := make(chan struct{})
			go func() {
				inst.server.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
				inst.server.Stop()
			}
		}
		if inst.closeConn != nil {
			_ = inst.closeConn()
		}
		if inst.credPath != "" {
			_ = os.Remove(inst.credPath)
		}
	})
}

func (m *Manager) monitor(ctx context.Context, inst *pluginInstance, regSrv *registrationServer) {
	ticker := time.NewTicker(m.heartbeatInterval)
	defer ticker.Stop()
	backoff := time.Duration(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-regSrv.heartbeat:
			m.mu.Lock()
			inst.failures = 0
			inst.backoff = 0
			inst.status = instanceRunning
			m.mu.Unlock()
			continue
		}
		m.mu.Lock()
		if inst.status == instanceStopped {
			m.mu.Unlock()
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, m.heartbeatTimeout)
		client := inst.client
		m.mu.Unlock()
		var err error
		if client != nil {
			_, err = client.Heartbeat(callCtx, &gen.HeartbeatRequest{PluginId: inst.pluginID})
		} else {
			err = errors.New("heartbeat: no plugin client")
		}
		cancel()
		if err == nil {
			m.mu.Lock()
			inst.failures = 0
			inst.backoff = 0
			inst.status = instanceRunning
			m.mu.Unlock()
			continue
		}
		m.mu.Lock()
		inst.failures++
		failures := inst.failures
		if failures >= heartbeatFailWarn {
			logger.WithComponent("plugin:"+inst.pluginID).Warnf("heartbeat failure %d: %v", failures, err)
		}
		if failures >= heartbeatFailStop {
			inst.status = instanceStopped
			inst.backoff = maxBackoff
			m.mu.Unlock()
			m.stopInstance(ctx, inst)
			return
		}
		if failures >= heartbeatFailWarn {
			if inst.backoff <= 0 {
				inst.backoff = time.Second
			} else {
				inst.backoff *= 2
				if inst.backoff > maxBackoff {
					inst.backoff = maxBackoff
				}
			}
			inst.status = instanceBackoff
		}
		backoff = inst.backoff
		m.mu.Unlock()
		if backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (m *Manager) RuntimeCredentialEnv(pluginID string) string {
	if err := validatePluginID(pluginID); err != nil {
		return ""
	}
	return credentialEnvName + "=" + filepath.Join(m.credDir(), pluginID)
}

var _ = gen.RegisterRuntimeServiceServer

func dialRuntime(endpoint string) (gen.RuntimeServiceClient, func() error, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return gen.NewRuntimeServiceClient(conn), conn.Close, nil
}
func startRuntimeProcess(ctx context.Context, binary, root string) (string, func() error, error) {
	cmd := exec.Command(binary, "-root", root, "-addr", "127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	stop := func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	addrCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := stdout.Read(buf)
		addrCh <- strings.TrimSpace(string(buf[:n]))
	}()
	select {
	case addr := <-addrCh:
		if _, _, err := net.SplitHostPort(addr); err != nil {
			_ = stop()
			return "", nil, fmt.Errorf("runtime did not report a valid address: %q", addr)
		}
		return addr, stop, nil
	case <-ctx.Done():
		_ = stop()
		return "", nil, ctx.Err()
	case <-time.After(10 * time.Second):
		_ = stop()
		return "", nil, errors.New("runtime handshake timeout")
	}
}
