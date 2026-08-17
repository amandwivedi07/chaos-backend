package email

import (
	"context"

	"go.uber.org/zap"
)

// LogSender "sends" by logging — the development default, so auth flows work
// without SMTP credentials.
type LogSender struct {
	log *zap.Logger
}

var _ Sender = (*LogSender)(nil)

func NewLogSender(log *zap.Logger) *LogSender { return &LogSender{log: log} }

func (s *LogSender) Send(_ context.Context, msg Message) error {
	s.log.Info("email (log driver)",
		zap.String("to", msg.To),
		zap.String("subject", msg.Subject),
		zap.String("body", msg.Body),
	)
	return nil
}
