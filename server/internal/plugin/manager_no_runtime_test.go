package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerStartWithoutNodeDoesNotPanic(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := NewManager(t.TempDir(), nil)
	require.NotPanics(t, func() {
		require.NoError(t, m.Start(context.Background()))
	})
	require.True(t, m.noRuntime)
}