package handler

import (
	"GOSpeak/internal/middleware"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/service"

	"github.com/gin-gonic/gin"
)

type DomainLayoutHandler struct {
	svc *service.DomainLayoutService
}

func NewDomainLayoutHandler(svc *service.DomainLayoutService) *DomainLayoutHandler {
	return &DomainLayoutHandler{svc: svc}
}

func (h *DomainLayoutHandler) Get(c *gin.Context) {
	userUUID := middleware.CurrentUserUUID(c)
	if userUUID == "" {
		pkg.Fail(c, pkg.INVALID_PARAMS, "not authenticated")
		return
	}
	layout, err := h.svc.GetLayout(userUUID)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, layout)
}

func (h *DomainLayoutHandler) Save(c *gin.Context) {
	userUUID := middleware.CurrentUserUUID(c)
	if userUUID == "" {
		pkg.Fail(c, pkg.INVALID_PARAMS, "not authenticated")
		return
	}
	var layout service.Layout
	if err := c.ShouldBindJSON(&layout); err != nil {
		pkg.Fail(c, pkg.INVALID_PARAMS, err.Error())
		return
	}
	saved, err := h.svc.SaveLayout(userUUID, &layout)
	if err != nil {
		pkg.HandleError(c, err)
		return
	}
	pkg.Success(c, saved)
}
