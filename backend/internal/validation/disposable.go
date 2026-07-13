package validation

import "strings"

// disposableDomains is a curated list of well-known temporary/throwaway
// email providers. Addresses at these domains are never considered
// sendable, regardless of MX status.
var disposableDomains = map[string]bool{
	"mailinator.com":    true,
	"guerrillamail.com": true,
	"10minutemail.com":  true,
	"tempmail.com":      true,
	"temp-mail.org":     true,
	"yopmail.com":       true,
	"trashmail.com":     true,
	"throwawaymail.com": true,
	"getnada.com":       true,
	"dispostable.com":   true,
	"fakeinbox.com":     true,
	"sharklasers.com":   true,
	"maildrop.cc":       true,
	"moakt.com":         true,
	"mailnesia.com":     true,
	"mintemail.com":     true,
	"discard.email":     true,
	"33mail.com":        true,
}

func isDisposableDomain(domain string) bool {
	return disposableDomains[strings.ToLower(domain)]
}
