// Package service owns the conversation business rules: who may read a thread,
// when Chaos speaks, and what happens when someone picks an option.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
	"github.com/chaosapp/backend/internal/domain/conversation/mapper"
	"github.com/chaosapp/backend/internal/domain/conversation/repository"
	groupservice "github.com/chaosapp/backend/internal/domain/group/service"
	userrepo "github.com/chaosapp/backend/internal/domain/user/repository"
	"github.com/chaosapp/backend/internal/realtime"
)

// Facts is the slice of the profile module this service needs: what Chaos
// knows about the person asking. Declared here as a port so the conversation
// module does not import the profile module's service.
type Facts interface {
	For(ctx context.Context, userID uuid.UUID) ([]FactLine, error)
}

// FactLine is one durable thing about a person, flattened for prompting.
type FactLine struct {
	Label string
	Value string
}

// GroupContext is the slice of the group module this service needs: the
// standing context to prepend to every Chaos turn in a conversation that
// belongs to a group. Declared here as a port, the same way Facts is.
type GroupContext interface {
	Context(ctx context.Context, groupID uuid.UUID) (name string, memory []string, err error)
}

// Pusher notifies members who are not looking at the app.
type Pusher interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, title, body, convID string)
}

type ConversationService interface {
	List(ctx context.Context, userID uuid.UUID) ([]dto.ConversationResponse, error)
	Get(ctx context.Context, convID, userID uuid.UUID) (*dto.ConversationResponse, error)
	Create(ctx context.Context, userID uuid.UUID, req dto.CreateConversationRequest) (*dto.ConversationResponse, error)
	Update(ctx context.Context, convID, userID uuid.UUID, req dto.UpdateConversationRequest) (*dto.ConversationResponse, error)
	Leave(ctx context.Context, convID, userID uuid.UUID) error

	Messages(ctx context.Context, convID, userID uuid.UUID) ([]dto.MessageResponse, error)
	Send(ctx context.Context, convID, userID uuid.UUID, req dto.SendMessageRequest) (*dto.SendMessageResponse, error)
	Ask(ctx context.Context, convID, userID uuid.UUID) (*dto.MessageResponse, error)
	Choose(ctx context.Context, convID, userID uuid.UUID, req dto.ChooseRequest) (*dto.MessageResponse, error)
	Vote(ctx context.Context, decisionID, userID uuid.UUID, req dto.VoteRequest) (*dto.DecisionResponse, error)

	AddMembers(ctx context.Context, convID, userID uuid.UUID, req dto.AddMembersRequest) (*dto.ConversationResponse, error)
	Invite(ctx context.Context, convID, userID uuid.UUID) (*dto.InviteResponse, error)
	Join(ctx context.Context, convID, userID uuid.UUID, req dto.JoinRequest) (*dto.ConversationResponse, error)
	MarkSeen(ctx context.Context, convID, userID uuid.UUID) error
	SearchUsers(ctx context.Context, query string, exclude uuid.UUID) ([]dto.UserLookupResponse, error)

	// Placeholder satisfies the group module's People port: both a group and a
	// conversation can gain someone who has not signed up yet, and they must
	// resolve to the same stand-in account.
	Placeholder(ctx context.Context, name string) (uuid.UUID, error)
	// ForGroup satisfies the group module's Threads port — the transcripts
	// "Ask this group" searches.
	ForGroup(ctx context.Context, groupID uuid.UUID, limit int) ([]groupservice.Thread, error)
}

// Previewer is the one thing this module answers without a membership check:
// what an invite link looks like to someone who has not joined. It is a
// separate interface so that "unauthenticated" is visible at the type level
// and cannot quietly grow a second method.
type Previewer interface {
	Preview(ctx context.Context, convID uuid.UUID) (*dto.InviteResponse, error)
}

type service struct {
	repo    repository.Repository
	users   userrepo.UserRepository
	chaos   Chaos
	facts   Facts
	groups  GroupContext
	hub     realtime.Notifier
	pusher  Pusher
	log     *zap.Logger
	baseURL string
}

// Bundle is what wiring receives: the authenticated surface, plus the one read
// that answers without a membership check.
type Bundle struct {
	ConversationService
	Previewer
}

func New(repo repository.Repository, users userrepo.UserRepository, chaos Chaos,
	facts Facts, groups GroupContext, hub realtime.Notifier, pusher Pusher,
	log *zap.Logger, baseURL string) Bundle {
	s := &service{
		repo: repo, users: users, chaos: chaos, facts: facts, groups: groups,
		hub: hub, pusher: pusher, log: log, baseURL: strings.TrimSuffix(baseURL, "/"),
	}
	return Bundle{ConversationService: s, Previewer: s}
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]dto.ConversationResponse, error) {
	list, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return mapper.ToConversations(list, time.Now()), nil
}

func (s *service) Get(ctx context.Context, convID, userID uuid.UUID) (*dto.ConversationResponse, error) {
	c, err := s.repo.GetForUser(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	out := mapper.ToConversation(c, time.Now())
	return &out, nil
}

func (s *service) Create(ctx context.Context, userID uuid.UUID, req dto.CreateConversationRequest) (*dto.ConversationResponse, error) {
	title := strings.TrimSpace(req.Title)
	emoji := strings.TrimSpace(req.Emoji)
	if emoji == "" {
		emoji = "✦"
	}
	if req.Direct && len(req.MemberIDs) > 0 {
		// Silently dropping one or the other would give the caller a thread
		// that is not what they asked for.
		return nil, apperrors.BadRequest("A private conversation cannot have other people in it")
	}

	c := &entity.Conversation{
		Emoji:     emoji,
		Title:     title,
		Titled:    title != "",
		Direct:    req.Direct,
		CreatedBy: userID,
	}
	if !c.Titled {
		// A placeholder, not a name. The header offers to name it, and the
		// first line sent will make Chaos do it anyway.
		c.Title = "New conversation"
	}
	if req.GroupID != "" {
		groupID, err := uuid.Parse(req.GroupID)
		if err != nil {
			return nil, apperrors.BadRequest("Invalid group id")
		}
		c.GroupID = &groupID
	}

	members := []uuid.UUID{userID}
	for _, raw := range req.MemberIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, apperrors.BadRequest("Invalid member id")
		}
		if id != userID {
			members = append(members, id)
		}
	}
	if err := s.repo.Create(ctx, c, members); err != nil {
		return nil, err
	}
	s.notify(members, realtime.Event{Type: realtime.EventConversationsChanged})

	fresh, err := s.repo.GetForUser(ctx, c.ID, userID)
	if err != nil {
		return nil, err
	}
	out := mapper.ToConversation(fresh, time.Now())
	return &out, nil
}

func (s *service) Update(ctx context.Context, convID, userID uuid.UUID, req dto.UpdateConversationRequest) (*dto.ConversationResponse, error) {
	current, err := s.repo.GetForUser(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	title, titled, emoji := current.Title, current.Titled, ""
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
		if title == "" {
			return nil, apperrors.BadRequest("Give it a name or leave it alone")
		}
		titled = true
	}
	if req.Emoji != nil {
		emoji = strings.TrimSpace(*req.Emoji)
	}
	if err := s.repo.Rename(ctx, convID, title, emoji, titled); err != nil {
		return nil, err
	}
	if req.GroupID != nil {
		// An empty string takes it out of whatever group it was in; null (the
		// field absent) leaves it alone.
		var groupID *uuid.UUID
		if trimmed := strings.TrimSpace(*req.GroupID); trimmed != "" {
			parsed, err := uuid.Parse(trimmed)
			if err != nil {
				return nil, apperrors.BadRequest("Invalid group id")
			}
			groupID = &parsed
		}
		if err := s.repo.SetGroup(ctx, convID, groupID); err != nil {
			return nil, err
		}
	}
	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventConversationsChanged})
	return s.Get(ctx, convID, userID)
}

func (s *service) Leave(ctx context.Context, convID, userID uuid.UUID) error {
	if _, err := s.repo.GetForUser(ctx, convID, userID); err != nil {
		return err
	}
	if err := s.repo.Leave(ctx, convID, userID); err != nil {
		return err
	}
	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventConversationsChanged})
	return nil
}

func (s *service) Messages(ctx context.Context, convID, userID uuid.UUID) ([]dto.MessageResponse, error) {
	if _, err := s.repo.GetForUser(ctx, convID, userID); err != nil {
		return nil, err
	}
	list, err := s.repo.ListMessages(ctx, convID, 200)
	if err != nil {
		return nil, err
	}
	// Opening the thread is reading it.
	if err := s.repo.MarkSeen(ctx, convID, userID); err != nil {
		s.log.Warn("mark seen failed", zap.Error(err))
	}
	return mapper.ToMessages(list), nil
}

func (s *service) MarkSeen(ctx context.Context, convID, userID uuid.UUID) error {
	member, err := s.repo.IsMember(ctx, convID, userID)
	if err != nil {
		return err
	}
	if !member {
		return apperrors.NotFound("Conversation not found")
	}
	return s.repo.MarkSeen(ctx, convID, userID)
}

// SearchUsers backs the add-people picker.
//
// An empty query is not an empty result: it answers with the people the caller
// already shares conversations with, which is what the sheet shows as chips
// before anyone types. Searching a directory of strangers is the fallback, not
// the default.
func (s *service) SearchUsers(ctx context.Context, query string, exclude uuid.UUID) ([]dto.UserLookupResponse, error) {
	if strings.TrimSpace(query) == "" {
		known, err := s.repo.KnownPeople(ctx, exclude, 30)
		if err != nil {
			return nil, err
		}
		out := make([]dto.UserLookupResponse, 0, len(known))
		for _, m := range known {
			out = append(out, dto.UserLookupResponse{
				ID:        m.UserID.String(),
				Name:      m.Name,
				Handle:    m.Handle,
				AvatarURL: m.AvatarURL,
				PaletteID: m.PaletteID,
				Note:      m.Note,
			})
		}
		return out, nil
	}

	users, err := s.users.Search(ctx, query, exclude, 20)
	if err != nil {
		return nil, err
	}
	out := make([]dto.UserLookupResponse, 0, len(users))
	for i := range users {
		out = append(out, dto.UserLookupResponse{
			ID:        users[i].ID.String(),
			Name:      users[i].Name,
			Handle:    users[i].Handle,
			AvatarURL: users[i].AvatarURL,
			PaletteID: users[i].PaletteID,
		})
	}
	return out, nil
}

// ---- notification helpers ----

func (s *service) notify(userIDs []uuid.UUID, event realtime.Event) {
	if s.hub != nil {
		s.hub.NotifyUsers(userIDs, event)
	}
}

func (s *service) notifyMembers(ctx context.Context, convID uuid.UUID, event realtime.Event) {
	ids, err := s.repo.MemberIDs(ctx, convID)
	if err != nil {
		s.log.Warn("notify members failed", zap.Error(err))
		return
	}
	event.ConversationID = convID.String()
	s.notify(ids, event)
}

// push tells members who are not connected that something happened. Anyone
// with a live socket is already looking at it.
func (s *service) push(ctx context.Context, convID uuid.UUID, exclude uuid.UUID, title, body string) {
	if s.pusher == nil {
		return
	}
	ids, err := s.repo.MemberIDs(ctx, convID)
	if err != nil {
		return
	}
	offline := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == exclude {
			continue
		}
		if s.hub != nil && s.hub.IsConnected(id) {
			continue
		}
		offline = append(offline, id)
	}
	if len(offline) > 0 {
		s.pusher.Notify(ctx, offline, title, body, convID.String())
	}
}

func (s *service) inviteURL(convID uuid.UUID) string {
	return fmt.Sprintf("%s/join/%s", s.baseURL, convID)
}
