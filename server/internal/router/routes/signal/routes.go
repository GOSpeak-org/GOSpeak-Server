package signal

import (
	"GOSpeak/internal/handler"
	"GOSpeak/internal/middleware"

	"github.com/gin-gonic/gin"
)

func Register(r *gin.RouterGroup, h *handler.SignalHandler) {
	r.POST("/webhook", h.LivekitWebhook)
}

func RegisterProtected(r *gin.RouterGroup, h *handler.SignalHandler) {
	// 房间/参与者枚举会泄露全站 SFU 房间名与跨域在线状态，仅限平台管理员。
	r.GET("/rooms", middleware.RequireRole("admin"), h.ListRooms)
	r.GET("/participants", middleware.RequireRole("admin"), h.ListParticipants)
	r.POST("/token", h.GetJoinToken)
	r.GET("/ws-endpoint", h.GetWSEndpoint)
}
