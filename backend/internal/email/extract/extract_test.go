package extract_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"watchup/automation/internal/email/extract"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func TestFromHTML_ExtractsAndPrioritizes(t *testing.T) {
	emails := extract.FromHTML(fixture(t, "company_page.html"))

	byAddress := map[string]extract.Email{}
	for _, e := range emails {
		byAddress[e.Address] = e
	}

	cases := []struct {
		address  string
		priority int
	}{
		{"partnership@acme.com", 1},
		{"business@acme.com", 2},
		{"hello@acme.com", 3},
		{"contact@acme.com", 4},
		{"info@acme.com", 5},
		{"support@acme.com", 6},
		{"sales@acme.com", 7},
	}
	for _, tc := range cases {
		e, ok := byAddress[tc.address]
		if !ok {
			t.Errorf("expected to find %s, got %+v", tc.address, emails)
			continue
		}
		if e.Priority != tc.priority {
			t.Errorf("%s: expected priority %d, got %d", tc.address, tc.priority, e.Priority)
		}
	}

	// Retina image suffix and placeholder domain must not be mistaken for emails.
	if _, ok := byAddress["logo@2x.png"]; ok {
		t.Error("logo@2x.png should have been filtered as a false positive")
	}
	if _, ok := byAddress["noreply@example.com"]; ok {
		t.Error("placeholder domain example.com should have been filtered")
	}
}

func TestFromHTML_SourcesTagged(t *testing.T) {
	emails := extract.FromHTML(fixture(t, "company_page.html"))
	sources := map[string]string{}
	for _, e := range emails {
		sources[e.Address] = e.Source
	}

	if sources["partnership@acme.com"] != "mailto" {
		t.Errorf("expected partnership@ to come from mailto, got %q", sources["partnership@acme.com"])
	}
	if sources["support@acme.com"] != "footer" {
		t.Errorf("expected support@ to come from footer, got %q", sources["support@acme.com"])
	}
	if sources["founder@acme.com"] != "header" {
		t.Errorf("expected founder@ to come from header, got %q", sources["founder@acme.com"])
	}
}

func TestFromHTML_Deduplicates(t *testing.T) {
	emails := extract.FromHTML(fixture(t, "company_page.html"))
	seen := map[string]int{}
	for _, e := range emails {
		seen[e.Address]++
	}
	for addr, count := range seen {
		if count > 1 {
			t.Errorf("%s appeared %d times, expected exactly once", addr, count)
		}
	}
}

func TestPriorityForLocalPart_UnknownIsLowest(t *testing.T) {
	got := extract.PriorityForLocalPart("random-unlisted-name")
	want := len(extract.TargetLocalParts) + 1
	if got != want {
		t.Errorf("expected unlisted local-part to get priority %d, got %d", want, got)
	}
}

func TestFromHTML_EmptyInput(t *testing.T) {
	emails := extract.FromHTML("")
	if len(emails) != 0 {
		t.Errorf("expected no emails from empty input, got %+v", emails)
	}
}
