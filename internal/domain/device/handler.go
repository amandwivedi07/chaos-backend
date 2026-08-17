package device

import (
	"github.com/gin-gonic/gin"

	"github.com/chaosapp/backend/internal/common/constants"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/middleware"
)

type RegisterRequest struct {
	Platform  string `json:"platform"   validate:"required,oneof=ios android"`
	PushToken string `json:"push_token" validate:"required,min=10,max=512"`
}

type Handler struct{ repo Repository }

func NewHandler(repo Repository) *Handler { return &Handler{repo: repo} }

// Register godoc
// @Summary  Register this device for push notifications
// @Tags     devices
// @Accept   json
// @Produce  json
// @Param    body body device.RegisterRequest true "device"
// @Success  200 {object} response.Body
// @Security BearerAuth
// @Router   /me/devices [post]
func (h *Handler) Register(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not authenticated"))
		return
	}
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperrors.BadRequest("Invalid JSON body"))
		return
	}
	if verr := utils.ValidateStruct(req); verr != nil {
		response.Error(c, verr)
		return
	}
	if err := h.repo.Register(c.Request.Context(), userID, req.Platform, req.PushToken); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Device registered", nil)
}

// Unregister godoc
// @Summary  Stop push to this device (sign-out)
// @Tags     devices
// @Produce  json
// @Param    token path string true "push token"
// @Success  200 {object} response.Body
// @Security BearerAuth
// @Router   /me/devices/{token} [delete]
func (h *Handler) Unregister(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not authenticated"))
		return
	}
	token := c.Param("token")
	if token == "" {
		response.Error(c, apperrors.BadRequest("Missing token"))
		return
	}
	if err := h.repo.Unregister(c.Request.Context(), userID, token); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Device removed", nil)
}

var _ = constants.CtxUserID // keep the constants dependency explicit
