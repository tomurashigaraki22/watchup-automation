package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"watchup/automation/internal/db/models"
)

func TestSearch_ByQueryAcrossEntities(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	repos.Companies.Create(ctx, &models.Company{Name: "Acme Streaming", Website: "https://acme-streaming.com", Status: models.CompanyStatusDiscovered})
	repos.Companies.Create(ctx, &models.Company{Name: "Beta Corp", Website: "https://beta.com", Status: models.CompanyStatusDiscovered})
	repos.Campaigns.Create(ctx, &models.OutreachCampaign{Name: "Acme Partnership Push", Status: models.CampaignStatusActive, DailyLimit: 25})

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/search?q=acme", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Companies []models.Company          `json:"companies"`
		Campaigns []models.OutreachCampaign `json:"campaigns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Companies) != 1 || result.Companies[0].Name != "Acme Streaming" {
		t.Fatalf("expected 1 matching company, got %+v", result.Companies)
	}
	if len(result.Campaigns) != 1 {
		t.Fatalf("expected 1 matching campaign, got %+v", result.Campaigns)
	}
}

func TestSearch_ByStatusOnly(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	repos.Companies.Create(ctx, &models.Company{Name: "A", Website: "https://a.com", Status: models.CompanyStatusCrawled})
	repos.Companies.Create(ctx, &models.Company{Name: "B", Website: "https://b.com", Status: models.CompanyStatusDiscovered})

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/search?status=crawled", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Companies []models.Company `json:"companies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Companies) != 1 || result.Companies[0].Name != "A" {
		t.Fatalf("expected only company A, got %+v", result.Companies)
	}
}

func TestSearch_RequiresQueryOrStatus(t *testing.T) {
	srv, _, token := newTestServer(t)
	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/search", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
