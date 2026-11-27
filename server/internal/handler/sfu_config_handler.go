package handler

import (
	"GOSpeak/internal/livekitproxy"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/service"
	"GOSpeak/internal/sfu"

	"github.com/gin-gonic/gin"
)

// SFUSwitchNotifier SFU 热切换后通知信令层强制断连。
type SFUSwitchNotifier interface {
	ForceSFUProviderSwitch(provider string)
}

type SFUConfigHandler struct {
	svc      *service.SFUConfigService
	notifier SFUSwitchNotifier
}

func NewSFUConfigHandler(svc *service.SFUConfigService, notifier SFUSwitchNotifier) *SFUConfigHandler {
	return &SFUConfigHandler{svc: svc, notifier: notifier}
}

// Get 返回当前激活 provider 的配置（密钥已脱敏）。
func (h *SFUConfigHandler) Get(c *gin.Context) {
	cfg, err := h.svc.Get()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, service.ToPublicSFUConfig(cfg))
}

// GetProvider 返回指定 provider 的配置（密钥已脱敏）。
func (h *SFUConfigHandler) GetProvider(c *gin.Context) {
	provider := c.Param("provider")
	cfg, err := h.svc.GetProviderConfig(provider)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, service.ToPublicSFUConfig(cfg))
}

// Update 更新指定 provider 的配置，并将其设为当前激活。
// 请求中密钥字段为空时保留旧值，响应中密钥已脱敏。
func (h *SFUConfigHandler) Update(c *gin.Context) {
	var req service.UpdateSFUConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	cfg, err := h.svc.UpdateFromDTO(&req)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	if h.notifier != nil {
		h.notifier.ForceSFUProviderSwitch(cfg.Provider)
	}
	pkg.Success(c, service.ToPublicSFUConfig(cfg))
}

// GetEffective 返回实际生效的配置视图（含来源标注与浏览器连接地址）。
// 与 Get 的 DB 草稿区分：这里展示运行参数，避免管理页看到的值与真实连接不一致。
func (h *SFUConfigHandler) GetEffective(c *gin.Context) {
	cfg, err := h.svc.EffectiveConfig()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	if cfg.LiveKitPublicURL != "" {
		cfg.BrowserServerURL = cfg.LiveKitPublicURL
	} else {
		cfg.BrowserServerURL = livekitproxy.ServerURLForRequest(c.Request)
	}
	pkg.Success(c, cfg)
}

// ListProviders 返回所有已配置 provider 的列表 + 当前激活的 provider。
func (h *SFUConfigHandler) ListProviders(c *gin.Context) {
	cfgs, active, err := h.svc.ListProviders()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{
		"providers":    service.ToPublicSFUConfigs(cfgs),
		"active":       active,
		"capabilities": sfu.AllProviderCapabilities(),
	})
}
