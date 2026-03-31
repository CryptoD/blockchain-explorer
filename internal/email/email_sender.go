package email

import "context"

// EmailSender is the transport boundary for sending mail. SMTP is one implementation ([SMTPSender]);
// use [NoopEmailSender] in tests when no network I/O is desired.
type EmailSender interface {
	Send(ctx context.Context, from Address, msg Message) error
	Name() string
}
