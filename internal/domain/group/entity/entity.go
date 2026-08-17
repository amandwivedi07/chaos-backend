// Package entity holds the group domain: a standing cast of people, and the
// context Chaos carries into everything they figure out together.
package entity

import (
	"time"

	"github.com/google/uuid"
)

// Group is the people you keep planning with.
//
// It is deliberately not a conversation. A conversation ends when the question
// is answered; a group is what is still true afterwards — who is in it, and
// what everyone already knows.
type Group struct {
	ID        uuid.UUID
	Name      string
	Emoji     string
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time

	Members []Member
	Memory  []Memory

	// ConversationCount is list-view only — "4 people · 3 conversations".
	ConversationCount int
}

type Member struct {
	UserID    uuid.UUID
	Name      string
	Handle    string
	AvatarURL string
	PaletteID string
	Note      string
}

// Memory is one thing the group knows: "Budget ceiling is 2L per person."
//
// Distinct from a personal fact, which is about one person. This is shared,
// and it is prepended to every Chaos turn in every conversation the group has
// — which is the whole reason a group exists rather than just a member list.
type Memory struct {
	ID        uuid.UUID
	GroupID   uuid.UUID
	Text      string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

// Answer is what "Ask this group" returns: a reply, plus the conversations it
// actually drew on so the person can go read them.
type Answer struct {
	Text string
	// References are conversation ids, already filtered to ones the caller can
	// open.
	References []uuid.UUID
}
