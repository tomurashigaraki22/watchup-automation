package api

import (
	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
)

type searchResults struct {
	Companies []models.Company          `json:"companies"`
	Emails    []models.Email            `json:"emails"`
	Campaigns []models.OutreachCampaign `json:"campaigns"`
}

// search handles GET /api/v1/search?q=&status= — searches across company
// name/website, email subject, and campaign name; an optional status filter
// narrows companies and emails by their status field.
func (s *Server) search(c *fiber.Ctx) error {
	q := c.Query("q")
	status := c.Query("status")
	if q == "" && status == "" {
		return fiber.NewError(fiber.StatusBadRequest, "provide ?q= and/or ?status=")
	}
	like := "%" + q + "%"

	companyQuery, companyArgs := "", []any{}
	if q != "" {
		companyQuery = "(LOWER(name) LIKE LOWER(?) OR LOWER(website) LIKE LOWER(?))"
		companyArgs = append(companyArgs, like, like)
	}
	if status != "" {
		companyQuery, companyArgs = appendClause(companyQuery, companyArgs, "status = ?", status)
	}
	companies, err := s.repos.Companies.ListWhere(c.Context(), 25, 0, companyQuery, companyArgs...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "search failed")
	}

	emailQuery, emailArgs := "", []any{}
	if q != "" {
		emailQuery, emailArgs = appendClause(emailQuery, emailArgs, "LOWER(subject) LIKE LOWER(?)", like)
	}
	if status != "" {
		emailQuery, emailArgs = appendClause(emailQuery, emailArgs, "status = ?", status)
	}
	emails, err := s.repos.Emails.ListWhere(c.Context(), 25, 0, emailQuery, emailArgs...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "search failed")
	}

	campaignQuery, campaignArgs := "", []any{}
	if q != "" {
		campaignQuery, campaignArgs = appendClause(campaignQuery, campaignArgs, "LOWER(name) LIKE LOWER(?)", like)
	}
	if status != "" {
		campaignQuery, campaignArgs = appendClause(campaignQuery, campaignArgs, "status = ?", status)
	}
	campaigns, err := s.repos.Campaigns.ListWhere(c.Context(), 25, 0, campaignQuery, campaignArgs...)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "search failed")
	}

	return c.JSON(searchResults{Companies: companies, Emails: emails, Campaigns: campaigns})
}
