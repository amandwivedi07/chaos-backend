package link

import (
	"github.com/gin-gonic/gin"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// Preview godoc
// @Summary Read a link's title, description and picture for a card preview
// @Tags links
// @Param url query string true "The link to preview"
// @Success 200 {object} response.Body
// @Router /links/preview [get]
func (h *Handler) Preview(c *gin.Context) {
	raw := c.Query("url")
	if raw == "" {
		response.Error(c, apperrors.BadRequest("No link to preview"))
		return
	}
	preview, err := h.svc.Preview(c.Request.Context(), raw)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Preview", preview)
}
