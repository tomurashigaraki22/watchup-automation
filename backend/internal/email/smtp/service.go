package smtp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
)

// ErrDailyLimitReached signals the campaign has hit its daily send cap.
var ErrDailyLimitReached = errors.New("smtp: daily send limit reached")

// ErrSuppressed signals the recipient is on the suppression list.
var ErrSuppressed = errors.New("smtp: recipient is suppressed")

// Service sends queued emails through Sender, enforcing the daily cap and
// suppression list, and persisting the outcome (Message-ID, SMTP response,
// status, sent_at).
type Service struct {
	sender     *Sender
	repo       *repository.Repositories
	publicBase string
	log        *zap.Logger
}

// NewService builds a send Service. publicBaseURL is used to build the
// open-tracking pixel and unsubscribe links embedded in each email.
func NewService(sender *Sender, repo *repository.Repositories, publicBaseURL string, log *zap.Logger) *Service {
	return &Service{sender: sender, repo: repo, publicBase: publicBaseURL, log: log}
}

// SendEmail sends email to contact under campaign's daily limit, unless
// contact is suppressed. If threadRoot is non-nil, the message is threaded
// onto it (In-Reply-To/References) — used for followups. On success, email
// is marked sent with its Message-ID and SMTP response; on failure, failed.
func (s *Service) SendEmail(ctx context.Context, email *models.Email, contact *models.Contact, campaign *models.OutreachCampaign, threadRoot *models.Email) error {
	_, suppressed, err := s.repo.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		return fmt.Errorf("smtp: check suppression: %w", err)
	}
	if suppressed {
		email.Status = models.EmailStatusFailed
		_ = s.repo.Emails.Update(ctx, email)
		s.log.Warn("smtp: skipped suppressed recipient", zap.String("email", contact.Email))
		return ErrSuppressed
	}

	startOfDay := time.Now().UTC().Truncate(24 * time.Hour)
	sentToday, err := s.repo.Emails.Count(ctx, "campaign_id = ? AND status = ? AND sent_at >= ?", campaign.ID, models.EmailStatusSent, startOfDay)
	if err != nil {
		return fmt.Errorf("smtp: count sent today: %w", err)
	}
	if int(sentToday) >= campaign.DailyLimit {
		s.log.Warn("smtp: daily limit reached", zap.Uint("campaign_id", campaign.ID), zap.Int("limit", campaign.DailyLimit))
		return ErrDailyLimitReached
	}

	unsubscribeURL := fmt.Sprintf("%s/api/v1/t/u/%d", s.publicBase, email.ID)
	openTrackingURL := fmt.Sprintf("%s/api/v1/t/o/%d", s.publicBase, email.ID)
	body := appendUnsubscribeFooter(email.Body, unsubscribeURL)

	msg := Message{
		To:              contact.Email,
		Subject:         email.Subject,
		BodyText:        body,
		OpenTrackingURL: openTrackingURL,
	}
	if threadRoot != nil && threadRoot.MessageID != "" {
		msg.InReplyTo = threadRoot.MessageID
		msg.References = threadRoot.MessageID
	}

	result, sendErr := s.sender.Send(ctx, msg)
	if sendErr != nil {
		email.Status = models.EmailStatusFailed
		email.SMTPResp = sendErr.Error()
		if err := s.repo.Emails.Update(ctx, email); err != nil {
			s.log.Error("smtp: failed to persist failed status", zap.Error(err))
		}
		s.log.Error("smtp: send failed", zap.String("to", contact.Email), zap.Error(sendErr))
		return fmt.Errorf("smtp: send: %w", sendErr)
	}

	now := time.Now().UTC()
	email.Status = models.EmailStatusSent
	email.MessageID = result.MessageID
	email.SMTPResp = result.Response
	email.SentAt = &now
	if err := s.repo.Emails.Update(ctx, email); err != nil {
		return fmt.Errorf("smtp: persist sent email: %w", err)
	}

	s.log.Info("smtp: email sent",
		zap.Uint("email_id", email.ID), zap.String("to", contact.Email), zap.String("message_id", result.MessageID))
	return nil
}

// appendUnsubscribeFooter adds the PRD-required unsubscribe notice: reply
// "unsubscribe" (handled by Phase 9's IMAP reply scan) or click a link.
func appendUnsubscribeFooter(body, unsubscribeURL string) string {
	return body + "\n\n---\nNot interested? Reply \"unsubscribe\" or click here to opt out: " + unsubscribeURL
}
