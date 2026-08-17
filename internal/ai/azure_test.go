package ai

import "testing"

// unfence is the only thing standing between a chatty model and a screen full
// of raw JSON, so it has to survive every wrapper one might add.
func TestUnfence(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare object", `{"text":"hi"}`, `{"text":"hi"}`},
		{"json fence", "```json\n{\"text\":\"hi\"}\n```", `{"text":"hi"}`},
		{"bare fence", "```\n{\"text\":\"hi\"}\n```", `{"text":"hi"}`},
		{"prose either side", `Sure! {"text":"hi"} Hope that helps.`, `{"text":"hi"}`},
		{"nested braces survive", `{"a":{"b":1}}`, `{"a":{"b":1}}`},
		{"leading whitespace", "\n\n  {\"text\":\"hi\"}", `{"text":"hi"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unfence(tc.in); got != tc.want {
				t.Errorf("unfence(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// sanitiseReply is what stops one over-eager turn from breaking the layout.
func TestSanitiseReply(t *testing.T) {
	t.Run("clamps a chatty turn to three cards", func(t *testing.T) {
		r := sanitiseReply(&Reply{
			Text:  "  four options  ",
			Cards: []Card{{Title: "a"}, {Title: "b"}, {Title: "c"}, {Title: "d"}},
		})
		if len(r.Cards) != 3 {
			t.Errorf("cards = %d, want 3", len(r.Cards))
		}
		if r.Text != "four options" {
			t.Errorf("text = %q, want it trimmed", r.Text)
		}
	})

	t.Run("a single card is a recommendation, not a comparison", func(t *testing.T) {
		// The text already made the recommendation; one card in a swipeable row
		// reads as a broken carousel.
		r := sanitiseReply(&Reply{Text: "go here", Cards: []Card{{Title: "a"}}})
		if r.Cards != nil {
			t.Errorf("cards = %v, want nil", r.Cards)
		}
	})

	t.Run("stars are clamped into range", func(t *testing.T) {
		r := sanitiseReply(&Reply{
			Text: "x",
			Cards: []Card{
				{Title: "a", Ratings: []Rating{{Label: "Beach", Stars: 9}}},
				{Title: "b", Ratings: []Rating{{Label: "Beach", Stars: 0}}},
			},
		})
		if got := r.Cards[0].Ratings[0].Stars; got != 5 {
			t.Errorf("stars = %d, want clamped to 5", got)
		}
		if got := r.Cards[1].Ratings[0].Stars; got != 1 {
			t.Errorf("stars = %d, want clamped to 1", got)
		}
	})

	t.Run("a one-option vote is not a vote", func(t *testing.T) {
		r := sanitiseReply(&Reply{
			Text:     "x",
			Decision: &Decision{Question: "Which?", Options: []string{"only one"}},
		})
		if r.Decision != nil {
			t.Error("expected a single-option decision to be dropped")
		}
	})

	t.Run("a questionless vote is dropped", func(t *testing.T) {
		r := sanitiseReply(&Reply{
			Text:     "x",
			Decision: &Decision{Options: []string{"a", "b"}},
		})
		if r.Decision != nil {
			t.Error("expected a decision with no question to be dropped")
		}
	})

	t.Run("a rambling action label is dropped rather than truncated", func(t *testing.T) {
		// It is a button. A sentence on a button is worse than no button.
		r := sanitiseReply(&Reply{
			Text:   "x",
			Action: "Click here to see all of the available options for the trip",
		})
		if r.Action != "" {
			t.Errorf("action = %q, want empty", r.Action)
		}
	})

	t.Run("memory lists are trimmed and de-blanked", func(t *testing.T) {
		r := sanitiseReply(&Reply{
			Text: "x",
			Memory: &Memory{
				Decided: []string{" 4 nights ", "", "under 2L"},
			},
		})
		want := []string{"4 nights", "under 2L"}
		if len(r.Memory.Decided) != len(want) {
			t.Fatalf("decided = %v, want %v", r.Memory.Decided, want)
		}
		for i := range want {
			if r.Memory.Decided[i] != want[i] {
				t.Errorf("decided[%d] = %q, want %q", i, r.Memory.Decided[i], want[i])
			}
		}
	})
}

func TestSanitiseFacts(t *testing.T) {
	t.Run("an unknown category falls back rather than failing the insert", func(t *testing.T) {
		// The column has a CHECK constraint; an invented category would 500 a
		// perfectly good extraction.
		out := sanitiseFacts([]Fact{{Label: "Vibe", Value: "loud", Category: "mood"}})
		if len(out) != 1 || out[0].Category != "basics" {
			t.Errorf("got %+v, want category coerced to basics", out)
		}
	})

	t.Run("blank labels and values are dropped", func(t *testing.T) {
		out := sanitiseFacts([]Fact{
			{Label: "", Value: "x", Category: "food"},
			{Label: "Food", Value: "  ", Category: "food"},
			{Label: " Food ", Value: " no seafood ", Category: "food"},
		})
		if len(out) != 1 {
			t.Fatalf("got %d facts, want 1", len(out))
		}
		if out[0].Label != "Food" || out[0].Value != "no seafood" {
			t.Errorf("got %+v, want trimmed", out[0])
		}
	})

	t.Run("an out-of-range confidence is replaced", func(t *testing.T) {
		out := sanitiseFacts([]Fact{
			{Label: "A", Value: "x", Category: "food", Confidence: 0},
			{Label: "B", Value: "x", Category: "food", Confidence: 7},
		})
		for _, f := range out {
			if f.Confidence <= 0 || f.Confidence > 1 {
				t.Errorf("%s confidence = %v, want within (0,1]", f.Label, f.Confidence)
			}
		}
	})

	t.Run("caps at eight, however many arrive", func(t *testing.T) {
		in := make([]Fact, 20)
		for i := range in {
			in[i] = Fact{Label: string(rune('a' + i)), Value: "x", Category: "food"}
		}
		if got := len(sanitiseFacts(in)); got != 8 {
			t.Errorf("got %d facts, want 8", got)
		}
	})
}

// Every prompt must contain the word "json": Azure rejects the json_object
// response format otherwise, and the failure is a 400 at runtime rather than a
// compile error.
func TestPromptsMentionJSON(t *testing.T) {
	prompts := map[string]string{
		"reply":   replySystemPrompt,
		"name":    nameSystemPrompt,
		"extract": extractSystemPrompt,
	}
	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			if !containsFold(prompt, "json") {
				t.Errorf("%s prompt does not mention JSON", name)
			}
		})
	}
}

func containsFold(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a|0x20 != b|0x20 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
