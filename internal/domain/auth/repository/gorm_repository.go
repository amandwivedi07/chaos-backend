package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/auth/entity"
)

// GORM persistence models — private to this package.

type refreshTokenModel struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID `gorm:"type:uuid;not null"`
	TokenHash  string    `gorm:"uniqueIndex;not null"`
	FamilyID   uuid.UUID `gorm:"type:uuid;not null"`
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *uuid.UUID `gorm:"type:uuid"`
	CreatedAt  time.Time
}

func (refreshTokenModel) TableName() string { return "refresh_tokens" }

type actionTokenModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null"`
	Purpose   string    `gorm:"not null"`
	TokenHash string    `gorm:"uniqueIndex;not null"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (actionTokenModel) TableName() string { return "action_tokens" }

type gormTokenRepository struct {
	db *gorm.DB
}

var _ TokenRepository = (*gormTokenRepository)(nil)

func NewGorm(db *gorm.DB) TokenRepository { return &gormTokenRepository{db: db} }

// ---- refresh sessions ----

func (r *gormTokenRepository) CreateRefresh(ctx context.Context, t *entity.RefreshToken) error {
	m := refreshTokenModel{
		ID: t.ID, UserID: t.UserID, TokenHash: t.TokenHash,
		FamilyID: t.FamilyID, ExpiresAt: t.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return apperrors.Database("refresh_tokens.create", err)
	}
	t.CreatedAt = m.CreatedAt
	return nil
}

func (r *gormTokenRepository) GetRefreshByHash(ctx context.Context, hash string) (*entity.RefreshToken, error) {
	var m refreshTokenModel
	err := r.db.WithContext(ctx).First(&m, "token_hash = ?", hash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.Unauthorized("Unknown refresh token")
		}
		return nil, apperrors.Database("refresh_tokens.get", err)
	}
	return &entity.RefreshToken{
		ID: m.ID, UserID: m.UserID, TokenHash: m.TokenHash, FamilyID: m.FamilyID,
		ExpiresAt: m.ExpiresAt, RevokedAt: m.RevokedAt, ReplacedBy: m.ReplacedBy,
		CreatedAt: m.CreatedAt,
	}, nil
}

// RevokeRefresh atomically claims the token; false means someone else already
// revoked it (concurrent rotation attempt — the caller must treat that as reuse).
func (r *gormTokenRepository) RevokeRefresh(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&refreshTokenModel{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]any{"revoked_at": time.Now().UTC(), "replaced_by": replacedBy})
	if res.Error != nil {
		return false, apperrors.Database("refresh_tokens.revoke", res.Error)
	}
	return res.RowsAffected > 0, nil
}

// LinkReplacement records which token superseded a revoked one (audit trail).
func (r *gormTokenRepository) LinkReplacement(ctx context.Context, id, replacedBy uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Model(&refreshTokenModel{}).
		Where("id = ?", id).Update("replaced_by", replacedBy)
	if res.Error != nil {
		return false, apperrors.Database("refresh_tokens.link", res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *gormTokenRepository) RevokeFamily(ctx context.Context, familyID uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&refreshTokenModel{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", time.Now().UTC()).Error
	if err != nil {
		return apperrors.Database("refresh_tokens.revoke_family", err)
	}
	return nil
}

func (r *gormTokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&refreshTokenModel{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", time.Now().UTC()).Error
	if err != nil {
		return apperrors.Database("refresh_tokens.revoke_all", err)
	}
	return nil
}

// ---- action tokens ----

func (r *gormTokenRepository) CreateAction(ctx context.Context, t *entity.ActionToken) error {
	m := actionTokenModel{
		ID: t.ID, UserID: t.UserID, Purpose: t.Purpose,
		TokenHash: t.TokenHash, ExpiresAt: t.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return apperrors.Database("action_tokens.create", err)
	}
	return nil
}

func (r *gormTokenRepository) GetActionByHash(ctx context.Context, purpose, hash string) (*entity.ActionToken, error) {
	var m actionTokenModel
	err := r.db.WithContext(ctx).
		First(&m, "purpose = ? AND token_hash = ?", purpose, hash).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.BadRequest("Invalid or expired token")
		}
		return nil, apperrors.Database("action_tokens.get", err)
	}
	return &entity.ActionToken{
		ID: m.ID, UserID: m.UserID, Purpose: m.Purpose, TokenHash: m.TokenHash,
		ExpiresAt: m.ExpiresAt, UsedAt: m.UsedAt, CreatedAt: m.CreatedAt,
	}, nil
}

func (r *gormTokenRepository) MarkActionUsed(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&actionTokenModel{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", time.Now().UTC()).Error
	if err != nil {
		return apperrors.Database("action_tokens.mark_used", err)
	}
	return nil
}

func (r *gormTokenRepository) InvalidateActions(ctx context.Context, userID uuid.UUID, purpose string) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND purpose = ? AND used_at IS NULL", userID, purpose).
		Delete(&actionTokenModel{}).Error
	if err != nil {
		return apperrors.Database("action_tokens.invalidate", err)
	}
	return nil
}
