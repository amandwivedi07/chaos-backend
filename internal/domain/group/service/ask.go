package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/ai"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/group/dto"
)

// askThreads is how many of the group's conversations are searched.
//
// Eight recent threads is roughly what fits in a prompt alongside the group's
// memory without truncating the transcripts into uselessness. Someone asking
// "what did we decide about Goa" means a recent thread, not one from a year
// ago.
const askThreads = 8

// Ask answers a question from everything the group has said.
//
// This is the one place Chaos reads across conversations rather than down one.
// The answer cites the threads it used, so the person can go and check —
// a recall nobody can verify is worse than no recall.
func (s *service) Ask(ctx context.Context, groupID, userID uuid.UUID, req dto.AskRequest) (*dto.AnswerResponse, error) {
	g, err := s.repo.GetForUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if !s.client.Enabled() {
		return nil, apperrors.Unavailable("Chaos is not switched on here")
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return nil, apperrors.BadRequest("Ask something")
	}
	if err := s.spend(ctx, userID); err != nil {
		return nil, err
	}

	threads, err := s.threads.ForGroup(ctx, groupID, askThreads)
	if err != nil {
		return nil, err
	}
	if len(threads) == 0 {
		// Nothing to remember yet. Say so rather than spending a model call to
		// have it say the same thing less clearly.
		return &dto.AnswerResponse{
			Text:       "Nothing to go on yet — this group hasn't talked about anything.",
			References: []dto.ReferenceResponse{},
		}, nil
	}

	memory := make([]string, 0, len(g.Memory))
	for _, m := range g.Memory {
		memory = append(memory, m.Text)
	}
	digests := make([]ai.ConversationDigest, 0, len(threads))
	for _, t := range threads {
		digests = append(digests, ai.ConversationDigest{
			ID:         t.ID.String(),
			Title:      t.Title,
			Transcript: t.Transcript,
		})
	}

	answer, err := s.client.Ask(ctx, ai.AskInput{
		GroupName:     g.Name,
		Memory:        memory,
		Question:      question,
		Conversations: digests,
	})
	if err != nil {
		s.log.Warn("group ask failed", zap.Error(err))
		return nil, apperrors.Unavailable("Chaos couldn't remember just now")
	}

	// Resolve the model's citations back to real threads. Anything it invented
	// is dropped rather than rendered as a dead chip.
	byID := make(map[string]Thread, len(threads))
	for _, t := range threads {
		byID[t.ID.String()] = t
	}
	refs := make([]dto.ReferenceResponse, 0, len(answer.References))
	for _, id := range answer.References {
		t, ok := byID[strings.TrimSpace(id)]
		if !ok {
			continue
		}
		refs = append(refs, dto.ReferenceResponse{
			ConversationID: t.ID.String(),
			Title:          t.Title,
			Emoji:          t.Emoji,
		})
	}

	return &dto.AnswerResponse{Text: answer.Text, References: refs}, nil
}

// spend counts an ask against the caller's hourly allowance — the same budget
// conversation turns and fact extraction draw on.
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
