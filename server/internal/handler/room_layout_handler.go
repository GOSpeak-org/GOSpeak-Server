package handler

import (
	"GOSpeak/internal/middleware"
	"GOSpeak/internal/permcode"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
)

type RoomLayoutHandler struct {
	svc       *service.RoomLayoutService
	permSvc   *service.PermissionService
	domainSvc *service.DomainService
	onSaved   func(domainUUID string)
}

func NewRoomLayoutHandler(svc *service.RoomLayoutService, permSvc *service.PermissionService, domainSvc *service.DomainService, onSaved func(string)) *RoomLayoutHandler {
	return &RoomLayoutHandler{svc: svc, permSvc: permSvc, domainSvc: domainSvc, onSaved: onSaved}
}

type roomLayoutGetReq struct {
	DomainUUID string `json:"domain_uuid" binding:"required"`
}

type roomLayoutSaveReq struct {
	DomainUUID         string                  `json:"domain_uuid" binding:"required"`
	Groups             []service.RoomGroupInput `json:"groups"`
	UngroupedPosition  int                     `json:"ungrouped_position"`
	UngroupedRoomUUIDs []string                `json:"ungrouped_room_uuids"`
}

func (h *RoomLayoutHandler) canManage(c *gin.Context, domainUUID string) bool {
	if h.permSvc != nil && h.permSvc.HasPermission(middleware.CurrentRole(c), permcode.PermDomainRoomLayoutManage) {
		return true
	}
	if h.domainSvc == nil {
		return false
	}
	return h.domainSvc.HasDomainPermission(domainUUID, middleware.CurrentUserUUID(c), permcode.PermDomainRoomLayoutManage)
}

// Get 读取域内房间分组布局。
// @Summary 读取域内房间分组布局
// @Tags 域
// @Accept json
// @Produce json
// @Param request body object true "域 UUID"
// @Success 200 {object} service.LayoutView
// @Router /domain/room-layout/get [post]
func (h *RoomLayoutHandler) Get(c *gin.Context) {
	var req roomLayoutGetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	view, appErr := h.svc.GetLayout(req.DomainUUID, h.canManage(c, req.DomainUUID))
	if appErr != nil {
		pkg.HandleError(c, appErr)
		return
	}
	pkg.Success(c, view)
}

// Save 保存域内房间分组与排序。
// @Summary 保存域内房间分组与排序
// @Tags 域
// @Accept json
// @Produce json
// @Param request body object true "完整布局"
// @Success 200 {object} service.LayoutView
// @Router /domain/room-layout/save [post]
func (h *RoomLayoutHandler) Save(c *gin.Context) {
	var req roomLayoutSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	if !h.canManage(c, req.DomainUUID) {
		pkg.Fail(c, pkg.FORBIDDEN, "无配置房间分组的权限")
		return
	}
	view, appErr := h.svc.SaveLayout(req.DomainUUID, req.Groups, req.UngroupedRoomUUIDs, req.UngroupedPosition)
	if appErr != nil {
		pkg.HandleError(c, appErr)
		return
	}
	if h.onSaved != nil {
		h.onSaved(req.DomainUUID)
	}
	pkg.Success(c, view)
}
