// Package entity holds auth domain entities: refresh-token sessions and
// one-time action tokens.
package entity

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken is one session credential (stored hashed).
// FamilyID groups a rotation chain; reuse of a revoked member burns the family.
type RefreshToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	FamilyID   uuid.UUID
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *uuid.UUID
	CreatedAt  time.Time
}

func (t RefreshToken) Active(now time.Time) bool {
	return t.RevokedAt == nil && now.Before(t.ExpiresAt)
}

// ActionToken is a one-time token for email verification / password reset.
type ActionToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Purpose   string // verify_email | reset_password
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

const (
	PurposeVerifyEmail   = "verify_email"
	PurposeResetPassword = "reset_password"
)

func (t ActionToken) Usable(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.ExpiresAt)
}
