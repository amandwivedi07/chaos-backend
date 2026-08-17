package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/chaosapp/backend/internal/common/constants"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/request"
	"github.com/chaosapp/backend/internal/domain/user/entity"
)

// gormUserRepository is the GORM adapter for UserRepository.
type gormUserRepository struct {
	db *gorm.DB
}

var _ UserRepository = (*gormUserRepository)(nil)

func NewGorm(db *gorm.DB) UserRepository { return &gormUserRepository{db: db} }

// sortWhitelist prevents ORDER BY injection: only these columns are sortable.
var sortWhitelist = map[string]string{
	"name":       "name",
	"email":      "email",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

func (r *gormUserRepository) Create(ctx context.Context, u *entity.User) error {
	m := fromEntity(u)
	if m.PaletteID == "" {
		m.PaletteID = r.nextPalette(ctx)
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict("An account with this email already exists")
		}
		return apperrors.Database("users.create", err)
	}
	*u = *m.toEntity()
	return nil
}

// nextPalette hands out the five member colours round-robin.
//
// Hashing a name instead looks tempting, but with five buckets short names
// collide constantly — a four-person conversation would routinely have three
// people the same colour, and telling who said what is the entire job of this
// field. Round-robin costs one COUNT at signup and guarantees the spread.
func (r *gormUserRepository) nextPalette(ctx context.Context) string {
	var n int64
	if err := r.db.WithContext(ctx).Model(&userModel{}).Count(&n).Error; err != nil {
		// Never fail a signup over a colour.
		return constants.Palettes[0]
	}
	return constants.Palettes[int(n)%len(constants.Palettes)]
}

func (r *gormUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error) {
	var m userModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("User not found")
		}
		return nil, apperrors.Database("users.get_by_id", err)
	}
	return m.toEntity(), nil
}

func (r *gormUserRepository) GetByEmail(ctx context.Context, email string) (*entity.User, error) {
	var m userModel
	err := r.db.WithContext(ctx).First(&m, "lower(email) = lower(?)", email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("User not found")
		}
		return nil, apperrors.Database("users.get_by_email", err)
	}
	return m.toEntity(), nil
}

// GetByFirebaseUID finds the account behind an Apple/Google identity.
func (r *gormUserRepository) GetByFirebaseUID(ctx context.Context, uid string) (*entity.User, error) {
	var m userModel
	err := r.db.WithContext(ctx).First(&m, "firebase_uid = ?", uid).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("User not found")
		}
		return nil, apperrors.Database("users.get_by_firebase_uid", err)
	}
	return m.toEntity(), nil
}

func (r *gormUserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&userModel{}).
		Where("lower(email) = lower(?)", email).Count(&count).Error
	if err != nil {
		return false, apperrors.Database("users.exists_by_email", err)
	}
	return count > 0, nil
}

func (r *gormUserRepository) Update(ctx context.Context, u *entity.User) error {
	m := fromEntity(u)
	res := r.db.WithContext(ctx).Model(&userModel{}).Where("id = ?", u.ID).
		Updates(map[string]any{
			"email":             m.Email,
			"password_hash":     m.PasswordHash,
			"name":              m.Name,
			"role":              m.Role,
			"palette_id":        m.PaletteID,
			"avatar_url":        m.AvatarURL,
			"email_verified_at": m.EmailVerifiedAt,
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return apperrors.Conflict("An account with this email already exists")
		}
		return apperrors.Database("users.update", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("User not found")
	}
	return nil
}

func (r *gormUserRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&userModel{}, "id = ?", id)
	if res.Error != nil {
		return apperrors.Database("users.soft_delete", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("User not found")
	}
	return nil
}

// AnonymizeAndDelete overwrites identifying fields, then soft-deletes the row.
// The row survives so messages keep their foreign keys, but nothing
// personal remains and the account can never be signed into again.
func (r *gormUserRepository) AnonymizeAndDelete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scrubbed := "deleted-" + id.String() + "@deleted.invalid"
		err := tx.Model(&userModel{}).Where("id = ?", id).Updates(map[string]any{
			"email":        scrubbed,
			"firebase_uid": nil, // frees the identity to start over
			"name":         "Someone",
			// The handle is derived from their address, so it is PII too — and
			// releasing it lets them reclaim it if they ever come back.
			"handle":                nil,
			"password_hash":         "", // unusable: bcrypt never matches ""
			"avatar_url":            nil,
			"palette_id":            constants.Palettes[0],
			"email_verified_at":     nil,
			"deletion_requested_at": time.Now().UTC(),
		}).Error
		if err != nil {
			return apperrors.Database("users.anonymize", err)
		}
		// Membership ends everywhere so peers stop seeing the account.
		if err := tx.Exec(`UPDATE space_members SET left_at = now()
			WHERE user_id = ? AND left_at IS NULL`, id).Error; err != nil {
			return apperrors.Database("users.leave_spaces", err)
		}
		if err := tx.Delete(&userModel{}, "id = ?", id).Error; err != nil {
			return apperrors.Database("users.soft_delete", err)
		}
		return nil
	})
}

func (r *gormUserRepository) List(
	ctx context.Context, p request.Pagination, f ListFilter,
) ([]entity.User, int64, error) {
	q := r.db.WithContext(ctx).Model(&userModel{})

	if f.Role != "" {
		q = q.Where("role = ?", f.Role)
	}
	if f.Verified != nil {
		if *f.Verified {
			q = q.Where("email_verified_at IS NOT NULL")
		} else {
			q = q.Where("email_verified_at IS NULL")
		}
	}
	if p.Search != "" {
		like := "%" + strings.ToLower(p.Search) + "%"
		q = q.Where("lower(name) LIKE ? OR lower(email) LIKE ?", like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperrors.Database("users.count", err)
	}

	column, ok := sortWhitelist[p.Sort]
	if !ok {
		column = "created_at"
	}
	var models []userModel
	err := q.Order(column + " " + p.Order).
		Limit(p.Limit).Offset(p.Offset()).
		Find(&models).Error
	if err != nil {
		return nil, 0, apperrors.Database("users.list", err)
	}

	users := make([]entity.User, len(models))
	for i := range models {
		users[i] = *models[i].toEntity()
	}
	return users, total, nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value") ||
		strings.Contains(err.Error(), "SQLSTATE 23505")
}

// Search backs the people directory. Name matches are prefix-ish (substring,
// case-insensitive) so "am" finds "Aman"; email must match exactly, so the
// directory can never be used to enumerate addresses.
func (r *gormUserRepository) Search(
	ctx context.Context, query string, exclude uuid.UUID, limit int,
) ([]entity.User, error) {
	q := strings.TrimSpace(query)
	if len(q) < 2 {
		return nil, nil
	}
	if limit <= 0 || limit > 25 {
		limit = 25
	}

	var models []userModel
	err := r.db.WithContext(ctx).
		Where("id <> ?", exclude).
		Where("name ILIKE ? OR lower(email) = lower(?)", "%"+q+"%", q).
		Order("name asc").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("users.search", err)
	}

	users := make([]entity.User, len(models))
	for i := range models {
		users[i] = *models[i].toEntity()
	}
	return users, nil
}

func (r *gormUserRepository) HandleTaken(ctx context.Context, handle string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&userModel{}).
		Where("handle = ?", handle).Count(&n).Error
	if err != nil {
		return false, apperrors.Database("users.handle_taken", err)
	}
	return n > 0, nil
}
