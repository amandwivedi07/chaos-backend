// Package device stores push tokens per user.
package device

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
)

type Device struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null"`
	Platform  string    `gorm:"not null"`
	PushToken string    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Device) TableName() string { return "devices" }

// Repository is the persistence port for push devices.
type Repository interface {
	Register(ctx context.Context, userID uuid.UUID, platform, token string) error
	Unregister(ctx context.Context, userID uuid.UUID, token string) error
	TokensFor(ctx context.Context, userIDs []uuid.UUID) ([]string, error)
	DeleteTokens(ctx context.Context, tokens []string) error
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
}

type gormRepository struct{ db *gorm.DB }

var _ Repository = (*gormRepository)(nil)

func NewGorm(db *gorm.DB) Repository { return &gormRepository{db: db} }

// Register claims the token for this user — the same phone signing into a new
// account must not keep delivering to the old one.
func (r *gormRepository) Register(ctx context.Context, userID uuid.UUID, platform, token string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO devices (user_id, platform, push_token)
		VALUES (?, ?, ?)
		ON CONFLICT (push_token) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    platform = EXCLUDED.platform,
		    updated_at = now()`, userID, platform, token).Error
	if err != nil {
		return apperrors.Database("devices.register", err)
	}
	return nil
}

func (r *gormRepository) Unregister(ctx context.Context, userID uuid.UUID, token string) error {
	err := r.db.WithContext(ctx).
		Exec(`DELETE FROM devices WHERE user_id = ? AND push_token = ?`,
			userID, token).Error
	if err != nil {
		return apperrors.Database("devices.unregister", err)
	}
	return nil
}

func (r *gormRepository) TokensFor(ctx context.Context, userIDs []uuid.UUID) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var rows []struct{ PushToken string }
	err := r.db.WithContext(ctx).
		Raw(`SELECT push_token FROM devices WHERE user_id IN ?`, userIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("devices.tokens_for", err)
	}
	tokens := make([]string, len(rows))
	for i, row := range rows {
		tokens[i] = row.PushToken
	}
	return tokens, nil
}

func (r *gormRepository) DeleteTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).
		Exec(`DELETE FROM devices WHERE push_token IN ?`, tokens).Error
	if err != nil {
		return apperrors.Database("devices.prune", err)
	}
	return nil
}

func (r *gormRepository) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Exec(`DELETE FROM devices WHERE user_id = ?`, userID).Error
	if err != nil {
		return apperrors.Database("devices.delete_all", err)
	}
	return nil
}
