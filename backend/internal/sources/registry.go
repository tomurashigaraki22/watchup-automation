package sources

import "context"

// Registry runs a fixed set of enabled CompanySources.
type Registry struct {
	sources []CompanySource
}

// NewRegistry builds a Registry over the given sources.
func NewRegistry(srcs ...CompanySource) *Registry {
	return &Registry{sources: srcs}
}

// Names lists the configured source names without invoking Discover — used
// for logging what's enabled and in tests that shouldn't make network calls.
func (r *Registry) Names() []string {
	names := make([]string, len(r.sources))
	for i, s := range r.sources {
		names[i] = s.Name()
	}
	return names
}

// Result is one source's discovery outcome.
type Result struct {
	Source    string
	Companies []Company
	Err       error
}

// DiscoverAll runs every registered source. A failure in one source is
// captured in its Result and does not block the others.
func (r *Registry) DiscoverAll(ctx context.Context) []Result {
	results := make([]Result, 0, len(r.sources))
	for _, s := range r.sources {
		companies, err := s.Discover(ctx)
		results = append(results, Result{Source: s.Name(), Companies: companies, Err: err})
	}
	return results
}
