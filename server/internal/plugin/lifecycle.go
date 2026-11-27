package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"GOSpeak/internal/logger"
	"GOSpeak/internal/plugin/gen"

	"google.golang.org/grpc"
)

const (
	pluginRoutePrefix = "/plugins/"
	pluginStateEnv    = stateEnvName
)

type InstallOptions struct {
	Source  string
	URL     string
	Ref     string
	Version string
}

type managerInstallOptions = InstallOptions

func registrationStatePath(root string) string {
	return filepath.Join(root, "runtime", "state", "_registrations.json")
}

func parseRegistrationState(raw []byte) (map[string]bool, error) {
	var out map[string]bool
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func marshalRegistrationState(in map[string]bool) ([]byte, error) {
	return json.Marshal(in)
}

func defaultConfirmPage(pluginID, mode, routePath, entryURL string) error {
	if mode != "spa" && mode != "iframe" {
		return fmt.Errorf("unsupported page mode %q", mode)
	}
	if routePath != pluginRoutePrefix+pluginID+"/" {
		return fmt.Errorf("page route must be %s", pluginRoutePrefix+pluginID+"/")
	}
	u, err := url.Parse(entryURL)
	if err != nil || u.Host == "" {
		return errors.New("invalid page entry_url")
	}
	return nil
}

func (m *Manager) fetchPlugin(ctx context.Context, opts InstallOptions, dest string) (PluginMetadata, string, error) {
	if err := validatePluginID(filepath.Base(dest)); err != nil {
		return PluginMetadata{}, "", err
	}
	if strings.TrimSpace(opts.URL) == "" {
		return PluginMetadata{}, "", errors.New("plugin source url is required")
	}
	tmpParent := filepath.Dir(dest)
	if err := os.MkdirAll(tmpParent, 0o755); err != nil {
		return PluginMetadata{}, "", err
	}
	staging, err := os.MkdirTemp(tmpParent, ".install-")
	if err != nil {
		return PluginMetadata{}, "", err
	}
	if err := m.runFetch(ctx, opts, staging); err != nil {
		return PluginMetadata{}, "", err
	}
	raw, err := os.ReadFile(filepath.Join(staging, "metadata.yaml"))
	if err != nil {
		return PluginMetadata{}, "", fmt.Errorf("read metadata: %w", err)
	}
	md, err := parseMetadata(raw)
	if err != nil {
		return PluginMetadata{}, "", err
	}
	if err := validatePluginID(md.PluginID); err != nil {
		return PluginMetadata{}, "", err
	}
	if err := os.RemoveAll(dest); err != nil {
		return PluginMetadata{}, "", err
	}
	if err := os.Rename(staging, dest); err != nil {
		return PluginMetadata{}, "", err
	}
	return md, dest, nil
}

func (m *Manager) runFetch(ctx context.Context, opts InstallOptions, dest string) error {
	source := strings.TrimSpace(opts.URL)
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return m.downloadTarball(ctx, source, dest)
	}
	cmd := m.deps.command("git", "clone", "--depth", "1", source, dest)
	return cmd.Run()
}

func (m *Manager) downloadTarball(ctx context.Context, rawURL, dest string) error {
	base := strings.TrimRight(rawURL, "/")
	for _, suffix := range []string{"/metadata.yaml", "/index.js", "/package.json"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+suffix, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", suffix, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("fetch %s: status %d", suffix, resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(suffix, "/")
		if err := os.WriteFile(filepath.Join(dest, name), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) npmInstall(ctx context.Context, dir string, md PluginMetadata) error {
	args := []string{"ci"}
	if md.NPMRegistry != "" {
		args = append(args, "--registry", md.NPMRegistry)
	}
	args = append(args, md.NPMArgs...)
	cmd := m.deps.command("npm", args...)
	if realCmd, ok := cmd.(*execCmd); ok {
		realCmd.cmd.Dir = dir
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm ci: %w", err)
	}
	return nil
}

func (m *Manager) Install(ctx context.Context, opts InstallOptions) error {
	m.mu.Lock()
	started := m.started
	m.mu.Unlock()
	if !started {
		return errors.New("plugin manager is not started")
	}
	staging := filepath.Join(m.binDir(), ".install-target-"+strconvFormat(opts.URL))
	md, stagedDir, err := m.fetchPlugin(ctx, opts, staging)
	if err != nil {
		_ = os.RemoveAll(staging)
		return m.pluginFailure("install", opts.URL, err)
	}
	if err := m.npmInstall(ctx, stagedDir, md); err != nil {
		_ = os.RemoveAll(stagedDir)
		return m.pluginFailure("install", md.PluginID, err)
	}
	m.mu.Lock()
	m.lastInstall[md.PluginID] = opts
	m.mu.Unlock()
	finalDir := m.pluginDir(md.PluginID)
	if err := os.RemoveAll(finalDir); err != nil {
		return err
	}
	if err := os.Rename(stagedDir, finalDir); err != nil {
		return err
	}
	if err := m.launch(ctx, md, finalDir, opts.Source); err != nil {
		return m.pluginFailure("launch", md.PluginID, err)
	}
	return nil
}

func strconvFormat(s string) string {
	sum := 0
	for _, r := range s {
		sum = sum*31 + int(r)
	}
	if sum < 0 {
		sum = -sum
	}
	return fmt.Sprintf("%x", sum)
}

func (m *Manager) pluginFailure(stage, id string, err error) error {
	logger.WithComponent("plugin").Warnf("%s plugin %s failed: %v (skipped)", stage, id, err)
	return fmt.Errorf("%s plugin %s: %w", stage, id, err)
}

func (m *Manager) launch(ctx context.Context, md PluginMetadata, dir, source string) error {
	m.stopPluginByID(md.PluginID)
	credPath := filepath.Join(m.credDir(), md.PluginID)
	if err := os.MkdirAll(filepath.Dir(credPath), 0o755); err != nil {
		return err
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := lis.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(credPath, []byte(fmt.Sprintf("127.0.0.1:%d\n", port)), 0o600); err != nil {
		lis.Close()
		return err
	}
	srv := grpc.NewServer()
	regSrv := &registrationServer{m: m, pluginID: md.PluginID, heartbeat: make(chan struct{}, 16)}
	if m.register != nil {
		resp, err := m.register(ctx, &gen.RegisterRequest{
			PluginId: md.PluginID, Name: md.Name, Version: md.Version,
		})
		if err != nil {
			logger.WithComponent("plugin").Warnf("register callback %s: %v", md.PluginID, err)
		} else {
			regSrv.apiEndpoint = resp.GetApiEndpoint()
		}
	}
	gen.RegisterRegistrationServiceServer(srv, regSrv)
	go func() { _ = srv.Serve(lis) }()
	stateDir := filepath.Join(m.stateDir(), md.PluginID)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		srv.Stop()
		return err
	}
	node, _, ok := resolveNode(m.rootDir)
	if !ok {
		return errors.New("no node runtime available")
	}
	env := append(os.Environ(), credentialEnvName+"="+credPath, pluginStateEnv+"="+stateDir)
	entry := filepath.Join(dir, md.Entrypoint)
	cmd := m.deps.command(node, entry)
	if realCmd, ok := cmd.(*execCmd); ok {
		realCmd.cmd.Dir = dir
		realCmd.cmd.Env = env
	}
	_ = env
	runCtx, cancel := context.WithCancel(context.Background())
	inst := &pluginInstance{
		pluginID: md.PluginID, metadata: md, dir: dir, credPath: credPath,
		server: srv, listener: lis, cmd: cmd, cancel: cancel, status: instanceRunning,
	}
	client, closeConn, err := m.deps.dial(net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)))
	if err != nil {
		cancel()
		srv.Stop()
		return err
	}
	inst.client = client
	inst.closeConn = closeConn
	m.mu.Lock()
	m.instances[md.PluginID] = inst
	m.registrations[md.PluginID] = true
	if _, ok := m.lastInstall[md.PluginID]; !ok {
		m.lastInstall[md.PluginID] = InstallOptions{Source: source}
	}
	m.cancel = cancel
	if err := m.saveRegistrationsLocked(); err != nil {
		logger.WithComponent("plugin").Warnf("persist registration %s: %v", md.PluginID, err)
	}
	m.mu.Unlock()
	if err := cmd.Start(); err != nil {
		m.stopPluginByID(md.PluginID)
		return fmt.Errorf("start plugin process: %w", err)
	}
	m.upsertRegistration(md.PluginID, md, source)
	m.recordRestart()
	go m.monitor(runCtx, inst, regSrv)
	return nil
}

func (m *Manager) stopPluginByID(id string) {
	m.mu.Lock()
	inst := m.instances[id]
	delete(m.instances, id)
	m.mu.Unlock()
	if inst != nil {
		m.stopInstance(context.Background(), inst)
	}
}

func (m *Manager) Upgrade(ctx context.Context, pluginID string) error {
	if err := validatePluginID(pluginID); err != nil {
		return err
	}
	m.mu.Lock()
	opts := m.lastInstall[pluginID]
	m.mu.Unlock()
	return m.Install(ctx, InstallOptions{Source: opts.Source, URL: opts.URL, Ref: opts.Ref, Version: opts.Version})
}

func (m *Manager) Reload(ctx context.Context, pluginID string) error {
	if err := validatePluginID(pluginID); err != nil {
		return err
	}
	m.mu.Lock()
	inst := m.instances[pluginID]
	m.mu.Unlock()
	if inst == nil {
		return fmt.Errorf("plugin %s is not installed", pluginID)
	}
	md := inst.metadata
	dir := inst.dir
	m.stopPluginByID(pluginID)
	return m.launch(ctx, md, dir, "")
}

func (m *Manager) Uninstall(ctx context.Context, pluginID string) error {
	if err := validatePluginID(pluginID); err != nil {
		return err
	}
	m.stopPluginByID(pluginID)
	if err := os.RemoveAll(m.pluginDir(pluginID)); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.registrations, pluginID)
	delete(m.lastInstall, pluginID)
	err := m.saveRegistrationsLocked()
	m.mu.Unlock()
	m.deleteRegistration(pluginID)
	logger.WithComponent("plugin:" + pluginID).Info("plugin uninstalled, state preserved")
	return err
}

func (m *Manager) Enable(ctx context.Context, pluginID string) error {
	return m.setEnabled(pluginID, true)
}
func (m *Manager) Disable(ctx context.Context, pluginID string) error {
	return m.setEnabled(pluginID, false)
}

func (m *Manager) setEnabled(pluginID string, enabled bool) error {
	if err := validatePluginID(pluginID); err != nil {
		return err
	}
	m.mu.Lock()
	m.registrations[pluginID] = enabled
	if err := m.saveRegistrationsLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	inst := m.instances[pluginID]
	if inst != nil {
		if enabled {
			inst.status = instanceRunning
		} else {
			inst.status = instanceStopped
		}
	}
	m.mu.Unlock()
	m.setRegistrationEnabled(pluginID, enabled)
	return nil
}

func (m *Manager) IsEnabled(pluginID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	enabled, ok := m.registrations[pluginID]
	return !ok || enabled
}

type runtimeClient struct {
	gen.UnimplementedRuntimeServiceServer
	mu          sync.Mutex
	m           *Manager
	client      gen.RuntimeServiceClient
	closeFn     func() error
	listener    net.Listener
	server      *grpc.Server
	stopProcess func() error
}

func newRuntimeClient(m *Manager) *runtimeClient { return &runtimeClient{m: m} }

func (r *runtimeClient) verifyNode(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	list, err := r.client.List(callCtx, &gen.ListRuntimesRequest{Kind: "node"})
	if err != nil {
		return fmt.Errorf("runtime handshake: %w", err)
	}
	for _, rt := range list.GetRuntimes() {
		if rt.GetKind() == "node" && rt.GetActive() {
			return nil
		}
	}
	return errors.New("runtime handshake: no active node runtime")
}

func (r *runtimeClient) Install(ctx context.Context, req *gen.InstallRequest) (*gen.InstallResponse, error) {
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client == nil {
		return nil, errors.New("runtime not connected")
	}
	return client.Install(ctx, req)
}

func (r *runtimeClient) List(ctx context.Context, req *gen.ListRuntimesRequest) (*gen.ListRuntimesResponse, error) {
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client == nil {
		return nil, errors.New("runtime not connected")
	}
	resp, err := client.List(ctx, req)
	if err != nil {
		return nil, err
	}
	sort.Slice(resp.Runtimes, func(i, j int) bool { return resp.Runtimes[i].Version < resp.Runtimes[j].Version })
	return resp, nil
}

func (r *runtimeClient) Use(ctx context.Context, req *gen.UseRuntimeRequest) (*gen.UseRuntimeResponse, error) {
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client == nil {
		return nil, errors.New("runtime not connected")
	}
	return client.Use(ctx, req)
}

func (r *runtimeClient) stop(timeout time.Duration) {
	r.mu.Lock()
	closeFn := r.closeFn
	server := r.server
	listener := r.listener
	stopProcess := r.stopProcess
	r.mu.Unlock()
	if stopProcess != nil {
		_ = stopProcess()
	}
	if server != nil {
		done := make(chan struct{})
		go func() { server.GracefulStop(); close(done) }()
		select {
		case <-done:
		case <-time.After(timeout):
			server.Stop()
		}
	}
	if listener != nil {
		_ = listener.Close()
	}
	if closeFn != nil {
		_ = closeFn()
	}
}

func (m *Manager) RuntimeInstall(ctx context.Context, kind, version, mirror string) (bool, bool, error) {
	r := m.currentRuntime()
	if r == nil {
		return false, false, errors.New("runtime not started")
	}
	resp, err := r.Install(ctx, &gen.InstallRequest{Kind: kind, Version: version, Mirror: mirror})
	if err != nil {
		return false, false, err
	}
	return resp.GetInstalled(), resp.GetUnsupported(), nil
}

func (m *Manager) RuntimeList(ctx context.Context, kind string) ([]*gen.RuntimeVersion, error) {
	r := m.currentRuntime()
	if r == nil {
		return nil, errors.New("runtime not started")
	}
	resp, err := r.List(ctx, &gen.ListRuntimesRequest{Kind: kind})
	if err != nil {
		return nil, err
	}
	return resp.GetRuntimes(), nil
}

func (m *Manager) RuntimeUse(ctx context.Context, kind, version string) (bool, bool, error) {
	r := m.currentRuntime()
	if r == nil {
		return false, false, errors.New("runtime not started")
	}
	resp, err := r.Use(ctx, &gen.UseRuntimeRequest{Kind: kind, Version: version})
	if err != nil {
		return false, false, err
	}
	return resp.GetOk(), resp.GetUnsupported(), nil
}

func (m *Manager) currentRuntime() *runtimeClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtime
}
