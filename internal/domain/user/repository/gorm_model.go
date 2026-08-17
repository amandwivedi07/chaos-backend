package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/chaosapp/backend/internal/domain/user/entity"
)

// userModel is the GORM persistence model — the ONLY place database column
// mapping exists. It never crosses the repository boundary.
type userModel struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email           *string   `gorm:"uniqueIndex"` // Apple may withhold it
	FirebaseUID     *string   `gorm:"uniqueIndex"`
	Provider        string    `gorm:"not null;default:password"`
	PasswordHash    *string
	Name            string  `gorm:"not null"`
	Handle          *string `gorm:"uniqueIndex"`
	Role            string  `gorm:"not null;default:user"`
	PaletteID       string  `gorm:"not null;default:ember"`
	AvatarURL       *string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"` // soft delete
}

func (userModel) TableName() string { return "users" }

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (m *userModel) toEntity() *entity.User {
	avatar := ""
	if m.AvatarURL != nil {
		avatar = *m.AvatarURL
	}
	return &entity.User{
		ID:              m.ID,
		Email:           str(m.Email),
		FirebaseUID:     str(m.FirebaseUID),
		Provider:        m.Provider,
		PasswordHash:    str(m.PasswordHash),
		Name:            m.Name,
		Handle:          str(m.Handle),
		Role:            m.Role,
		PaletteID:       m.PaletteID,
		AvatarURL:       avatar,
		EmailVerifiedAt: m.EmailVerifiedAt,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func fromEntity(u *entity.User) *userModel {
	var avatar *string
	if u.AvatarURL != "" {
		avatar = &u.AvatarURL
	}
	provider := u.Provider
	if provider == "" {
		provider = "password"
	}
	return &userModel{
		ID:              u.ID,
		Email:           ptr(u.Email),
		FirebaseUID:     ptr(u.FirebaseUID),
		Provider:        provider,
		PasswordHash:    ptr(u.PasswordHash),
		Name:            u.Name,
		Handle:          ptr(u.Handle),
		Role:            u.Role,
		PaletteID:       u.PaletteID,
		AvatarURL:       avatar,
		EmailVerifiedAt: u.EmailVerifiedAt,
	}
}
