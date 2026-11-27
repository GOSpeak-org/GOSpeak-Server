package plugin

import (
	"context"
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/plugin/gen"
	"GOSpeak/internal/repository"

	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type recordingRegistrationStore struct {
	upserted []*model.PluginRegistration
	enabled  map[string]bool
	deleted  []string
}

func newRecordingRegistrationStore() *recordingRegistrationStore {
	return &recordingRegistrationStore{enabled: make(map[string]bool)}
}

func (s *recordingRegistrationStore) Upsert(reg *model.PluginRegistration) error {
	copy := *reg
	s.upserted = append(s.upserted, &copy)
	return nil
}

func (s *recordingRegistrationStore) SetEnabled(pluginID string, enabled bool) error {
	s.enabled[pluginID] = enabled
	return nil
}

func (s *recordingRegistrationStore) Delete(pluginID string) error {
	s.deleted = append(s.deleted, pluginID)
	return nil
}

var _ RegistrationStore = (*recordingRegistrationStore)(nil)

func TestManagerCallsRegistrationStore(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.store"))
	m, _, _ := makeManager(t, repo, func(context.Context, *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		return &gen.RegisterResponse{}, nil
	})
	store := newRecordingRegistrationStore()
	m.SetRegistrationStore(store)
	require.NoError(t, m.Start(context.Background()))
	defer func() { _ = m.Stop(context.Background()) }()

	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
	require.NotEmpty(t, store.upserted)
	require.Equal(t, "com.example.store", store.upserted[0].PluginID)
	require.Equal(t, "market", store.upserted[0].Source)
	require.True(t, store.upserted[0].Enabled)

	require.NoError(t, m.Disable(context.Background(), "com.example.store"))
	require.Equal(t, map[string]bool{"com.example.store": false}, store.enabled)
	require.NoError(t, m.Enable(context.Background(), "com.example.store"))
	require.Equal(t, map[string]bool{"com.example.store": true}, store.enabled)

	require.NoError(t, m.Uninstall(context.Background(), "com.example.store"))
	require.Equal(t, []string{"com.example.store"}, store.deleted)
}

func TestManagerInstallWritesSQLiteRegistration(t *testing.T) {
	repo := newManagerTestRepo(t, metadataFor("com.example.sqlite"))
	m, _, _ := makeManager(t, repo, func(context.Context, *gen.RegisterRequest) (*gen.RegisterResponse, error) {
		return &gen.RegisterResponse{}, nil
	})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PluginRegistration{}))
	store := repository.NewPluginRegistrationStore(db)
	m.SetRegistrationStore(store)
	require.NoError(t, m.Start(context.Background()))
	defer func() { _ = m.Stop(context.Background()) }()

	require.NoError(t, m.Install(context.Background(), managerInstallOptions{Source: "market", URL: managerTestRepoURL(repo)}))
	var row model.PluginRegistration
	require.NoError(t, db.Where("plugin_id = ?", "com.example.sqlite").First(&row).Error)
	require.Equal(t, "market", row.Source)
	require.True(t, row.Enabled)

	require.NoError(t, m.Disable(context.Background(), "com.example.sqlite"))
	require.NoError(t, db.Where("plugin_id = ?", "com.example.sqlite").First(&row).Error)
	require.False(t, row.Enabled)

	require.NoError(t, m.Uninstall(context.Background(), "com.example.sqlite"))
	require.Equal(t, int64(0), func() int64 {
		var n int64
		require.NoError(t, db.Model(&model.PluginRegistration{}).Count(&n).Error)
		return n
	}())
}
