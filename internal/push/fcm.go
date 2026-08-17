package push

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

// FCM sends through Firebase Cloud Messaging (one app credential covers both
// APNs and Android).
type FCM struct {
	client *messaging.Client
	log    *zap.Logger
}

var _ Sender = (*FCM)(nil)

// NewFCM builds a sender from a service-account JSON file. An empty path
// yields (nil, nil) so callers can fall back to Disabled.
func NewFCM(ctx context.Context, credentialsFile string, log *zap.Logger) (*FCM, error) {
	if credentialsFile == "" {
		return nil, nil
	}
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, fmt.Errorf("firebase app: %w", err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase messaging: %w", err)
	}
	return &FCM{client: client, log: log}, nil
}

func (f *FCM) Enabled() bool { return f != nil && f.client != nil }

func (f *FCM) Send(ctx context.Context, msg Message) ([]string, error) {
	if !f.Enabled() || len(msg.Tokens) == 0 {
		return nil, nil
	}
	// Data-only payload + a generic notification: the OS shows "Something is
	// waiting on Space", never the card itself.
	res, err := f.client.SendEachForMulticast(ctx, &messaging.MulticastMessage{
		Tokens: msg.Tokens,
		Data: map[string]string{
			"conversation_id": msg.ConversationID,
			"kind":            msg.Kind,
		},
		Notification: &messaging.Notification{Title: msg.Title, Body: msg.Body},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: "chaos_messages",
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{"apns-priority": "10"},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{Sound: "default"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("fcm send: %w", err)
	}

	var invalid []string
	for i, r := range res.Responses {
		if r.Success {
			continue
		}
		if messaging.IsUnregistered(r.Error) || messaging.IsInvalidArgument(r.Error) {
			invalid = append(invalid, msg.Tokens[i])
		} else {
			f.log.Warn("fcm delivery failed", zap.Error(r.Error))
		}
	}
	return invalid, nil
}
