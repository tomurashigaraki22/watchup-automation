// Package crawler visits a company's website and the allow-listed pages
// beneath it (contact, about, team, company, privacy, legal, terms,
// support), extracting a description, product/technology signals, support
// and social links, and contact emails.
package crawler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"

	"watchup/automation/internal/email/extract"
)

const (
	// MaxPages caps total pages visited per company, per the PRD.
	MaxPages = 25
	// MaxDepth caps link hops from the homepage, per the PRD.
	MaxDepth = 2
)

var targetPathKeywords = []string{"contact", "about", "team", "company", "privacy", "legal", "terms", "support"}

// Result is everything extracted from crawling one company's website.
type Result struct {
	Description  string
	Products     []string
	Technologies []string
	SupportLinks []string
	SocialLinks  []string
	Emails       []extract.Email
	PagesCrawled int
}

// Crawl visits websiteURL and same-domain pages whose path matches the
// allow-listed keywords, capped at MaxPages pages and MaxDepth link hops.
func Crawl(ctx context.Context, websiteURL string) (*Result, error) {
	base, err := url.Parse(websiteURL)
	if err != nil || base.Hostname() == "" {
		return nil, fmt.Errorf("crawler: invalid website url %q", websiteURL)
	}

	result := &Result{}
	seenEmail := map[string]bool{}
	seenTech := map[string]bool{}
	seenSupport := map[string]bool{}
	seenSocial := map[string]bool{}
	seenProduct := map[string]bool{}
	pagesVisited := 0

	c := colly.NewCollector(
		colly.MaxDepth(MaxDepth),
		colly.UserAgent("WatchUpOutreachBot/1.0 (+https://watchup.space)"),
	)
	c.AllowedDomains = []string{base.Hostname()}
	c.SetRequestTimeout(15 * time.Second)
	_ = c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: 1, Delay: 150 * time.Millisecond})

	c.OnRequest(func(r *colly.Request) {
		if ctx.Err() != nil || pagesVisited >= MaxPages {
			r.Abort()
			return
		}
		pagesVisited++
	})

	c.OnHTML("html", func(e *colly.HTMLElement) {
		doc := e.DOM
		extractDescription(doc, result)
		extractProducts(doc, result, seenProduct)
		extractTechnologies(e, result, seenTech)
		extractLinks(e, result, seenSupport, seenSocial)

		pageHTML, _ := doc.Html()
		for _, em := range extract.FromHTML(pageHTML) {
			if !seenEmail[em.Address] {
				seenEmail[em.Address] = true
				result.Emails = append(result.Emails, em)
			}
		}
	})

	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		target := e.Request.AbsoluteURL(href)
		if target == "" {
			return
		}
		u, err := url.Parse(target)
		if err != nil || u.Hostname() != base.Hostname() {
			return
		}
		if !isTargetPath(u.Path) {
			return
		}
		_ = e.Request.Visit(target)
	})

	if err := c.Visit(base.String()); err != nil {
		return nil, fmt.Errorf("crawler: visit %s: %w", base.String(), err)
	}
	c.Wait()

	result.PagesCrawled = pagesVisited
	return result, nil
}

// isTargetPath reports whether path looks like one of the allow-listed
// crawl targets. The root path is excluded since it's always visited first.
func isTargetPath(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	lower := strings.ToLower(path)
	for _, kw := range targetPathKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func extractDescription(doc *goquery.Selection, result *Result) {
	if result.Description != "" {
		return
	}
	if meta, ok := doc.Find(`meta[name="description"]`).Attr("content"); ok && strings.TrimSpace(meta) != "" {
		result.Description = strings.TrimSpace(meta)
		return
	}
	if meta, ok := doc.Find(`meta[property="og:description"]`).Attr("content"); ok && strings.TrimSpace(meta) != "" {
		result.Description = strings.TrimSpace(meta)
	}
}

func extractProducts(doc *goquery.Selection, result *Result, seen map[string]bool) {
	doc.Find("h1, h2").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" || len(text) > 80 {
			return
		}
		key := strings.ToLower(text)
		if seen[key] {
			return
		}
		seen[key] = true
		if len(result.Products) < 10 {
			result.Products = append(result.Products, text)
		}
	})
}

// technologySignatures maps a substring found in page HTML to a technology name.
var technologySignatures = map[string]string{
	"wp-content":               "WordPress",
	"wp-includes":              "WordPress",
	"cdn.shopify.com":          "Shopify",
	"webflow":                  "Webflow",
	"static.wixstatic":         "Wix",
	"squarespace":              "Squarespace",
	"js.hs-scripts.com":        "HubSpot",
	"js.stripe.com":            "Stripe",
	"cdn.segment.com":          "Segment",
	"google-analytics.com":     "Google Analytics",
	"googletagmanager.com":     "Google Tag Manager",
	"widget.intercom.io":       "Intercom",
	"zdassets.com":             "Zendesk",
	"_next/static":             "Next.js",
	"cdn.jsdelivr.net/npm/vue": "Vue.js",
}

func extractTechnologies(e *colly.HTMLElement, result *Result, seen map[string]bool) {
	lower := strings.ToLower(string(e.Response.Body))
	for sig, tech := range technologySignatures {
		if seen[tech] {
			continue
		}
		if strings.Contains(lower, sig) {
			seen[tech] = true
			result.Technologies = append(result.Technologies, tech)
		}
	}
}

var socialDomains = []string{"twitter.com", "x.com", "linkedin.com", "facebook.com", "instagram.com", "github.com", "youtube.com", "tiktok.com"}
var supportKeywords = []string{"support", "help", "faq", "docs"}
var helpdeskDomains = []string{"zendesk.com", "intercom.io", "freshdesk.com", "helpscout"}

func extractLinks(e *colly.HTMLElement, result *Result, seenSupport, seenSocial map[string]bool) {
	e.ForEach("a[href]", func(_ int, el *colly.HTMLElement) {
		href := el.Attr("href")
		if href == "" {
			return
		}
		target := e.Request.AbsoluteURL(href)
		lower := strings.ToLower(target)

		for _, d := range socialDomains {
			if strings.Contains(lower, d) {
				if !seenSocial[target] {
					seenSocial[target] = true
					if len(result.SocialLinks) < 10 {
						result.SocialLinks = append(result.SocialLinks, target)
					}
				}
				break
			}
		}

		isHelpdesk := false
		for _, d := range helpdeskDomains {
			if strings.Contains(lower, d) {
				isHelpdesk = true
				break
			}
		}
		hasSupportKeyword := false
		for _, kw := range supportKeywords {
			if strings.Contains(lower, kw) {
				hasSupportKeyword = true
				break
			}
		}
		if (isHelpdesk || hasSupportKeyword) && !seenSupport[target] {
			seenSupport[target] = true
			if len(result.SupportLinks) < 10 {
				result.SupportLinks = append(result.SupportLinks, target)
			}
		}
	})
}
