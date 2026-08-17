// Package handler is the thin HTTP layer for authentication.
package handler

import (
	"github.com/gin-gonic/gin"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/auth/dto"
	"github.com/chaosapp/backend/internal/domain/auth/service"
	usermapper "github.com/chaosapp/backend/internal/domain/user/mapper"
	"github.com/chaosapp/backend/internal/middleware"
)

type Handler struct {
	auth service.AuthService
}

func New(auth service.AuthService) *Handler { return &Handler{auth: auth} }

// bindAndValidate parses JSON + runs validator; one helper so every endpoint
// behaves identically.
func bindAndValidate[T any](c *gin.Context) (T, bool) {
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

// Register godoc
// @Summary Register with email + password
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.RegisterRequest true "registration"
// @Success 201 {object} response.Body{data=dto.AuthResponse}
// @Router  /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	req, ok := bindAndValidate[dto.RegisterRequest](c)
	if !ok {
		return
	}
	user, tokens, err := h.auth.Register(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, "Account created — check your inbox to verify your email", dto.AuthResponse{
		User: usermapper.ToUserResponse(user), Tokens: tokens,
	})
}

// Login godoc
// @Summary Login with email + password
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.LoginRequest true "credentials"
// @Success 200 {object} response.Body{data=dto.AuthResponse}
// @Router  /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	req, ok := bindAndValidate[dto.LoginRequest](c)
	if !ok {
		return
	}
	user, tokens, err := h.auth.Login(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Signed in", dto.AuthResponse{
		User: usermapper.ToUserResponse(user), Tokens: tokens,
	})
}

// FirebaseLogin godoc
// @Summary Sign in with Apple or Google (Firebase ID token)
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.FirebaseLoginRequest true "firebase id token"
// @Success 200 {object} response.Body{data=dto.AuthResponse}
// @Router  /auth/firebase [post]
func (h *Handler) FirebaseLogin(c *gin.Context) {
	req, ok := bindAndValidate[dto.FirebaseLoginRequest](c)
	if !ok {
		return
	}
	user, tokens, isNew, err := h.auth.SignInWithFirebase(c.Request.Context(), req.IDToken)
	if err != nil {
		response.Error(c, err)
		return
	}
	message := "Signed in"
	if isNew {
		message = "Welcome to Space"
	}
	response.OK(c, message, dto.AuthResponse{
		User: usermapper.ToUserResponse(user), Tokens: tokens, IsNew: isNew,
	})
}

// Refresh godoc
// @Summary Rotate the refresh token
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.RefreshTokenRequest true "refresh token"
// @Success 200 {object} response.Body{data=dto.TokenPair}
// @Router  /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	req, ok := bindAndValidate[dto.RefreshTokenRequest](c)
	if !ok {
		return
	}
	pair, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Token refreshed", pair)
}

// Logout godoc
// @Summary Revoke a refresh token
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.RefreshTokenRequest true "refresh token"
// @Success 200 {object} response.Body
// @Router  /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	req, ok := bindAndValidate[dto.RefreshTokenRequest](c)
	if !ok {
		return
	}
	if err := h.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Signed out", nil)
}

// ForgotPassword godoc
// @Summary Request a password-reset email
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.ForgotPasswordRequest true "email"
// @Success 200 {object} response.Body
// @Router  /auth/forgot-password [post]
func (h *Handler) ForgotPassword(c *gin.Context) {
	req, ok := bindAndValidate[dto.ForgotPasswordRequest](c)
	if !ok {
		return
	}
	if err := h.auth.ForgotPassword(c.Request.Context(), req.Email); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "If that email exists, a reset link is on its way", nil)
}

// ResetPassword godoc
// @Summary Reset password with a one-time token
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.ResetPasswordRequest true "token + new password"
// @Success 200 {object} response.Body
// @Router  /auth/reset-password [post]
func (h *Handler) ResetPassword(c *gin.Context) {
	req, ok := bindAndValidate[dto.ResetPasswordRequest](c)
	if !ok {
		return
	}
	if err := h.auth.ResetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Password reset — sign in with your new password", nil)
}

// VerifyEmail godoc
// @Summary Verify email with a one-time token
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.VerifyEmailRequest true "token"
// @Success 200 {object} response.Body
// @Router  /auth/verify-email [post]
func (h *Handler) VerifyEmail(c *gin.Context) {
	req, ok := bindAndValidate[dto.VerifyEmailRequest](c)
	if !ok {
		return
	}
	if err := h.auth.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Email verified", nil)
}

// ChangePassword godoc
// @Summary Change password (authenticated)
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.ChangePasswordRequest true "current + new password"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router  /auth/change-password [post]
func (h *Handler) ChangePassword(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not authenticated"))
		return
	}
	req, okReq := bindAndValidate[dto.ChangePasswordRequest](c)
	if !okReq {
		return
	}
	err := h.auth.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Password changed — other sessions were signed out", nil)
}

// UpdateMe godoc
// @Summary Update the current user's profile (name, avatar)
// @Tags    auth
// @Accept  json
// @Produce json
// @Param   body body dto.UpdateMeRequest true "fields"
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router  /auth/me [patch]
func (h *Handler) UpdateMe(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not authenticated"))
		return
	}
	req, okReq := bindAndValidate[dto.UpdateMeRequest](c)
	if !okReq {
		return
	}
	user, err := h.auth.UpdateProfile(c.Request.Context(), userID, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Profile updated", usermapper.ToUserResponse(user))
}

// DeleteAccount godoc
// @Summary Permanently delete my account
// @Tags    auth
// @Produce json
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router  /auth/account [delete]
func (h *Handler) DeleteAccount(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not authenticated"))
		return
	}
	if err := h.auth.DeleteAccount(c.Request.Context(), userID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Your account and its traces are gone", nil)
}

// Me godoc
// @Summary Current authenticated user
// @Tags    auth
// @Produce json
// @Success 200 {object} response.Body
// @Security BearerAuth
// @Router  /auth/me [get]
func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Error(c, apperrors.Unauthorized("Not authenticated"))
		return
	}
	user, err := h.auth.CurrentUser(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, "Current user", usermapper.ToUserResponse(user))
}
