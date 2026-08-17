// Package entity holds the conversation domain: the thread, what was said in
// it, the options Chaos put forward, and the votes the group ran.
package entity

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Message kinds. A message is either something a person said, a turn from
// Chaos, or the room narrating itself ("Meera joined the conversation.").
const (
	KindUser   = "user"
	KindChaos  = "chaos"
	KindSystem = "system"
)

// Member roles.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Conversation struct {
	ID    uuid.UUID
	Emoji string
	Title string
	// Titled is false while the title is still a placeholder, which is what
	// lets the header offer "Name this conversation" instead of a fake name.
	Titled bool
	// GroupID is the standing cast this conversation belongs to, if any. It is
	// what makes the group's memory apply here.
	GroupID *uuid.UUID
	// Direct means you and Chaos, nobody else. It changes when Chaos speaks:
	// there is no group to wait for, so it answers every message.
	Direct    bool
	CreatedBy uuid.UUID
	// Decided and Open are Chaos keeping score: what is settled, what is still
	// unanswered. Rewritten wholesale each turn — see ai.Memory.
	Decided   []string
	Open      []string
	CreatedAt time.Time
	UpdatedAt time.Time

	Members []Member

	// List-view only.
	LastMessage  *Message
	Unread       int
	MessageCount int
	// GroupName saves the home list a second lookup for the group badge.
	GroupName string
}

type Member struct {
	UserID    uuid.UUID
	Name      string
	Handle    string
	AvatarURL string
	PaletteID string
	// Note is the one line about this person that changes what Chaos should
	// suggest — "Nightlife is non-negotiable". Derived from their profile.
	Note     string
	Role     string
	LastSeen *time.Time
}

// Presence derives the soft presence label from a heartbeat.
func Presence(lastSeen *time.Time, now time.Time) string {
	if lastSeen == nil {
		return "away"
	}
	switch d := now.Sub(*lastSeen); {
	case d < 2*time.Minute:
		return "here"
	case d < time.Hour:
		return "recent"
	default:
		return "away"
	}
}

type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	// AuthorID is nil for Chaos turns and for system lines — neither has a
	// user row, and giving them one would put them in the member list.
	AuthorID *uuid.UUID
	Kind     string
	Text     string
	// Action is an optional button label Chaos attached to its turn.
	Action string
	SentAt time.Time

	Cards    []Card
	Decision *Decision
}

func (m Message) FromChaos() bool { return m.Kind == KindChaos }

// OwnMessage is one line a person wrote, carrying the conversation it was
// written in. The title is what makes a fact mined from it attributable —
// "Bangkok in September?" rather than a bare id.
type OwnMessage struct {
	ConversationTitle string
	Text              string
	SentAt            time.Time
}

// Card is one option in the swipeable row under a Chaos turn.
type Card struct {
	ID        uuid.UUID
	MessageID uuid.UUID
	Emoji     string
	Title     string
	Tagline   string
	Why       string
	Position  int
	Ratings   []Rating
}

type Rating struct {
	Label string
	Stars int
}

// Decision is a vote attached to a Chaos turn.
type Decision struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	Question       string
	// ResolvedOptionID is set once one option has a clear lead. It is derived,
	// not chosen: see Resolve.
	ResolvedOptionID *uuid.UUID
	Options          []Option
}

type Option struct {
	ID       uuid.UUID
	Label    string
	Position int
	Votes    []uuid.UUID
}

// Resolve returns the winning option, if there is one.
//
// A winner needs at least two votes AND a strict lead. One person clicking
// their own suggestion is not the group deciding, and a tie is precisely the
// argument the vote was meant to end — neither should close the question.
func (d Decision) Resolve() *uuid.UUID {
	var first, second *Option
	for i := range d.Options {
		o := &d.Options[i]
		switch {
		case first == nil || len(o.Votes) > len(first.Votes):
			second, first = first, o
		case second == nil || len(o.Votes) > len(second.Votes):
			second = o
		}
	}
	if first == nil || len(first.Votes) < 2 {
		return nil
	}
	if second != nil && len(second.Votes) >= len(first.Votes) {
		return nil
	}
	id := first.ID
	return &id
}

// ---- when Chaos speaks ----

var (
	mentionRE  = regexp.MustCompile(`(?i)@chaos`)
	questionRE = regexp.MustCompile(`\?\s*$`)
)

// Mentioned reports whether a line addresses Chaos directly.
func Mentioned(text string) bool { return mentionRE.MatchString(text) }

// ShouldReply decides whether a newly sent line earns a turn from Chaos.
//
// The rule is about not being the friend who answers every message. Being
// named always works. Otherwise Chaos waits: a question only pulls it in once
// the group has gone back and forth a couple of times, and a thread that has
// run five messages without it gets one turn whether or not anyone asked.
//
// sinceChaos is how many messages have been said since its last turn (or the
// whole history if it has never spoken).
//
// direct short-circuits all of it: a conversation with nobody else in it is
// you talking TO Chaos, and restraint there just reads as being ignored.
func ShouldReply(text string, totalMessages, sinceChaos int, direct bool) bool {
	switch {
	case direct:
		return true
	case Mentioned(text):
		return true
	case totalMessages == 0:
		return true
	case questionRE.MatchString(strings.TrimSpace(text)) && sinceChaos >= 2:
		return true
	default:
		return sinceChaos >= 5
	}
}
