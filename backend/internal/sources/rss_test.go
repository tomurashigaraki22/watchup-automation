package sources_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"watchup/automation/internal/sources"
)

func TestRSSSource_Discover(t *testing.T) {
	feed := `<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Launches</title>
    <item>
      <title>Acme</title>
      <link>https://acme.com</link>
      <description>Acme does things</description>
    </item>
    <item>
      <title>No Link Co</title>
      <link></link>
      <description>should be skipped</description>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(feed))
	}))
	defer server.Close()

	src := sources.NewRSSSource(server.URL)
	companies, err := src.Discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(companies) != 1 {
		t.Fatalf("expected 1 company (link-less item skipped), got %d: %+v", len(companies), companies)
	}
	if companies[0].Name != "Acme" || companies[0].Website != "https://acme.com" {
		t.Fatalf("unexpected company: %+v", companies[0])
	}
}

func TestRSSSource_Discover_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	src := sources.NewRSSSource(server.URL)
	if _, err := src.Discover(context.Background()); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}
