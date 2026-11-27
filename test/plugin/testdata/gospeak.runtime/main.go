package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
)

const defaultMirror = "https://nodejs.org/dist/"

type runtimeServer struct {
	UnimplementedRuntimeServiceServer

	root   string
	mu     sync.Mutex
	active map[string]string
	client *http.Client
}

func newRuntimeServer(root string) *runtimeServer {
	return &runtimeServer{
		root:   root,
		active: map[string]string{},
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *runtimeServer) Install(ctx context.Context, req *InstallRequest) (*InstallResponse, error) {
	if req.GetKind() == "python" {
		return &InstallResponse{Unsupported: true}, nil
	}
	if req.GetKind() != "node" {
		return nil, fmt.Errorf("unsupported kind: %s", req.GetKind())
	}
	version := req.GetVersion()
	if version == "" {
		return nil, errors.New("version is required")
	}
	mirror := strings.TrimSpace(req.GetMirror())
	if mirror == "" {
		mirror = defaultMirror
	}
	if !strings.HasSuffix(mirror, "/") {
		mirror += "/"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := installNode(ctx, s.root, mirror, version, s.client); err != nil {
		return nil, err
	}
	return &InstallResponse{Installed: true}, nil
}

func (s *runtimeServer) List(ctx context.Context, req *ListRuntimesRequest) (*ListRuntimesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kinds := []string{"node"}
	if req.GetKind() != "" {
		kinds = []string{req.GetKind()}
	}
	result := make([]*RuntimeVersion, 0)
	for _, kind := range kinds {
		if kind != "node" && kind != "python" {
			continue
		}
		dir := filepath.Join(s.root, "runtime", "download", kind)
		entries, err := os.ReadDir(dir)
		if err != nil {
			entries = nil
		}
		active := s.active[kind]
		if active == "" && kind == "node" {
			active = detectActive(s.root, kind)
		}
		if active == "" && kind == "node" {
			if p, err := exec.LookPath("node"); err == nil {
				if out, err := exec.Command(p, "--version").Output(); err == nil && len(out) > 0 {
					active = "system"
				}
			}
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			version := strings.TrimPrefix(entry.Name(), "v")
			result = append(result, &RuntimeVersion{Kind: kind, Version: version, Active: version == active})
		}
		if kind == "node" && active == "system" && len(result) == 0 {
			result = append(result, &RuntimeVersion{Kind: "node", Version: "system", Active: true})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Version < result[j].Version
	})
	return &ListRuntimesResponse{Runtimes: result}, nil
}

func (s *runtimeServer) Use(ctx context.Context, req *UseRuntimeRequest) (*UseRuntimeResponse, error) {
	if req.GetKind() == "python" {
		return &UseRuntimeResponse{Unsupported: true}, nil
	}
	if req.GetKind() != "node" {
		return nil, fmt.Errorf("unsupported kind: %s", req.GetKind())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	version := req.GetVersion()
	source := filepath.Join(s.root, "runtime", "download", "node", "v"+version, "bin", "node")
	if info, err := os.Stat(source); err != nil || info.IsDir() {
		return &UseRuntimeResponse{Ok: false}, nil
	}
	link := filepath.Join(s.root, "runtime", "bin", "node")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return nil, err
	}
	tmp := link + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Symlink(source, tmp); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, link); err != nil {
		return nil, err
	}
	if err := os.Chmod(source, 0o755); err != nil {
		return nil, err
	}
	s.active["node"] = version
	return &UseRuntimeResponse{Ok: true}, nil
}

func installNode(ctx context.Context, root, mirror, version string, client *http.Client) error {
	base := fmt.Sprintf("v%s/node-v%s-linux-x64", version, version)
	tarName := base + ".tar.gz"
	sumName := "SHASUMS256.txt"
	tarBytes, err := fetch(ctx, client, mirror+tarName)
	if err != nil {
		return fmt.Errorf("download tarball: %w", err)
	}
	sumBytes, err := fetch(ctx, client, mirror+sumName)
	if err != nil {
		return fmt.Errorf("download SHASUMS256: %w", err)
	}
	expected, err := lookupSum(string(sumBytes), tarName)
	if err != nil {
		return err
	}
	if actual := fmt.Sprintf("%x", sha256Sum(tarBytes)); !strings.EqualFold(actual, expected) {
		return errors.New("tarball checksum mismatch")
	}
	dest := filepath.Join(root, "runtime", "download", "node", "v"+version)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(dest), ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractTarGz(tarBytes, staging, base); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(staging, base), dest); err != nil {
		return err
	}
	return nil
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

func lookupSum(content, name string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == name {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no SHASUMS256 entry for %s", name)
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func extractTarGz(data []byte, dest, prefix string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !strings.HasPrefix(hdr.Name, prefix+"/") {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg {
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func detectActive(root, kind string) string {
	link := filepath.Join(root, "runtime", "bin", kind)
	target, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(target), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.HasPrefix(parts[i], "v") {
			return strings.TrimPrefix(parts[i], "v")
		}
	}
	return ""
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	root := flag.String("root", "/app/plugins", "plugin root directory")
	flag.Parse()
	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	server := grpc.NewServer()
	RegisterRuntimeServiceServer(server, newRuntimeServer(*root))
	fmt.Println(lis.Addr().String())
	if err := server.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
