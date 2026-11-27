package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"GOSpeak/internal/plugin/gen"
)

type managerTestRepo struct {
	server *httptest.Server
	files  map[string]string
	log    []string
	mu     sync.Mutex
}

func newManagerTestRepo(t *testing.T, metadata string) *managerTestRepo {
	t.Helper()
	r := &managerTestRepo{files: map[string]string{
		"metadata.yaml": metadata,
		"index.js":      "// fake plugin\n",
		"package.json":  "{\"name\":\"fake\",\"version\":\"1.0.0\"}",
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, "/")
		r.mu.Lock()
		r.log = append(r.log, name)
		body, ok := r.files[name]
		r.mu.Unlock()
		if !ok {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write([]byte(body))
	})
	r.server = httptest.NewServer(mux)
	t.Cleanup(r.server.Close)
	return r
}

func (r *managerTestRepo) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.log...)
}

func writeManagerScripts(t *testing.T, binDir, gitLog, npmLog string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	if runtime.GOOS != "windows" {
		git := "#!/bin/sh\necho git $* >> " + shellQuote(gitLog) + "\nexec /bin/cp -R \"$4\" \"$5\"\n"
		npm := "#!/bin/sh\necho npm $* >> " + shellQuote(npmLog) + "\nexit 0\n"
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"), []byte(git), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "npm"), []byte(npm), 0o755))
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func managerTestRepoURL(repo *managerTestRepo) string { return repo.server.URL }

func makeManager(t *testing.T, repo *managerTestRepo, register RegisterFunc) (*Manager, string, *managerTestEnv) {
	t.Helper()
	root := t.TempDir()
	binDir := filepath.Join(root, "testbin")
	gitLog := filepath.Join(root, "git.log")
	npmLog := filepath.Join(root, "npm.log")
	writeManagerScripts(t, binDir, gitLog, npmLog)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	env := &managerTestEnv{root: root, gitLog: gitLog, npmLog: npmLog}
	nodeDir := filepath.Join(root, "runtime", "bin")
	require.NoError(t, os.MkdirAll(nodeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nodeDir, "node"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	m := NewManager(root, register)
	m.heartbeatInterval = 40 * time.Millisecond
	m.heartbeatTimeout = 150 * time.Millisecond
	m.shutdownTimeout = 500 * time.Millisecond
	m.deps.runtimeBinary = func() string { return managerFakeRuntimeBinary(t) }
	m.deps.command = func(name string, args ...string) managerCmd {
		if name == "npm" {
			_ = os.WriteFile(npmLog, []byte(readLog(t, npmLog)+"npm "+strings.Join(args, " ")+"\n"), 0o644)
		}
		return &managerFakeCmd{name: name, args: args}
	}
	m.deps.runtimeStart = func(ctx context.Context, binary, root string) (string, func() error, error) {
		return "127.0.0.1:1", func() error { return nil }, nil
	}
	m.deps.runtimeDial = func(endpoint string) (gen.RuntimeServiceClient, func() error, error) {
		return &managerHealthyRuntimeClient{}, func() error { return nil }, nil
	}
	m.deps.dial = func(endpoint string) (gen.RegistrationServiceClient, func() error, error) {
		return &managerHealthyRegistrationClient{}, func() error { return nil }, nil
	}
	return m, root, env
}

type managerTestEnv struct {
	root   string
	gitLog string
	npmLog string
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func metadataFor(id string) string {
	return fmt.Sprintf("runtime: node\nplugin_id: %s\nname: fake\nversion: 1.0.0\nentrypoint: index.js\n", id)
}

func TestManagerBusinessFailuresAreSkipped(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.bad"))
	failNpm := true
	m, _, env := makeManager(t, repo, func(context.Context, *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		return &gen.RegisterResponse{}, nil
	})
	m.deps.command = func(name string, args ...string) managerCmd {
		if name == "npm" {
			_ = os.WriteFile(env.npmLog, []byte(readLog(t, env.npmLog)+"npm "+strings.Join(args, " ")+"\n"), 0o644)
		}
		if name == "npm" && failNpm {
			return &managerFakeCmd{name: name, args: args, fail: errors.New("npm ci failed")}
		}
		return &managerFakeCmd{name: name, args: args}
	}
	require.NoError(t, m.Start(context.Background()))
	defer func() { _ = m.Stop(context.Background()) }()
	err := m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)})
	require.Error(t, err)
	require.Contains(t, readLog(t, env.npmLog), "ci")
	failNpm = false
	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
}

func TestManagerHeartbeatBackoffAndStopLimit(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.hb"))
	client := &managerFailingRegistrationClient{}
	m, _, _ := makeManager(t, repo, func(_ context.Context, req *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		return &gen.RegisterResponse{ApiEndpoint: "127.0.0.1:0"}, nil
	})
	m.deps.dial = func(endpoint string) (gen.RegistrationServiceClient, func() error, error) {
		return client, func() error { return nil }, nil
	}
	require.NoError(t, m.Start(context.Background()))
	defer func() { _ = m.Stop(context.Background()) }()
	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		state := m.instances["com.example.hb"].status
		m.mu.Unlock()
		if state == instanceStopped {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.mu.Lock()
	inst := m.instances["com.example.hb"]
	require.Equal(t, instanceStopped, inst.status)
	require.Equal(t, 5, inst.failures)
	require.GreaterOrEqual(t, inst.backoff, time.Second)
	m.mu.Unlock()
	client.mu.Lock()
	require.GreaterOrEqual(t, client.n, 5)
	client.mu.Unlock()
}

func TestManagerEnableDisablePersists(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.toggle"))
	m, root, _ := makeManager(t, repo, func(context.Context, *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		return &gen.RegisterResponse{}, nil
	})
	require.NoError(t, m.Start(context.Background()))
	defer func() { _ = m.Stop(context.Background()) }()
	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
	require.NoError(t, m.Disable(context.Background(), "com.example.toggle"))
	data, err := os.ReadFile(registrationStatePath(root))
	require.NoError(t, err)
	require.Contains(t, string(data), "\"com.example.toggle\":false")
	require.NoError(t, m.Enable(context.Background(), "com.example.toggle"))
	data, err = os.ReadFile(registrationStatePath(root))
	require.NoError(t, err)
	require.Contains(t, string(data), "\"com.example.toggle\":true")
}

func TestRegistrationServerPassesThroughAPIEndpoint(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.endpoint"))
	const apiEndpoint = "127.0.0.1:12345"
	m, _, _ := makeManager(t, repo, func(context.Context, *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		return &gen.RegisterResponse{ApiEndpoint: apiEndpoint}, nil
	})
	require.NoError(t, m.Start(context.Background()))
	defer func() { _ = m.Stop(context.Background()) }()
	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
	inst := m.instances["com.example.endpoint"]
	require.NotNil(t, inst)
	srv := &registrationServer{m: m, pluginID: "com.example.endpoint", heartbeat: make(chan struct{}, 1), apiEndpoint: apiEndpoint}
	resp, err := srv.Register(context.Background(), &gen.RegisterRequest{PluginId: "com.example.endpoint"})
	require.NoError(t, err)
	require.Equal(t, apiEndpoint, resp.GetApiEndpoint())
}

func TestManagerReloadAndStopGraceful(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.reload"))
	regs := 0
	m, _, _ := makeManager(t, repo, func(context.Context, *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		regs++
		return &gen.RegisterResponse{}, nil
	})
	require.NoError(t, m.Start(context.Background()))
	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
	require.Equal(t, 1, regs)
	require.NoError(t, m.Reload(context.Background(), "com.example.reload"))
	require.Equal(t, 2, regs)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, m.Stop(ctx))
	require.NotEmpty(t, m.stoppedAt())
}

type managerFailingRegistrationClient struct {
	mu sync.Mutex
	n  int
}

func (c *managerFailingRegistrationClient) Register(context.Context, *gen.RegisterRequest, ...grpc.CallOption) (*gen.RegisterResponse, error) {
	return &gen.RegisterResponse{}, nil
}

func (c *managerFailingRegistrationClient) Heartbeat(context.Context, *gen.HeartbeatRequest, ...grpc.CallOption) (*gen.HeartbeatResponse, error) {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	return nil, errors.New("heartbeat failed")
}

type managerHealthyRuntimeClient struct{}

func (*managerHealthyRuntimeClient) Install(context.Context, *gen.InstallRequest, ...grpc.CallOption) (*gen.InstallResponse, error) {
	return &gen.InstallResponse{Installed: true}, nil
}
func (*managerHealthyRuntimeClient) List(context.Context, *gen.ListRuntimesRequest, ...grpc.CallOption) (*gen.ListRuntimesResponse, error) {
	return &gen.ListRuntimesResponse{Runtimes: []*gen.RuntimeVersion{{Kind: "node", Version: "20.0.0", Active: true}}}, nil
}
func (*managerHealthyRuntimeClient) Use(context.Context, *gen.UseRuntimeRequest, ...grpc.CallOption) (*gen.UseRuntimeResponse, error) {
	return &gen.UseRuntimeResponse{Ok: true}, nil
}

var _ gen.RuntimeServiceClient = (*managerHealthyRuntimeClient)(nil)

type managerFakeCmd struct {
	name string
	args []string
	fail error
}

func (c *managerFakeCmd) Run() error { return c.fail }
func (c *managerFakeCmd) Start() error {
	if c.fail != nil {
		return c.fail
	}
	return nil
}
func (c *managerFakeCmd) Wait() error        { return nil }
func (c *managerFakeCmd) ProcessKill() error { return nil }

func managerFakeRuntimeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gospeak.runtime")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
	return path
}

type managerHealthyRegistrationClient struct{}

func (*managerHealthyRegistrationClient) Register(context.Context, *gen.RegisterRequest, ...grpc.CallOption) (*gen.RegisterResponse, error) {
	return &gen.RegisterResponse{}, nil
}
func (*managerHealthyRegistrationClient) Heartbeat(context.Context, *gen.HeartbeatRequest, ...grpc.CallOption) (*gen.HeartbeatResponse, error) {
	return &gen.HeartbeatResponse{}, nil
}
