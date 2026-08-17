// Package dto defines the transport shapes for the user module.
// Database models are NEVER exposed; every endpoint speaks these types.
package dto

import "time"

// ---- Requests ----

type CreateUserRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	Name     string `json:"name"     validate:"required,min=2,max=60"`
	Role     string `json:"role"     validate:"omitempty,oneof=user admin"`
}

type UpdateUserRequest struct {
	Name      *string `json:"name"       validate:"omitempty,min=2,max=60"`
	AvatarURL *string `json:"avatar_url" validate:"omitempty,url"`
	Role      *string `json:"role"       validate:"omitempty,oneof=user admin"`
}

// ---- Responses ----

type UserResponse struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Handle        string    `json:"handle,omitempty"`
	Role          string    `json:"role"`
	PaletteID     string    `json:"palette_id"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
