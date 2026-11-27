package handler

import (
	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
)

type PushTokenHandler struct {
	svc *service.PushTokenService
}

func NewPushTokenHandler(svc *service.PushTokenService) *PushTokenHandler {
	return &PushTokenHandler{svc: svc}
}

// Register
// @Summary      上报推送 token
// @Description  持久化当前登录用户的移动端推送 token，按 token 幂等覆盖。
// @Tags         用户
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body  object{token=string,platform=string}  true  "推送 token"
// @Success      200  {object}  pkg.Response
// @Router       /user/push-token [post]
func (h *PushTokenHandler) Register(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Platform string `json:"platform"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	if err := h.svc.Register(middleware.CurrentUserUUID(c), req.Token, req.Platform); err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, gin.H{"ok": true})
}
