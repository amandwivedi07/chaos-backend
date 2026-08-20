package entity

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// ShouldReply is the whole personality of the product: Chaos is the friend who
// answers when it matters, not the one who replies to everything.
func TestShouldReply(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		totalMessages int
		sinceChaos    int
		direct        bool
		want          bool
	}{
		{"named directly", "@Chaos what do you think?", 12, 0, false, true},
		{"named mid-sentence, any case", "ok @chaos help", 12, 1, false, true},
		// Being addressed by name is the way people actually do it.
		{"name and a comma, no @", "Chaos, explain this to all of us.", 12, 0, false, true},
		{"name with no punctuation at all", "Chaos argue both sides", 12, 0, false, true},
		{"greeting then the name", "hey chaos can you settle this", 12, 0, false, true},
		{"name at the very end", "what do you think, chaos?", 12, 0, false, true},
		{"name opening a later sentence", "I disagree. Chaos, who is right?", 12, 0, false, true},
		{"first line in an empty room", "Where should we go?", 0, 0, false, true},
		{"question with nothing said since its last turn", "Where to?", 4, 0, false, false},
		{"question once the group has said anything", "Where to?", 1, 1, false, true},
		{"question after some back and forth", "Can we keep it under 2L?", 4, 2, false, true},
		{"statement one line after its last turn", "I want a beach.", 4, 1, false, false},
		{"trailing whitespace still reads as a question", "Where to?  ", 4, 2, false, true},
		// The mark does not have to be the last character. This exact message
		// was sent and ignored.
		{"question mark mid-message", "Give me a option to choose...? And also why and what for", 6, 2, false, true},
		{"question buried in the middle", "wait what? we said Tuesday", 6, 1, false, true},
		{"stalled thread gets one turn unasked", "and nightlife", 9, 3, false, true},
		{"just under the stall threshold", "and nightlife", 9, 2, false, false},
		// A private thread is you talking TO Chaos. Restraint there reads as
		// being ignored, so every one of the restrained cases flips.
		{"private: a bare statement still gets an answer", "I want a beach.", 4, 3, true, true},
		{"private: even the second message", "hello", 1, 1, true, true},
		{"private: well under every threshold", "ok", 9, 0, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldReply(tc.text, tc.totalMessages, tc.sinceChaos, tc.direct)
			if got != tc.want {
				t.Errorf("ShouldReply(%q, %d, %d, direct=%v) = %v, want %v",
					tc.text, tc.totalMessages, tc.sinceChaos, tc.direct, got, tc.want)
			}
		})
	}
}

func option(label string, votes ...uuid.UUID) Option {
	return Option{ID: uuid.New(), Label: label, Votes: votes}
}

// Resolve closes a question only when the group actually agreed. One person
// clicking their own suggestion is not a decision, and a tie is exactly the
// argument the vote was meant to end.
func TestDecisionResolve(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name    string
		options []Option
		want    string // label of the winner, "" for none
	}{
		{"nobody voted", []Option{option("Bangkok"), option("Bali")}, ""},
		{"one vote is not a decision", []Option{option("Bangkok", a), option("Bali")}, ""},
		{"clear lead wins", []Option{option("Bangkok", a, b), option("Bali", c)}, "Bangkok"},
		{"a tie stays open", []Option{option("Bangkok", a, b), option("Bali", c, a)}, ""},
		{"three-way split with a leader", []Option{
			option("Bangkok", a, b, c), option("Bali", a), option("Phuket"),
		}, "Bangkok"},
		{"no options at all", nil, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Decision{Options: tc.options}
			got := d.Resolve()
			if tc.want == "" {
				if got != nil {
					t.Fatalf("expected no winner, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %q to win, got no winner", tc.want)
			}
			for _, o := range tc.options {
				if o.ID == *got {
					if o.Label != tc.want {
						t.Fatalf("winner = %q, want %q", o.Label, tc.want)
					}
					return
				}
			}
			t.Fatalf("winner id %v matches no option", *got)
		})
	}
}

func TestPresence(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }

	tests := []struct {
		name     string
		lastSeen *time.Time
		want     string
	}{
		{"never seen", nil, "away"},
		{"seconds ago", at(30 * time.Second), "here"},
		{"ten minutes ago", at(10 * time.Minute), "recent"},
		{"yesterday", at(30 * time.Hour), "away"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Presence(tc.lastSeen, now); got != tc.want {
				t.Errorf("Presence = %q, want %q", got, tc.want)
			}
		})
	}
}

// Mentioned is what makes tagging work at all. It used to require a literal
// "@chaos", which nobody types — so being asked something by name did nothing.
func TestMentioned(t *testing.T) {
	addressed := []string{
		"@Chaos what do you think?",
		"ok @chaos help",
		"Chaos, explain this to all of us.",
		"chaos: settle this",
		"Chaos argue both sides",
		"hey chaos",
		"Hey Chaos, fact-check that",
		"pls chaos catch me up",
		"so what do you reckon, chaos?",
		"I disagree. Chaos, who is right?",
	}
	for _, text := range addressed {
		if !Mentioned(text) {
			t.Errorf("Mentioned(%q) = false, want true", text)
		}
	}

	// Talking about the app, or using the ordinary word, is not addressing it.
	ignored := []string{
		"this weekend was absolute chaos honestly",
		"the chaos app is pretty good",
		"total chaos in the office today",
		"that meeting was chaos!",
		"it was pure chaos.",
		"last night was chaos",
		"I want a beach.",
		"",
	}
	for _, text := range ignored {
		if Mentioned(text) {
			t.Errorf("Mentioned(%q) = true, want false", text)
		}
	}
}
