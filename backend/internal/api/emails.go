package api

import (
	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
	emailsmtp "watchup/automation/internal/email/smtp"
	"watchup/automation/internal/queue"
)

// listEmails handles GET /api/v1/emails?status=&campaign_id=&company_id=&limit=&offset=
func (s *Server) listEmails(c *fiber.Ctx) error {
	limit, offset := pagination(c)

	query, args := "", []any{}
	if status := c.Query("status"); status != "" {
		query, args = appendClause(query, args, "status = ?", status)
	}
	if campaignID := c.QueryInt("campaign_id", 0); campaignID > 0 {
		query, args = appendClause(query, args, "campaign_id = ?", campaignID)
	}
	if companyID := c.QueryInt("company_id", 0); companyID > 0 {
		query, args = appendClause(query, args, "company_id = ?", companyID)
	}

	var (
		emails []models.Email
		err    error
	)
	if query != "" {
		emails, err = s.repos.Emails.ListWhere(c.Context(), limit, offset, query, args...)
	} else {
		emails, err = s.repos.Emails.List(c.Context(), limit, offset)
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list emails")
	}
	return c.JSON(fiber.Map{"emails": emails})
}

func appendClause(query string, args []any, clause string, arg any) (string, []any) {
	if query != "" {
		query += " AND "
	}
	return query + clause, append(args, arg)
}

// emailPreview is the "Email Preview" view: subject + rendered HTML/plaintext.
type emailPreview struct {
	models.Email
	BodyHTML string `json:"body_html"`
}

// getEmail handles GET /api/v1/emails/:id
func (s *Server) getEmail(c *fiber.Ctx) error {
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	email, err := s.repos.Emails.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "email not found")
	}
	return c.JSON(emailPreview{Email: *email, BodyHTML: emailsmtp.RenderPreviewHTML(email.Body)})
}

type emailUpdateRequest struct {
	Subject *string `json:"subject"`
	Body    *string `json:"body"`
}

// updateEmail handles PATCH /api/v1/emails/:id — allowed only while draft,
// per the PRD's "AI generates, human approves, then sends" manual mode.
func (s *Server) updateEmail(c *fiber.Ctx) error {
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	email, err := s.repos.Emails.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "email not found")
	}
	if email.Status != models.EmailStatusDraft {
		return fiber.NewError(fiber.StatusConflict, "only draft emails can be edited")
	}

	var req emailUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Subject != nil {
		email.Subject = *req.Subject
	}
	if req.Body != nil {
		email.Body = *req.Body
		email.BodyText = *req.Body
	}
	if err := s.repos.Emails.Update(c.Context(), email); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update email")
	}
	s.audit(c, "email.edit", "email", email.ID, "")
	return c.JSON(email)
}

// sendEmail handles POST /api/v1/emails/:id/send — the manual-approval
// trigger: enqueues the same JobSend the automatic pipeline uses, so daily
// limits/suppression/pacing are enforced identically either way.
func (s *Server) sendEmail(c *fiber.Ctx) error {
	if s.queue == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "send queue is not available")
	}
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	email, err := s.repos.Emails.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "email not found")
	}
	if email.Status != models.EmailStatusDraft {
		return fiber.NewError(fiber.StatusConflict, "only draft emails can be sent")
	}

	if err := s.queue.Enqueue(c.Context(), queue.Job{Type: queue.JobSend, EmailID: email.ID, CampaignID: email.CampaignID}); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to enqueue send")
	}
	s.audit(c, "email.send_triggered", "email", email.ID, "")
	return c.JSON(fiber.Map{"status": "queued"})
}
