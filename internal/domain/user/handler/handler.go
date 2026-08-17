// Package handler is the thin HTTP layer for the user module:
// parse → validate DTO → call service → map → respond. No business logic.
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/request"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/user/dto"
	"github.com/chaosapp/backend/internal/domain/user/mapper"
	"github.com/chaosapp/backend/internal/domain/user/repository"
	"github.com/chaosapp/backend/internal/domain/user/service"
)

type Handler struct {
	users service.UserService
}

func New(users service.UserService) *Handler { return &Handler{users: users} }

// Create godoc
// @Summary  Create a user (admin)
// @Tags     users
// @Accept   json
// @Produce  json
// @Param    body body dto.CreateUserRequest true "user"
// @Success  201 {object} response.Body{data=dto.UserResponse}
// @Security BearerAuth
// @Router   /users [post]
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperrors.BadRequest("Invalid JSON body"))
		return
	}
	if verr := utils.ValidateStruct(req); verr != nil {
		response.Error(c, verr)
		return
	}
	user, err := h.users.Create(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "User created", mapper.ToUserResponse(user))
}

// Get godoc
// @Summary  Get a user by id
// @Tags     users
// @Produce  json
// @Param    id path string true "user id (uuid)"
// @Success  200 {object} response.Body{data=dto.UserResponse}
// @Security BearerAuth
// @Router   /users/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	user, err := h.users.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "User fetched", mapper.ToUserResponse(user))
}

// List godoc
// @Summary  List users with pagination, search, filter, sort (admin)
// @Tags     users
// @Produce  json
// @Param    page query int false "page"
// @Param    limit query int false "limit"
// @Param    search query string false "search name/email"
// @Param    sort query string false "name|email|created_at|updated_at"
// @Param    order query string false "asc|desc"
// @Param    role query string false "user|admin"
// @Success  200 {object} response.Body{data=response.ListData}
// @Security BearerAuth
// @Router   /users [get]
func (h *Handler) List(c *gin.Context) {
	p := request.ParsePagination(c)
	filter := repository.ListFilter{Role: c.Query("role")}
	if v := c.Query("verified"); v == "true" || v == "false" {
		verified := v == "true"
		filter.Verified = &verified
	}
	users, total, err := h.users.List(c.Request.Context(), p, filter)
	if err != nil {
		response.Error(c, err)
		return
	}
	totalPages := int((total + int64(p.Limit) - 1) / int64(p.Limit))
	response.List(c, "Users fetched", mapper.ToUserResponses(users), response.Meta{
		Page: p.Page, Limit: p.Limit, Total: total, TotalPages: totalPages,
	})
}

// Update godoc
// @Summary  Update a user (admin)
// @Tags     users
// @Accept   json
// @Produce  json
// @Param    id path string true "user id (uuid)"
// @Param    body body dto.UpdateUserRequest true "fields"
// @Success  200 {object} response.Body{data=dto.UserResponse}
// @Security BearerAuth
// @Router   /users/{id} [patch]
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, apperrors.BadRequest("Invalid JSON body"))
		return
	}
	if verr := utils.ValidateStruct(req); verr != nil {
		response.Error(c, verr)
		return
	}
	user, err := h.users.Update(c.Request.Context(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "User updated", mapper.ToUserResponse(user))
}

// Delete godoc
// @Summary  Soft-delete a user (admin)
// @Tags     users
// @Produce  json
// @Param    id path string true "user id (uuid)"
// @Success  200 {object} response.Body
// @Security BearerAuth
// @Router   /users/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.users.Delete(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "User deleted", nil)
}

func parseID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperrors.BadRequest("Invalid user id"))
		return uuid.Nil, false
	}
	return id, true
}
