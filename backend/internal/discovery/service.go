// Package discovery orchestrates CompanySources: running them, normalizing
// website domains, and upserting results into the companies table with
// status=discovered — deduped so re-running never creates duplicates.
package discovery

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/sources"
)

// Service runs discovery sources and persists their results.
type Service struct {
	repo *repository.Repositories
	log  *zap.Logger
}

// NewService builds a discovery Service.
func NewService(repo *repository.Repositories, log *zap.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Stats summarizes one discovery run.
type Stats struct {
	Inserted int
	Skipped  int
	Errors   int
}

// RunSource discovers companies from a single source and upserts them. The
// returned error is only set if the source itself fails outright (e.g. a
// malformed CSV or an unreachable feed) — per-company problems are counted
// in Stats.Errors instead so one bad row doesn't abort the whole run.
func (s *Service) RunSource(ctx context.Context, src sources.CompanySource) (Stats, error) {
	companies, err := src.Discover(ctx)
	if err != nil {
		s.log.Warn("discovery source failed", zap.String("source", src.Name()), zap.Error(err))
		return Stats{}, err
	}
	return s.upsert(ctx, src.Name(), companies), nil
}

// RunRegistry runs every source in the registry, continuing past individual
// source failures, and returns aggregate stats.
func (s *Service) RunRegistry(ctx context.Context, reg *sources.Registry) Stats {
	var total Stats
	for _, result := range reg.DiscoverAll(ctx) {
		if result.Err != nil {
			s.log.Warn("discovery source failed", zap.String("source", result.Source), zap.Error(result.Err))
			total.Errors++
			continue
		}
		st := s.upsert(ctx, result.Source, result.Companies)
		total.Inserted += st.Inserted
		total.Skipped += st.Skipped
		total.Errors += st.Errors
	}
	return total
}

// upsert normalizes each company's website and inserts it if no company
// with that normalized website already exists. Existing companies are left
// untouched (not overwritten) so manual edits made in the dashboard survive
// rediscovery.
func (s *Service) upsert(ctx context.Context, sourceName string, companies []sources.Company) Stats {
	var stats Stats
	for _, c := range companies {
		website, err := sources.NormalizeWebsite(c.Website)
		if err != nil {
			s.log.Warn("discovery: skipping company with invalid website",
				zap.String("source", sourceName), zap.String("raw_website", c.Website), zap.Error(err))
			stats.Errors++
			continue
		}

		_, found, err := s.repo.Companies.First(ctx, "website = ?", website)
		if err != nil {
			s.log.Error("discovery: lookup company failed", zap.String("website", website), zap.Error(err))
			stats.Errors++
			continue
		}
		if found {
			stats.Skipped++
			continue
		}

		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = website
		}

		company := &models.Company{
			Name:        name,
			Website:     website,
			Industry:    c.Industry,
			Description: c.Description,
			Employees:   c.Employees,
			Status:      models.CompanyStatusDiscovered,
		}
		if err := s.repo.Companies.Create(ctx, company); err != nil {
			s.log.Error("discovery: insert company failed", zap.String("website", website), zap.Error(err))
			stats.Errors++
			continue
		}
		stats.Inserted++
		s.log.Info("discovery: company inserted",
			zap.String("source", sourceName), zap.String("website", website), zap.String("name", name))
	}
	return stats
}
