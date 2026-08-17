// Package handler is the thin HTTP layer for conversations and messages.
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/domain/conversation/service"
	"github.com/chaosapp/backend/internal/middleware"
)

type Handler struct {
	conversations service.ConversationService
	// previews resolves an invite for someone who is not a member yet.
	previews service.Previewer
}

func New(conversations service.ConversationService, previews service.Previewer) *Handler {
	return &Handler{conversations: conversations, previews: previews}
}

// userAnd pulls the caller and a uuid path param, answering the request itself
// on failure so call sites stay two lines long.
func userAnd(c *gin.Context, param string) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid id"))
		return uuid.Nil, uuid.Nil, false
	}
	return userID, id, true
}

// bind reads and validates a JSON body, answering the request on failure.
func bind[T any](c *gin.Context) (T, bool) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperrors.BadRequest("Invalid JSON body"))
		return req, false
	}
	if verr := utils.ValidateStruct(req); verr != nil {
		response.Error(c, verr)
		return req, false
	}
	return req, true
}

// ---- conversations ----

// List godoc
// @Summary Every conversation you are in, newest activity first
// @Tags conversations
// @Success 200 {object} response.Body
// @Router /conversations [get]
func (h *Handler) List(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return
	}
	list, err := h.conversations.List(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Conversations fetched", list)
}

// Create godoc
// @Summary Start a conversation
// @Tags conversations
// @Success 201 {object} response.Body
// @Router /conversations [post]
func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return
	}
	req, ok := bind[dto.CreateConversationRequest](c)
	if !ok {
		return
	}
	conv, err := h.conversations.Create(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Something to figure out", conv)
}

func (h *Handler) Get(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	conv, err := h.conversations.Get(c.Request.Context(), convID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Conversation fetched", conv)
}

// Update renames a conversation, or changes its emoji.
func (h *Handler) Update(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.UpdateConversationRequest](c)
	if !ok {
		return
	}
	conv, err := h.conversations.Update(c.Request.Context(), convID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Renamed", conv)
}

func (h *Handler) Leave(c *gin.Context) {
	userID, convID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	if err := h.conversations.Leave(c.Request.Context(), convID, userID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Left quietly", nil)
}
