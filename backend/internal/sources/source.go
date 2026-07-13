// Package sources implements pluggable company discovery. Every source
// (CSV import, RSS, GitHub Organizations, directories, ...) satisfies the
// same CompanySource interface so new sources can be added without touching
// the registry, the discovery service, or the scheduler.
package sources

import "context"

// Company is the raw output of a discovery source, prior to persistence.
type Company struct {
	Name        string
	Website     string
	Industry    string
	Description string
	Employees   string
}

// CompanySource discovers candidate companies from one external source.
type CompanySource interface {
	Name() string
	Discover(ctx context.Context) ([]Company, error)
}
