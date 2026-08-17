// Package repository is the persistence port for profiles and facts.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/domain/profile/entity"
)

type Repository interface {
	Get(ctx context.Context, userID uuid.UUID) (*entity.Profile, error)
	Upsert(ctx context.Context, p *entity.Profile) error
	// SetNote stores the one line the conversation module reads when it builds
	// a prompt. Derived from the facts, so it is written, never edited.
	SetNote(ctx context.Context, userID uuid.UUID, note string) error
	SetLearnedCount(ctx context.Context, userID uuid.UUID, n int) error

	ListFacts(ctx context.Context, userID uuid.UUID) ([]entity.Fact, error)
	GetFact(ctx context.Context, factID uuid.UUID) (*entity.Fact, error)
	// UpsertFacts merges by label: a fact with a label already on file replaces
	// its value rather than sitting next to it, so a changed mind reads as a
	// correction and not as two contradictory rows.
	UpsertFacts(ctx context.Context, userID uuid.UUID, facts []entity.Fact) (added int, err error)
	UpdateFact(ctx context.Context, factID uuid.UUID, value *string, confirmed *bool) error
	DeleteFact(ctx context.Context, factID uuid.UUID) error
	CountBySource(ctx context.Context, userID uuid.UUID) (map[string]int, error)

	ListConnections(ctx context.Context, userID uuid.UUID) ([]entity.Connection, error)
	MarkConnected(ctx context.Context, userID uuid.UUID, source string) error
}
