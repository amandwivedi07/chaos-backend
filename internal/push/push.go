// Package push delivers notifications to devices that have no live socket.
// Payloads are data-only so the app renders them itself and a tap can open the
// exact conversation.
package push

import "context"

// Message is what a device receives: enough to route, and a preview line.
type Message struct {
	Tokens         []string
	Title          string
	Body           string
	ConversationID string
	Kind           string // "message" | "invite"
}

// Sender is the port; Disabled is used when no credentials are configured.
type Sender interface {
	// Send returns the tokens FCM rejected as permanently invalid so the
	// caller can prune them.
	Send(ctx context.Context, msg Message) (invalid []string, err error)
	Enabled() bool
}

// Disabled is a no-op Sender (dev without Firebase credentials).
type Disabled struct{}

var _ Sender = Disabled{}

func (Disabled) Send(context.Context, Message) ([]string, error) { return nil, nil }
func (Disabled) Enabled() bool                                   { return false }
