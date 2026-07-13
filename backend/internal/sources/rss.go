package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RSSSource discovers companies from a generic RSS 2.0 feed, treating each
// <item> as one company candidate (title -> name, link -> website,
// description -> description). Works with any company/product/startup
// directory that exposes an RSS feed, not a specific provider.
type RSSSource struct {
	URL    string
	client *http.Client
}

// NewRSSSource builds an RSSSource for the given feed URL.
func NewRSSSource(feedURL string) *RSSSource {
	return &RSSSource{URL: feedURL, client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *RSSSource) Name() string { return "rss:" + s.URL }

type rssFeed struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (s *RSSSource) Discover(ctx context.Context) ([]Company, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("sources: rss: build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sources: rss: fetch %s: %w", s.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sources: rss: %s returned status %d", s.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sources: rss: read body: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("sources: rss: parse xml: %w", err)
	}

	companies := make([]Company, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" {
			continue
		}
		companies = append(companies, Company{
			Name:        strings.TrimSpace(item.Title),
			Website:     link,
			Description: strings.TrimSpace(item.Description),
		})
	}
	return companies, nil
}
