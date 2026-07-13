package sources_test

import (
	"testing"

	"watchup/automation/internal/sources"
)

func TestNormalizeWebsite(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://acme.com", "https://acme.com", false},
		{"http://Acme.com/about", "https://acme.com", false},
		{"https://www.acme.com/", "https://acme.com", false},
		{"acme.com", "https://acme.com", false},
		{"WWW.ACME.COM", "https://acme.com", false},
		{"", "", true},
		{"   ", "", true},
	}
	for _, tc := range cases {
		got, err := sources.NormalizeWebsite(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NormalizeWebsite(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeWebsite(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("NormalizeWebsite(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
