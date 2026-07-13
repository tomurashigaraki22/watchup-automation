package crawler

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
)

// Service crawls a company's website and persists the results: contacts
// (deduped per company+email) and a consolidated description, then marks
// the company status=crawled.
type Service struct {
	repo *repository.Repositories
	log  *zap.Logger
}

// NewService builds a crawler Service.
func NewService(repo *repository.Repositories, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// CrawlCompany crawls company.Website, persists extracted contacts, updates
// the company's description, and advances its status to "crawled".
func (s *Service) CrawlCompany(ctx context.Context, company *models.Company) (*Result, error) {
	result, err := Crawl(ctx, company.Website)
	if err != nil {
		return nil, fmt.Errorf("crawler: crawl %s: %w", company.Website, err)
	}

	inserted := 0
	for _, e := range result.Emails {
		_, found, err := s.repo.Contacts.First(ctx, "company_id = ? AND email = ?", company.ID, e.Address)
		if err != nil {
			s.log.Error("crawler: contact lookup failed",
				zap.Uint("company_id", company.ID), zap.String("email", e.Address), zap.Error(err))
			continue
		}
		if found {
			continue
		}
		contact := &models.Contact{
			CompanyID: company.ID,
			Email:     e.Address,
			Source:    e.Source,
			Priority:  e.Priority,
		}
		if err := s.repo.Contacts.Create(ctx, contact); err != nil {
			s.log.Error("crawler: contact insert failed",
				zap.Uint("company_id", company.ID), zap.String("email", e.Address), zap.Error(err))
			continue
		}
		inserted++
	}

	if desc := composeDescription(result); desc != "" {
		company.Description = desc
	}
	company.Status = models.CompanyStatusCrawled
	if err := s.repo.Companies.Update(ctx, company); err != nil {
		return result, fmt.Errorf("crawler: update company: %w", err)
	}

	s.log.Info("crawler: company crawled",
		zap.Uint("company_id", company.ID), zap.String("website", company.Website),
		zap.Int("pages", result.PagesCrawled), zap.Int("emails_found", len(result.Emails)),
		zap.Int("contacts_inserted", inserted))

	return result, nil
}

// composeDescription folds the crawl's structured findings (which have no
// dedicated DB columns in the PRD schema) into one readable description
// string, so downstream AI analysis still has that context to work with.
func composeDescription(r *Result) string {
	var b strings.Builder
	if r.Description != "" {
		b.WriteString(r.Description)
	}
	if len(r.Products) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("Products/pages: " + strings.Join(r.Products, ", ") + ".")
	}
	if len(r.Technologies) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("Technologies detected: " + strings.Join(r.Technologies, ", ") + ".")
	}
	return strings.TrimSpace(b.String())
}
