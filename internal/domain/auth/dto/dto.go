// Package dto defines transport shapes for the auth module.
package dto

import userdto "github.com/chaosapp/backend/internal/domain/user/dto"

// ---- Requests ----

type RegisterRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	Name     string `json:"name"     validate:"required,min=2,max=60"`
}

type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// FirebaseLoginRequest carries the ID token minted by Sign in with
// Apple/Google through Firebase on the device.
type FirebaseLoginRequest struct {
	IDToken string `json:"id_token" validate:"required,min=20"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token"    validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

type UpdateMeRequest struct {
	Name      *string `json:"name"       validate:"omitempty,min=2,max=60"`
	AvatarURL *string `json:"avatar_url" validate:"omitempty,url"`
	PaletteID *string `json:"palette_id" validate:"omitempty,oneof=tide mint rose sun iris"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password"     validate:"required,min=8,max=72"`
}

// ---- Responses ----

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // access-token TTL, seconds
}

type AuthResponse struct {
	User   userdto.UserResponse `json:"user"`
	Tokens TokenPair            `json:"tokens"`
	IsNew  bool                 `json:"is_new"` // first sign-in → show onboarding
}
