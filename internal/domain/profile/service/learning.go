package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/ai"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/profile/dto"
	"github.com/chaosapp/backend/internal/domain/profile/entity"
)

// sourceLabels are what goes on the chip under each fact.
var sourceLabels = map[string]string{
	entity.SourceChatGPT: "ChatGPT",
	entity.SourceClaude:  "Claude",
	entity.SourceGemini:  "Gemini",
	entity.SourceYou:     "You",
	entity.SourceChaos:   "Chaos",
}

// importPrompt is what a person pastes into their other assistant. It asks for
// exactly the shape this module can use — durable preferences, one line each —
// because an unstructured chat export mostly extracts to nothing.
const importPrompt = `I'm building a personal context profile for a group-planning AI called Chaos. Based on everything you know about me from our conversations, write a compact list of my durable preferences and constraints: travel style, budget, food, dietary needs, availability, tastes, work style, and how I make decisions. One short line per fact, no fluff, no one-off logistics. I'll paste this into Chaos.`

// Prompt returns the ready-made prompt plus the link that opens the other
// assistant with it already typed.
func (s *service) Prompt(source string) (*dto.PromptResponse, error) {
	if !entity.ValidImportSource(source) {
		return nil, apperrors.BadRequest("Not an assistant Chaos knows")
	}
	q := url.QueryEscape(importPrompt)
	var link string
	switch source {
	case entity.SourceClaude:
		link = "https://claude.ai/new?q=" + q
	case entity.SourceGemini:
		link = "https://gemini.google.com/app?q=" + q
	default:
		link = "https://chatgpt.com/?q=" + q
	}
	return &dto.PromptResponse{Source: source, Prompt: importPrompt, URL: link}, nil
}

// AddFact is the profile composer. The person types a sentence about
// themselves and gets structured facts back — and because they said it, the
// results land confirmed and at full confidence.
func (s *service) AddFact(ctx context.Context, userID uuid.UUID, req dto.AddFactRequest) (*dto.LearnResponse, error) {
	return s.extract(ctx, userID, extractRequest{
		Source:     entity.SourceYou,
		Label:      sourceLabels[entity.SourceYou],
		Transcript: req.Text,
		Confirmed:  true,
	})
}

// Learn takes a paste from another assistant.
func (s *service) Learn(ctx context.Context, userID uuid.UUID, req dto.LearnRequest) (*dto.LearnResponse, error) {
	if !entity.ValidImportSource(req.Source) {
		return nil, apperrors.BadRequest("Not an assistant Chaos knows")
	}
	out, err := s.extract(ctx, userID, extractRequest{
		Source:     req.Source,
		Label:      sourceLabels[req.Source],
		Transcript: req.Transcript,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.MarkConnected(ctx, userID, req.Source); err != nil {
		s.log.Warn("mark connected failed", zap.Error(err))
	}
	return out, nil
}

type extractRequest struct {
	Source     string
	Label      string
	Transcript string
	// Confirmed marks the results as the person's own words rather than a
	// model's reading of them.
	Confirmed bool
}

func (s *service) extract(ctx context.Context, userID uuid.UUID, req extractRequest) (*dto.LearnResponse, error) {
	if !s.client.Enabled() {
		return nil, apperrors.Unavailable("Chaos is not switched on here")
	}
	transcript := strings.TrimSpace(req.Transcript)
	if transcript == "" {
		return nil, apperrors.BadRequest("Tell Chaos something about you")
	}
	if err := s.spend(ctx, userID); err != nil {
		return nil, err
	}

	existing, err := s.repo.ListFacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	known := make([]ai.Fact, 0, len(existing))
	for _, f := range existing {
		known = append(known, ai.Fact{Label: f.Label, Value: f.Value})
	}

	found, err := s.client.Extract(ctx, ai.ExtractInput{
		Source:     req.Label,
		Transcript: transcript,
		Existing:   known,
	})
	if err != nil {
		s.log.Warn("extract failed", zap.Error(err))
		return nil, apperrors.Unavailable("Chaos couldn't read that just now")
	}

	facts := make([]entity.Fact, 0, len(found))
	for _, f := range found {
		confidence := f.Confidence
		if req.Confirmed {
			// The person said it themselves; a model's hedge does not apply.
			confidence = 1
		}
		facts = append(facts, entity.Fact{
			Label: f.Label, Value: f.Value, Category: f.Category,
			Confidence: confidence, Source: req.Source,
			SourceLabel: req.Label, Confirmed: req.Confirmed,
		})
	}
	added, err := s.repo.UpsertFacts(ctx, userID, facts)
	if err != nil {
		return nil, err
	}
	s.refreshNote(ctx, userID)

	fresh, err := s.repo.ListFacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &dto.LearnResponse{Facts: toFactResponses(fresh), Added: added}, nil
}

// refreshNote rewrites the one line the conversation module reads when it
// prompts. It is derived from the facts rather than typed, so it can never
// drift out of date with them.
func (s *service) refreshNote(ctx context.Context, userID uuid.UUID) {
	facts, err := s.repo.ListFacts(ctx, userID)
	if err != nil {
		return
	}
	parts := make([]string, 0, 3)
	for _, f := range facts {
		if !f.Confirmed && f.Confidence < 0.7 {
			continue
		}
		parts = append(parts, f.Value)
		if len(parts) == 3 {
			break
		}
	}
	note := strings.Join(parts, " · ")
	if len([]rune(note)) > 140 {
		note = strings.TrimSpace(string([]rune(note)[:140]))
	}
	if err := s.repo.SetNote(ctx, userID, note); err != nil {
		s.log.Warn("set note failed", zap.Error(err))
	}
}

// spend counts an extraction against the caller's hourly allowance. Shared
// budget with conversation turns: both cost money and the point is to bound a
// runaway client, not to price them separately.
func (s *service) spend(ctx context.Context, userID uuid.UUID) error {
	key := fmt.Sprintf("chaos:quota:%s", userID)
	n, err := s.cache.Incr(ctx, key, time.Hour)
	if err != nil {
		s.log.Warn("quota check failed", zap.Error(err))
		return nil
	}
	if n > int64(s.perHour) {
		return apperrors.RateLimited(
			"Chaos has done a lot of thinking this hour. Give it a few minutes.")
	}
	return nil
}

// minNewMessages is how much has to be said before a refresh is worth a model
// call. Mining after every single line would burn the hourly budget on
// "haha" and "ok", and produce nothing.
const minNewMessages = 2

// Refresh mines what the person has said since the last run.
//
// This is what makes the profile screen's promise — "these fill in
// automatically as you chat" — actually true. It is deliberately cheap to
// call: the client fires it on every profile open, and the counter check
// short-circuits before anything expensive happens.
func (s *service) Refresh(ctx context.Context, userID uuid.UUID) (*dto.LearnResponse, error) {
	if s.history == nil || !s.client.Enabled() {
		return s.currentFacts(ctx, userID)
	}

	total, err := s.history.CountOwnMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if total == 0 || total-profile.LearnedCount < minNewMessages {
		return s.currentFacts(ctx, userID)
	}

	messages, err := s.history.OwnMessages(ctx, userID, 80)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return s.currentFacts(ctx, userID)
	}

	// Each line carries the conversation it came from, so the model can tell a
	// standing preference from a one-off inside a single trip.
	var b strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&b, "[%s] %s\n", m.ConversationTitle, m.Text)
	}
	// The chip should name where the fact came from, and the most recent
	// conversation is the one the person just came from.
	label := messages[len(messages)-1].ConversationTitle
	if label == "" {
		label = "Chaos"
	}

	out, err := s.extract(ctx, userID, extractRequest{
		Source:     entity.SourceChaos,
		Label:      label,
		Transcript: b.String(),
	})
	// Move the watermark either way. A failed extraction that keeps retrying
	// on every profile open would spend the whole hourly budget on the same
	// eighty messages.
	if markErr := s.repo.SetLearnedCount(ctx, userID, total); markErr != nil {
		s.log.Warn("set learned count failed", zap.Error(markErr))
	}
	if err != nil {
		s.log.Warn("refresh failed", zap.Error(err))
		return s.currentFacts(ctx, userID)
	}
	return out, nil
}

// currentFacts is the no-op answer: what is already known, nothing added.
func (s *service) currentFacts(ctx context.Context, userID uuid.UUID) (*dto.LearnResponse, error) {
	facts, err := s.repo.ListFacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &dto.LearnResponse{Facts: toFactResponses(facts), Added: 0}, nil
}
