package imap

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
)

// lookback bounds how far back each scan searches — recent-only keeps scans
// fast and is sufficient since we re-scan on every scheduler tick.
const lookback = 30 * 24 * time.Hour

// Scanner implements workers.ReplyScanner: it fetches recent inbox messages
// and matches them against sent email to detect replies, bounces, and
// unsubscribe requests.
type Scanner struct {
	fetcher Fetcher
	repo    *repository.Repositories
	log     *zap.Logger
}

// NewScanner builds a Scanner.
func NewScanner(fetcher Fetcher, repo *repository.Repositories, log *zap.Logger) *Scanner {
	return &Scanner{fetcher: fetcher, repo: repo, log: log}
}

// Scan fetches recent inbox messages and processes each one.
func (s *Scanner) Scan(ctx context.Context) error {
	messages, err := s.fetcher.FetchRecent(ctx, time.Now().Add(-lookback))
	if err != nil {
		return err
	}
	for _, m := range messages {
		s.processMessage(ctx, m)
	}
	return nil
}

func (s *Scanner) processMessage(ctx context.Context, m Message) {
	if isBounce(m) {
		s.handleBounce(ctx, m)
		return
	}

	email, found := s.matchToSentEmail(ctx, m)
	if !found {
		return
	}

	if isUnsubscribeReply(m) {
		s.suppress(ctx, email, models.SuppressionReasonUnsubscribe)
	}

	if email.Replied {
		return
	}
	email.Replied = true
	if err := s.repo.Emails.Update(ctx, email); err != nil {
		s.log.Error("imap: failed to mark email replied", zap.Uint("email_id", email.ID), zap.Error(err))
		return
	}
	s.log.Info("imap: reply detected", zap.Uint("email_id", email.ID), zap.String("from", m.From))
	s.cancelPendingFollowups(ctx, email.ID)
}

// matchToSentEmail finds the sent Email a reply/bounce refers to, by
// checking In-Reply-To then each entry in References against Message-ID.
func (s *Scanner) matchToSentEmail(ctx context.Context, m Message) (*models.Email, bool) {
	candidates := make([]string, 0, len(m.References)+1)
	if m.InReplyTo != "" {
		candidates = append(candidates, m.InReplyTo)
	}
	candidates = append(candidates, m.References...)

	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		email, found, err := s.repo.Emails.First(ctx, "message_id = ?", id)
		if err != nil {
			s.log.Error("imap: lookup email by message id failed", zap.String("message_id", id), zap.Error(err))
			continue
		}
		if found {
			return email, true
		}
	}
	return nil, false
}

func (s *Scanner) cancelPendingFollowups(ctx context.Context, emailID uint) {
	followups, err := s.repo.Followups.ListWhere(ctx, 0, 0, "email_id = ? AND sent = ? AND canceled = ?", emailID, false, false)
	if err != nil {
		s.log.Error("imap: list pending followups failed", zap.Uint("email_id", emailID), zap.Error(err))
		return
	}
	for i := range followups {
		followups[i].Canceled = true
		if err := s.repo.Followups.Update(ctx, &followups[i]); err != nil {
			s.log.Error("imap: cancel followup failed", zap.Uint("followup_id", followups[i].ID), zap.Error(err))
		}
	}
}

// --- bounce handling ---

var bounceSubjectPatterns = []string{
	"undelivered mail returned to sender",
	"delivery status notification (failure)",
	"mail delivery failed",
	"undeliverable",
	"failure notice",
}

var bounceFromPatterns = []string{"mailer-daemon", "postmaster", "mail delivery subsystem"}

func isBounce(m Message) bool {
	subject := strings.ToLower(m.Subject)
	for _, p := range bounceSubjectPatterns {
		if strings.Contains(subject, p) {
			return true
		}
	}
	from := strings.ToLower(m.From)
	for _, p := range bounceFromPatterns {
		if strings.Contains(from, p) {
			return true
		}
	}
	return false
}

var hardStatusCodeRegex = regexp.MustCompile(`\b5\.\d\.\d\b`)
var softStatusCodeRegex = regexp.MustCompile(`\b4\.\d\.\d\b`)

// isHardBounce inspects the bounce body's SMTP status code: 5.x.x is
// permanent (hard), 4.x.x is transient (soft). Defaults to hard when no
// status code is found, since an unrecognized bounce is safer to suppress
// than to keep retrying against.
func isHardBounce(m Message) bool {
	if hardStatusCodeRegex.MatchString(m.BodyText) {
		return true
	}
	if softStatusCodeRegex.MatchString(m.BodyText) {
		return false
	}
	return true
}

func (s *Scanner) handleBounce(ctx context.Context, m Message) {
	email, found := s.matchToSentEmail(ctx, m)
	if !found {
		s.log.Warn("imap: bounce received but could not match to a sent email", zap.String("subject", m.Subject))
		return
	}

	email.Bounced = true
	if err := s.repo.Emails.Update(ctx, email); err != nil {
		s.log.Error("imap: failed to mark email bounced", zap.Uint("email_id", email.ID), zap.Error(err))
		return
	}

	hard := isHardBounce(m)
	s.log.Info("imap: bounce detected", zap.Uint("email_id", email.ID), zap.Bool("hard", hard))

	if hard {
		// Hard bounce: suppress and never retry.
		s.suppress(ctx, email, models.SuppressionReasonHardBounce)
		s.cancelPendingFollowups(ctx, email.ID)
	}
	// Soft bounce: marked bounced but not suppressed — a future send attempt
	// (e.g. a followup) may still succeed.
}

func (s *Scanner) suppress(ctx context.Context, email *models.Email, reason string) {
	contact, err := s.repo.Contacts.GetByID(ctx, email.ContactID)
	if err != nil {
		s.log.Error("imap: load contact for suppression failed", zap.Uint("contact_id", email.ContactID), zap.Error(err))
		return
	}
	_, exists, err := s.repo.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		s.log.Error("imap: check existing suppression failed", zap.String("email", contact.Email), zap.Error(err))
		return
	}
	if exists {
		return
	}
	if err := s.repo.Suppressions.Create(ctx, &models.Suppression{Email: contact.Email, Reason: reason}); err != nil {
		s.log.Error("imap: create suppression failed", zap.String("email", contact.Email), zap.Error(err))
		return
	}
	s.log.Info("imap: contact suppressed", zap.String("email", contact.Email), zap.String("reason", reason))
}

// isUnsubscribeReply reports whether a genuine reply's body asks to unsubscribe.
func isUnsubscribeReply(m Message) bool {
	return strings.Contains(strings.ToLower(m.BodyText), "unsubscribe")
}
