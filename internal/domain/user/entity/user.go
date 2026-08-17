// Package entity holds the pure domain entity for users.
// No GORM tags, no JSON tags — persistence and transport shapes live in the
// repository model and DTOs respectively.
package entity

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID
	Email           string
	FirebaseUID     string // set for Apple/Google accounts
	Provider        string // password | google.com | apple.com
	PasswordHash    string
	Name            string
	Handle          string // @handle derived from the email local-part
	Role            string
	PaletteID       string
	AvatarURL       string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (u User) IsAdmin() bool         { return u.Role == "admin" }
func (u User) IsEmailVerified() bool { return u.EmailVerifiedAt != nil }
