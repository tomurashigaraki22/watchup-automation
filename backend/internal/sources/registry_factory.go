package sources

import (
	"strings"

	"watchup/automation/internal/config"
)

// BuildFromConfig constructs the registry of scheduled discovery sources
// enabled via DISCOVERY_SOURCES. CSV import is intentionally excluded here —
// it's triggered on demand via POST /api/v1/companies/import, not run by the
// hourly scheduler.
func BuildFromConfig(cfg *config.Config) *Registry {
	var enabled []CompanySource
	for _, name := range cfg.DiscoverySources {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "github", "github_orgs":
			enabled = append(enabled, NewGitHubOrgsSource(cfg.GitHubOrgsQuery, cfg.GitHubToken))
		case "rss":
			for _, feedURL := range cfg.RSSFeedURLs {
				enabled = append(enabled, NewRSSSource(feedURL))
			}
		case "product_hunt":
			enabled = append(enabled, NewProductHuntSource())
		case "yc_directory":
			enabled = append(enabled, NewYCDirectorySource())
		case "ai_directory":
			enabled = append(enabled, NewAIDirectorySource())
		case "saas_directory":
			enabled = append(enabled, NewSaaSDirectorySource())
		}
	}
	return NewRegistry(enabled...)
}
