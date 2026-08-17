// Package dto defines transport shapes for the profile module.
package dto

import "time"

// ---- Requests ----

type UpdateProfileRequest struct {
	Name   *string `json:"name"       validate:"omitempty,min=1,max=60"`
	Handle *string `json:"handle"     validate:"omitempty,min=2,max=30,alphanum"`
	City   *string `json:"city"       validate:"omitempty,max=60"`
	Bio    *string `json:"bio"        validate:"omitempty,max=280"`
	// Deliberately NOT validate:"omitempty,url". On a *string, omitempty tests
	// the pointer for nil, not the string for empty — so a pointer to "" runs
	// the url rule and 400s, which makes removing a photo impossible. The
	// service checks the URL instead, where "" can mean "clear it".
	AvatarURL *string `json:"avatar_url" validate:"omitempty"`
}

// AddFactRequest is the profile composer: free text in, facts out. The person
// types "I never book anything over six hours" and gets a fact, not a note.
type AddFactRequest struct {
	Text string `json:"text" validate:"required,min=2,max=12000"`
}

type UpdateFactRequest struct {
	Value     *string `json:"value"     validate:"omitempty,min=1,max=280"`
	Confirmed *bool   `json:"confirmed"`
}

// LearnRequest is a pasted export from another assistant.
type LearnRequest struct {
	Source     string `json:"source"     validate:"required,oneof=chatgpt claude gemini"`
	Transcript string `json:"transcript" validate:"required,min=20,max=12000"`
}

// ---- Responses ----

type FactResponse struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Value       string    `json:"value"`
	Category    string    `json:"category"`
	Confidence  float64   `json:"confidence"`
	Source      string    `json:"source"`
	SourceLabel string    `json:"source_label,omitempty"`
	Confirmed   bool      `json:"confirmed"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ConnectionResponse struct {
	Source   string    `json:"source"`
	SyncedAt time.Time `json:"synced_at"`
	// Learned is how many facts currently come from this assistant — the
	// "3 learned" line under each tile.
	Learned int `json:"learned"`
}

type ProfileResponse struct {
	UserID      string               `json:"user_id"`
	Name        string               `json:"name"`
	Handle      string               `json:"handle,omitempty"`
	City        string               `json:"city,omitempty"`
	Bio         string               `json:"bio,omitempty"`
	AvatarURL   string               `json:"avatar_url,omitempty"`
	PaletteID   string               `json:"palette_id"`
	Note        string               `json:"note,omitempty"`
	Facts       []FactResponse       `json:"facts"`
	Connections []ConnectionResponse `json:"connections"`
}

// LearnResponse reports what a paste actually produced, so the screen can say
// "Added 3 inputs" or "Nothing new to add" rather than just going quiet.
type LearnResponse struct {
	Facts []FactResponse `json:"facts"`
	Added int            `json:"added"`
}

// PromptResponse is the ready-made prompt the profile screen hands to another
// assistant, plus the deep link that opens it there.
type PromptResponse struct {
	Source string `json:"source"`
	Prompt string `json:"prompt"`
	URL    string `json:"url"`
}
