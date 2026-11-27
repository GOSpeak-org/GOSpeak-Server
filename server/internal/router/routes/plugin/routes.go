package plugin

import (
	"GOSpeak/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterProtected 注册插件管理接口；调用方负责挂载 JWT/管理员角色中间件。
func RegisterProtected(r *gin.RouterGroup, h *handler.PluginHandler) {
	r.GET("/list", h.List)
	r.GET("/runtime/list", h.RuntimeList)

	r.POST("/install", h.Install)
	r.POST("/upgrade", h.Upgrade)
	r.POST("/reload", h.Reload)
	r.POST("/uninstall", h.Uninstall)
	r.POST("/enable", h.Enable)
	r.POST("/disable", h.Disable)
	r.POST("/config", h.Config)
	r.POST("/runtime/install", h.RuntimeInstall)
	r.POST("/runtime/use", h.RuntimeUse)
}
