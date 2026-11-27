package service

import (
	"context"
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/plugin"
	"GOSpeak/internal/plugin/gen"
	"GOSpeak/internal/repository"

	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type pluginStoreTestManager struct{}

func (*pluginStoreTestManager) Install(context.Context, plugin.InstallOptions) error { return nil }
func (*pluginStoreTestManager) Upgrade(context.Context, string) error              { return nil }
func (*pluginStoreTestManager) Reload(context.Context, string) error              { return nil }
func (*pluginStoreTestManager) Uninstall(context.Context, string) error           { return nil }
func (*pluginStoreTestManager) Enable(context.Context, string) error              { return nil }
func (*pluginStoreTestManager) Disable(context.Context, string) error             { return nil }
func (*pluginStoreTestManager) RuntimeInstall(context.Context, string, string, string) (bool, bool, error) {
	return false, false, nil
}
func (*pluginStoreTestManager) RuntimeList(context.Context, string) ([]*gen.RuntimeVersion, error) {
	return nil, nil
}
func (*pluginStoreTestManager) RuntimeUse(context.Context, string, string) (bool, bool, error) {
	return false, false, nil
}

func TestPluginServiceReadsStoreRowsAndDisable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PluginRegistration{}))
	store := repository.NewPluginRegistrationStore(db)
	svc := NewPluginService(db, &pluginStoreTestManager{})

	reg := model.NewPluginRegistration("com.example.store", "market", "1.0.0")
	require.NoError(t, store.Upsert(&reg))

	items, err := svc.List()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "com.example.store", items[0].PluginID)
	require.Equal(t, "market", items[0].Source)
	require.True(t, items[0].Enabled)

	require.NoError(t, store.SetEnabled("com.example.store", false))
	items, err = svc.List()
	require.NoError(t, err)
	require.False(t, items[0].Enabled)

	require.NoError(t, store.Delete("com.example.store"))
	items, err = svc.List()
	require.NoError(t, err)
	require.Empty(t, items)
}
