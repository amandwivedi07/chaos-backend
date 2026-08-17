// Adding people to a conversation, and the invite link that lets someone
// join one they were sent.
package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/middleware"
)

func (h *Handler) AddMembers(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.AddMembersRequest](c)
	if !ok {
		return
	}
	if len(req.MemberIDs) == 0 && strings.TrimSpace(req.Name) == "" {
		response.Error(c, apperrors.BadRequest("Name someone to add"))
		return
	}
	conv, err := h.conversations.AddMembers(c.Request.Context(), convID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "They're in", conv)
}

// Invite returns the shareable link for a conversation you are already in.
func (h *Handler) Invite(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	invite, err := h.conversations.Invite(c.Request.Context(), convID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Invite", invite)
}

// Preview renders an invite for someone who has not joined yet. It exposes
// only the title and the members' first names — enough to know what you are
// walking into, and nothing that was said.
func (h *Handler) Preview(c *gin.Context) {
	convID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid id"))
		return
	}
	invite, err := h.previews.Preview(c.Request.Context(), convID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Invite", invite)
}

func (h *Handler) Join(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.JoinRequest](c)
	if !ok {
		return
	}
	conv, err := h.conversations.Join(c.Request.Context(), convID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "You're in", conv)
}

// SearchUsers godoc
// @Summary Search people to add to a conversation
// @Tags directory
// @Param q query string true "at least 2 characters"
// @Success 200 {object} response.Body
// @Router /directory/search [get]
func (h *Handler) SearchUsers(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return
	}
	// A blank query asks for the people you already know; one character is
	// still too little to search on, so it answers the same way rather than
	// flashing a list of strangers on the first keystroke.
	query := strings.TrimSpace(c.Query("q"))
	if len(query) == 1 {
		query = ""
	}
	users, err := h.conversations.SearchUsers(c.Request.Context(), query, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Found", users)
}
