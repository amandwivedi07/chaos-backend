package email

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/chaosapp/backend/internal/config"
)

// SMTPSender delivers over plain SMTP with AUTH (STARTTLS handled by server).
type SMTPSender struct {
	cfg config.EmailConfig
}

var _ Sender = (*SMTPSender)(nil)

func NewSMTPSender(cfg config.EmailConfig) *SMTPSender { return &SMTPSender{cfg: cfg} }

func (s *SMTPSender) Send(_ context.Context, msg Message) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPass, s.cfg.SMTPHost)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		s.cfg.From, msg.To, msg.Subject, msg.Body)
	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{msg.To}, []byte(body)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
