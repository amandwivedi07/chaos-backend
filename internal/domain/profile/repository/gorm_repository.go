package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/profile/entity"
)

type profileModel struct {
	UserID       uuid.UUID `gorm:"type:uuid;primaryKey"`
	Bio          string
	City         string
	Note         string
	LastSeenAt   *time.Time
	LearnedCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (profileModel) TableName() string { return "profiles" }

type factModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID      uuid.UUID
	Label       string
	Value       string
	Category    string
	Confidence  float64
	Source      string
	SourceLabel string
	Confirmed   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (factModel) TableName() string { return "facts" }

type connectionModel struct {
	UserID   uuid.UUID `gorm:"primaryKey"`
	Source   string    `gorm:"primaryKey"`
	SyncedAt time.Time
}

func (connectionModel) TableName() string { return "ai_connections" }

type gormRepository struct{ db *gorm.DB }

var _ Repository = (*gormRepository)(nil)

func NewGorm(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Get(ctx context.Context, userID uuid.UUID) (*entity.Profile, error) {
	var m profileModel
	err := r.db.WithContext(ctx).First(&m, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// A profile is created lazily; an account with none is simply empty,
		// not broken.
		return &entity.Profile{UserID: userID}, nil
	}
	if err != nil {
		return nil, apperrors.Database("get profile", err)
	}
	return &entity.Profile{
		UserID:       m.UserID,
		Bio:          m.Bio,
		City:         m.City,
		Note:         m.Note,
		LastSeenAt:   m.LastSeenAt,
		LearnedCount: m.LearnedCount,
	}, nil
}

func (r *gormRepository) Upsert(ctx context.Context, p *entity.Profile) error {
	m := profileModel{
		UserID: p.UserID, Bio: p.Bio, City: p.City,
		Note: p.Note, LearnedCount: p.LearnedCount,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"bio", "city", "note", "learned_count", "updated_at",
		}),
	}).Create(&m).Error
	if err != nil {
		return apperrors.Database("upsert profile", err)
	}
	return nil
}

func (r *gormRepository) SetNote(ctx context.Context, userID uuid.UUID, note string) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO profiles (user_id, note) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET note = EXCLUDED.note, updated_at = now()`,
		userID, note).Error
	if err != nil {
		return apperrors.Database("set note", err)
	}
	return nil
}

func (r *gormRepository) SetLearnedCount(ctx context.Context, userID uuid.UUID, n int) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO profiles (user_id, learned_count) VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE SET learned_count = EXCLUDED.learned_count, updated_at = now()`,
		userID, n).Error
	if err != nil {
		return apperrors.Database("set learned count", err)
	}
	return nil
}

func toFact(m factModel) entity.Fact {
	return entity.Fact{
		ID: m.ID, UserID: m.UserID, Label: m.Label, Value: m.Value,
		Category: m.Category, Confidence: m.Confidence, Source: m.Source,
		SourceLabel: m.SourceLabel, Confirmed: m.Confirmed,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ListFacts returns newest first — the profile screen reads top-down and the
// thing just learnt is the thing worth checking.
func (r *gormRepository) ListFacts(ctx context.Context, userID uuid.UUID) ([]entity.Fact, error) {
	var models []factModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC").Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list facts", err)
	}
	out := make([]entity.Fact, len(models))
	for i, m := range models {
		out[i] = toFact(m)
	}
	return out, nil
}

func (r *gormRepository) GetFact(ctx context.Context, factID uuid.UUID) (*entity.Fact, error) {
	var m factModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", factID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("Input not found")
		}
		return nil, apperrors.Database("get fact", err)
	}
	fact := toFact(m)
	return &fact, nil
}

// UpsertFacts merges a batch by label (case-insensitively).
//
// A confirmed fact is never overwritten by a model: the person has already
// said this is right, and quietly replacing it would make the screen's "Looks
// right" button meaningless.
func (r *gormRepository) UpsertFacts(ctx context.Context, userID uuid.UUID, facts []entity.Fact) (int, error) {
	if len(facts) == 0 {
		return 0, nil
	}
	added := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []factModel
		if err := tx.Where("user_id = ?", userID).Find(&existing).Error; err != nil {
			return err
		}
		byLabel := make(map[string]factModel, len(existing))
		for _, m := range existing {
			byLabel[strings.ToLower(m.Label)] = m
		}

		now := time.Now()
		for _, f := range facts {
			key := strings.ToLower(f.Label)
			if old, ok := byLabel[key]; ok {
				if old.Confirmed && !f.Confirmed {
					continue
				}
				err := tx.Model(&factModel{}).Where("id = ?", old.ID).
					Updates(map[string]any{
						"value":        f.Value,
						"category":     f.Category,
						"confidence":   f.Confidence,
						"source":       f.Source,
						"source_label": f.SourceLabel,
						"confirmed":    f.Confirmed || old.Confirmed,
						"updated_at":   now,
					}).Error
				if err != nil {
					return err
				}
				continue
			}
			m := factModel{
				ID: uuid.New(), UserID: userID, Label: f.Label, Value: f.Value,
				Category: f.Category, Confidence: f.Confidence, Source: f.Source,
				SourceLabel: f.SourceLabel, Confirmed: f.Confirmed,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
			byLabel[key] = m
			added++
		}
		return nil
	})
	if err != nil {
		return 0, apperrors.Database("upsert facts", err)
	}
	return added, nil
}

func (r *gormRepository) UpdateFact(ctx context.Context, factID uuid.UUID, value *string, confirmed *bool) error {
	updates := map[string]any{"updated_at": time.Now()}
	if value != nil {
		// An edited fact is the person's own words now, whatever wrote it
		// first — so the chip and the confidence bar both change hands.
		updates["value"] = *value
		updates["source"] = entity.SourceYou
		updates["source_label"] = "You"
		updates["confirmed"] = true
		updates["confidence"] = 1.0
	}
	if confirmed != nil {
		updates["confirmed"] = *confirmed
		if *confirmed {
			updates["confidence"] = 1.0
		}
	}
	err := r.db.WithContext(ctx).Model(&factModel{}).
		Where("id = ?", factID).Updates(updates).Error
	if err != nil {
		return apperrors.Database("update fact", err)
	}
	return nil
}

func (r *gormRepository) DeleteFact(ctx context.Context, factID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&factModel{}, "id = ?", factID).Error; err != nil {
		return apperrors.Database("delete fact", err)
	}
	return nil
}

func (r *gormRepository) CountBySource(ctx context.Context, userID uuid.UUID) (map[string]int, error) {
	var rows []struct {
		Source string
		N      int
	}
	err := r.db.WithContext(ctx).Model(&factModel{}).
		Select("source, count(*) AS n").
		Where("user_id = ?", userID).Group("source").Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("count facts", err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.Source] = row.N
	}
	return out, nil
}

func (r *gormRepository) ListConnections(ctx context.Context, userID uuid.UUID) ([]entity.Connection, error) {
	var models []connectionModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).Find(&models).Error; err != nil {
		return nil, apperrors.Database("list connections", err)
	}
	out := make([]entity.Connection, len(models))
	for i, m := range models {
		out[i] = entity.Connection{Source: m.Source, SyncedAt: m.SyncedAt}
	}
	return out, nil
}

func (r *gormRepository) MarkConnected(ctx context.Context, userID uuid.UUID, source string) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "source"}},
		DoUpdates: clause.AssignmentColumns([]string{"synced_at"}),
	}).Create(&connectionModel{
		UserID: userID, Source: source, SyncedAt: time.Now(),
	}).Error
	if err != nil {
		return apperrors.Database("mark connected", err)
	}
	return nil
}
