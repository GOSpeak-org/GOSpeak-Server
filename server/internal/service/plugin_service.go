package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/plugin"
	"GOSpeak/internal/plugin/gen"

	"gorm.io/gorm"
)

var _ PluginManagerLike = (*plugin.Manager)(nil)

const pluginMaxSourceLength = 512

// PluginManagerLike 是管理面对 PluginManager 生命周期与运行时能力的最小依赖。
// 由组合根注入真实 Manager；首版未实现的能力可返回 PluginUnsupported。
type PluginManagerLike interface {
	Install(ctx context.Context, opts plugin.InstallOptions) error
	Upgrade(ctx context.Context, pluginID string) error
	Reload(ctx context.Context, pluginID string) error
	Uninstall(ctx context.Context, pluginID string) error
	Enable(ctx context.Context, pluginID string) error
	Disable(ctx context.Context, pluginID string) error
	RuntimeInstall(ctx context.Context, kind, version, mirror string) (installed bool, unsupported bool, err error)
	RuntimeList(ctx context.Context, kind string) ([]*gen.RuntimeVersion, error)
	RuntimeUse(ctx context.Context, kind, version string) (ok bool, unsupported bool, err error)
}

// PluginUnsupported 表示 Manager 尚未实现该能力（如 Python 运行时）；service 会把它
// 转换成带 unsupported 标记的响应，供页面显示「未开放」而不是报错。
var PluginUnsupported = errors.New("plugin capability not supported")

type PluginRuntimeInstallResult struct {
	Installed   bool `json:"installed"`
	Unsupported bool `json:"unsupported"`
}

type PluginRuntimeUseResult struct {
	OK          bool `json:"ok"`
	Unsupported bool `json:"unsupported"`
}

type PluginRuntimeVersion struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
	Active  bool   `json:"active"`
}

type PluginItem struct {
	PluginID  string `json:"plugin_id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	Version   string `json:"version"`
	Enabled   bool   `json:"enabled"`
	RoutePath string `json:"route_path"`
}

type PluginConfigItem struct {
	PluginID   string `json:"plugin_id"`
	ConfigJSON string `json:"config_json"`
}

// PluginService 编排插件生命周期与注册表持久化，不感知 HTTP。
type PluginService struct {
	db      *gorm.DB
	manager PluginManagerLike
}

func NewPluginService(db *gorm.DB, manager PluginManagerLike) *PluginService {
	return &PluginService{db: db, manager: manager}
}

func (s *PluginService) managerErr(err error) error {
	if errors.Is(err, PluginUnsupported) {
		return pkg.NewAppError(pkg.INVALID_PARAMS, "plugin capability is not supported")
	}
	if err == nil {
		return nil
	}
	return pkg.NewAppErrorWithCause(pkg.INTERNAL_ERROR, err)
}

func (s *PluginService) requireManager() error {
	if s.manager == nil {
		return pkg.NewAppError(pkg.INTERNAL_ERROR, "plugin manager is not available")
	}
	return nil
}

func (s *PluginService) Install(ctx context.Context, source, url, ref, version string) error {
	if err := s.requireManager(); err != nil {
		return err
	}
	source = strings.TrimSpace(source)
	url = strings.TrimSpace(url)
	if source == "" {
		source = "github"
	}
	if url == "" {
		url = source
	}
	if len(source)+len(url) > pluginMaxSourceLength {
		return pkg.NewAppError(pkg.INVALID_PARAMS, "source is too long")
	}
	if err := s.manager.Install(ctx, plugin.InstallOptions{
		Source:  source,
		URL:     url,
		Ref:     strings.TrimSpace(ref),
		Version: strings.TrimSpace(version),
	}); err != nil {
		return s.managerErr(err)
	}
	return nil
}

func (s *PluginService) Upgrade(ctx context.Context, pluginID string) error {
	if err := s.requirePluginID(pluginID); err != nil {
		return err
	}
	if err := s.requireManager(); err != nil {
		return err
	}
	if err := s.manager.Upgrade(ctx, pluginID); err != nil {
		return s.managerErr(err)
	}
	return nil
}

func (s *PluginService) Reload(ctx context.Context, pluginID string) error {
	if err := s.requirePluginID(pluginID); err != nil {
		return err
	}
	if err := s.requireManager(); err != nil {
		return err
	}
	if err := s.manager.Reload(ctx, pluginID); err != nil {
		return s.managerErr(err)
	}
	return nil
}

func (s *PluginService) Uninstall(ctx context.Context, pluginID string) error {
	if err := s.requirePluginID(pluginID); err != nil {
		return err
	}
	if err := s.requireManager(); err != nil {
		return err
	}
	if err := s.manager.Uninstall(ctx, pluginID); err != nil {
		return s.managerErr(err)
	}
	if err := s.db.Where("plugin_id = ?", pluginID).Delete(&model.PluginRegistration{}).Error; err != nil {
		return pkg.NewAppErrorWithCause(pkg.INTERNAL_ERROR, err)
	}
	return nil
}

func (s *PluginService) setEnabled(ctx context.Context, pluginID string, enabled bool) error {
	if err := s.requirePluginID(pluginID); err != nil {
		return err
	}
	if err := s.requireManager(); err != nil {
		return err
	}
	var fn func(context.Context, string) error
	if enabled {
		fn = s.manager.Enable
	} else {
		fn = s.manager.Disable
	}
	if err := fn(ctx, pluginID); err != nil {
		return s.managerErr(err)
	}
	if err := s.db.Model(&model.PluginRegistration{}).
		Where("plugin_id = ?", pluginID).
		Update("enabled", enabled).Error; err != nil {
		return pkg.NewAppErrorWithCause(pkg.INTERNAL_ERROR, err)
	}
	return nil
}

func (s *PluginService) Enable(ctx context.Context, pluginID string) error {
	return s.setEnabled(ctx, pluginID, true)
}

func (s *PluginService) Disable(ctx context.Context, pluginID string) error {
	return s.setEnabled(ctx, pluginID, false)
}

func (s *PluginService) requirePluginID(pluginID string) error {
	if strings.TrimSpace(pluginID) == "" {
		return pkg.NewAppError(pkg.INVALID_PARAMS, "plugin_id is required")
	}
	return nil
}

// SetConfig 写入插件配置 JSON；由管理端负责 JSON 合法性校验。
func (s *PluginService) SetConfig(pluginID, configJSON string) (*PluginConfigItem, error) {
	if err := s.requirePluginID(pluginID); err != nil {
		return nil, err
	}
	cfg := model.PluginConfig{PluginID: pluginID, ConfigJSON: configJSON}
	if err := s.db.Save(&cfg).Error; err != nil {
		return nil, pkg.NewAppErrorWithCause(pkg.INTERNAL_ERROR, err)
	}
	return &PluginConfigItem{PluginID: pluginID, ConfigJSON: configJSON}, nil
}

// List 合并注册表与运行时状态；注册表为空时仍返回空列表。
func (s *PluginService) List() ([]PluginItem, error) {
	var rows []model.PluginRegistration
	if err := s.db.Order("plugin_id").Find(&rows).Error; err != nil {
		return nil, pkg.NewAppErrorWithCause(pkg.INTERNAL_ERROR, err)
	}
	items := make([]PluginItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PluginItem{
			PluginID:  row.PluginID,
			Source:    row.Source,
			Version:   row.Version,
			Enabled:   row.Enabled,
			RoutePath: fmt.Sprintf("/plugins/%s/", row.PluginID),
		})
	}
	return items, nil
}

// RuntimeInstall 安装指定语言版本；Python 未开放时返回 unsupported=true（code=0）。
func (s *PluginService) RuntimeInstall(ctx context.Context, kind, version, mirror string) (*PluginRuntimeInstallResult, error) {
	if err := s.requireManager(); err != nil {
		return nil, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	version = strings.TrimSpace(version)
	if kind == "" || version == "" {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "kind and version are required")
	}
	if kind != "node" && kind != "python" {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "unsupported runtime kind")
	}
	installed, unsupported, err := s.manager.RuntimeInstall(ctx, kind, version, strings.TrimSpace(mirror))
	if errors.Is(err, PluginUnsupported) || unsupported {
		return &PluginRuntimeInstallResult{Unsupported: true}, nil
	}
	if err != nil {
		return nil, s.managerErr(err)
	}
	return &PluginRuntimeInstallResult{Installed: installed}, nil
}

// RuntimeUse 切换当前激活版本；不支持的语言返回 unsupported=true（code=0）。
func (s *PluginService) RuntimeUse(ctx context.Context, kind, version string) (*PluginRuntimeUseResult, error) {
	if err := s.requireManager(); err != nil {
		return nil, err
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	version = strings.TrimSpace(version)
	if kind == "" || version == "" {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "kind and version are required")
	}
	ok, unsupported, err := s.manager.RuntimeUse(ctx, kind, version)
	if errors.Is(err, PluginUnsupported) || unsupported {
		return &PluginRuntimeUseResult{Unsupported: true}, nil
	}
	if err != nil {
		return nil, s.managerErr(err)
	}
	return &PluginRuntimeUseResult{OK: ok}, nil
}

// ListRuntimes 返回已安装版本；Manager 尚未就绪时返回空列表而不是报错。
func (s *PluginService) ListRuntimes(ctx context.Context, kind string) ([]PluginRuntimeVersion, error) {
	if err := s.requireManager(); err != nil {
		return nil, err
	}
	versions, err := s.manager.RuntimeList(ctx, strings.ToLower(strings.TrimSpace(kind)))
	if errors.Is(err, PluginUnsupported) {
		return []PluginRuntimeVersion{}, nil
	}
	if err != nil {
		return nil, s.managerErr(err)
	}
	out := make([]PluginRuntimeVersion, 0, len(versions))
	for _, v := range versions {
		out = append(out, PluginRuntimeVersion{Kind: v.GetKind(), Version: v.GetVersion(), Active: v.GetActive()})
	}
	return out, nil
}
