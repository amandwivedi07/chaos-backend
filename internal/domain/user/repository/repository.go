// Package repository defines the user persistence port (interface) and its
// GORM adapter. Services depend ONLY on the interface.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/common/request"
	"github.com/chaosapp/backend/internal/domain/user/entity"
)

// ListFilter narrows List results; zero values mean "no filter".
type ListFilter struct {
	Role     string
	Verified *bool
}

// UserRepository is the persistence port for users.
type UserRepository interface {
	Create(ctx context.Context, u *entity.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error)
	GetByEmail(ctx context.Context, email string) (*entity.User, error)
	GetByFirebaseUID(ctx context.Context, uid string) (*entity.User, error)
	Update(ctx context.Context, u *entity.User) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	// AnonymizeAndDelete scrubs PII then soft-deletes (GDPR / App Store).
	AnonymizeAndDelete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, p request.Pagination, f ListFilter) ([]entity.User, int64, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	HandleTaken(ctx context.Context, handle string) (bool, error)
	// Search finds people by name or exact email for the directory. It never
	// returns the caller, deleted accounts, or more than limit rows.
	Search(ctx context.Context, query string, exclude uuid.UUID, limit int) ([]entity.User, error)
}
