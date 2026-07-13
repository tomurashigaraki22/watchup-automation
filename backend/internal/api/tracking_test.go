package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"watchup/automation/internal/db/models"
)

func TestTrackOpen_IsPublic_MarksOpened(t *testing.T) {
	srv, repos, _ := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	repos.Companies.Create(ctx, company)
	contact := &models.Contact{CompanyID: company.ID, Email: "a@acme.com"}
	repos.Contacts.Create(ctx, contact)
	campaign := &models.OutreachCampaign{Name: "Test", DailyLimit: 25}
	repos.Campaigns.Create(ctx, campaign)
	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Status: models.EmailStatusSent}
	repos.Emails.Create(ctx, email)

	// No Authorization header — must still succeed since recipients click this.
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/t/o/%d", email.ID), nil)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	got, err := repos.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if !got.Opened {
		t.Fatal("expected email marked opened")
	}
}

func TestTrackUnsubscribe_IsPublic_Suppresses(t *testing.T) {
	srv, repos, _ := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	repos.Companies.Create(ctx, company)
	contact := &models.Contact{CompanyID: company.ID, Email: "a@acme.com"}
	repos.Contacts.Create(ctx, contact)
	campaign := &models.OutreachCampaign{Name: "Test", DailyLimit: 25}
	repos.Campaigns.Create(ctx, campaign)
	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Status: models.EmailStatusSent}
	repos.Emails.Create(ctx, email)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/t/u/%d", email.ID), nil)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	_, suppressed, err := repos.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		t.Fatalf("check suppression: %v", err)
	}
	if !suppressed {
		t.Fatal("expected contact suppressed")
	}
}
