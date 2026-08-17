// Package mapper converts between domain entities and DTOs.
// One direction per function; no logic beyond shape translation.
package mapper

import (
	"github.com/chaosapp/backend/internal/domain/user/dto"
	"github.com/chaosapp/backend/internal/domain/user/entity"
)

func ToUserResponse(u *entity.User) dto.UserResponse {
	return dto.UserResponse{
		ID:            u.ID.String(),
		Email:         u.Email,
		Name:          u.Name,
		Handle:        u.Handle,
		Role:          u.Role,
		PaletteID:     u.PaletteID,
		AvatarURL:     u.AvatarURL,
		EmailVerified: u.IsEmailVerified(),
		CreatedAt:     u.CreatedAt,
		UpdatedAt:     u.UpdatedAt,
	}
}

func ToUserResponses(users []entity.User) []dto.UserResponse {
	out := make([]dto.UserResponse, len(users))
	for i := range users {
		out[i] = ToUserResponse(&users[i])
	}
	return out
}
