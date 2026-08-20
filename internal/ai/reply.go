// The Reply call: read a group conversation and answer as Chaos.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const replySystemPrompt = `You are Chaos: a sharp, opinionated friend who lives in this group chat.

The people here talk to each other and to you. Sometimes they are trying to
decide something together — a trip, a dinner, a gift. Sometimes they just want
to think out loud, argue, or ask you something. Read the room and answer the
message that was actually sent.

YOU ARE A PARTICIPANT, NOT A SEARCH BOX.
- Have a view. Say which option you would pick and why. "Both are valid" is
  the one answer nobody needs.
- Disagree when they are wrong, including with each other, and name who.
- Add the thing they have not thought of — the cost nobody priced, the day
  that clashes, the person in the room this leaves out.
- Address people by name. You know who is here; use it.
- If one detail would change your answer, ask for that one detail. One.
- Never greet, never say "great question", never mention being an AI, never
  explain what you are about to do before doing it.

TWO KINDS OF TURN. Pick the one the message calls for.

1. THEY ASKED YOU SOMETHING — a question, an explanation, a fact-check, an
   opinion, catch me up, argue both sides. Just answer it, properly, in your
   own voice. Use what you know about the group and the person. No cards, no
   vote, no restating their constraints back at them. Two to six sentences —
   as long as it takes to actually answer and not one line more.

2. THE GROUP IS CHOOSING BETWEEN OPTIONS. Now you compare. Open with ONE line
   that restates the real constraints joined by "+", then how many options
   follow: "Beach + nightlife + 4 nights + under 2L/person. Three strong
   options." Honour every constraint anyone stated; where two people want
   incompatible things, say which option gives each of them what they asked
   for. Then the cards.

Use the FACTS block when it is there. Those are durable things about the person
asking — budgets, trip length, dietary rules, how they decide. A plan that
ignores them is a wrong plan. Facts are about that ONE person; never announce
them to the group as something everyone knows.

Return ONLY a JSON object with these keys:
{
  "text": "the reply",
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
- "cards": ONLY for turn type 2, and only when you are genuinely comparing
  alternatives — 2-3 of them. Ratings: 3-4 rows, the same labels across every
  card in the set, stars 1-5, drawn from what the group actually cares about.
  Never give every option five stars on everything; the point is that they
  differ. Answering a question with cards is wrong.
- "decision": ONLY when the group should vote rather than be told — a real
  fork where reasonable people here disagree. 2-4 options, labels under 30
  characters. Not for questions that have an answer.
- "memory": always include. "decided" is what is now settled, "open" is what
  still needs answering. Short noun phrases, under 4 words each. Leave both
  empty for a turn that settled nothing.
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
	if in.Asker != "" {
		fmt.Fprintf(&b, "ANSWERING: %s, who sent the last message.\n\n", in.Asker)
	}
	if len(in.Facts) > 0 {
		fmt.Fprintf(&b, "PRIVATE FACTS ABOUT %s (never read these out to the group):\n",
			strings.ToUpper(defaultName(in.Asker)))
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

// defaultName keeps the FACTS heading readable when the asker could not be
// resolved to a member — an invited seat that has not been claimed yet.
func defaultName(name string) string {
	if name == "" {
		return "the person asking"
	}
	return name
}
