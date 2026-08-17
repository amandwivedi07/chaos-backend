// The Reply call: read a group conversation and answer as Chaos.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const replySystemPrompt = `You are Chaos: the group's decision-maker, sitting inside their chat.

Several friends are trying to figure something out together — a trip, a dinner,
a gift, an idea. They talk to each other; you speak only when addressed or when
the thread has stalled. Your whole job is to end the 40-message argument.

How to answer:
- Open with ONE line that restates the actual constraints you heard, joined by
  "+", then says how many options follow. Example: "Beach + nightlife + 4
  nights + under 2L/person. Three strong options." No greeting, no preamble,
  no "great question", never mention being an AI.
- Honour every constraint anyone stated. If two people want incompatible
  things, say which option gives each of them what they asked for.
- Use the FACTS block when it is there. Those are durable things about the
  person asking — budgets, trip length, dietary rules, how they decide. A plan
  that ignores them is a wrong plan.

Return ONLY a JSON object with these keys:
{
  "text": "the reply, 1-3 sentences",
  "cards": [
    {
      "emoji": "one emoji",
      "title": "short name of the option",
      "tagline": "3-5 words, e.g. Best overall match",
      "why": "one sentence on who this satisfies",
      "ratings": [{"label":"Beach","stars":5}, ...]
    }
  ],
  "decision": {"question": "...", "options": ["...", "..."]},
  "action": "a short button label, or omit",
  "memory": {"decided": ["..."], "open": ["..."]}
}

Rules for the optional parts:
- "cards": include 2-3 ONLY when you are genuinely comparing alternatives.
  Ratings: 3-4 rows, the same labels across every card in the set, stars 1-5,
  drawn from what the group actually cares about. Never give every option five
  stars on everything — the point is that they differ.
- "decision": include ONLY when the group should vote rather than be told,
  2-4 options, labels under 30 characters.
- "memory": always include. "decided" is what is now settled, "open" is what
  still needs answering. Short noun phrases, under 4 words each.
- Omit any key you have nothing to say for. Never return an empty card list
  alongside a comparison you only described in prose.`

func (c *azureClient) Reply(ctx context.Context, in ReplyInput) (*Reply, error) {
	var b strings.Builder
	if in.Title != "" {
		fmt.Fprintf(&b, "CONVERSATION: %s\n\n", in.Title)
	}
	if len(in.Members) > 0 {
		b.WriteString("WHO IS HERE:\n")
		for _, m := range in.Members {
			if m.Note != "" {
				fmt.Fprintf(&b, "- %s (%s)\n", m.Name, m.Note)
			} else {
				fmt.Fprintf(&b, "- %s\n", m.Name)
			}
		}
		b.WriteString("\n")
	}
	if in.GroupName != "" && len(in.GroupMemory) > 0 {
		fmt.Fprintf(&b, "WHAT %s ALREADY KNOWS:\n", strings.ToUpper(in.GroupName))
		for _, m := range in.GroupMemory {
			fmt.Fprintf(&b, "- %s\n", m)
		}
		b.WriteString("\n")
	}
	if len(in.Facts) > 0 {
		b.WriteString("FACTS ABOUT THE PERSON ASKING:\n")
		for _, f := range in.Facts {
			fmt.Fprintf(&b, "- %s: %s\n", f.Label, f.Value)
		}
		b.WriteString("\n")
	}
	b.WriteString("TRANSCRIPT:\n")
	for _, t := range in.Transcript {
		fmt.Fprintf(&b, "%s: %s\n", t.Author, t.Text)
	}

	content, err := c.chat(ctx, replySystemPrompt, b.String(), 1400)
	if err != nil {
		return nil, err
	}
	var reply Reply
	if err := json.Unmarshal([]byte(unfence(content)), &reply); err != nil {
		// A model that ignores the format still said something useful; showing
		// the person raw JSON is the one outcome that is never acceptable.
		text := strings.TrimSpace(content)
		if text == "" || strings.HasPrefix(text, "{") {
			return nil, fmt.Errorf("unparseable reply: %w", err)
		}
		return &Reply{Text: text}, nil
	}
	if strings.TrimSpace(reply.Text) == "" {
		return nil, errors.New("reply had no text")
	}
	return sanitiseReply(&reply), nil
}

// sanitiseReply clamps everything the model could overshoot on, so the client
// never has to defend itself against a chatty turn.
func sanitiseReply(r *Reply) *Reply {
	r.Text = strings.TrimSpace(r.Text)
	if len(r.Cards) > 3 {
		r.Cards = r.Cards[:3]
	}
	// One card is not a comparison; it is a recommendation, and the text
	// already said it.
	if len(r.Cards) == 1 {
		r.Cards = nil
	}
	for i := range r.Cards {
		if len(r.Cards[i].Ratings) > 4 {
			r.Cards[i].Ratings = r.Cards[i].Ratings[:4]
		}
		for j := range r.Cards[i].Ratings {
			r.Cards[i].Ratings[j].Stars = clamp(r.Cards[i].Ratings[j].Stars, 1, 5)
		}
	}
	if r.Decision != nil {
		if len(r.Decision.Options) > 4 {
			r.Decision.Options = r.Decision.Options[:4]
		}
		if strings.TrimSpace(r.Decision.Question) == "" || len(r.Decision.Options) < 2 {
			r.Decision = nil
		}
	}
	if r.Memory != nil {
		r.Memory.Decided = trimList(r.Memory.Decided, 6)
		r.Memory.Open = trimList(r.Memory.Open, 6)
	}
	r.Action = strings.TrimSpace(r.Action)
	if len(r.Action) > 40 {
		r.Action = ""
	}
	return r
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func trimList(in []string, max int) []string {
	out := make([]string, 0, max)
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
		if len(out) == max {
			break
		}
	}
	return out
}
