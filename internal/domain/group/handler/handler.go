// Package handler is the thin HTTP layer for groups.
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/group/dto"
	"github.com/chaosapp/backend/internal/domain/group/service"
	"github.com/chaosapp/backend/internal/middleware"
)

type Handler struct{ groups service.Service }

func New(groups service.Service) *Handler { return &Handler{groups: groups} }

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

// List godoc
// @Summary Your groups
// @Tags groups
// @Success 200 {object} response.Body
// @Router /groups [get]
func (h *Handler) List(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return
	}
	groups, err := h.groups.List(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Groups fetched", groups)
}

// Create godoc
// @Summary Make a group
// @Tags groups
// @Success 201 {object} response.Body
// @Router /groups [post]
func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return
	}
	req, ok := bind[dto.CreateGroupRequest](c)
	if !ok {
		return
	}
	group, err := h.groups.Create(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Group made", group)
}

func (h *Handler) Get(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	group, err := h.groups.Get(c.Request.Context(), groupID, userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Group fetched", group)
}

func (h *Handler) Update(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.UpdateGroupRequest](c)
	if !ok {
		return
	}
	group, err := h.groups.Update(c.Request.Context(), groupID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Renamed", group)
}

func (h *Handler) Delete(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	if err := h.groups.Delete(c.Request.Context(), groupID, userID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Group dissolved", nil)
}

func (h *Handler) AddMember(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.AddMemberRequest](c)
	if !ok {
		return
	}
	group, err := h.groups.AddMember(c.Request.Context(), groupID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "They're in", group)
}

func (h *Handler) RemoveMember(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	memberID, err := uuid.Parse(c.Param("memberId"))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid member id"))
		return
	}
	group, err := h.groups.RemoveMember(c.Request.Context(), groupID, userID, memberID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Removed", group)
}

// AddMemory godoc
// @Summary Tell the group something to remember
// @Tags groups
// @Success 201 {object} response.Body
// @Router /groups/{id}/memory [post]
func (h *Handler) AddMemory(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.AddMemoryRequest](c)
	if !ok {
		return
	}
	group, err := h.groups.AddMemory(c.Request.Context(), groupID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Noted", group)
}

func (h *Handler) DeleteMemory(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	memoryID, err := uuid.Parse(c.Param("memoryId"))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid id"))
		return
	}
	group, err := h.groups.DeleteMemory(c.Request.Context(), groupID, userID, memoryID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Forgotten", group)
}

// Ask godoc
// @Summary Ask a question across everything this group has said
// @Tags groups
// @Success 200 {object} response.Body
// @Router /groups/{id}/ask [post]
func (h *Handler) Ask(c *gin.Context) {
	userID, groupID, ok := userAnd(c, "id")
	if !ok {
		return
	}
	req, ok := bind[dto.AskRequest](c)
	if !ok {
		return
	}
	answer, err := h.groups.Ask(c.Request.Context(), groupID, userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Remembered", answer)
}

// Collaborators godoc
// @Summary The people you share the most conversations with
// @Tags people
// @Success 200 {object} response.Body
// @Router /people/collaborators [get]
func (h *Handler) Collaborators(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return
	}
	people, err := h.groups.Collaborators(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Collaborators", people)
}
