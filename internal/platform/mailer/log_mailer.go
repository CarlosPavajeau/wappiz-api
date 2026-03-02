package mailer

import (
	"context"
	"log"
)

type LogMailer struct{}

func NewLogMailer() Mailer {
	return &LogMailer{}
}

func (m *LogMailer) Send(ctx context.Context, email Email) error {
	log.Printf("[mailer] TO: %s | SUBJECT: %s", email.To, email.Subject)
	return nil
}
