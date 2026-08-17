// Package repository is the persistence port for conversations and messages.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

type Repository interface {
	// Conversations.
	ListForUser(ctx context.Context, userID uuid.UUID) ([]entity.Conversation, error)
	GetForUser(ctx context.Context, convID, userID uuid.UUID) (*entity.Conversation, error)
	// Get skips the membership check. Used only by the invite preview, which
	// has to render for someone who is not in the room yet.
	Get(ctx context.Context, convID uuid.UUID) (*entity.Conversation, error)
	Create(ctx context.Context, c *entity.Conversation, memberIDs []uuid.UUID) error
	Rename(ctx context.Context, convID uuid.UUID, title, emoji string, titled bool) error
	// SetGroup moves a conversation into a group, or out of one when nil.
	SetGroup(ctx context.Context, convID uuid.UUID, groupID *uuid.UUID) error
	// ListForGroup is the group screen's own list, plus the transcripts
	// "Ask this group" searches.
	ListForGroup(ctx context.Context, groupID uuid.UUID, limit int) ([]entity.Conversation, error)
	SetMemory(ctx context.Context, convID uuid.UUID, decided, open []string) error
	Delete(ctx context.Context, convID uuid.UUID) error
	Leave(ctx context.Context, convID, userID uuid.UUID) error
	IsMember(ctx context.Context, convID, userID uuid.UUID) (bool, error)
	MemberIDs(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error)
	AddMember(ctx context.Context, convID, userID uuid.UUID) error
	TouchActivity(ctx context.Context, convID uuid.UUID) error
	HeartbeatUser(ctx context.Context, userID uuid.UUID) error
	MarkSeen(ctx context.Context, convID, userID uuid.UUID) error
	UnreadCounts(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]int, error)

	// Messages. InsertMessage writes the message, its cards and its decision in
	// one transaction — a Chaos turn is never half-visible.
	ListMessages(ctx context.Context, convID uuid.UUID, limit int) ([]entity.Message, error)
	// OwnMessages is everything one person has said, across every conversation
	// they are in, newest last. It exists for the profile module, which mines
	// it for durable facts — see service.History there.
	OwnMessages(ctx context.Context, userID uuid.UUID, limit int) ([]entity.OwnMessage, error)
	// CountOwnMessages is the cheap half of that: how many they have said at
	// all, so the profile can skip the expensive read when nothing is new.
	CountOwnMessages(ctx context.Context, userID uuid.UUID) (int, error)
	// KnownPeople is everyone the caller already shares a conversation with,
	// most recently active first. It is what the add-people sheet offers
	// before anyone types — a directory of strangers would be worse than
	// nothing there.
	KnownPeople(ctx context.Context, userID uuid.UUID, limit int) ([]entity.Member, error)
	InsertMessage(ctx context.Context, m *entity.Message) error
	CountSinceChaos(ctx context.Context, convID uuid.UUID) (total int, sinceChaos int, err error)

	// Cards + decisions.
	GetCard(ctx context.Context, cardID uuid.UUID) (*entity.Card, uuid.UUID, error)
	GetDecision(ctx context.Context, decisionID uuid.UUID) (*entity.Decision, error)
	// Vote replaces the caller's vote inside the decision (one vote each), then
	// stores the recomputed winner. Returns the decision as it now stands.
	Vote(ctx context.Context, decisionID, optionID, userID uuid.UUID) (*entity.Decision, error)
}
