package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watchup/automation/internal/db/models"
)

func TestGetMetrics_ComputesCounts(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusAnalyzed}
	repos.Companies.Create(ctx, company)
	repos.Companies.Create(ctx, &models.Company{Name: "Beta", Website: "https://beta.com", Status: models.CompanyStatusDiscovered})

	contact := &models.Contact{CompanyID: company.ID, Email: "a@acme.com", Verified: true}
	repos.Contacts.Create(ctx, contact)
	repos.Contacts.Create(ctx, &models.Contact{CompanyID: company.ID, Email: "b@acme.com", Verified: false})

	campaign := &models.OutreachCampaign{Name: "Test", Status: models.CampaignStatusActive, DailyLimit: 25}
	repos.Campaigns.Create(ctx, campaign)

	now := time.Now().UTC()
	repos.Emails.Create(ctx, &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Status: models.EmailStatusSent, SentAt: &now, Opened: true, Replied: true})
	repos.Emails.Create(ctx, &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Status: models.EmailStatusSent, SentAt: &now, Bounced: true})
	repos.Emails.Create(ctx, &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Status: models.EmailStatusDraft})

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var m struct {
		CompaniesDiscovered int64   `json:"companies_discovered"`
		EmailsExtracted     int64   `json:"emails_extracted"`
		EmailsVerified      int64   `json:"emails_verified"`
		EmailsSent          int64   `json:"emails_sent"`
		Replies             int64   `json:"replies"`
		OpenRate            float64 `json:"open_rate"`
		BounceRate          float64 `json:"bounce_rate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if m.CompaniesDiscovered != 2 {
		t.Errorf("expected 2 companies discovered, got %d", m.CompaniesDiscovered)
	}
	if m.EmailsExtracted != 2 {
		t.Errorf("expected 2 emails extracted, got %d", m.EmailsExtracted)
	}
	if m.EmailsVerified != 1 {
		t.Errorf("expected 1 verified, got %d", m.EmailsVerified)
	}
	if m.EmailsSent != 2 {
		t.Errorf("expected 2 sent, got %d", m.EmailsSent)
	}
	if m.Replies != 1 {
		t.Errorf("expected 1 reply, got %d", m.Replies)
	}
	if m.OpenRate != 0.5 {
		t.Errorf("expected open rate 0.5 (1/2 sent), got %f", m.OpenRate)
	}
	if m.BounceRate != 0.5 {
		t.Errorf("expected bounce rate 0.5 (1/2 sent), got %f", m.BounceRate)
	}
}

func TestGetMetrics_NoDivideByZero(t *testing.T) {
	srv, _, token := newTestServer(t)
	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 even with no data, got %d", resp.StatusCode)
	}
}
