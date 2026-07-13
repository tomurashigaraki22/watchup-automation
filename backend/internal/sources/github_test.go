package sources_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watchup/automation/internal/sources"
)

func TestGitHubOrgsSource_Discover(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"login":"acme","html_url":"https://github.com/acme"},{"login":"nodetail","html_url":"https://github.com/nodetail"}]}`))
	})
	mux.HandleFunc("/orgs/acme", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"acme","name":"Acme Inc","blog":"https://acme.com","bio":"We do things"}`))
	})
	mux.HandleFunc("/orgs/nodetail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	src := sources.NewGitHubOrgsSource("video", "")
	src.BaseURL = server.URL

	companies, err := src.Discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(companies) != 2 {
		t.Fatalf("expected 2 companies, got %d: %+v", len(companies), companies)
	}
	if companies[0].Name != "Acme Inc" || companies[0].Website != "https://acme.com" || companies[0].Description != "We do things" {
		t.Fatalf("unexpected first company: %+v", companies[0])
	}
	// Detail fetch failed for "nodetail" -> falls back to login/html_url.
	if companies[1].Name != "nodetail" || companies[1].Website != "https://github.com/nodetail" {
		t.Fatalf("unexpected fallback company: %+v", companies[1])
	}

	if !strings.Contains(src.Name(), "video") {
		t.Fatalf("unexpected source name: %s", src.Name())
	}
}

func TestGitHubOrgsSource_Discover_SearchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	src := sources.NewGitHubOrgsSource("video", "")
	src.BaseURL = server.URL

	if _, err := src.Discover(context.Background()); err == nil {
		t.Fatal("expected error when search endpoint fails")
	}
}
