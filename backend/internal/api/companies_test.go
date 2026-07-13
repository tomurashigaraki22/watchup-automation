package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"watchup/automation/internal/db/models"
)

func TestImportCompaniesCSV_EndToEnd(t *testing.T) {
	srv, repos, token := newTestServer(t)

	csv := "name,website\nAcme,https://acme.com\nBeta,https://beta.io\n"
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/companies/import", bytes.NewBufferString(csv)), token)
	req.Header.Set("Content-Type", "text/csv")

	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Inserted int `json:"inserted"`
		Skipped  int `json:"skipped"`
		Errors   int `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Inserted != 2 {
		t.Fatalf("expected 2 inserted, got %+v", result)
	}

	list, err := repos.Companies.List(req.Context(), 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 companies persisted, got %d", len(list))
	}

	// Re-importing the same CSV must not duplicate rows.
	req2 := authed(httptest.NewRequest(http.MethodPost, "/api/v1/companies/import", bytes.NewBufferString(csv)), token)
	req2.Header.Set("Content-Type", "text/csv")
	resp2, err := srv.App.Test(req2)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	defer resp2.Body.Close()

	var result2 struct {
		Inserted int `json:"inserted"`
		Skipped  int `json:"skipped"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&result2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result2.Inserted != 0 || result2.Skipped != 2 {
		t.Fatalf("expected second import to be a pure skip, got %+v", result2)
	}
}

func TestImportCompaniesCSV_EmptyBody(t *testing.T) {
	srv, _, token := newTestServer(t)

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/companies/import", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestImportCompaniesCSV_RequiresAuth(t *testing.T) {
	srv, _, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/companies/import", bytes.NewBufferString("name,website\nA,https://a.com\n"))
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}
}

func TestListCompanies_AndGetCompanyDetail(t *testing.T) {
	srv, repos, token := newTestServer(t)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusAnalyzed, Description: "Acme does things"}
	if err := repos.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com", Priority: 1, Verified: true}
	if err := repos.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	listReq := authed(httptest.NewRequest(http.MethodGet, "/api/v1/companies", nil), token)
	listResp, err := srv.App.Test(listReq)
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listResult struct {
		Companies []models.Company `json:"companies"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResult.Companies) != 1 {
		t.Fatalf("expected 1 company, got %d", len(listResult.Companies))
	}

	detailReq := authed(httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/companies/%d", company.ID), nil), token)
	detailResp, err := srv.App.Test(detailReq)
	if err != nil {
		t.Fatalf("detail request: %v", err)
	}
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", detailResp.StatusCode)
	}
	var detail struct {
		models.Company
		Contacts []models.Contact `json:"contacts"`
	}
	if err := json.NewDecoder(detailResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != company.ID || len(detail.Contacts) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestGetCompany_NotFound(t *testing.T) {
	srv, _, token := newTestServer(t)
	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/companies/9999", nil), token)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
