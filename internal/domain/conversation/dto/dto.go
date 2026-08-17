// Package dto defines transport shapes for conversations and messages.
package dto

import "time"

// ---- Requests ----

type CreateConversationRequest struct {
	// Both optional: "Skip" on the home screen starts an untitled conversation
	// and lets Chaos name it from the first line.
	Title     string   `json:"title"      validate:"omitempty,max=60"`
	Emoji     string   `json:"emoji"      validate:"omitempty,max=8"`
	MemberIDs []string `json:"member_ids" validate:"omitempty,max=32,dive,uuid4"`
	// GroupID scopes the conversation to a standing group, so its memory
	// applies and it shows under that group.
	GroupID string `json:"group_id" validate:"omitempty,uuid4"`
	// Direct starts a private thread with Chaos alone. Mutually exclusive with
	// members: the service rejects the combination rather than silently
	// picking one.
	Direct bool `json:"direct"`
}

type UpdateConversationRequest struct {
	Title *string `json:"title" validate:"omitempty,min=1,max=60"`
	Emoji *string `json:"emoji" validate:"omitempty,max=8"`
	// GroupID moves the conversation into a group; an empty string takes it
	// out of one. Null leaves it alone.
	GroupID *string `json:"group_id" validate:"omitempty"`
}

type SendMessageRequest struct {
	Text string `json:"text" validate:"required,max=4000"`
	// SpeakingAs lets the demo/companion mode post as another member of the
	// conversation — the avatar button left of the composer. Must be a member.
	SpeakingAs string `json:"speaking_as" validate:"omitempty,uuid4"`
}

type AddMembersRequest struct {
	MemberIDs []string `json:"member_ids" validate:"omitempty,max=32,dive,uuid4"`
	// Name adds someone who has no account yet — they show up as a member and
	// claim the seat when they open the invite link.
	Name string `json:"name" validate:"omitempty,min=1,max=60"`
}

type JoinRequest struct {
	Name     string `json:"name"      validate:"required,min=1,max=60"`
	PhotoURL string `json:"photo_url" validate:"omitempty,url"`
}

type ChooseRequest struct {
	CardID string `json:"card_id" validate:"required,uuid4"`
}

type VoteRequest struct {
	OptionID string `json:"option_id" validate:"required,uuid4"`
}

// ---- Responses ----

type MemberResponse struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Handle    string `json:"handle,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	PaletteID string `json:"palette_id"`
	Note      string `json:"note,omitempty"`
	Role      string `json:"role"`
	Presence  string `json:"presence"`
}

type RatingResponse struct {
	Label string `json:"label"`
	Stars int    `json:"stars"`
}

type CardResponse struct {
	ID      string           `json:"id"`
	Emoji   string           `json:"emoji"`
	Title   string           `json:"title"`
	Tagline string           `json:"tagline"`
	Why     string           `json:"why"`
	Ratings []RatingResponse `json:"ratings"`
}

type OptionResponse struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Votes []string `json:"votes"` // user ids, so the client can stack avatars
}

type DecisionResponse struct {
	ID               string           `json:"id"`
	Question         string           `json:"question"`
	ResolvedOptionID string           `json:"resolved_option_id,omitempty"`
	Options          []OptionResponse `json:"options"`
}

type MessageResponse struct {
	ID string `json:"id"`
	// Kind is user | chaos | system. Clients switch layout on this rather than
	// sniffing whether author_id is empty.
	Kind     string            `json:"kind"`
	AuthorID string            `json:"author_id,omitempty"`
	Text     string            `json:"text"`
	Action   string            `json:"action,omitempty"`
	SentAt   time.Time         `json:"sent_at"`
	Cards    []CardResponse    `json:"cards,omitempty"`
	Decision *DecisionResponse `json:"decision,omitempty"`
}

type ConversationResponse struct {
	ID      string `json:"id"`
	Emoji   string `json:"emoji"`
	Title   string `json:"title"`
	Titled  bool   `json:"titled"`
	GroupID string `json:"group_id,omitempty"`
	// GroupName saves the home list a second fetch to render the group badge.
	GroupName string `json:"group_name,omitempty"`
	Direct    bool   `json:"direct"`
	// MessageCount drives "6 messages · 2h" on the group screen.
	MessageCount int              `json:"message_count"`
	Members      []MemberResponse `json:"members"`
	Decided      []string         `json:"decided,omitempty"`
	Open         []string         `json:"open,omitempty"`
	Unread       int              `json:"unread"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	// Preview is the last thing said, for the home list.
	Preview string `json:"preview,omitempty"`
}

// SendMessageResponse carries both halves of a send: the line the person just
// wrote, and Chaos's answer when the turn earned one. Returning them together
// means the client never has to guess whether to wait.
type SendMessageResponse struct {
	Message MessageResponse  `json:"message"`
	Reply   *MessageResponse `json:"reply,omitempty"`
	// Thinking is true when Chaos owes an answer that is still being written —
	// it will arrive over the socket. Only set when the reply is deferred.
	Thinking bool `json:"thinking"`
}

// InviteResponse is what the join screen renders before anyone types a name.
type InviteResponse struct {
	ConversationID string   `json:"conversation_id"`
	Title          string   `json:"title"`
	Emoji          string   `json:"emoji"`
	URL            string   `json:"url"`
	MemberNames    []string `json:"member_names"`
}

type UserLookupResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Handle    string `json:"handle,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	PaletteID string `json:"palette_id"`
	Note      string `json:"note,omitempty"`
}
