// Package mailer provides a thin wrapper around the Resend email API for
// sending transactional emails.
//
// Create a [Mailer] with [New], then call [Mailer.Send] to dispatch a message.
// The [Mailer] interface is exposed so callers can substitute a mock in tests
// without making real API requests.
//
//	m := mailer.New(mailer.Config{
//	    ApiKey:    os.Getenv("RESEND_API_KEY"),
//	    FromEmail: "no-reply@example.com",
//	})
//
//	err := m.Send(ctx, mailer.Email{
//	    To:      "customer@example.com",
//	    Subject: "Your appointment is confirmed",
//	    Body:    "<p>See you tomorrow!</p>",
//	})
package mailer
