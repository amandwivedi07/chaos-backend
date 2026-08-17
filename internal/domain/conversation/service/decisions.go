package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
	"github.com/chaosapp/backend/internal/domain/conversation/mapper"
	groupservice "github.com/chaosapp/backend/internal/domain/group/service"
	"github.com/chaosapp/backend/internal/realtime"
)

// Choose is "Choose this →" on an option card. It says so out loud in the
// thread rather than silently flipping a flag, because the whole point of the
// card is to end the argument in front of everyone, and it adds the option to
// what the conversation now considers settled.
func (s *service) Choose(ctx context.Context, convID, userID uuid.UUID, req dto.ChooseRequest) (*dto.MessageResponse, error) {
	cardID, err := uuid.Parse(req.CardID)
	if err != nil {
		return nil, apperrors.BadRequest("Invalid option id")
	}
	conv, err := s.repo.GetForUser(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	card, cardConvID, err := s.repo.GetCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if cardConvID != convID {
		return nil, apperrors.NotFound("Option not found")
	}

	message := &entity.Message{
		ConversationID: convID,
		AuthorID:       &userID,
		Kind:           entity.KindUser,
		Text:           fmt.Sprintf("I'm going with %s %s", card.Title, card.Emoji),
	}
	if err := s.repo.InsertMessage(ctx, message); err != nil {
		return nil, err
	}

	decided := append([]string{}, conv.Decided...)
	if !contains(decided, card.Title) {
		decided = append(decided, card.Title)
		if err := s.repo.SetMemory(ctx, convID, decided, conv.Open); err != nil {
			return nil, err
		}
	}

	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventMessagesChanged})
	s.push(ctx, convID, userID, s.pushTitle(conv), message.Text)

	out := mapper.ToMessage(message)
	return &out, nil
}

// Vote toggles the caller's vote on a decision and returns the fresh tally, so
// the client can redraw the avatars and the "we have a winner" line without a
// second round trip.
func (s *service) Vote(ctx context.Context, decisionID, userID uuid.UUID, req dto.VoteRequest) (*dto.DecisionResponse, error) {
	optionID, err := uuid.Parse(req.OptionID)
	if err != nil {
		return nil, apperrors.BadRequest("Invalid option id")
	}
	decision, err := s.repo.GetDecision(ctx, decisionID)
	if err != nil {
		return nil, err
	}
	member, err := s.repo.IsMember(ctx, decision.ConversationID, userID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, apperrors.NotFound("Vote not found")
	}

	fresh, err := s.repo.Vote(ctx, decisionID, optionID, userID)
	if err != nil {
		return nil, err
	}
	s.notifyMembers(ctx, decision.ConversationID,
		realtime.Event{Type: realtime.EventMessagesChanged})
	return mapper.ToDecision(fresh), nil
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// ForGroup gives the group module the transcripts "Ask this group" searches.
//
// It is a read of conversations the caller has already been proved a member of
// — the group service checks group membership before calling, and a group's
// conversations are by construction the group's business.
func (s *service) ForGroup(ctx context.Context, groupID uuid.UUID, limit int) ([]groupservice.Thread, error) {
	convs, err := s.repo.ListForGroup(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]groupservice.Thread, 0, len(convs))
	for i := range convs {
		conv := &convs[i]
		history, err := s.repo.ListMessages(ctx, conv.ID, 20)
		if err != nil {
			return nil, err
		}
		names := make(map[uuid.UUID]string, len(conv.Members))
		for _, m := range conv.Members {
			names[m.UserID] = m.Name
		}
		var b strings.Builder
		for _, msg := range history {
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
			fmt.Fprintf(&b, "%s: %s\n", author, msg.Text)
		}
		out = append(out, groupservice.Thread{
			ID:         conv.ID,
			Title:      conv.Title,
			Emoji:      conv.Emoji,
			Transcript: b.String(),
		})
	}
	return out, nil
}
