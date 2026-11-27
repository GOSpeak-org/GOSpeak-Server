package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"GOSpeak/internal/model"
	"GOSpeak/internal/plugin"
	"GOSpeak/internal/plugin/gen"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type fakePluginManager struct {
	installed   []plugin.InstallOptions
	upgraded    []string
	reloaded    []string
	uninstalled []string
	enabled     []string
	disabled    []string
	runtimeSeen [][3]string
}

func (f *fakePluginManager) Install(_ context.Context, opts plugin.InstallOptions) error {
	f.installed = append(f.installed, opts)
	return nil
}

func (f *fakePluginManager) Upgrade(_ context.Context, id string) error {
	f.upgraded = append(f.upgraded, id)
	return nil
}

func (f *fakePluginManager) Reload(_ context.Context, id string) error {
	f.reloaded = append(f.reloaded, id)
	return nil
}

func (f *fakePluginManager) Uninstall(_ context.Context, id string) error {
	f.uninstalled = append(f.uninstalled, id)
	return nil
}

func (f *fakePluginManager) Enable(_ context.Context, id string) error {
	f.enabled = append(f.enabled, id)
	return nil
}

func (f *fakePluginManager) Disable(_ context.Context, id string) error {
	f.disabled = append(f.disabled, id)
	return nil
}

func (f *fakePluginManager) RuntimeInstall(_ context.Context, kind, version, mirror string) (bool, bool, error) {
	f.runtimeSeen = append(f.runtimeSeen, [3]string{kind, version, mirror})
	return kind == "node", kind == "python", nil
}

func (f *fakePluginManager) RuntimeList(_ context.Context, kind string) ([]*gen.RuntimeVersion, error) {
	if kind == "python" {
		return nil, service.PluginUnsupported
	}
	return []*gen.RuntimeVersion{{Kind: "node", Version: "v20.19.0", Active: true}}, nil
}

func (f *fakePluginManager) RuntimeUse(_ context.Context, kind, version string) (bool, bool, error) {
	f.runtimeSeen = append(f.runtimeSeen, [3]string{kind, version, ""})
	return kind == "node", kind == "python", nil
}

func newPluginTestEnv(t *testing.T) (*gin.Engine, *fakePluginManager, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PluginRegistration{}, &model.PluginConfig{}))
	mgr := &fakePluginManager{}
	h := NewPluginHandler(service.NewPluginService(db, mgr))
	r := gin.New()
	r.GET("/list", h.List)
	r.POST("/install", h.Install)
	r.POST("/upgrade", h.Upgrade)
	r.POST("/reload", h.Reload)
	r.POST("/uninstall", h.Uninstall)
	r.POST("/enable", h.Enable)
	r.POST("/disable", h.Disable)
	r.POST("/config", h.Config)
	r.GET("/runtime/list", h.RuntimeList)
	r.POST("/runtime/install", h.RuntimeInstall)
	r.POST("/runtime/use", h.RuntimeUse)
	return r, mgr, db
}

func postJSON(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPluginLifecycleAndPersistence(t *testing.T) {
	r, mgr, db := newPluginTestEnv(t)

	w := postJSON(r, "/install", `{"source":"https://github.com/example/demo","ref":"v1"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"code":0`)
	require.Len(t, mgr.installed, 1)
	require.Equal(t, "https://github.com/example/demo", mgr.installed[0].URL)
	require.Equal(t, "v1", mgr.installed[0].Ref)

	require.NoError(t, db.Create(&model.PluginRegistration{PluginID: "com.example.demo", Enabled: true}).Error)

	w = postJSON(r, "/disable", `{"plugin_id":"com.example.demo"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"com.example.demo"}, mgr.disabled)
	var row model.PluginRegistration
	require.NoError(t, db.Where("plugin_id = ?", "com.example.demo").First(&row).Error)
	require.False(t, row.Enabled)

	w = postJSON(r, "/enable", `{"plugin_id":"com.example.demo"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"com.example.demo"}, mgr.enabled)
	require.NoError(t, db.Where("plugin_id = ?", "com.example.demo").First(&row).Error)
	require.True(t, row.Enabled)

	postJSON(r, "/upgrade", `{"plugin_id":"com.example.demo"}`)
	postJSON(r, "/reload", `{"plugin_id":"com.example.demo"}`)
	require.Equal(t, []string{"com.example.demo"}, mgr.upgraded)
	require.Equal(t, []string{"com.example.demo"}, mgr.reloaded)

	w = postJSON(r, "/config", `{"plugin_id":"com.example.demo","config_json":{"a":1}}`)
	require.Equal(t, http.StatusOK, w.Code)
	var cfg model.PluginConfig
	require.NoError(t, db.Where("plugin_id = ?", "com.example.demo").First(&cfg).Error)
	require.JSONEq(t, `{"a":1}`, cfg.ConfigJSON)

	w = postJSON(r, "/uninstall", `{"plugin_id":"com.example.demo"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"com.example.demo"}, mgr.uninstalled)
	require.Equal(t, int64(0), countRegistrations(t, db))
}

func countRegistrations(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.PluginRegistration{}).Count(&n).Error)
	return n
}

func TestPluginListAndValidation(t *testing.T) {
	r, _, db := newPluginTestEnv(t)
	require.NoError(t, db.Create(&model.PluginRegistration{PluginID: "com.example.demo", Enabled: true}).Error)

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "com.example.demo")

	w = postJSON(r, "/enable", `{}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":2001`)

	w = postJSON(r, "/config", `{"plugin_id":"com.example.demo","config_json":{"bad":}`)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPluginRuntimeInstallAndUse(t *testing.T) {
	r, mgr, _ := newPluginTestEnv(t)

	w := postJSON(r, "/runtime/install", `{"kind":"node","version":"v20.19.0","mirror":"https://mirror"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"installed":true`)
	require.Equal(t, [3]string{"node", "v20.19.0", "https://mirror"}, mgr.runtimeSeen[0])

	w = postJSON(r, "/runtime/install", `{"kind":"python","version":"3.12"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"unsupported":true`)

	w = postJSON(r, "/runtime/use", `{"kind":"python","version":"3.12"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"unsupported":true`)

	req := httptest.NewRequest(http.MethodGet, "/runtime/list?kind=node", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "v20.19.0")

	req = httptest.NewRequest(http.MethodGet, "/runtime/list?kind=python", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"items":[]`)
}

func TestPluginRequiresManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	h := NewPluginHandler(service.NewPluginService(db, nil))
	r := gin.New()
	r.POST("/install", h.Install)
	w := postJSON(r, "/install", `{"source":"x"}`)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), `"code":5001`)
}
