// Package repository is the persistence port for groups.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/domain/group/entity"
)

type Repository interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]entity.Group, error)
	GetForUser(ctx context.Context, groupID, userID uuid.UUID) (*entity.Group, error)
	Create(ctx context.Context, g *entity.Group, memberIDs []uuid.UUID) error
	Update(ctx context.Context, groupID uuid.UUID, name, emoji string) error
	Delete(ctx context.Context, groupID uuid.UUID) error

	IsMember(ctx context.Context, groupID, userID uuid.UUID) (bool, error)
	AddMember(ctx context.Context, groupID, userID uuid.UUID) error
	RemoveMember(ctx context.Context, groupID, userID uuid.UUID) error
	MemberIDs(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error)

	AddMemory(ctx context.Context, m *entity.Memory) error
	DeleteMemory(ctx context.Context, groupID, memoryID uuid.UUID) error
	// Memory is the shared context every Chaos turn in this group is prompted
	// with. Read on the hot path, so it stays one query.
	Memory(ctx context.Context, groupID uuid.UUID) ([]entity.Memory, error)
	// Name is a single-column read for the same hot path. No membership check:
	// the caller has already proved they can see the conversation, which is
	// what grants them the group's context.
	Name(ctx context.Context, groupID uuid.UUID) (string, error)

	// Collaborators ranks the people the caller shares conversations with,
	// most-shared first. It backs the People screen.
	Collaborators(ctx context.Context, userID uuid.UUID) ([]entity.Member, []int, error)
}
