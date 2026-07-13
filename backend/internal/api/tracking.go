package api

import (
	"encoding/base64"

	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
)

// transparentPixelGIF is a minimal 1x1 transparent GIF served for open tracking.
var transparentPixelGIF = mustDecodeBase64("R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==")

func mustDecodeBase64(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// trackOpen handles GET /api/v1/t/o/:id — an invisible pixel embedded in
// each sent email's HTML body. Loading it marks the email opened.
func (s *Server) trackOpen(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err == nil && id > 0 {
		if email, getErr := s.repos.Emails.GetByID(c.Context(), uint(id)); getErr == nil && !email.Opened {
			email.Opened = true
			if updErr := s.repos.Emails.Update(c.Context(), email); updErr != nil {
				s.log.Warn("tracking: failed to mark email opened")
			}
		}
	}
	c.Set("Content-Type", "image/gif")
	c.Set("Cache-Control", "no-store")
	return c.Send(transparentPixelGIF)
}

// trackUnsubscribe handles GET /api/v1/t/u/:id — the click-based unsubscribe
// link in every outreach email's footer. Suppresses the contact so they're
// never emailed again.
func (s *Server) trackUnsubscribe(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid tracking id")
	}

	email, err := s.repos.Emails.GetByID(c.Context(), uint(id))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "email not found")
	}

	if !email.Clicked {
		email.Clicked = true
		_ = s.repos.Emails.Update(c.Context(), email)
	}

	contact, err := s.repos.Contacts.GetByID(c.Context(), email.ContactID)
	if err == nil {
		_, exists, _ := s.repos.Suppressions.First(c.Context(), "email = ?", contact.Email)
		if !exists {
			if err := s.repos.Suppressions.Create(c.Context(), &models.Suppression{
				Email:  contact.Email,
				Reason: models.SuppressionReasonUnsubscribe,
			}); err != nil {
				s.log.Error("tracking: failed to record suppression")
			}
		}
	}

	return c.SendString("You've been unsubscribed and won't receive further emails from us.")
}
