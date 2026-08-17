// Package handler is the thin HTTP layer for the profile module.
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/profile/dto"
	"github.com/chaosapp/backend/internal/domain/profile/service"
	"github.com/chaosapp/backend/internal/middleware"
)

type Handler struct{ profiles service.Service }

func New(profiles service.Service) *Handler { return &Handler{profiles: profiles} }

func caller(c *gin.Context) (uuid.UUID, bool) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not signed in"))
		return uuid.Nil, false
	}
	return userID, true
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

// Get godoc
// @Summary Your profile and everything Chaos knows about you
// @Tags profile
// @Success 200 {object} response.Body
// @Router /me/profile [get]
func (h *Handler) Get(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	profile, err := h.profiles.Get(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Profile", profile)
}

func (h *Handler) Update(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	req, ok := bind[dto.UpdateProfileRequest](c)
	if !ok {
		return
	}
	profile, err := h.profiles.Update(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Saved", profile)
}

// AddFact godoc
// @Summary Tell Chaos something about you — free text in, facts out
// @Tags profile
// @Success 201 {object} response.Body
// @Router /me/facts [post]
func (h *Handler) AddFact(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	req, ok := bind[dto.AddFactRequest](c)
	if !ok {
		return
	}
	out, err := h.profiles.AddFact(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Noted", out)
}

// UpdateFact edits a value, or confirms one with "Looks right".
func (h *Handler) UpdateFact(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	factID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid id"))
		return
	}
	req, ok := bind[dto.UpdateFactRequest](c)
	if !ok {
		return
	}
	fact, err := h.profiles.UpdateFact(c.Request.Context(), userID, factID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Updated", fact)
}

// DeleteFact is "Forget".
func (h *Handler) DeleteFact(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	factID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid id"))
		return
	}
	if err := h.profiles.DeleteFact(c.Request.Context(), userID, factID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Forgotten", nil)
}

// Learn godoc
// @Summary Bring your context over from another assistant
// @Tags profile
// @Success 201 {object} response.Body
// @Router /me/facts/learn [post]
func (h *Handler) Learn(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	req, ok := bind[dto.LearnRequest](c)
	if !ok {
		return
	}
	out, err := h.profiles.Learn(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Learnt", out)
}

// Refresh godoc
// @Summary Mine what you have said since last time for new facts
// @Tags profile
// @Success 200 {object} response.Body
// @Router /me/facts/refresh [post]
//
// Safe to call on every profile open: it short-circuits on a counter before
// spending anything.
func (h *Handler) Refresh(c *gin.Context) {
	userID, ok := caller(c)
	if !ok {
		return
	}
	out, err := h.profiles.Refresh(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Up to date", out)
}

// Prompt returns the ready-made prompt and the deep link that opens the other
// assistant with it already typed.
func (h *Handler) Prompt(c *gin.Context) {
	out, err := h.profiles.Prompt(c.Param("source"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Prompt", out)
}
