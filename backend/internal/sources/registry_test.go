package sources_test

import (
	"context"
	"errors"
	"testing"

	"watchup/automation/internal/sources"
)

type fakeSource struct {
	name      string
	companies []sources.Company
	err       error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Discover(_ context.Context) ([]sources.Company, error) {
	return f.companies, f.err
}

func TestRegistry_DiscoverAll_ContinuesPastFailures(t *testing.T) {
	ok := fakeSource{name: "ok", companies: []sources.Company{{Name: "Acme", Website: "acme.com"}}}
	bad := fakeSource{name: "bad", err: errors.New("boom")}

	reg := sources.NewRegistry(ok, bad)
	if got := reg.Names(); len(got) != 2 || got[0] != "ok" || got[1] != "bad" {
		t.Fatalf("unexpected names: %v", got)
	}

	results := reg.DiscoverAll(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Err != nil || len(results[0].Companies) != 1 {
		t.Fatalf("expected ok source to succeed, got %+v", results[0])
	}
	if results[1].Err == nil {
		t.Fatalf("expected bad source to report its error")
	}
}
