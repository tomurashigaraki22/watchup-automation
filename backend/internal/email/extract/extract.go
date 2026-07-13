// Package extract finds candidate contact email addresses on a crawled HTML
// page: in mailto: links, header/footer regions, inline scripts, JSON-LD
// structured data, and a whole-body regex sweep — then ranks each by the
// WatchUp email-prioritization rules.
package extract

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Email is one discovered address plus where it was found and its send priority.
type Email struct {
	Address  string
	Source   string // mailto | header | footer | script | structured_data | html_body
	Priority int    // 1 = highest priority (partnership@); larger = lower priority
}

var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// TargetLocalParts are the local-parts (before @) the PRD specifically asks
// for, in priority order (highest first). Addresses with other local-parts
// are still captured but rank lowest.
var TargetLocalParts = []string{
	"partnership",
	"business",
	"hello",
	"contact",
	"info",
	"support",
	"sales",
	"founder",
	"admin",
}

// PriorityForLocalPart ranks a local-part per TargetLocalParts; unmatched
// local-parts get the lowest priority (one past the end of the list).
func PriorityForLocalPart(localPart string) int {
	for i, p := range TargetLocalParts {
		if localPart == p {
			return i + 1
		}
	}
	return len(TargetLocalParts) + 1
}

// nonEmailTLDs filters common false positives like "logo@2x.png" retina
// image suffixes, which superficially match the email regex.
var nonEmailTLDs = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "svg": true,
	"webp": true, "ico": true, "css": true, "js": true,
}

// placeholderDomains filters template/example addresses that aren't real
// contacts (e.g. left over from a site builder's default theme).
var placeholderDomains = map[string]bool{
	"example.com": true, "domain.com": true, "yourdomain.com": true,
	"yoursite.com": true, "email.com": true, "sentry.io": true, "wixpress.com": true,
}

func isLikelyValidEmail(addr string) bool {
	idx := strings.LastIndex(addr, "@")
	if idx < 0 || idx == len(addr)-1 {
		return false
	}
	domain := addr[idx+1:]
	if placeholderDomains[domain] {
		return false
	}
	parts := strings.Split(domain, ".")
	tld := strings.ToLower(parts[len(parts)-1])
	return !nonEmailTLDs[tld]
}

// FromHTML extracts and de-duplicates candidate emails from a full HTML
// page, tagging each with the region it was found in.
func FromHTML(html string) []Email {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return dedupe(filterValid(scanText(html, "html_body")))
	}

	var found []Email
	found = append(found, mailtoLinks(doc)...)
	found = append(found, scanRegion(doc, "header", "header")...)
	found = append(found, scanRegion(doc, "footer", "footer")...)
	found = append(found, scanScripts(doc)...)
	found = append(found, structuredData(doc)...)

	bodyHTML, _ := doc.Find("body").Html()
	if bodyHTML == "" {
		bodyHTML = html // documents without an explicit <body> (e.g. fragments)
	}
	found = append(found, scanText(bodyHTML, "html_body")...)

	return dedupe(filterValid(found))
}

func mailtoLinks(doc *goquery.Document) []Email {
	var out []Email
	doc.Find(`a[href^="mailto:"]`).Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		addr := strings.TrimPrefix(href, "mailto:")
		if idx := strings.IndexAny(addr, "?"); idx >= 0 {
			addr = addr[:idx]
		}
		addr = strings.TrimSpace(addr)
		if emailRegex.MatchString(addr) {
			out = append(out, newEmail(addr, "mailto"))
		}
	})
	return out
}

func scanRegion(doc *goquery.Document, selector, source string) []Email {
	var out []Email
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		html, _ := s.Html()
		out = append(out, scanText(html, source)...)
	})
	return out
}

func scanScripts(doc *goquery.Document) []Email {
	var out []Email
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		if typ, _ := s.Attr("type"); typ == "application/ld+json" {
			return // handled separately by structuredData
		}
		out = append(out, scanText(s.Text(), "script")...)
	})
	return out
}

func structuredData(doc *goquery.Document) []Email {
	var out []Email
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		var data any
		if err := json.Unmarshal([]byte(s.Text()), &data); err != nil {
			return
		}
		for _, addr := range findEmailsInJSON(data) {
			out = append(out, newEmail(addr, "structured_data"))
		}
	})
	return out
}

func findEmailsInJSON(v any) []string {
	var out []string
	switch val := v.(type) {
	case string:
		out = append(out, emailRegex.FindAllString(val, -1)...)
	case map[string]any:
		for _, child := range val {
			out = append(out, findEmailsInJSON(child)...)
		}
	case []any:
		for _, child := range val {
			out = append(out, findEmailsInJSON(child)...)
		}
	}
	return out
}

func scanText(text, source string) []Email {
	var out []Email
	for _, addr := range emailRegex.FindAllString(text, -1) {
		out = append(out, newEmail(addr, source))
	}
	return out
}

func newEmail(addr, source string) Email {
	addr = strings.ToLower(strings.TrimSpace(addr))
	localPart := addr
	if idx := strings.Index(addr, "@"); idx >= 0 {
		localPart = addr[:idx]
	}
	return Email{Address: addr, Source: source, Priority: PriorityForLocalPart(localPart)}
}

func filterValid(emails []Email) []Email {
	var out []Email
	for _, e := range emails {
		if isLikelyValidEmail(e.Address) {
			out = append(out, e)
		}
	}
	return out
}

// dedupe keeps the first (highest-priority-source) occurrence of each address.
func dedupe(emails []Email) []Email {
	seen := make(map[string]bool, len(emails))
	var out []Email
	for _, e := range emails {
		if seen[e.Address] {
			continue
		}
		seen[e.Address] = true
		out = append(out, e)
	}
	return out
}
