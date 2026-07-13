package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// GitHubOrgsSource discovers companies via GitHub's public Search API,
// searching for organizations matching Query. An optional token raises the
// unauthenticated rate limit; requests work without one.
type GitHubOrgsSource struct {
	Query   string
	Token   string
	BaseURL string // overridable in tests; defaults to https://api.github.com
	client  *http.Client
}

// NewGitHubOrgsSource builds a GitHubOrgsSource. token may be empty.
func NewGitHubOrgsSource(query, token string) *GitHubOrgsSource {
	return &GitHubOrgsSource{
		Query:   query,
		Token:   token,
		BaseURL: "https://api.github.com",
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *GitHubOrgsSource) Name() string { return "github_orgs:" + s.Query }

type githubSearchResponse struct {
	Items []struct {
		Login   string `json:"login"`
		HTMLURL string `json:"html_url"`
	} `json:"items"`
}

type githubOrgDetail struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	Blog  string `json:"blog"`
	Bio   string `json:"bio"`
}

func (s *GitHubOrgsSource) Discover(ctx context.Context) ([]Company, error) {
	searchURL := fmt.Sprintf("%s/search/users?q=%s&per_page=25", s.BaseURL, url.QueryEscape(s.Query+" type:org"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("sources: github: build search request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sources: github: search request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sources: github: search returned status %d", resp.StatusCode)
	}

	var search githubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&search); err != nil {
		return nil, fmt.Errorf("sources: github: decode search response: %w", err)
	}

	companies := make([]Company, 0, len(search.Items))
	for _, item := range search.Items {
		detail, err := s.fetchOrgDetail(ctx, item.Login)
		if err != nil {
			// One org's detail failing shouldn't abort the whole discovery run.
			companies = append(companies, Company{Name: item.Login, Website: item.HTMLURL})
			continue
		}
		website := detail.Blog
		if website == "" {
			website = item.HTMLURL
		}
		name := detail.Name
		if name == "" {
			name = detail.Login
		}
		companies = append(companies, Company{
			Name:        name,
			Website:     website,
			Description: detail.Bio,
		})
	}
	return companies, nil
}

func (s *GitHubOrgsSource) fetchOrgDetail(ctx context.Context, login string) (*githubOrgDetail, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+"/orgs/"+url.PathEscape(login), nil)
	if err != nil {
		return nil, err
	}
	s.setHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sources: github: org detail status %d", resp.StatusCode)
	}

	var detail githubOrgDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (s *GitHubOrgsSource) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
}
