package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/ai"
	"github.com/chaosapp/backend/internal/cache"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

// ReplyRequest is everything a turn needs, in domain terms. The Chaos port
// owns the translation into prompt shapes so the service never assembles
// strings for a model.
type ReplyRequest struct {
	Conversation *entity.Conversation
	History      []entity.Message
	Facts        []FactLine
	// GroupName and GroupMemory are the standing context of the group this
	// conversation belongs to, if any.
	GroupName   string
	GroupMemory []string
}

// ChaosReply is a turn, already in domain shapes and ready to persist.
type ChaosReply struct {
	Text     string
	Action   string
	Cards    []entity.Card
	Decision *entity.Decision
	Decided  []string
	Open     []string
}

// Chaos is the product-level port over the raw model client: it enforces the
// spend cap and converts between domain entities and prompt shapes.
type Chaos interface {
	Reply(ctx context.Context, userID uuid.UUID, req ReplyRequest) (*ChaosReply, error)
	Name(ctx context.Context, purpose string) (*ai.Name, error)
	Enabled() bool
}

type chaos struct {
	client  ai.Client
	cache   cache.Store
	log     *zap.Logger
	perHour int
}

// NewChaos wraps the generative client with the things the product needs
// around it — currently one thing: a per-person hourly ceiling, because every
// message sent can trigger a billed call.
func NewChaos(client ai.Client, c cache.Store, log *zap.Logger, perHour int) Chaos {
	if perHour <= 0 {
		perHour = 120
	}
	return &chaos{client: client, cache: c, log: log, perHour: perHour}
}

func (c *chaos) Enabled() bool { return c.client.Enabled() }

// spend counts a call against the caller's hourly allowance.
func (c *chaos) spend(ctx context.Context, userID uuid.UUID) error {
	key := fmt.Sprintf("chaos:quota:%s", userID)
	n, err := c.cache.Incr(ctx, key, time.Hour)
	if err != nil {
		// A cache outage must not take the feature down; log and allow.
		c.log.Warn("chaos quota check failed", zap.Error(err))
		return nil
	}
	if n > int64(c.perHour) {
		return apperrors.RateLimited(
			"Chaos has done a lot of thinking this hour. Give it a few minutes.")
	}
	return nil
}

func (c *chaos) Reply(ctx context.Context, userID uuid.UUID, req ReplyRequest) (*ChaosReply, error) {
	if !c.client.Enabled() {
		return nil, apperrors.Unavailable("Chaos is not switched on here")
	}
	if err := c.spend(ctx, userID); err != nil {
		return nil, err
	}

	reply, err := c.client.Reply(ctx, buildInput(req))
	if err != nil {
		c.log.Warn("chaos reply failed", zap.Error(err))
		return nil, apperrors.Unavailable("Chaos couldn't answer just now")
	}
	return toDomain(reply), nil
}

func (c *chaos) Name(ctx context.Context, purpose string) (*ai.Name, error) {
	if !c.client.Enabled() {
		return nil, apperrors.Unavailable("Chaos is not switched on here")
	}
	return c.client.Name(ctx, purpose)
}

// buildInput turns the thread into the flat shapes the model reads. Names are
// resolved here so the transcript reads like a conversation rather than a
// table of user ids.
func buildInput(req ReplyRequest) ai.ReplyInput {
	conv := req.Conversation
	names := make(map[uuid.UUID]string, len(conv.Members))
	members := make([]ai.Person, 0, len(conv.Members))
	for _, m := range conv.Members {
		names[m.UserID] = m.Name
		members = append(members, ai.Person{Name: m.Name, Note: m.Note})
	}

	transcript := make([]ai.Turn, 0, len(req.History))
	for _, msg := range req.History {
		if msg.Kind == entity.KindSystem {
			continue
		}
		author := "Chaos"
		if msg.AuthorID != nil {
			if name, ok := names[*msg.AuthorID]; ok {
				author = name
			} else {
				author = "Someone"
			}
		}
		transcript = append(transcript, ai.Turn{Author: author, Text: msg.Text})
	}

	facts := make([]ai.Fact, 0, len(req.Facts))
	for _, f := range req.Facts {
		facts = append(facts, ai.Fact{Label: f.Label, Value: f.Value})
	}

	title := ""
	if conv.Titled {
		title = conv.Title
	}
	return ai.ReplyInput{
		Title:       title,
		Members:     members,
		Transcript:  transcript,
		Facts:       facts,
		GroupName:   req.GroupName,
		GroupMemory: req.GroupMemory,
	}
}

// toDomain converts the model's answer into entities. Ids are left zero: the
// repository assigns them when the turn is written.
func toDomain(r *ai.Reply) *ChaosReply {
	out := &ChaosReply{
		Text:   strings.TrimSpace(r.Text),
		Action: r.Action,
	}
	for _, c := range r.Cards {
		card := entity.Card{
			Emoji: c.Emoji, Title: c.Title, Tagline: c.Tagline, Why: c.Why,
		}
		for _, rt := range c.Ratings {
			card.Ratings = append(card.Ratings, entity.Rating{Label: rt.Label, Stars: rt.Stars})
		}
		out.Cards = append(out.Cards, card)
	}
	if r.Decision != nil {
		decision := &entity.Decision{Question: r.Decision.Question}
		for i, label := range r.Decision.Options {
			decision.Options = append(decision.Options,
				entity.Option{Label: label, Position: i})
		}
		out.Decision = decision
	}
	if r.Memory != nil {
		out.Decided = r.Memory.Decided
		out.Open = r.Memory.Open
	}
	return out
}
