package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watchup/automation/internal/crawler"
)

func newTestSite() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><meta name="description" content="Acme builds tools."></head>
<body>
<h1>Acme Home</h1>
<a href="/about">About</a>
<a href="/contact">Contact</a>
<a href="/support">Support</a>
<a href="https://twitter.com/acme">Twitter</a>
<a href="https://offsite.example/whatever">Off-site link</a>
<script src="https://cdn.shopify.com/s/files/x.js"></script>
</body></html>`))
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>About Us</h1><header>founder@acme.com</header></body></html>`))
	})
	mux.HandleFunc("/contact", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><a href="mailto:partnership@acme.com">Email us</a></body></html>`))
	})
	mux.HandleFunc("/support", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><footer>support@acme.com</footer></body></html>`))
	})
	mux.HandleFunc("/irrelevant-page", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>should not be visited via crawl of allow-listed paths</body></html>`))
	})

	return httptest.NewServer(mux)
}

func TestCrawl_VisitsAllowedPathsAndExtracts(t *testing.T) {
	server := newTestSite()
	defer server.Close()

	result, err := crawler.Crawl(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("crawl: %v", err)
	}

	if result.Description != "Acme builds tools." {
		t.Errorf("unexpected description: %q", result.Description)
	}
	if result.PagesCrawled < 4 {
		t.Errorf("expected at least 4 pages crawled (home+about+contact+support), got %d", result.PagesCrawled)
	}
	if result.PagesCrawled > crawler.MaxPages {
		t.Errorf("expected at most %d pages, got %d", crawler.MaxPages, result.PagesCrawled)
	}

	byAddress := map[string]string{}
	for _, e := range result.Emails {
		byAddress[e.Address] = e.Source
	}
	if byAddress["partnership@acme.com"] != "mailto" {
		t.Errorf("expected partnership@acme.com via mailto, got %+v", result.Emails)
	}
	if byAddress["founder@acme.com"] != "header" {
		t.Errorf("expected founder@acme.com via header, got %+v", result.Emails)
	}
	if byAddress["support@acme.com"] != "footer" {
		t.Errorf("expected support@acme.com via footer, got %+v", result.Emails)
	}

	foundTwitter := false
	for _, l := range result.SocialLinks {
		if strings.Contains(l, "twitter.com") {
			foundTwitter = true
		}
	}
	if !foundTwitter {
		t.Errorf("expected twitter.com social link, got %+v", result.SocialLinks)
	}

	foundShopify := false
	for _, tech := range result.Technologies {
		if tech == "Shopify" {
			foundShopify = true
		}
	}
	if !foundShopify {
		t.Errorf("expected Shopify technology detection, got %+v", result.Technologies)
	}

	for _, p := range result.Products {
		if strings.Contains(strings.ToLower(p), "irrelevant") {
			t.Errorf("crawler visited an off-allow-list page: %+v", result.Products)
		}
	}
}

func TestCrawl_InvalidURL(t *testing.T) {
	if _, err := crawler.Crawl(context.Background(), "not a url"); err == nil {
		t.Fatal("expected error for invalid url")
	}
}

func TestCrawl_RespectsContextCancellation(t *testing.T) {
	server := newTestSite()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := crawler.Crawl(ctx, server.URL)
	// The homepage request itself may still race through before cancellation
	// is observed by colly's OnRequest hook; what matters is it doesn't crawl
	// the full allow-listed set once canceled.
	if err == nil && result.PagesCrawled > 1 {
		t.Errorf("expected cancellation to stop further crawling, got %d pages", result.PagesCrawled)
	}
}
