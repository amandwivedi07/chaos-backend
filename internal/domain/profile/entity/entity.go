// Package entity holds the profile domain: who a person is beyond their login,
// and the durable facts Chaos has learnt about them.
package entity

import (
	"time"

	"github.com/google/uuid"
)

// Fact categories. The client groups and labels by these, so the set is closed
// — an unknown category from a model is coerced to "basics" before it lands.
const (
	CategoryBasics      = "basics"
	CategoryTaste       = "taste"
	CategoryConstraints = "constraints"
	CategoryMoney       = "money"
	CategoryFood        = "food"
	CategoryStyle       = "style"
)

// Sources a fact can come from. "you" outranks every model: a person
// correcting a fact about themselves is not a guess.
const (
	SourceChaos   = "chaos"
	SourceChatGPT = "chatgpt"
	SourceClaude  = "claude"
	SourceGemini  = "gemini"
	SourceYou     = "you"
)

// ImportSources are the assistants a person can bring context over from.
var ImportSources = []string{SourceChatGPT, SourceClaude, SourceGemini}

func ValidImportSource(s string) bool {
	for _, v := range ImportSources {
		if v == s {
			return true
		}
	}
	return false
}

// Profile is the part of a person that changes what a group should plan.
// Note is the single line the conversation module reads when prompting.
type Profile struct {
	UserID     uuid.UUID
	Bio        string
	City       string
	Note       string
	LastSeenAt *time.Time
	// LearnedCount is how many of this person's own messages had been written
	// the last time Chaos mined them for facts. Comparing it against the
	// current count is what stops the profile screen re-reading the same
	// history on every open.
	LearnedCount int
	Connections  []Connection
}

// Connection records that a person has brought context over from another
// assistant, and when.
type Connection struct {
	Source   string
	SyncedAt time.Time
}

// Fact is one durable thing about a person.
type Fact struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Label  string
	Value  string
	// Category is one of the constants above.
	Category string
	// Confidence is how sure the extractor was, 0-1. A confirmed fact is 1.
	Confidence float64
	Source     string
	// SourceLabel is what to print on the chip: the conversation it came from,
	// or the assistant it was imported from.
	SourceLabel string
	// Confirmed is the person having said "Looks right", or having typed the
	// fact themselves. Confirmed facts are never overwritten by a model.
	Confirmed bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
