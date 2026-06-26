// Package mailer provides the small delivery boundary used by authentication
// flows. Production uses SMTP; tests inject a recorder and local development
// points SMTP_HOST at Mailpit.
package mailer

import (
	"context"
	"fmt"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender interface {
	Send(context.Context, Message) error
}

type disabled struct{}

func (disabled) Send(context.Context, Message) error { return nil }

type smtpSender struct {
	addr string
	host string
	user string
	pass string
	from string
}

// NewFromEnv returns a no-op sender when SMTP_HOST is unset. This keeps local
// API-only tests self-contained without ever logging credential-bearing links.
func NewFromEnv() Sender {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		return disabled{}
	}
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	user := strings.TrimSpace(os.Getenv("SMTP_USER"))
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if from == "" {
		from = user
	}
	return &smtpSender{
		addr: host + ":" + port,
		host: host,
		user: user,
		pass: os.Getenv("SMTP_PASS"),
		from: from,
	}
}

func (s *smtpSender) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	from, err := mail.ParseAddress(s.from)
	if err != nil {
		return fmt.Errorf("invalid SMTP_FROM: %w", err)
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	subject := strings.NewReplacer("\r", "", "\n", "").Replace(msg.Subject)
	body := []byte(
		"From: " + from.String() + "\r\n" +
			"To: " + to.String() + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + msg.Text + "\r\n",
	)
	var auth smtp.Auth
	if s.user != "" {
		auth = smtp.PlainAuth("", s.user, s.pass, s.host)
	}
	if err := smtp.SendMail(s.addr, auth, from.Address, []string{to.Address}, body); err != nil {
		return fmt.Errorf("send SMTP message: %w", err)
	}
	return nil
}
