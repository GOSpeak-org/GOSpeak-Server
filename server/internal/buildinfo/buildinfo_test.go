package buildinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func withVersion(t *testing.T) {
	t.Helper()
	prev := Version
	t.Cleanup(func() { Version = prev })
}

func withCommit(t *testing.T) {
	t.Helper()
	prev := Commit
	t.Cleanup(func() { Commit = prev })
}

func TestResolveCommitReadsCOMMITFile(t *testing.T) {
	withCommit(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "COMMIT"), []byte("f349de6defe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	Commit = "unknown"
	ResolveCommit()
	if Commit != "f349de6defe" {
		t.Fatalf("expected COMMIT file value, got %q", Commit)
	}
}

func TestResolveCommitKeepsInjectedCommit(t *testing.T) {
	withCommit(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "COMMIT"), []byte("abc1234"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	Commit = "injected"
	ResolveCommit()
	if Commit != "injected" {
		t.Fatalf("ldflags-injected commit must win, got %q", Commit)
	}
}

func TestResolveVersionReadsVERSIONFile(t *testing.T) {
	withVersion(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	Version = "dev"
	ResolveVersion()
	if Version != "1.2.3" {
		t.Fatalf("expected VERSION file value, got %q", Version)
	}
}

func TestResolveVersionFindsRepositoryRootFile(t *testing.T) {
	withVersion(t)
	// 模拟 app/server 运行目录：../../VERSION 即仓库根文件。
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("9.9.9"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "app", "server")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	Version = "dev"
	ResolveVersion()
	if Version != "9.9.9" {
		t.Fatalf("expected parent VERSION file value, got %q", Version)
	}
}

func TestResolveVersionKeepsInjectedVersion(t *testing.T) {
	withVersion(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.2.3"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	Version = "2.0.0"
	ResolveVersion()
	if Version != "2.0.0" {
		t.Fatalf("ldflags-injected version must win, got %q", Version)
	}
}

func TestResolveVersionIgnoresInvalidContent(t *testing.T) {
	withVersion(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("dev"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	Version = "dev"
	ResolveVersion()
	if Version != "dev" {
		t.Fatalf("invalid VERSION content must not be used, got %q", Version)
	}
}
