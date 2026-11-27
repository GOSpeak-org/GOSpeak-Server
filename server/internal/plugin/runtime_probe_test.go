package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveNodePrefersManaged(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "runtime", "bin")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	node := filepath.Join(dir, "node")
	require.NoError(t, os.WriteFile(node, []byte("#!/bin/sh\necho v20.0.0\n"), 0o755))
	p, s, ok := resolveNode(root)
	require.True(t, ok)
	require.Equal(t, "managed", s)
	require.Equal(t, node, p)
}

func TestResolveNodeFallsBackToPATH(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	sn := filepath.Join(bin, "node")
	require.NoError(t, os.WriteFile(sn, []byte("#!/bin/sh\necho v22.0.0\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	p, s, ok := resolveNode(root)
	require.True(t, ok)
	require.Equal(t, "system", s)
	require.Equal(t, sn, p)
}

func TestResolveNodeNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, _, ok := resolveNode(t.TempDir())
	require.False(t, ok)
}