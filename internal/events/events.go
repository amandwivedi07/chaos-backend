// Package events is a minimal async event bus. Domain services publish;
// workers subscribe. Keeps slow side effects (email) off the request path and
// gives new modules (notifications, analytics) a place to plug in without
// modifying existing code.
package events

import "context"

// Event names.
const (
	UserRegistered         = "user.registered"
	PasswordResetRequested = "auth.password_reset_requested"
)

type Event struct {
	Name    string
	Payload map[string]string
}

// Handler processes one event; returning error only logs (at-most-once bus).
type Handler func(ctx context.Context, e Event) error

// Bus is the publish port used by services.
type Bus interface {
	Publish(e Event)
}
