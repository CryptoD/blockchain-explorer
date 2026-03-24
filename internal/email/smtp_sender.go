package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPSender struct {
	Host        string
	Port        int
	Username    string
	Password    string
	UseSTARTTLS bool
	SkipVerify  bool
}

func (s *SMTPSender) Name() string { return "smtp" }

func (s *SMTPSender) Send(ctx context.Context, from Address, msg Message) error {
	if s == nil {
		return fmt.Errorf("smtp sender is nil")
	}
	if s.Host == "" || s.Port <= 0 {
		return fmt.Errorf("smtp host/port not configured")
	}
	if from.Email == "" || msg.To.Email == "" {
		return fmt.Errorf("missing from/to")
	}

	raw, err := buildMIME(from, msg)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", s.Port))

	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Quit() }()

	if s.UseSTARTTLS {
		cfg := &tls.Config{ServerName: s.Host, InsecureSkipVerify: s.SkipVerify}
		if err := c.StartTLS(cfg); err != nil {
			return fmt.Errorf("starttls failed: %w", err)
		}
	}

	if strings.TrimSpace(s.Username) != "" {
		auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth failed: %w", err)
		}
	}

	if err := c.Mail(from.Email); err != nil {
		return err
	}
	if err := c.Rcpt(msg.To.Email); err != nil {
		return err
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(raw)); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}
