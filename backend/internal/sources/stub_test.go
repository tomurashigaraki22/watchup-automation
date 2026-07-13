package sources_test

import (
	"context"
	"testing"

	"watchup/automation/internal/sources"
)

func TestStubSource_ReturnsDescriptiveError(t *testing.T) {
	stubs := []*sources.StubSource{
		sources.NewProductHuntSource(),
		sources.NewYCDirectorySource(),
		sources.NewAIDirectorySource(),
		sources.NewSaaSDirectorySource(),
	}
	for _, s := range stubs {
		companies, err := s.Discover(context.Background())
		if err == nil {
			t.Errorf("%s: expected error, got none", s.Name())
		}
		if companies != nil {
			t.Errorf("%s: expected nil companies, got %+v", s.Name(), companies)
		}
	}
}
