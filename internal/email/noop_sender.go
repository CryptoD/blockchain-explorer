package email

import "context"

// NoopEmailSender implements [EmailSender] without sending mail. Useful for tests and dry runs.
type NoopEmailSender struct{}

func (NoopEmailSender) Name() string { return "noop" }

func (NoopEmailSender) Send(ctx context.Context, from Address, msg Message) error {
	_ = ctx
	_ = from
	_ = msg
	return nil
}

var _ EmailSender = (*NoopEmailSender)(nil)
