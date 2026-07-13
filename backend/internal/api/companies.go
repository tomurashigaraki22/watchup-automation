package api

import (
	"bytes"
	"io"

	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/discovery"
	"watchup/automation/internal/sources"
)

// listCompanies handles GET /api/v1/companies?status=&limit=&offset=
func (s *Server) listCompanies(c *fiber.Ctx) error {
	limit, offset := pagination(c)

	var (
		companies []models.Company
		err       error
	)
	if status := c.Query("status"); status != "" {
		companies, err = s.repos.Companies.ListWhere(c.Context(), limit, offset, "status = ?", status)
	} else {
		companies, err = s.repos.Companies.List(c.Context(), limit, offset)
	}
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list companies")
	}
	return c.JSON(fiber.Map{"companies": companies})
}

// companyDetail is the "Company Page" view: website, description, contacts
// (emails), AI generations (summary/generated emails), and email history.
type companyDetail struct {
	models.Company
	Contacts      []models.Contact      `json:"contacts"`
	Emails        []models.Email        `json:"emails"`
	AIGenerations []models.AIGeneration `json:"ai_generations"`
}

// getCompany handles GET /api/v1/companies/:id
func (s *Server) getCompany(c *fiber.Ctx) error {
	id, err := parseIDParam(c)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid company id")
	}

	company, err := s.repos.Companies.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "company not found")
	}

	contacts, err := s.repos.Contacts.ListWhere(c.Context(), 0, 0, "company_id = ?", company.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load contacts")
	}
	emails, err := s.repos.Emails.ListWhere(c.Context(), 0, 0, "company_id = ?", company.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load emails")
	}
	generations, err := s.repos.AIGenerations.ListWhere(c.Context(), 0, 0, "company_id = ?", company.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load ai generations")
	}

	return c.JSON(companyDetail{
		Company:       *company,
		Contacts:      contacts,
		Emails:        emails,
		AIGenerations: generations,
	})
}

// importCompaniesCSV handles POST /api/v1/companies/import — accepts a CSV
// file (multipart form field "file") or a raw CSV body, parses it via the
// CSV discovery source, and upserts new companies (deduped by website).
func (s *Server) importCompaniesCSV(c *fiber.Ctx) error {
	var reader io.Reader

	if fh, err := c.FormFile("file"); err == nil {
		f, openErr := fh.Open()
		if openErr != nil {
			return fiber.NewError(fiber.StatusBadRequest, "could not open uploaded file")
		}
		defer f.Close()
		reader = f
	} else {
		body := c.Body()
		if len(body) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, `provide a CSV file under form field "file" or a raw CSV body`)
		}
		reader = bytes.NewReader(body)
	}

	svc := discovery.NewService(s.repos, s.log)
	stats, err := svc.RunSource(c.Context(), sources.NewCSVSource(reader))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "csv import failed: "+err.Error())
	}
	s.audit(c, "companies.import", "company", 0, "csv import")

	return c.JSON(fiber.Map{
		"inserted": stats.Inserted,
		"skipped":  stats.Skipped,
		"errors":   stats.Errors,
	})
}
