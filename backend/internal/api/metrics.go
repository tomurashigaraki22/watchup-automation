package api

import (
	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
)

type metricsResponse struct {
	CompaniesDiscovered int64                 `json:"companies_discovered"`
	CompaniesCrawled    int64                 `json:"companies_crawled"`
	CompaniesAnalyzed   int64                 `json:"companies_analyzed"`
	EmailsExtracted     int64                 `json:"emails_extracted"`
	EmailsVerified      int64                 `json:"emails_verified"`
	EmailsSent          int64                 `json:"emails_sent"`
	Replies             int64                 `json:"replies"`
	OpenRate            float64               `json:"open_rate"`
	BounceRate          float64               `json:"bounce_rate"`
	FollowupsSent       int64                 `json:"followups_sent"`
	FollowupsPending    int64                 `json:"followups_pending"`
	Campaigns           []campaignPerformance `json:"campaigns"`
}

type campaignPerformance struct {
	CampaignID uint    `json:"campaign_id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Sent       int64   `json:"sent"`
	Replies    int64   `json:"replies"`
	OpenRate   float64 `json:"open_rate"`
}

// getMetrics handles GET /api/v1/metrics — the dashboard's summary numbers.
// Company "reached at least X" counts use status ordering since the schema
// tracks one current status per company, not per-stage timestamps.
func (s *Server) getMetrics(c *fiber.Ctx) error {
	ctx := c.Context()

	companiesDiscovered, err := s.repos.Companies.Count(ctx, "")
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	companiesCrawled, err := s.repos.Companies.Count(ctx, "status IN ?",
		[]string{models.CompanyStatusCrawled, models.CompanyStatusValidated, models.CompanyStatusAnalyzed})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	companiesAnalyzed, err := s.repos.Companies.Count(ctx, "status = ?", models.CompanyStatusAnalyzed)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}

	emailsExtracted, err := s.repos.Contacts.Count(ctx, "")
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	emailsVerified, err := s.repos.Contacts.Count(ctx, "verified = ?", true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}

	emailsSent, err := s.repos.Emails.Count(ctx, "status = ?", models.EmailStatusSent)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	replies, err := s.repos.Emails.Count(ctx, "replied = ?", true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	opened, err := s.repos.Emails.Count(ctx, "opened = ?", true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	bounced, err := s.repos.Emails.Count(ctx, "bounced = ?", true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}

	followupsSent, err := s.repos.Followups.Count(ctx, "sent = ?", true)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	followupsPending, err := s.repos.Followups.Count(ctx, "sent = ? AND canceled = ?", false, false)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}

	campaigns, err := s.repos.Campaigns.List(ctx, 0, 0)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to compute metrics")
	}
	perf := make([]campaignPerformance, 0, len(campaigns))
	for _, camp := range campaigns {
		sent, _ := s.repos.Emails.Count(ctx, "campaign_id = ? AND status = ?", camp.ID, models.EmailStatusSent)
		campReplies, _ := s.repos.Emails.Count(ctx, "campaign_id = ? AND replied = ?", camp.ID, true)
		campOpened, _ := s.repos.Emails.Count(ctx, "campaign_id = ? AND opened = ?", camp.ID, true)
		perf = append(perf, campaignPerformance{
			CampaignID: camp.ID, Name: camp.Name, Status: camp.Status,
			Sent: sent, Replies: campReplies, OpenRate: rate(campOpened, sent),
		})
	}

	return c.JSON(metricsResponse{
		CompaniesDiscovered: companiesDiscovered,
		CompaniesCrawled:    companiesCrawled,
		CompaniesAnalyzed:   companiesAnalyzed,
		EmailsExtracted:     emailsExtracted,
		EmailsVerified:      emailsVerified,
		EmailsSent:          emailsSent,
		Replies:             replies,
		OpenRate:            rate(opened, emailsSent),
		BounceRate:          rate(bounced, emailsSent),
		FollowupsSent:       followupsSent,
		FollowupsPending:    followupsPending,
		Campaigns:           perf,
	})
}

func rate(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
