package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"GOSpeak/internal/plugin/gen"
)

const runtimeMain = "../../../test/plugin/testdata/gospeak.runtime/main.go"

type fakeMirror struct {
	server     *httptest.Server
	tarball    []byte
	sums       string
	requestLog []string
}

func newFakeMirror(t *testing.T, version, script string, corrupt bool) *fakeMirror {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	base := fmt.Sprintf("v%s/node-v%s-linux-x64", version, version)
	body := []byte(script)
	hdr := &tar.Header{Name: base + "/bin/node", Mode: 0o755, Size: int64(len(body))}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	m := &fakeMirror{tarball: buf.Bytes()}
	sum := fmt.Sprintf("%x", sha256.Sum256(m.tarball))
	if corrupt {
		sum = strings.Repeat("0", 64)
	}
	m.sums = fmt.Sprintf("%s  v%s/node-v%s-linux-x64.tar.gz\n", sum, version, version)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		m.requestLog = append(m.requestLog, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			_, _ = w.Write(m.tarball)
		case strings.HasSuffix(r.URL.Path, "SHASUMS256.txt"):
			_, _ = w.Write([]byte(m.sums))
		default:
			http.NotFound(w, r)
		}
	})
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

type runtimeProcess struct {
	client gen.RuntimeServiceClient
	cmd    *exec.Cmd
	conn   *grpc.ClientConn
	root   string
}

func startRuntime(t *testing.T, root string) *runtimeProcess {
	t.Helper()
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "gospeak.runtime")
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "test", "plugin", "testdata", "gospeak.runtime"))
	require.NoError(t, err)
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	stdoutFile, err := os.CreateTemp(t.TempDir(), "runtime-*.out")
	require.NoError(t, err)
	cmd := exec.Command(binary, "-root", root)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stdoutFile
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(10 * time.Second)
	var addr string
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(stdoutFile.Name())
		if readErr == nil {
			line := strings.TrimSpace(string(data))
			if _, parseErr := net.ResolveTCPAddr("tcp", line); parseErr == nil && line != "" {
				addr = line
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.NotEmpty(t, addr, "runtime did not report its address")

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &runtimeProcess{
		client: gen.NewRuntimeServiceClient(conn),
		cmd:    cmd,
		conn:   conn,
		root:   root,
	}
}

func TestRuntimeStartDoesNotDownload(t *testing.T) {
	isolated := t.TempDir()
	if goBin, err := exec.LookPath("go"); err == nil {
		require.NoError(t, os.Symlink(goBin, filepath.Join(isolated, "go")))
	}
	t.Setenv("PATH", isolated)
	root := t.TempDir()
	proc := startRuntime(t, root)
	mirror := newFakeMirror(t, "20.0.0", "#!/bin/sh\necho v20.0.0\n", false)
	resp, err := proc.client.List(context.Background(), &gen.ListRuntimesRequest{})
	require.NoError(t, err)
	require.Empty(t, resp.GetRuntimes())
	require.Empty(t, mirror.requestLog)
	_, statErr := os.Stat(filepath.Join(root, "runtime", "bin", "node"))
	require.True(t, os.IsNotExist(statErr))
}

func TestRuntimeInstallNodeAndUse(t *testing.T) {
	root := t.TempDir()
	proc := startRuntime(t, root)
	mirror := newFakeMirror(t, "20.0.0", "#!/bin/sh\necho v20.0.0\n", false)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	install, err := proc.client.Install(ctx, &gen.InstallRequest{
		Kind: "node", Version: "20.0.0", Mirror: mirror.server.URL, Reason: "test", RequestId: "r1",
	})
	require.NoError(t, err)
	require.True(t, install.GetInstalled())

	use, err := proc.client.Use(ctx, &gen.UseRuntimeRequest{Kind: "node", Version: "20.0.0", Reason: "test", RequestId: "r2"})
	require.NoError(t, err)
	require.True(t, use.GetOk())

	link := filepath.Join(root, "runtime", "bin", "node")
	output, err := exec.Command(link, "--version").Output()
	require.NoError(t, err)
	require.Equal(t, "v20.0.0\n", string(output))

	list, err := proc.client.List(ctx, &gen.ListRuntimesRequest{Kind: "node"})
	require.NoError(t, err)
	require.Len(t, list.GetRuntimes(), 1)
	require.Equal(t, "20.0.0", list.GetRuntimes()[0].GetVersion())
	require.True(t, list.GetRuntimes()[0].GetActive())
}

func TestRuntimeChecksumFailureKeepsNoSymlink(t *testing.T) {
	root := t.TempDir()
	proc := startRuntime(t, root)
	mirror := newFakeMirror(t, "20.0.0", "#!/bin/sh\necho v20.0.0\n", true)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := proc.client.Install(ctx, &gen.InstallRequest{Kind: "node", Version: "20.0.0", Mirror: mirror.server.URL})
	require.Error(t, err)
	_, statErr := os.Lstat(filepath.Join(root, "runtime", "bin", "node"))
	require.True(t, os.IsNotExist(statErr))
}

func TestRuntimeMirrorUnreachable(t *testing.T) {
	root := t.TempDir()
	proc := startRuntime(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := proc.client.Install(ctx, &gen.InstallRequest{
		Kind: "node", Version: "20.0.0", Mirror: "http://127.0.0.1:1",
	})
	require.Error(t, err)
}

func TestRuntimeInstallPythonUnsupported(t *testing.T) {
	root := t.TempDir()
	proc := startRuntime(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := proc.client.Install(ctx, &gen.InstallRequest{Kind: "python", Version: "3.12.0"})
	require.NoError(t, err)
	require.True(t, resp.GetUnsupported())
	require.False(t, resp.GetInstalled())
	use, err := proc.client.Use(ctx, &gen.UseRuntimeRequest{Kind: "python", Version: "3.12.0"})
	require.NoError(t, err)
	require.True(t, use.GetUnsupported())
}
