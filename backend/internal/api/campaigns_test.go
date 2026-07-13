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

func TestCreateAndListCampaigns(t *testing.T) {
	srv, _, token := newTestServer(t)

	createReq := authed(httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(`{"name":"Q3 Outreach","daily_limit":50,"send_mode":"automatic"}`)), token)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := srv.App.Test(createReq)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var created models.OutreachCampaign
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Name != "Q3 Outreach" || created.DailyLimit != 50 || created.SendMode != "automatic" {
		t.Fatalf("unexpected created campaign: %+v", created)
	}

	listReq := authed(httptest.NewRequest(http.MethodGet, "/api/v1/campaigns", nil), token)
	listResp, err := srv.App.Test(listReq)
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer listResp.Body.Close()
	var listResult struct {
		Campaigns []models.OutreachCampaign `json:"campaigns"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	// The seed helper creates a default campaign too, so expect >= 1.
	found := false
	for _, c := range listResult.Campaigns {
		if c.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected created campaign in list, got %+v", listResult.Campaigns)
	}
}

func TestUpdatePauseResumeCampaign(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()
	campaign := &models.OutreachCampaign{Name: "Test", Status: models.CampaignStatusActive, DailyLimit: 25, SendMode: models.SendModeManual}
	if err := repos.Campaigns.Create(ctx, campaign); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	patchReq := authed(httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/campaigns/%d", campaign.ID), strings.NewReader(`{"daily_limit":100}`)), token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := srv.App.Test(patchReq)
	if err != nil {
		t.Fatalf("patch request: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", patchResp.StatusCode)
	}
	var patched models.OutreachCampaign
	if err := json.NewDecoder(patchResp.Body).Decode(&patched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if patched.DailyLimit != 100 {
		t.Fatalf("expected daily_limit updated to 100, got %d", patched.DailyLimit)
	}

	pauseReq := authed(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/campaigns/%d/pause", campaign.ID), nil), token)
	pauseResp, err := srv.App.Test(pauseReq)
	if err != nil {
		t.Fatalf("pause request: %v", err)
	}
	defer pauseResp.Body.Close()
	got, err := repos.Campaigns.GetByID(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("get campaign: %v", err)
	}
	if got.Status != models.CampaignStatusPaused {
		t.Fatalf("expected paused, got %q", got.Status)
	}

	resumeReq := authed(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/campaigns/%d/resume", campaign.ID), nil), token)
	if _, err := srv.App.Test(resumeReq); err != nil {
		t.Fatalf("resume request: %v", err)
	}
	got, err = repos.Campaigns.GetByID(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("get campaign: %v", err)
	}
	if got.Status != models.CampaignStatusActive {
		t.Fatalf("expected active, got %q", got.Status)
	}
}

func TestDeleteCampaign_SoftDeletes(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()
	campaign := &models.OutreachCampaign{Name: "ToDelete", Status: models.CampaignStatusActive, DailyLimit: 25}
	if err := repos.Campaigns.Create(ctx, campaign); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	req := authed(httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/campaigns/%d", campaign.ID), nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	got, err := repos.Campaigns.GetByID(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("expected campaign row to still exist (soft delete): %v", err)
	}
	if got.Status != models.CampaignStatusDeleted {
		t.Fatalf("expected status=deleted, got %q", got.Status)
	}
}

func TestCloneCampaign(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()
	original := &models.OutreachCampaign{Name: "Original", Status: models.CampaignStatusActive, DailyLimit: 42, SendMode: models.SendModeAutomatic}
	if err := repos.Campaigns.Create(ctx, original); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	req := authed(httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/campaigns/%d/clone", original.ID), nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("clone request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var clone models.OutreachCampaign
	if err := json.NewDecoder(resp.Body).Decode(&clone); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if clone.ID == original.ID || clone.DailyLimit != 42 || clone.SendMode != models.SendModeAutomatic {
		t.Fatalf("unexpected clone: %+v", clone)
	}
	if clone.Status != models.CampaignStatusPaused {
		t.Fatalf("expected clone to start paused, got %q", clone.Status)
	}
}
