package discovery_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/discovery"
	"watchup/automation/internal/sources"
	"watchup/automation/internal/testutil"
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

func newService(t *testing.T) (*discovery.Service, *repository.Repositories) {
	t.Helper()
	repo := repository.NewRepositories(testutil.NewDB(t))
	return discovery.NewService(repo, zap.NewNop()), repo
}

func TestService_RunSource_InsertsWithDiscoveredStatus(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService(t)

	src := fakeSource{name: "test", companies: []sources.Company{
		{Name: "Acme", Website: "https://acme.com"},
	}}

	stats, err := svc.RunSource(ctx, src)
	if err != nil {
		t.Fatalf("run source: %v", err)
	}
	if stats.Inserted != 1 || stats.Skipped != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	list, err := repo.Companies.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Status != models.CompanyStatusDiscovered {
		t.Fatalf("expected 1 discovered company, got %+v", list)
	}
}

func TestService_RunSource_DedupesWithinOneRun(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService(t)

	// Same company via two different URL forms should collapse to one row.
	src := fakeSource{name: "test", companies: []sources.Company{
		{Name: "Acme", Website: "http://Acme.com/about"},
		{Name: "Acme Again", Website: "https://www.acme.com/"},
	}}

	stats, err := svc.RunSource(ctx, src)
	if err != nil {
		t.Fatalf("run source: %v", err)
	}
	if stats.Inserted != 1 || stats.Skipped != 1 {
		t.Fatalf("expected 1 inserted + 1 skipped, got %+v", stats)
	}

	list, err := repo.Companies.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 company, got %d: %+v", len(list), list)
	}
}

func TestService_RunSource_RerunDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService(t)

	src := fakeSource{name: "test", companies: []sources.Company{
		{Name: "Acme", Website: "https://acme.com"},
	}}

	if _, err := svc.RunSource(ctx, src); err != nil {
		t.Fatalf("first run: %v", err)
	}
	stats, err := svc.RunSource(ctx, src)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if stats.Inserted != 0 || stats.Skipped != 1 {
		t.Fatalf("expected second run to be a pure skip, got %+v", stats)
	}

	list, err := repo.Companies.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected still only 1 company after rerun, got %d", len(list))
	}
}

func TestService_RunSource_InvalidWebsiteCountsAsError(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService(t)

	src := fakeSource{name: "test", companies: []sources.Company{
		{Name: "Bad", Website: ""},
	}}

	stats, err := svc.RunSource(ctx, src)
	if err != nil {
		t.Fatalf("run source: %v", err)
	}
	if stats.Errors != 1 || stats.Inserted != 0 {
		t.Fatalf("expected 1 error, got %+v", stats)
	}

	list, err := repo.Companies.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no companies inserted, got %d", len(list))
	}
}

func TestService_RunSource_SourceFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	src := fakeSource{name: "test", err: errors.New("boom")}
	if _, err := svc.RunSource(ctx, src); err == nil {
		t.Fatal("expected error when source fails")
	}
}

func TestService_RunRegistry_AggregatesAcrossSources(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService(t)

	ok1 := fakeSource{name: "s1", companies: []sources.Company{{Name: "Acme", Website: "acme.com"}}}
	ok2 := fakeSource{name: "s2", companies: []sources.Company{{Name: "Beta", Website: "beta.io"}}}
	bad := fakeSource{name: "s3", err: errors.New("boom")}

	reg := sources.NewRegistry(ok1, ok2, bad)
	stats := svc.RunRegistry(ctx, reg)

	if stats.Inserted != 2 {
		t.Fatalf("expected 2 inserted, got %+v", stats)
	}
	if stats.Errors != 1 {
		t.Fatalf("expected 1 source-level error counted, got %+v", stats)
	}

	list, err := repo.Companies.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 companies, got %d", len(list))
	}
}
