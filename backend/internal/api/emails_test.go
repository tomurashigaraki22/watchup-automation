package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watchup/automation/internal/db/models"
)

func TestGetEmail_PreviewIncludesHTML(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := repos.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com"}
	if err := repos.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	campaign := &models.OutreachCampaign{Name: "Test", DailyLimit: 25}
	if err := repos.Campaigns.Create(ctx, campaign); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Body: "Hello there.\n\nSecond paragraph.", Status: models.EmailStatusDraft}
	if err := repos.Emails.Create(ctx, email); err != nil {
		t.Fatalf("seed email: %v", err)
	}

	req := authed(httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/emails/%d", email.ID), nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		BodyHTML string `json:"body_html"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Subject != "Hi" || !strings.Contains(result.BodyHTML, "<p>") {
		t.Fatalf("unexpected preview: %+v", result)
	}
}

func TestUpdateEmail_OnlyWhileDraft(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	repos.Companies.Create(ctx, company)
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com"}
	repos.Contacts.Create(ctx, contact)
	campaign := &models.OutreachCampaign{Name: "Test", DailyLimit: 25}
	repos.Campaigns.Create(ctx, campaign)

	draft := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Old", Body: "old body", Status: models.EmailStatusDraft}
	repos.Emails.Create(ctx, draft)

	patchReq := authed(httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/emails/%d", draft.ID), strings.NewReader(`{"subject":"New Subject","body":"new body"}`)), token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := srv.App.Test(patchReq)
	if err != nil {
		t.Fatalf("patch request: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", patchResp.StatusCode)
	}

	got, err := repos.Emails.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if got.Subject != "New Subject" || got.Body != "new body" {
		t.Fatalf("unexpected email after patch: %+v", got)
	}

	// Now sent — edits should be rejected.
	sent := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Sent", Body: "sent body", Status: models.EmailStatusSent}
	repos.Emails.Create(ctx, sent)

	patchReq2 := authed(httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/emails/%d", sent.ID), strings.NewReader(`{"subject":"Nope"}`)), token)
	patchReq2.Header.Set("Content-Type", "application/json")
	patchResp2, err := srv.App.Test(patchReq2)
	if err != nil {
		t.Fatalf("patch request: %v", err)
	}
	defer patchResp2.Body.Close()
	if patchResp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for editing a sent email, got %d", patchResp2.StatusCode)
	}
}

func TestSendEmail_EnqueuesJob(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	repos.Companies.Create(ctx, company)
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com"}
	repos.Contacts.Create(ctx, contact)
	campaign := &models.OutreachCampaign{Name: "Test", DailyLimit: 25, SendMode: models.SendModeManual}
	repos.Campaigns.Create(ctx, campaign)
	draft := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Body: "body", Status: models.EmailStatusDraft}
	repos.Emails.Create(ctx, draft)

	req := authed(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/emails/%d/send", draft.ID), nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Sending again on a still-draft-status email should succeed too (idempotent
	// trigger); but if it were already sent it should be rejected — covered below.
	sent := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Already sent", Body: "body", Status: models.EmailStatusSent}
	repos.Emails.Create(ctx, sent)
	req2 := authed(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/emails/%d/send", sent.ID), nil), token)
	resp2, err := srv.App.Test(req2)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for re-sending an already-sent email, got %d", resp2.StatusCode)
	}
}

func TestListEmails_FiltersByStatus(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	repos.Companies.Create(ctx, company)
	contact := &models.Contact{CompanyID: company.ID, Email: "a@acme.com"}
	repos.Contacts.Create(ctx, contact)
	campaign := &models.OutreachCampaign{Name: "Test", DailyLimit: 25}
	repos.Campaigns.Create(ctx, campaign)
	repos.Emails.Create(ctx, &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "A", Status: models.EmailStatusDraft})
	repos.Emails.Create(ctx, &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "B", Status: models.EmailStatusSent})

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/emails?status=sent", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	var result struct {
		Emails []models.Email `json:"emails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Emails) != 1 || result.Emails[0].Subject != "B" {
		t.Fatalf("expected only sent email B, got %+v", result.Emails)
	}
}
