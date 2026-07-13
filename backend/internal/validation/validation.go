// Package validation scores an email address's deliverability before it's
// ever queued to send: syntax, MX records, disposable-domain filtering, and
// an optional live SMTP catch-all probe (off by default to protect sender
// reputation).
package validation

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Result is the deliverability assessment for one address.
type Result struct {
	Valid      bool
	Score      int // 0-100
	Reasons    []string
	HasMX      bool
	Disposable bool
	CatchAll   bool
}

// validThreshold is the minimum score to consider an address sendable.
const validThreshold = 60

type mxLookupFunc func(ctx context.Context, domain string) ([]*net.MX, error)

// Validator checks email deliverability signals.
type Validator struct {
	// SMTPProbe enables a live SMTP RCPT TO probe against the domain's MX
	// host. Off by default: probing arbitrary mail servers can itself harm
	// sender reputation, and outbound port 25 is often blocked anyway.
	SMTPProbe bool

	lookupMX mxLookupFunc
}

// Option configures a Validator.
type Option func(*Validator)

// WithMXLookup overrides the MX resolver — used in tests to avoid live DNS.
func WithMXLookup(fn mxLookupFunc) Option {
	return func(v *Validator) { v.lookupMX = fn }
}

// NewValidator builds a Validator. smtpProbe enables the live RCPT TO check.
func NewValidator(smtpProbe bool, opts ...Option) *Validator {
	v := &Validator{SMTPProbe: smtpProbe, lookupMX: net.DefaultResolver.LookupMX}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Validate scores addr's deliverability. It never errors for a bad address —
// a low Score / Valid=false is the answer. It only errors for unexpected
// internal failures (which do not occur in the current implementation, but
// the signature stays error-returning for forward compatibility).
func (v *Validator) Validate(ctx context.Context, addr string) (Result, error) {
	result := Result{}

	addr = strings.TrimSpace(strings.ToLower(addr))
	if _, err := mail.ParseAddress(addr); err != nil {
		result.Reasons = append(result.Reasons, "invalid syntax")
		return result, nil
	}
	result.Score += 20

	domain := domainOf(addr)
	if domain == "" {
		result.Reasons = append(result.Reasons, "no domain")
		return result, nil
	}

	if isDisposableDomain(domain) {
		result.Disposable = true
		result.Reasons = append(result.Reasons, "disposable domain")
		return result, nil // never sendable regardless of MX status
	}
	result.Score += 20

	mxRecords, err := v.lookupMX(ctx, domain)
	if err != nil || len(mxRecords) == 0 {
		result.Reasons = append(result.Reasons, "no MX records")
		return result, nil
	}
	result.HasMX = true
	result.Score += 40

	if v.SMTPProbe {
		accepted, catchAll, probeErr := v.smtpProbe(ctx, mxRecords[0].Host, addr)
		switch {
		case probeErr != nil:
			result.Reasons = append(result.Reasons, "smtp probe failed: "+probeErr.Error())
		case accepted && !catchAll:
			result.Score += 20
		case accepted && catchAll:
			result.CatchAll = true
			result.Score += 10
			result.Reasons = append(result.Reasons, "catch-all domain")
		default:
			result.Reasons = append(result.Reasons, "smtp probe rejected address")
		}
	} else {
		// Without a live probe we can't distinguish catch-all; award partial
		// credit for having valid MX records.
		result.Score += 10
	}

	if result.Score > 100 {
		result.Score = 100
	}
	result.Valid = result.Score >= validThreshold
	return result, nil
}

func domainOf(addr string) string {
	idx := strings.LastIndex(addr, "@")
	if idx < 0 {
		return ""
	}
	return addr[idx+1:]
}

// smtpProbe connects to the domain's MX host and issues RCPT TO for addr
// (to check acceptance) and a random unlikely-to-exist address at the same
// domain (to detect a catch-all server accepting everything).
func (v *Validator) smtpProbe(ctx context.Context, mxHost, addr string) (accepted, catchAll bool, err error) {
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", strings.TrimSuffix(mxHost, ".")+":25")
	if err != nil {
		return false, false, err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		return false, false, err
	}
	defer client.Close()

	if err := client.Hello("watchup.space"); err != nil {
		return false, false, err
	}
	if err := client.Mail("verify@watchup.space"); err != nil {
		return false, false, err
	}

	if err := client.Rcpt(addr); err != nil {
		return false, false, nil
	}
	accepted = true

	probe, err := randomLocalPart(12)
	if err != nil {
		return accepted, false, err
	}
	randomAddr := probe + "@" + domainOf(addr)
	if err := client.Rcpt(randomAddr); err == nil {
		catchAll = true
	}

	return accepted, catchAll, nil
}

func randomLocalPart(n int) (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", fmt.Errorf("validation: generate probe address: %w", err)
		}
		b[i] = letters[idx.Int64()]
	}
	return "watchup-probe-" + string(b), nil
}
