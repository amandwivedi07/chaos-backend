// Package repository defines the auth persistence port and its GORM adapter.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/domain/auth/entity"
)

// TokenRepository persists refresh sessions and one-time action tokens.
type TokenRepository interface {
	// Refresh sessions.
	CreateRefresh(ctx context.Context, t *entity.RefreshToken) error
	GetRefreshByHash(ctx context.Context, hash string) (*entity.RefreshToken, error)
	// RevokeRefresh returns false when the row was already revoked (lost race).
	RevokeRefresh(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) (bool, error)
	LinkReplacement(ctx context.Context, id, replacedBy uuid.UUID) (bool, error)
	RevokeFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error

	// One-time action tokens.
	CreateAction(ctx context.Context, t *entity.ActionToken) error
	GetActionByHash(ctx context.Context, purpose, hash string) (*entity.ActionToken, error)
	MarkActionUsed(ctx context.Context, id uuid.UUID) error
	InvalidateActions(ctx context.Context, userID uuid.UUID, purpose string) error
}
