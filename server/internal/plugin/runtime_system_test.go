package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"GOSpeak/internal/plugin/gen"
)

func TestRuntimeListReportsSystemNode(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	sn := filepath.Join(bin, "node")
	require.NoError(t, os.WriteFile(sn, []byte("#!/bin/sh\necho v22\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	proc := startRuntime(t, root)
	list, err := proc.client.List(context.Background(), &gen.ListRuntimesRequest{Kind: "node"})
	require.NoError(t, err)
	found := false
	for _, rt := range list.GetRuntimes() {
		if rt.GetVersion() == "system" && rt.GetActive() {
			found = true
		}
	}
	require.True(t, found)
}