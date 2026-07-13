// Package smtp sends outreach email through Hostinger SMTP: retries with
// exponential backoff, Message-ID generation, threading headers for
// followups, an unsubscribe footer, and open-tracking pixel embedding.
package smtp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"watchup/automation/internal/config"
)

// Result captures what happened when a send succeeded.
type Result struct {
	MessageID string
	Response  string
}

// Sender delivers Messages with retry.
type Sender struct {
	from      string
	domain    string
	transport transport
}

// Option configures a Sender.
type Option func(*Sender)

// WithTransport overrides the delivery transport — used in tests to avoid a
// live SMTP connection.
func WithTransport(t transport) Option {
	return func(s *Sender) { s.transport = t }
}

// NewSender builds a Sender using SMTP settings from cfg. The "From" address
// is cfg.SenderEmail (e.g. partnership@watchup.space), which may differ from
// the SMTP_USERNAME mailbox that actually authenticates.
func NewSender(cfg *config.Config, opts ...Option) *Sender {
	domain := cfg.SenderEmail
	if idx := strings.LastIndex(domain, "@"); idx >= 0 {
		domain = domain[idx+1:]
	}
	s := &Sender{
		from:      cfg.SenderEmail,
		domain:    domain,
		transport: newTLSTransport(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// From returns the configured "From" address.
func (s *Sender) From() string { return s.from }

// Send delivers msg with up to 3 attempts and exponential backoff, per the
// PRD's SMTP retry policy.
func (s *Sender) Send(ctx context.Context, msg Message) (Result, error) {
	if msg.From == "" {
		msg.From = s.from
	}
	if msg.MessageID == "" {
		msg.MessageID = generateMessageID(s.domain)
	}

	raw, err := buildRawMessage(msg)
	if err != nil {
		return Result{}, fmt.Errorf("smtp: build message: %w", err)
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err := s.transport.Send(msg.From, msg.To, raw); err != nil {
			lastErr = err
			if attempt < maxAttempts {
				backoff := time.Duration(attempt*attempt) * time.Second
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return Result{}, ctx.Err()
				}
			}
			continue
		}
		return Result{MessageID: msg.MessageID, Response: "250 OK"}, nil
	}
	return Result{}, fmt.Errorf("smtp: send failed after %d attempts: %w", maxAttempts, lastErr)
}
