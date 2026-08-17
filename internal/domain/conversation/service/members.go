package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
	userentity "github.com/chaosapp/backend/internal/domain/user/entity"
	"github.com/chaosapp/backend/internal/realtime"
)

// AddMembers pulls people into the thread. Anyone added sees everything said
// so far — that is the product promise on the sheet, and it is why there is no
// per-message access control anywhere in this module.
func (s *service) AddMembers(ctx context.Context, convID, userID uuid.UUID, req dto.AddMembersRequest) (*dto.ConversationResponse, error) {
	conv, err := s.repo.GetForUser(ctx, convID, userID)
	if err != nil {
		return nil, err
	}

	existing := make(map[uuid.UUID]bool, len(conv.Members))
	for _, m := range conv.Members {
		existing[m.UserID] = true
	}

	added := make([]string, 0, len(req.MemberIDs)+1)
	for _, raw := range req.MemberIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, apperrors.BadRequest("Invalid member id")
		}
		if existing[id] {
			continue
		}
		user, err := s.users.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := s.repo.AddMember(ctx, convID, id); err != nil {
			return nil, err
		}
		existing[id] = true
		added = append(added, user.Name)
	}

	if name := strings.TrimSpace(req.Name); name != "" {
		// Someone with no account yet. They get a placeholder person so the
		// group can plan around them, and the invite link claims the seat.
		placeholder, err := s.Placeholder(ctx, name)
		if err != nil {
			return nil, err
		}
		if !existing[placeholder] {
			if err := s.repo.AddMember(ctx, convID, placeholder); err != nil {
				return nil, err
			}
			added = append(added, name)
		}
	}

	for _, name := range added {
		s.system(ctx, convID, fmt.Sprintf("%s joined the conversation.", name))
	}
	if len(added) > 0 {
		s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventConversationsChanged})
	}
	return s.Get(ctx, convID, userID)
}

// Invite returns the link and the preview the join screen renders.
func (s *service) Invite(ctx context.Context, convID, userID uuid.UUID) (*dto.InviteResponse, error) {
	conv, err := s.repo.GetForUser(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	return s.inviteOf(conv), nil
}

// Preview is Invite for someone who is not in the room yet — the link they
// were sent has to render before they type a name.
func (s *service) Preview(ctx context.Context, convID uuid.UUID) (*dto.InviteResponse, error) {
	conv, err := s.repo.Get(ctx, convID)
	if err != nil {
		return nil, err
	}
	return s.inviteOf(conv), nil
}

func (s *service) inviteOf(conv *entity.Conversation) *dto.InviteResponse {
	names := make([]string, 0, len(conv.Members))
	for _, m := range conv.Members {
		names = append(names, m.Name)
	}
	return &dto.InviteResponse{
		ConversationID: conv.ID.String(),
		Title:          conv.Title,
		Emoji:          conv.Emoji,
		URL:            s.inviteURL(conv.ID),
		MemberNames:    names,
	}
}

// Join takes the caller into a conversation from an invite link, setting the
// display name and photo they chose on the way in.
func (s *service) Join(ctx context.Context, convID, userID uuid.UUID, req dto.JoinRequest) (*dto.ConversationResponse, error) {
	if _, err := s.repo.Get(ctx, convID); err != nil {
		return nil, err
	}
	already, err := s.repo.IsMember(ctx, convID, userID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if name != "" && name != user.Name {
		user.Name = name
		if req.PhotoURL != "" {
			user.AvatarURL = req.PhotoURL
		}
		if err := s.users.Update(ctx, user); err != nil {
			return nil, err
		}
	} else if req.PhotoURL != "" && req.PhotoURL != user.AvatarURL {
		user.AvatarURL = req.PhotoURL
		if err := s.users.Update(ctx, user); err != nil {
			return nil, err
		}
	}

	if !already {
		if err := s.repo.AddMember(ctx, convID, userID); err != nil {
			return nil, err
		}
		s.system(ctx, convID, fmt.Sprintf("%s joined the conversation.", user.Name))
		s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventConversationsChanged})
	}
	return s.Get(ctx, convID, userID)
}

// Placeholder finds or creates the stand-in account for a named person who has
// not signed up. Matching on name is deliberately loose — the sheet is a
// planning aid, not an identity system, and the invite link is what actually
// binds a seat to a real account.
//
// The row has no email and no password hash, so it can never be signed into.
// Someone opening the invite link joins as themselves; the placeholder is only
// ever a name on a card until then.
func (s *service) Placeholder(ctx context.Context, name string) (uuid.UUID, error) {
	found, err := s.users.Search(ctx, name, uuid.Nil, 5)
	if err != nil {
		return uuid.Nil, err
	}
	for i := range found {
		if strings.EqualFold(found[i].Name, name) {
			return found[i].ID, nil
		}
	}
	user := &userentity.User{
		Name:     name,
		Role:     "user",
		Provider: "placeholder",
	}
	if err := s.users.Create(ctx, user); err != nil {
		return uuid.Nil, err
	}
	return user.ID, nil
}

// system writes one of the room's own lines ("Meera joined the conversation.").
// It has no author and never counts as unread.
func (s *service) system(ctx context.Context, convID uuid.UUID, text string) {
	message := &entity.Message{
		ConversationID: convID,
		Kind:           entity.KindSystem,
		Text:           text,
	}
	if err := s.repo.InsertMessage(ctx, message); err != nil {
		return
	}
	s.notifyMembers(ctx, convID, realtime.Event{Type: realtime.EventMessagesChanged})
}
