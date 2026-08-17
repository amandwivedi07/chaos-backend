package push

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Registry is the slice of the device module the notifier needs.
type Registry interface {
	TokensFor(ctx context.Context, userIDs []uuid.UUID) ([]string, error)
	DeleteTokens(ctx context.Context, tokens []string) error
}

// Notifier joins the device registry to a Sender, so domain services can say
// "tell these people" without knowing what a push token is. It satisfies the
// conversation module's Pusher port.
type Notifier struct {
	registry Registry
	sender   Sender
	log      *zap.Logger
}

func NewNotifier(registry Registry, sender Sender, log *zap.Logger) *Notifier {
	return &Notifier{registry: registry, sender: sender, log: log}
}

// Notify is best-effort by design: a person's message is already committed by
// the time this runs, and a failed notification must never surface as a failed
// send.
func (n *Notifier) Notify(ctx context.Context, userIDs []uuid.UUID, title, body, convID string) {
	if n == nil || len(userIDs) == 0 || !n.sender.Enabled() {
		return
	}
	tokens, err := n.registry.TokensFor(ctx, userIDs)
	if err != nil || len(tokens) == 0 {
		return
	}
	invalid, err := n.sender.Send(ctx, Message{
		Tokens:         tokens,
		Title:          title,
		Body:           clip(body, 140),
		ConversationID: convID,
		Kind:           "message",
	})
	if err != nil {
		n.log.Warn("push failed", zap.Error(err))
	}
	// FCM tells us which tokens are permanently dead; keeping them would mean
	// retrying a device that no longer exists on every future message.
	if len(invalid) > 0 {
		if err := n.registry.DeleteTokens(ctx, invalid); err != nil {
			n.log.Warn("prune tokens failed", zap.Error(err))
		}
	}
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
