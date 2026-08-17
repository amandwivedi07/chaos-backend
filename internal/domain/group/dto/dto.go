// Package dto defines transport shapes for the group module.
package dto

import "time"

// ---- Requests ----

type CreateGroupRequest struct {
	Name      string   `json:"name"       validate:"required,min=1,max=60"`
	Emoji     string   `json:"emoji"      validate:"omitempty,max=8"`
	MemberIDs []string `json:"member_ids" validate:"omitempty,max=64,dive,uuid4"`
}

type UpdateGroupRequest struct {
	Name  *string `json:"name"  validate:"omitempty,min=1,max=60"`
	Emoji *string `json:"emoji" validate:"omitempty,max=8"`
}

type AddMemberRequest struct {
	UserID string `json:"user_id" validate:"omitempty,uuid4"`
	// Name adds someone who has no account yet, the same way a conversation
	// does.
	Name string `json:"name" validate:"omitempty,min=1,max=60"`
}

type AddMemoryRequest struct {
	Text string `json:"text" validate:"required,min=2,max=280"`
}

// AskRequest is a question put to the whole group rather than to one thread.
type AskRequest struct {
	Question string `json:"question" validate:"required,min=2,max=1000"`
}

// ---- Responses ----

type MemberResponse struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Handle    string `json:"handle,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	PaletteID string `json:"palette_id"`
	Note      string `json:"note,omitempty"`
}

type MemoryResponse struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type GroupResponse struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Emoji   string           `json:"emoji"`
	Members []MemberResponse `json:"members"`
	Memory  []MemoryResponse `json:"memory"`
	// ConversationCount drives "4 people · 3 conversations".
	ConversationCount int       `json:"conversation_count"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ReferenceResponse is one conversation an answer drew on, with enough to
// render a tappable chip without a second fetch.
type ReferenceResponse struct {
	ConversationID string `json:"conversation_id"`
	Title          string `json:"title"`
	Emoji          string `json:"emoji"`
}

type AnswerResponse struct {
	Text       string              `json:"text"`
	References []ReferenceResponse `json:"references"`
}

// CollaboratorResponse is one row of "Frequent collaborators": someone you
// share conversations with, and how many.
type CollaboratorResponse struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Handle    string `json:"handle,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	PaletteID string `json:"palette_id"`
	Note      string `json:"note,omitempty"`
	// Conversations is how many you are both in — the number on the right and
	// the sort key.
	Conversations int `json:"conversations"`
}
