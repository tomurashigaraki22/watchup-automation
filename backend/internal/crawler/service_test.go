package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"watchup/automation/internal/crawler"
	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/testutil"
)

func TestService_CrawlCompany_PersistsContactsAndMarksCrawled(t *testing.T) {
	server := newTestSite()
	defer server.Close()

	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	svc := crawler.NewService(repo, zap.NewNop())

	company := &models.Company{Name: "Acme", Website: server.URL, Status: models.CompanyStatusDiscovered}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	result, err := svc.CrawlCompany(ctx, company)
	if err != nil {
		t.Fatalf("crawl company: %v", err)
	}
	if len(result.Emails) == 0 {
		t.Fatal("expected some emails to be found")
	}

	got, err := repo.Companies.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	if got.Status != models.CompanyStatusCrawled {
		t.Errorf("expected status=crawled, got %q", got.Status)
	}
	if got.Description == "" {
		t.Error("expected description to be populated")
	}

	contacts, err := repo.Contacts.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}
	if len(contacts) != len(result.Emails) {
		t.Fatalf("expected %d contacts persisted, got %d", len(result.Emails), len(contacts))
	}
	for _, c := range contacts {
		if c.CompanyID != company.ID {
			t.Errorf("contact %+v has wrong company_id", c)
		}
	}
}

func TestService_CrawlCompany_RerunDoesNotDuplicateContacts(t *testing.T) {
	server := newTestSite()
	defer server.Close()

	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	svc := crawler.NewService(repo, zap.NewNop())

	company := &models.Company{Name: "Acme", Website: server.URL, Status: models.CompanyStatusDiscovered}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	if _, err := svc.CrawlCompany(ctx, company); err != nil {
		t.Fatalf("first crawl: %v", err)
	}
	first, err := repo.Contacts.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}

	if _, err := svc.CrawlCompany(ctx, company); err != nil {
		t.Fatalf("second crawl: %v", err)
	}
	second, err := repo.Contacts.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list contacts: %v", err)
	}

	if len(second) != len(first) {
		t.Fatalf("expected recrawl not to duplicate contacts: first=%d second=%d", len(first), len(second))
	}
}

func TestService_CrawlCompany_InvalidWebsite(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	svc := crawler.NewService(repo, zap.NewNop())

	company := &models.Company{Name: "Bad", Website: "not a url", Status: models.CompanyStatusDiscovered}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	if _, err := svc.CrawlCompany(ctx, company); err == nil {
		t.Fatal("expected error for invalid website")
	}
}

func TestService_CrawlCompany_EmptySiteStillMarksCrawled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>no contact info here</body></html>`))
	}))
	defer server.Close()

	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	svc := crawler.NewService(repo, zap.NewNop())

	company := &models.Company{Name: "Empty", Website: server.URL, Status: models.CompanyStatusDiscovered}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	if _, err := svc.CrawlCompany(ctx, company); err != nil {
		t.Fatalf("crawl: %v", err)
	}

	got, err := repo.Companies.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	if got.Status != models.CompanyStatusCrawled {
		t.Errorf("expected status=crawled even with no findings, got %q", got.Status)
	}
}
