// Package mapper converts conversation entities into transport shapes.
// One direction per function; no logic beyond shape translation.
package mapper

import (
	"time"

	"github.com/chaosapp/backend/internal/domain/conversation/dto"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

func ToMember(m entity.Member, now time.Time) dto.MemberResponse {
	return dto.MemberResponse{
		UserID:    m.UserID.String(),
		Name:      m.Name,
		Handle:    m.Handle,
		AvatarURL: m.AvatarURL,
		PaletteID: m.PaletteID,
		Note:      m.Note,
		Role:      m.Role,
		Presence:  entity.Presence(m.LastSeen, now),
	}
}

func ToConversation(c *entity.Conversation, now time.Time) dto.ConversationResponse {
	members := make([]dto.MemberResponse, len(c.Members))
	for i, m := range c.Members {
		members[i] = ToMember(m, now)
	}
	preview := ""
	if c.LastMessage != nil {
		preview = c.LastMessage.Text
	}
	groupID := ""
	if c.GroupID != nil {
		groupID = c.GroupID.String()
	}
	return dto.ConversationResponse{
		ID:           c.ID.String(),
		Emoji:        c.Emoji,
		Title:        c.Title,
		Titled:       c.Titled,
		GroupID:      groupID,
		GroupName:    c.GroupName,
		Direct:       c.Direct,
		MessageCount: c.MessageCount,
		Members:      members,
		Decided:      c.Decided,
		Open:         c.Open,
		Unread:       c.Unread,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		Preview:      preview,
	}
}

func ToConversations(list []entity.Conversation, now time.Time) []dto.ConversationResponse {
	out := make([]dto.ConversationResponse, len(list))
	for i := range list {
		out[i] = ToConversation(&list[i], now)
	}
	return out
}

func ToCard(c entity.Card) dto.CardResponse {
	ratings := make([]dto.RatingResponse, len(c.Ratings))
	for i, r := range c.Ratings {
		ratings[i] = dto.RatingResponse{Label: r.Label, Stars: r.Stars}
	}
	return dto.CardResponse{
		ID:      c.ID.String(),
		Emoji:   c.Emoji,
		Title:   c.Title,
		Tagline: c.Tagline,
		Why:     c.Why,
		Ratings: ratings,
	}
}

func ToDecision(d *entity.Decision) *dto.DecisionResponse {
	if d == nil {
		return nil
	}
	options := make([]dto.OptionResponse, len(d.Options))
	for i, o := range d.Options {
		votes := make([]string, len(o.Votes))
		for j, v := range o.Votes {
			votes[j] = v.String()
		}
		options[i] = dto.OptionResponse{ID: o.ID.String(), Label: o.Label, Votes: votes}
	}
	resolved := ""
	if d.ResolvedOptionID != nil {
		resolved = d.ResolvedOptionID.String()
	}
	return &dto.DecisionResponse{
		ID:               d.ID.String(),
		Question:         d.Question,
		ResolvedOptionID: resolved,
		Options:          options,
	}
}

func ToMessage(m *entity.Message) dto.MessageResponse {
	author := ""
	if m.AuthorID != nil {
		author = m.AuthorID.String()
	}
	var cards []dto.CardResponse
	if len(m.Cards) > 0 {
		cards = make([]dto.CardResponse, len(m.Cards))
		for i, c := range m.Cards {
			cards[i] = ToCard(c)
		}
	}
	return dto.MessageResponse{
		ID:       m.ID.String(),
		Kind:     m.Kind,
		AuthorID: author,
		Text:     m.Text,
		Action:   m.Action,
		SentAt:   m.SentAt,
		Cards:    cards,
		Decision: ToDecision(m.Decision),
	}
}

func ToMessages(list []entity.Message) []dto.MessageResponse {
	out := make([]dto.MessageResponse, len(list))
	for i := range list {
		out[i] = ToMessage(&list[i])
	}
	return out
}
