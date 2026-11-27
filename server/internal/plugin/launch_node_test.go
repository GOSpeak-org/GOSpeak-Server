package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLaunchUsesSystemNodeFallback(t *testing.T) {
	m, _, _ := makeManager(t, nil, nil)
	require.NoError(t, os.RemoveAll(filepath.Join(m.rootDir, "runtime", "bin", "node")))
	bin := t.TempDir()
	sn := filepath.Join(bin, "node")
	require.NoError(t, os.WriteFile(sn, []byte("#!/bin/sh\necho v22\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var launched string
	m.deps.command = func(name string, args ...string) managerCmd {
		launched = name
		return &managerFakeCmd{name: name, args: args}
	}
	md := PluginMetadata{PluginID: "fallback", Name: "fake", Version: "1.0.0", Entrypoint: "index.js"}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte(""), 0o644))
	require.NoError(t, m.launch(context.Background(), md, dir, ""))
	require.Equal(t, sn, launched)
}