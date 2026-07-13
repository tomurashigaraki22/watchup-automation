// Package workers dispatches queued pipeline jobs to the phase services
// built in earlier phases (discovery, crawler, validation, ai, smtp),
// chaining each company through crawl -> validate -> analyze -> generate ->
// send, and scheduling/sending followups.
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/ai"
	"watchup/automation/internal/crawler"
	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/discovery"
	emailsmtp "watchup/automation/internal/email/smtp"
	"watchup/automation/internal/queue"
	"watchup/automation/internal/sources"
	"watchup/automation/internal/validation"
)

// ReplyScanner detects replies/bounces/unsubscribes against sent email. The
// concrete implementation is internal/email/imap.Scanner (Phase 9); this
// interface keeps workers decoupled from the IMAP package.
type ReplyScanner interface {
	Scan(ctx context.Context) error
}

// Handlers bundles the services each job type dispatches to.
type Handlers struct {
	Repo       *repository.Repositories
	Discovery  *discovery.Service
	Registry   *sources.Registry
	Crawler    *crawler.Service
	Validation *validation.Service
	AI         *ai.Service
	SMTP       *emailsmtp.Service
	Queue      *queue.Queue
	Replies    ReplyScanner
	Log        *zap.Logger
}

// Process dispatches one job to its handler. A returned error is audit-
// logged by the caller (Worker); it does not crash the worker loop.
func (h *Handlers) Process(ctx context.Context, job queue.Job) error {
	switch job.Type {
	case queue.JobDiscover:
		return h.handleDiscover(ctx)
	case queue.JobCrawl:
		return h.handleCrawl(ctx, job)
	case queue.JobValidate:
		return h.handleValidate(ctx, job)
	case queue.JobAnalyze:
		return h.handleAnalyze(ctx, job)
	case queue.JobGenerate:
		return h.handleGenerate(ctx, job)
	case queue.JobSend:
		return h.handleSend(ctx, job)
	case queue.JobFollowup:
		return h.handleFollowup(ctx, job)
	case queue.JobReplyScan:
		return h.handleReplyScan(ctx)
	default:
		return fmt.Errorf("workers: unknown job type %q", job.Type)
	}
}

func (h *Handlers) handleDiscover(ctx context.Context) error {
	stats := h.Discovery.RunRegistry(ctx, h.Registry)
	h.Log.Info("workers: discovery run complete",
		zap.Int("inserted", stats.Inserted), zap.Int("skipped", stats.Skipped), zap.Int("errors", stats.Errors))
	return nil
}

func (h *Handlers) handleCrawl(ctx context.Context, job queue.Job) error {
	company, err := h.Repo.Companies.GetByID(ctx, job.CompanyID)
	if err != nil {
		return fmt.Errorf("workers: load company %d: %w", job.CompanyID, err)
	}
	if _, err := h.Crawler.CrawlCompany(ctx, company); err != nil {
		return fmt.Errorf("workers: crawl company %d: %w", job.CompanyID, err)
	}
	return h.Queue.Enqueue(ctx, queue.Job{Type: queue.JobValidate, CompanyID: company.ID})
}

func (h *Handlers) handleValidate(ctx context.Context, job queue.Job) error {
	company, err := h.Repo.Companies.GetByID(ctx, job.CompanyID)
	if err != nil {
		return fmt.Errorf("workers: load company %d: %w", job.CompanyID, err)
	}

	contacts, err := h.Repo.Contacts.ListWhere(ctx, 0, 0, "company_id = ? AND verified = ?", company.ID, false)
	if err != nil {
		return fmt.Errorf("workers: list unverified contacts for company %d: %w", company.ID, err)
	}
	for i := range contacts {
		if _, err := h.Validation.ValidateContact(ctx, &contacts[i]); err != nil {
			h.Log.Error("workers: validate contact failed",
				zap.Uint("contact_id", contacts[i].ID), zap.Error(err))
		}
	}

	company.Status = models.CompanyStatusValidated
	if err := h.Repo.Companies.Update(ctx, company); err != nil {
		return fmt.Errorf("workers: mark company %d validated: %w", company.ID, err)
	}
	return h.Queue.Enqueue(ctx, queue.Job{Type: queue.JobAnalyze, CompanyID: company.ID})
}

func (h *Handlers) handleAnalyze(ctx context.Context, job queue.Job) error {
	company, err := h.Repo.Companies.GetByID(ctx, job.CompanyID)
	if err != nil {
		return fmt.Errorf("workers: load company %d: %w", job.CompanyID, err)
	}
	analysis, err := h.AI.AnalyzeCompany(ctx, company)
	if err != nil {
		return fmt.Errorf("workers: analyze company %d: %w", job.CompanyID, err)
	}
	return h.generateAndMaybeSend(ctx, company, analysis)
}

// handleGenerate re-triggers email generation for a company outside the
// normal analyze->generate chain (e.g. a future manual "regenerate" action).
// It recovers the most recent analysis from the ai_generations audit table
// since Analysis isn't otherwise persisted in full on the company row.
func (h *Handlers) handleGenerate(ctx context.Context, job queue.Job) error {
	company, err := h.Repo.Companies.GetByID(ctx, job.CompanyID)
	if err != nil {
		return fmt.Errorf("workers: load company %d: %w", job.CompanyID, err)
	}
	analysis, err := h.mostRecentAnalysis(ctx, company.ID)
	if err != nil {
		return fmt.Errorf("workers: load prior analysis for company %d: %w", company.ID, err)
	}
	return h.generateAndMaybeSend(ctx, company, analysis)
}

func (h *Handlers) mostRecentAnalysis(ctx context.Context, companyID uint) (ai.Analysis, error) {
	gens, err := h.Repo.AIGenerations.ListWhere(ctx, 1, 0, "company_id = ? AND kind = ?", companyID, models.AIGenerationKindAnalysis)
	if err != nil {
		return ai.Analysis{}, err
	}
	if len(gens) == 0 {
		return ai.Analysis{}, nil
	}
	var analysis ai.Analysis
	_ = json.Unmarshal([]byte(gens[0].Response), &analysis)
	return analysis, nil
}

func (h *Handlers) generateAndMaybeSend(ctx context.Context, company *models.Company, analysis ai.Analysis) error {
	contact, found, err := bestVerifiedContact(ctx, h.Repo, company.ID)
	if err != nil {
		return fmt.Errorf("workers: find best contact for company %d: %w", company.ID, err)
	}
	if !found {
		h.Log.Info("workers: no verified contact yet, skipping generation", zap.Uint("company_id", company.ID))
		return nil
	}

	campaign, found, err := firstActiveCampaign(ctx, h.Repo)
	if err != nil {
		return fmt.Errorf("workers: find active campaign: %w", err)
	}
	if !found {
		h.Log.Warn("workers: no active campaign, skipping generation", zap.Uint("company_id", company.ID))
		return nil
	}

	email, err := h.AI.GenerateEmailForContact(ctx, company, contact, analysis, campaign.ID)
	if err != nil {
		return fmt.Errorf("workers: generate email for company %d: %w", company.ID, err)
	}

	if campaign.SendMode != models.SendModeAutomatic {
		h.Log.Info("workers: email drafted, awaiting manual approval",
			zap.Uint("email_id", email.ID), zap.String("campaign_send_mode", campaign.SendMode))
		return nil
	}
	return h.Queue.Enqueue(ctx, queue.Job{Type: queue.JobSend, EmailID: email.ID, CampaignID: campaign.ID})
}

func (h *Handlers) handleSend(ctx context.Context, job queue.Job) error {
	email, err := h.Repo.Emails.GetByID(ctx, job.EmailID)
	if err != nil {
		return fmt.Errorf("workers: load email %d: %w", job.EmailID, err)
	}
	contact, err := h.Repo.Contacts.GetByID(ctx, email.ContactID)
	if err != nil {
		return fmt.Errorf("workers: load contact %d: %w", email.ContactID, err)
	}
	campaign, err := h.Repo.Campaigns.GetByID(ctx, email.CampaignID)
	if err != nil {
		return fmt.Errorf("workers: load campaign %d: %w", email.CampaignID, err)
	}

	if err := h.SMTP.SendEmail(ctx, email, contact, campaign, nil); err != nil {
		if errors.Is(err, emailsmtp.ErrDailyLimitReached) || errors.Is(err, emailsmtp.ErrSuppressed) {
			// Not a failure worth crashing the job over: daily-limit emails
			// stay queued for the next scheduler pass; suppressed emails stop here.
			h.Log.Info("workers: send skipped", zap.Uint("email_id", email.ID), zap.Error(err))
			return nil
		}
		return fmt.Errorf("workers: send email %d: %w", email.ID, err)
	}

	return h.scheduleFollowups(ctx, email)
}

// scheduleFollowups creates the Day 5 / 12 / 20 follow-up rows after a
// successful send, per the PRD's default sequence.
func (h *Handlers) scheduleFollowups(ctx context.Context, email *models.Email) error {
	if email.SentAt == nil {
		return nil
	}
	dayOffsets := [3]int{5, 12, 20}
	for i, days := range dayOffsets {
		f := &models.Followup{
			EmailID:     email.ID,
			Sequence:    i + 1,
			ScheduledAt: email.SentAt.Add(time.Duration(days) * 24 * time.Hour),
		}
		if err := h.Repo.Followups.Create(ctx, f); err != nil {
			return fmt.Errorf("workers: schedule followup %d for email %d: %w", i+1, email.ID, err)
		}
	}
	return nil
}

func (h *Handlers) handleFollowup(ctx context.Context, job queue.Job) error {
	followup, err := h.Repo.Followups.GetByID(ctx, job.FollowupID)
	if err != nil {
		return fmt.Errorf("workers: load followup %d: %w", job.FollowupID, err)
	}
	if followup.Sent || followup.Canceled {
		return nil
	}

	original, err := h.Repo.Emails.GetByID(ctx, followup.EmailID)
	if err != nil {
		return fmt.Errorf("workers: load original email %d: %w", followup.EmailID, err)
	}
	if original.Replied {
		return h.cancelFollowup(ctx, followup)
	}

	contact, err := h.Repo.Contacts.GetByID(ctx, original.ContactID)
	if err != nil {
		return fmt.Errorf("workers: load contact %d: %w", original.ContactID, err)
	}
	_, suppressed, err := h.Repo.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		return fmt.Errorf("workers: check suppression for %s: %w", contact.Email, err)
	}
	if suppressed {
		return h.cancelFollowup(ctx, followup)
	}

	company, err := h.Repo.Companies.GetByID(ctx, original.CompanyID)
	if err != nil {
		return fmt.Errorf("workers: load company %d: %w", original.CompanyID, err)
	}

	generated, err := h.AI.GenerateFollowup(ctx, company, original.Subject, followup.Sequence)
	if err != nil {
		return fmt.Errorf("workers: generate followup content: %w", err)
	}

	body := ai.ComposeEmailBody(generated)
	newEmail := &models.Email{
		CampaignID: original.CampaignID,
		CompanyID:  original.CompanyID,
		ContactID:  original.ContactID,
		Subject:    generated.Subject,
		Body:       body,
		BodyText:   body,
		Status:     models.EmailStatusDraft,
	}
	if err := h.Repo.Emails.Create(ctx, newEmail); err != nil {
		return fmt.Errorf("workers: persist followup email: %w", err)
	}

	campaign, err := h.Repo.Campaigns.GetByID(ctx, original.CampaignID)
	if err != nil {
		return fmt.Errorf("workers: load campaign %d: %w", original.CampaignID, err)
	}

	if err := h.SMTP.SendEmail(ctx, newEmail, contact, campaign, original); err != nil {
		if errors.Is(err, emailsmtp.ErrDailyLimitReached) {
			// Leave followup unsent/unscheduled-cancel; next scheduler pass retries.
			h.Log.Info("workers: followup send deferred (daily limit)", zap.Uint("followup_id", followup.ID))
			return nil
		}
		if errors.Is(err, emailsmtp.ErrSuppressed) {
			return h.cancelFollowup(ctx, followup)
		}
		return fmt.Errorf("workers: send followup: %w", err)
	}

	now := time.Now().UTC()
	followup.Sent = true
	followup.SentAt = &now
	return h.Repo.Followups.Update(ctx, followup)
}

func (h *Handlers) cancelFollowup(ctx context.Context, followup *models.Followup) error {
	followup.Canceled = true
	if err := h.Repo.Followups.Update(ctx, followup); err != nil {
		return fmt.Errorf("workers: cancel followup %d: %w", followup.ID, err)
	}
	return nil
}

func (h *Handlers) handleReplyScan(ctx context.Context) error {
	if h.Replies == nil {
		return nil
	}
	if err := h.Replies.Scan(ctx); err != nil {
		return fmt.Errorf("workers: reply scan: %w", err)
	}
	return nil
}

// bestVerifiedContact picks the highest-priority (lowest Priority number)
// verified contact for a company.
func bestVerifiedContact(ctx context.Context, repo *repository.Repositories, companyID uint) (*models.Contact, bool, error) {
	contacts, err := repo.Contacts.ListWhere(ctx, 0, 0, "company_id = ? AND verified = ?", companyID, true)
	if err != nil {
		return nil, false, err
	}
	if len(contacts) == 0 {
		return nil, false, nil
	}
	best := contacts[0]
	for _, c := range contacts[1:] {
		if c.Priority < best.Priority {
			best = c
		}
	}
	return &best, true, nil
}

// firstActiveCampaign picks the campaign new outreach is attached to.
// Companies aren't tied to a specific campaign in the PRD schema, so this
// build attaches new emails to the first active campaign.
func firstActiveCampaign(ctx context.Context, repo *repository.Repositories) (*models.OutreachCampaign, bool, error) {
	campaigns, err := repo.Campaigns.ListWhere(ctx, 1, 0, "status = ?", models.CampaignStatusActive)
	if err != nil {
		return nil, false, err
	}
	if len(campaigns) == 0 {
		return nil, false, nil
	}
	return &campaigns[0], true, nil
}
