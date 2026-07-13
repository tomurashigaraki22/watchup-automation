package sources_test

import (
	"testing"

	"watchup/automation/internal/config"
	"watchup/automation/internal/sources"
)

func TestBuildFromConfig(t *testing.T) {
	cfg := &config.Config{
		DiscoverySources:  []string{"github", "product_hunt", "yc_directory", "ai_directory", "saas_directory", "rss"},
		GitHubOrgsQueries: []string{"developer tools", "API platform"},
		RSSFeedURLs:       []string{"https://example.com/feed.xml"},
	}

	reg := sources.BuildFromConfig(cfg)
	names := reg.Names()

	want := map[string]bool{
		"github_orgs:developer tools":      true,
		"github_orgs:API platform":         true,
		"product_hunt":                     true,
		"yc_directory":                     true,
		"ai_directory":                     true,
		"saas_directory":                   true,
		"rss:https://example.com/feed.xml": true,
	}
	if len(names) != len(want) {
		t.Fatalf("expected %d sources, got %d: %v", len(want), len(names), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected source name: %s", n)
		}
	}
}

func TestBuildFromConfig_UnknownSourceIgnored(t *testing.T) {
	cfg := &config.Config{DiscoverySources: []string{"unknown_source"}}
	reg := sources.BuildFromConfig(cfg)
	if len(reg.Names()) != 0 {
		t.Fatalf("expected no sources for unknown name, got %v", reg.Names())
	}
}
