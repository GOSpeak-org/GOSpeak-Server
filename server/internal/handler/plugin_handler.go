package handler

import (
	"encoding/json"

	"GOSpeak/internal/pkg"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
)

// PluginHandler 提供插件生命周期、配置与运行时的管理入口。
type PluginHandler struct {
	svc *service.PluginService
}

func NewPluginHandler(svc *service.PluginService) *PluginHandler {
	return &PluginHandler{svc: svc}
}

func (h *PluginHandler) requireService(c *gin.Context) bool {
	if h.svc == nil {
		pkg.Fail(c, pkg.INTERNAL_ERROR)
		return false
	}
	return true
}

func (h *PluginHandler) pluginID(c *gin.Context, id string) (string, bool) {
	id = trimSpace(id)
	if id == "" {
		pkg.Fail(c, pkg.INVALID_PARAMS, "plugin_id is required")
		return "", false
	}
	return id, true
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

// Install
// @Summary      安装插件
// @Description  从市场或 GitHub 拉取插件并启动
// @Tags         插件
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  object{source=string,url=string,ref=string,version=string}  true  "安装来源"
// @Success      200      {object}  pkg.Response
// @Router       /plugin/install [post]
func (h *PluginHandler) Install(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		Source  string `json:"source"`
		URL     string `json:"url"`
		Ref     string `json:"ref"`
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Source == "" && req.URL == "" {
		pkg.Fail(c, pkg.INVALID_PARAMS, "source or url is required")
		return
	}
	if err := h.svc.Install(c.Request.Context(), req.Source, req.URL, req.Ref, req.Version); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"source": req.Source, "url": req.URL})
}

func (h *PluginHandler) Upgrade(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		PluginID string `json:"plugin_id"`
	}
	_ = c.ShouldBindJSON(&req)
	id, ok := h.pluginID(c, req.PluginID)
	if !ok {
		return
	}
	if err := h.svc.Upgrade(c.Request.Context(), id); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"plugin_id": id})
}

func (h *PluginHandler) Reload(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		PluginID string `json:"plugin_id"`
	}
	_ = c.ShouldBindJSON(&req)
	id, ok := h.pluginID(c, req.PluginID)
	if !ok {
		return
	}
	if err := h.svc.Reload(c.Request.Context(), id); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"plugin_id": id})
}

func (h *PluginHandler) Uninstall(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		PluginID string `json:"plugin_id"`
	}
	_ = c.ShouldBindJSON(&req)
	id, ok := h.pluginID(c, req.PluginID)
	if !ok {
		return
	}
	if err := h.svc.Uninstall(c.Request.Context(), id); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"plugin_id": id})
}

func (h *PluginHandler) Enable(c *gin.Context) {
	h.setEnable(c, true)
}

func (h *PluginHandler) Disable(c *gin.Context) {
	h.setEnable(c, false)
}

func (h *PluginHandler) setEnable(c *gin.Context, enabled bool) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		PluginID string `json:"plugin_id"`
	}
	_ = c.ShouldBindJSON(&req)
	id, ok := h.pluginID(c, req.PluginID)
	if !ok {
		return
	}
	var err error
	if enabled {
		err = h.svc.Enable(c.Request.Context(), id)
	} else {
		err = h.svc.Disable(c.Request.Context(), id)
	}
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"plugin_id": id, "enabled": enabled})
}

func (h *PluginHandler) Config(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		PluginID   string          `json:"plugin_id"`
		ConfigJSON json.RawMessage `json:"config_json"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, "invalid request body")
		return
	}
	id, ok := h.pluginID(c, req.PluginID)
	if !ok {
		return
	}
	raw := string(req.ConfigJSON)
	if raw == "" {
		raw = "{}"
	}
	if !json.Valid([]byte(raw)) {
		pkg.Fail(c, pkg.INVALID_PARAMS, "config_json must be valid JSON")
		return
	}
	item, err := h.svc.SetConfig(id, raw)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, item)
}

// List
// @Summary      插件列表
// @Description  返回已注册插件与启用状态
// @Tags         插件
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  pkg.Response
// @Router       /plugin/list [get]
func (h *PluginHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	items, err := h.svc.List()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"items": items})
}

func (h *PluginHandler) RuntimeInstall(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Version string `json:"version"`
		Mirror  string `json:"mirror"`
	}
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.RuntimeInstall(c.Request.Context(), req.Kind, req.Version, req.Mirror)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, result)
}

func (h *PluginHandler) RuntimeUse(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.RuntimeUse(c.Request.Context(), req.Kind, req.Version)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, result)
}

func (h *PluginHandler) RuntimeList(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	versions, err := h.svc.ListRuntimes(c.Request.Context(), c.Query("kind"))
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"items": versions})
}
