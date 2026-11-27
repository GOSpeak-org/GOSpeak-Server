package handler

import (
	"fmt"
	"time"

	"GOSpeak/internal/audit"
	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/service"
	"GOSpeak/internal/signal"

	"github.com/gin-gonic/gin"
)

// MuteBroadcaster 禁言事件广播接口，由 signal.Hub 实现
type MuteBroadcaster interface {
	BroadcastMute(userUUID string, info *signal.MuteInfo)
	BroadcastUnmute(userUUID string)
}

type MuteHandler struct {
	muteSvc     *service.MuteService
	userSvc     *service.UserService
	broadcaster MuteBroadcaster
	auditor     *audit.Service
}

func NewMuteHandler(muteSvc *service.MuteService, userSvc *service.UserService, broadcaster MuteBroadcaster, auditor *audit.Service) *MuteHandler {
	return &MuteHandler{muteSvc: muteSvc, userSvc: userSvc, broadcaster: broadcaster, auditor: auditor}
}

// CreateMute
// @Summary      禁言用户
// @Description  对用户进行定时或永久禁言
// @Tags         禁言
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      object{user_uuid=string,duration=int64,permanent=bool,reason=string}  true  "禁言参数"
// @Success      200      {object}  pkg.Response
// @Router       /mute/create [post]
func (h *MuteHandler) CreateMute(c *gin.Context) {
	var req struct {
		UserUUID  string `json:"user_uuid" binding:"required"`
		Duration  int64  `json:"duration"`
		Permanent bool   `json:"permanent"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	if !req.Permanent && req.Duration <= 0 {
		pkg.Fail(c, pkg.INVALID_PARAMS, "duration is required for non-permanent mute")
		return
	}
	muterUUID := middleware.CurrentUserUUID(c)
	if muterUUID == "" {
		pkg.Fail(c, pkg.TOKEN_NOT_EXIST, "missing user identity in context")
		return
	}
	muter, err := h.userSvc.GetByUUID(muterUUID)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	mute, err := h.muteSvc.MuteUser(muterUUID, req.UserUUID, req.Duration, req.Permanent, req.Reason)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	if h.auditor != nil {
		h.auditor.Log(audit.Entry{
			ActorID:    muter.ID,
			ActorUUID:  muter.UUID,
			ActorName:  muter.Name,
			Action:     audit.ActionMuteUser,
			TargetType: audit.TargetUser,
			TargetID:   req.UserUUID,
			Detail:     fmt.Sprintf("permanent=%v duration=%ds reason=%q", req.Permanent, req.Duration, req.Reason),
			IP:         audit.AuditIP(c),
			Success:    true,
		})
	}
	if h.broadcaster != nil {
		info := &signal.MuteInfo{
			UserUUID:  mute.UserUUID,
			Duration:  mute.Duration,
			Permanent: mute.Permanent,
			Reason:    mute.Reason,
		}
		if mute.ExpiresAt != nil {
			info.ExpiresAt = mute.ExpiresAt.Format(time.RFC3339)
		}
		h.broadcaster.BroadcastMute(req.UserUUID, info)
	}
	pkg.Success(c, mute)
}

// CancelMute
// @Summary      取消禁言
// @Description  取消用户的禁言
// @Tags         禁言
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      object{user_uuid=string}  true  "用户 UUID"
// @Success      200     {object}  pkg.Response
// @Router       /mute/cancel [post]
func (h *MuteHandler) CancelMute(c *gin.Context) {
	var req struct {
		UserUUID string `json:"user_uuid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	if err := h.muteSvc.UnmuteUser(req.UserUUID); err != nil {
		pkg.HandleError(c, err)
		return
	}
	if h.auditor != nil {
		ua, un := auditActor(c)
		h.auditor.Log(audit.Entry{
			ActorUUID:  ua,
			ActorName:  un,
			Action:     audit.ActionUnmuteUser,
			TargetType: audit.TargetUser,
			TargetID:   req.UserUUID,
			IP:         audit.AuditIP(c),
			Success:    true,
		})
	}
	if h.broadcaster != nil {
		h.broadcaster.BroadcastUnmute(req.UserUUID)
	}
	pkg.Success(c, nil)
}

// GetMuteStatus
// @Summary      查询禁言状态
// @Description  查询指定用户是否被禁言
// @Tags         禁言
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      object{user_uuid=string}  true  "用户 UUID"
// @Success      200     {object}  pkg.Response
// @Router       /mute/status [post]
func (h *MuteHandler) GetMuteStatus(c *gin.Context) {
	var req struct {
		UserUUID string `json:"user_uuid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	mute, err := h.muteSvc.GetMuteStatus(req.UserUUID)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, mute)
}

// ListMutes
// @Summary      禁言列表
// @Description  获取所有生效禁言记录
// @Tags         禁言
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  pkg.Response
// @Router       /mute/list [post]
func (h *MuteHandler) ListMutes(c *gin.Context) {
	mutes, err := h.muteSvc.ListActiveMutes()
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, mutes)
}
