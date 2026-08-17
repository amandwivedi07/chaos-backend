// Package ai is the port for everything generative in Chaos. There are exactly
// four things the product asks a model to do, and they are deliberately
// narrow:
//
//	Reply   — read a group conversation and answer as Chaos, optionally with
//	          comparison cards or a vote to run.
//	Name    — turn an opening line into a short title and one emoji.
//	Extract — read a transcript and pull out durable facts about a person.
//	Ask     — answer from everything a group has said, and cite the threads.
//
// Azure OpenAI is one adapter; Disabled is the other, so the server runs fine
// without credentials.
package ai

import "context"

// Rating is one row of the little star table on a comparison card.
type Rating struct {
	Label string `json:"label"`
	Stars int    `json:"stars"`
}

// Card is one option Chaos is putting forward. Three of these become the
// swipeable row under a reply.
type Card struct {
	Emoji   string   `json:"emoji"`
	Title   string   `json:"title"`
	Tagline string   `json:"tagline"`
	Why     string   `json:"why"`
	Ratings []Rating `json:"ratings"`
}

// Decision is a vote Chaos wants the group to run. Options are labels only;
// the tally lives in the database, not in the model.
type Decision struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// Memory is Chaos keeping score of the conversation: what is settled and what
// is still open. It is overwritten wholesale on each turn, never appended to,
// so a group that changes its mind is not haunted by the old answer.
type Memory struct {
	Decided []string `json:"decided"`
	Open    []string `json:"open"`
}

// Reply is one Chaos turn. Everything except Text is optional — most turns are
// a couple of sentences and nothing else.
type Reply struct {
	Text     string    `json:"text"`
	Cards    []Card    `json:"cards"`
	Decision *Decision `json:"decision"`
	Action   string    `json:"action"`
	Memory   *Memory   `json:"memory"`
}

// Name is the title Chaos gives a conversation once it knows what it is about.
type Name struct {
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
}

// Fact is one durable thing about a person: a constraint, a taste, a budget, a
// way of deciding. Never a one-off logistic.
type Fact struct {
	Label      string  `json:"label"`
	Value      string  `json:"value"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
}

// Person is a conversation member as the model sees them — a name and the one
// line of context that changes what Chaos should suggest.
type Person struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

// Turn is one line of transcript.
type Turn struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

// ReplyInput is everything the model needs to answer a group.
type ReplyInput struct {
	Title      string
	Members    []Person
	Transcript []Turn
	// GroupName and GroupMemory are the standing context of the group this
	// conversation belongs to, if any. Carrying it into every turn is the
	// whole reason a group exists rather than a bare member list.
	GroupName   string
	GroupMemory []string
	// Facts the group has accumulated about the person who asked. This is the
	// whole point of the profile: the plan should already know they cannot do
	// more than four nights.
	Facts []Fact
}

// ExtractInput is a body of text to mine for durable facts, plus what is
// already known so the model does not restate it.
type ExtractInput struct {
	Source     string // "ChatGPT", "Claude", "Gemini", "You", "Chaos group chats"
	Transcript string
	Existing   []Fact
}

// Answer is a recall from a group's history: what they worked out, and which
// conversations it came from.
type Answer struct {
	Text string `json:"text"`
	// References are conversation ids, as given to the model.
	References []string `json:"references"`
}

// Client is what the AI services depend on.
type Client interface {
	Reply(ctx context.Context, in ReplyInput) (*Reply, error)
	Name(ctx context.Context, purpose string) (*Name, error)
	Extract(ctx context.Context, in ExtractInput) ([]Fact, error)
	Ask(ctx context.Context, in AskInput) (*Answer, error)
	Enabled() bool
}

// Disabled stands in when no Azure credentials are configured. It never
// pretends to work — callers surface a clear "not available" instead.
type Disabled struct{}

func (Disabled) Reply(context.Context, ReplyInput) (*Reply, error) {
	return nil, ErrNotConfigured
}

func (Disabled) Name(context.Context, string) (*Name, error) {
	return nil, ErrNotConfigured
}

func (Disabled) Extract(context.Context, ExtractInput) ([]Fact, error) {
	return nil, ErrNotConfigured
}

func (Disabled) Ask(context.Context, AskInput) (*Answer, error) {
	return nil, ErrNotConfigured
}

func (Disabled) Enabled() bool { return false }
