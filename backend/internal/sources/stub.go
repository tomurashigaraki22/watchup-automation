package sources

import (
	"context"
	"fmt"
)

// StubSource is a discovery source with no usable public API or credentials
// wired up yet. It satisfies CompanySource (so it slots into the registry
// like any real source) but returns an empty result with a descriptive
// error, letting the registry log why it produced nothing without failing
// the whole discovery run.
type StubSource struct {
	SourceName string
	Reason     string
}

// NewStubSource builds a StubSource. name is used as the registry entry
// name; reason explains what's missing (credentials, a stable endpoint, ...).
func NewStubSource(name, reason string) *StubSource {
	return &StubSource{SourceName: name, Reason: reason}
}

func (s *StubSource) Name() string { return s.SourceName }

func (s *StubSource) Discover(_ context.Context) ([]Company, error) {
	return nil, fmt.Errorf("sources: %s: not implemented yet — %s", s.SourceName, s.Reason)
}

// NewProductHuntSource is a stub: Product Hunt discovery requires an OAuth
// GraphQL token (PRODUCT_HUNT_TOKEN) and client, not wired up yet.
func NewProductHuntSource() *StubSource {
	return NewStubSource("product_hunt", "requires a Product Hunt developer token and GraphQL client")
}

// NewYCDirectorySource is a stub: Y Combinator's company directory has no
// stable public API and needs a dedicated scraper or partner access.
func NewYCDirectorySource() *StubSource {
	return NewStubSource("yc_directory", "no stable public API — requires a dedicated scraper or partner access")
}

// NewAIDirectorySource is a stub until a specific AI-tools directory
// endpoint is chosen and configured.
func NewAIDirectorySource() *StubSource {
	return NewStubSource("ai_directory", "no specific AI directory endpoint configured yet")
}

// NewSaaSDirectorySource is a stub until a specific SaaS directory endpoint
// is chosen and configured.
func NewSaaSDirectorySource() *StubSource {
	return NewStubSource("saas_directory", "no specific SaaS directory endpoint configured yet")
}
