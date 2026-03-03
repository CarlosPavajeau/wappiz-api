package mailer

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v3"
)

type ResendMailer struct {
	client    *resend.Client
	fromEmail string
}

func NewResendMailer(apiKey, fromEmail string) Mailer {
	return &ResendMailer{
		client:    resend.NewClient(apiKey),
		fromEmail: fromEmail,
	}
}

func (m *ResendMailer) Send(ctx context.Context, email Email) error {
	params := &resend.SendEmailRequest{
		From:    m.fromEmail,
		To:      []string{email.To},
		Subject: email.Subject,
		Html:    email.Body,
	}

	_, err := m.client.Emails.SendWithContext(ctx, params)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}

	return nil
}
