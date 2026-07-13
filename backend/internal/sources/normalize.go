package sources

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeWebsite lowercases the host, strips a leading "www.", and drops
// path/query/fragment/scheme variation, so the same company discovered via
// different URLs ("http://Acme.com/about", "https://www.acme.com/") maps to
// one canonical website used for de-duplication.
func NormalizeWebsite(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("sources: normalize: empty website")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("sources: normalize: parse %q: %w", raw, err)
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return "", fmt.Errorf("sources: normalize: no host in %q", raw)
	}
	return "https://" + host, nil
}
