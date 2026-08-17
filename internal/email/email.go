// Package email defines the outbound-mail port and its adapters.
// Services depend on Sender only; swap SMTP for SES/SendGrid by adding an
// adapter and changing one line of wiring.
package email

import "context"

type Message struct {
	To      string
	Subject string
	Body    string // plain text
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}
