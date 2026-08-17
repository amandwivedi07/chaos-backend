package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
	"github.com/chaosapp/backend/internal/domain/conversation/mapper"
	"github.com/chaosapp/backend/internal/realtime"
)

// Send posts a line and, when the turn earns it, Chaos's answer with it.
//
// The person's message is committed before the model is called. A model that
// is slow, rate-limited or switched off must never cost someone the thing they
// typed — the worst case is a message with no reply under it.
func (s *service) Send(ctx context.Context, convID, userID uuid.UUID, req dto.SendMessageRequest) (*dto.SendMessageResponse, error) {
	conv, err := s.repo.GetForUser(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, apperrors.BadRequest("Say something first")
	}

	author, err := s.speaker(conv, userID, req.SpeakingAs)
	if err != nil {
		return nil, err
	}

	total, sinceChaos, err := s.repo.CountSinceChaos(ctx, convID)
	if err != nil {
		return nil, err
	}

	message := &entity.Message{
		ConversationID: convID,
		AuthorID:       &author,
		Kind:           entity.KindUser,
		Text:           text,
	}
	if err := s.repo.InsertMessage(ctx, message); err != nil {
		return nil, err
	}
	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventMessagesChanged})
	s.push(ctx, convID, author, s.pushTitle(conv), text)

	out := &dto.SendMessageResponse{Message: mapper.ToMessage(message)}

	// An untitled conversation gets its name from the first thing said in it.
	if !conv.Titled {
		s.nameFrom(ctx, convID, text)
	}

	if !entity.ShouldReply(text, total, sinceChaos, conv.Direct) {
		return out, nil
	}
	reply, err := s.chaosTurn(ctx, convID, userID)
	if err != nil {
		// Chaos being unavailable is not a failed send. The line is already in
		// the thread; the client shows a quiet note instead of an error.
		s.log.Warn("chaos turn failed", zap.Error(err))
		return out, nil
	}
	answer := mapper.ToMessage(reply)
	out.Reply = &answer
	return out, nil
}

// Ask forces a turn — the client's "@Chaos" affordance, and what the empty
// state's suggestion chips end up doing.
func (s *service) Ask(ctx context.Context, convID, userID uuid.UUID) (*dto.MessageResponse, error) {
	if _, err := s.repo.GetForUser(ctx, convID, userID); err != nil {
		return nil, err
	}
	reply, err := s.chaosTurn(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	out := mapper.ToMessage(reply)
	return &out, nil
}

// speaker resolves who the line is attributed to. Normally the caller; the
// composer's avatar button can post as another member so one person can walk
// through a group conversation on their own device.
func (s *service) speaker(conv *entity.Conversation, userID uuid.UUID, speakingAs string) (uuid.UUID, error) {
	if speakingAs == "" {
		return userID, nil
	}
	id, err := uuid.Parse(speakingAs)
	if err != nil {
		return uuid.Nil, apperrors.BadRequest("Invalid speaker")
	}
	for _, m := range conv.Members {
		if m.UserID == id {
			return id, nil
		}
	}
	return uuid.Nil, apperrors.Forbidden("They are not in this conversation")
}

// chaosTurn builds the prompt from the thread, asks the model, and commits the
// answer plus whatever it decided to remember.
func (s *service) chaosTurn(ctx context.Context, convID, askerID uuid.UUID) (*entity.Message, error) {
	if !s.chaos.Enabled() {
		return nil, apperrors.Unavailable("Chaos is not switched on here")
	}
	conv, err := s.repo.GetForUser(ctx, convID, askerID)
	if err != nil {
		return nil, err
	}
	history, err := s.repo.ListMessages(ctx, convID, 30)
	if err != nil {
		return nil, err
	}

	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventChaosThinking})

	groupName, groupMemory := s.groupContext(ctx, conv)
	reply, err := s.chaos.Reply(ctx, askerID, ReplyRequest{
		Conversation: conv,
		History:      history,
		Facts:        s.factsFor(ctx, askerID),
		GroupName:    groupName,
		GroupMemory:  groupMemory,
	})
	if err != nil {
		return nil, err
	}

	message := &entity.Message{
		ConversationID: convID,
		Kind:           entity.KindChaos,
		Text:           reply.Text,
		Action:         reply.Action,
		Cards:          reply.Cards,
		Decision:       reply.Decision,
	}
	if err := s.repo.InsertMessage(ctx, message); err != nil {
		return nil, err
	}
	if reply.Decided != nil || reply.Open != nil {
		if err := s.repo.SetMemory(ctx, convID, reply.Decided, reply.Open); err != nil {
			s.log.Warn("set memory failed", zap.Error(err))
		}
	}
	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventMessagesChanged})
	s.push(ctx, convID, uuid.Nil, s.pushTitle(conv), reply.Text)
	return message, nil
}

// groupContext is the standing context of the group this conversation belongs
// to. Missing or unreadable is not an error — most conversations have no
// group, and a Chaos turn without the shared memory is worse than none but far
// better than a failed send.
func (s *service) groupContext(ctx context.Context, conv *entity.Conversation) (string, []string) {
	if conv.GroupID == nil || s.groups == nil {
		return "", nil
	}
	name, memory, err := s.groups.Context(ctx, *conv.GroupID)
	if err != nil {
		s.log.Warn("group context lookup failed", zap.Error(err))
		return "", nil
	}
	return name, memory
}

func (s *service) factsFor(ctx context.Context, userID uuid.UUID) []FactLine {
	if s.facts == nil {
		return nil
	}
	lines, err := s.facts.For(ctx, userID)
	if err != nil {
		s.log.Warn("facts lookup failed", zap.Error(err))
		return nil
	}
	return lines
}

// nameFrom asks Chaos for a title and emoji, falling back to the opening line
// itself. Failing to name a conversation must never fail the send, so this
// swallows its errors deliberately.
func (s *service) nameFrom(ctx context.Context, convID uuid.UUID, purpose string) {
	name, emoji := "", "✦"
	if s.chaos.Enabled() {
		if got, err := s.chaos.Name(ctx, purpose); err == nil {
			name, emoji = got.Name, got.Emoji
		} else {
			s.log.Warn("naming failed", zap.Error(err))
		}
	}
	if name == "" {
		name = clip(purpose, 34)
	}
	if err := s.repo.Rename(ctx, convID, name, emoji, true); err != nil {
		s.log.Warn("rename failed", zap.Error(err))
		return
	}
	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventConversationsChanged})
}

func clip(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return strings.TrimSpace(string(r[:max]))
}

func (s *service) pushTitle(conv *entity.Conversation) string {
	if conv.Titled && conv.Title != "" {
		return conv.Title
	}
	return "Chaos"
}
