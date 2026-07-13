// Package gemini is the sole concrete AI provider for this build, calling
// Google's Gemini REST generateContent endpoint. It implements ai.Provider.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"watchup/automation/internal/ai"
	"watchup/automation/internal/ai/prompts"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Client is the Gemini ai.Provider implementation.
type Client struct {
	apiKey     string
	model      string
	BaseURL    string // overridable in tests; defaults to the real Gemini API
	httpClient *http.Client
	prompts    *prompts.Set
}

var _ ai.Provider = (*Client)(nil)

// New builds a Gemini client. promptsDir must contain analysis.txt,
// email.txt, and followup_1/2/3.txt.
func New(apiKey, model, promptsDir string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini: GEMINI_API_KEY is required")
	}
	if model == "" {
		return nil, fmt.Errorf("gemini: model is required")
	}
	p, err := prompts.Load(promptsDir)
	if err != nil {
		return nil, fmt.Errorf("gemini: load prompts: %w", err)
	}
	return &Client{
		apiKey:     apiKey,
		model:      model,
		BaseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		prompts:    p,
	}, nil
}

func (c *Client) Analyze(ctx context.Context, in ai.CompanyContext) (ai.Analysis, ai.GenerationMeta, error) {
	prompt, err := prompts.Render(c.prompts.Analysis, in)
	if err != nil {
		return ai.Analysis{}, ai.GenerationMeta{}, fmt.Errorf("gemini: analyze: %w", err)
	}
	text, tokens, err := c.generate(ctx, prompt, 0.4)
	if err != nil {
		return ai.Analysis{}, ai.GenerationMeta{}, fmt.Errorf("gemini: analyze: %w", err)
	}
	var analysis ai.Analysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		return ai.Analysis{}, ai.GenerationMeta{}, fmt.Errorf("gemini: analyze: parse response: %w; raw=%s", err, text)
	}
	return analysis, ai.GenerationMeta{Model: c.model, Tokens: tokens, Prompt: prompt, Raw: text}, nil
}

func (c *Client) GenerateEmail(ctx context.Context, in ai.EmailContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	prompt, err := prompts.Render(c.prompts.Email, in)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate email: %w", err)
	}
	// Higher temperature: every email must be unique, never templated-feeling.
	text, tokens, err := c.generate(ctx, prompt, 0.9)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate email: %w", err)
	}
	var email ai.GeneratedEmail
	if err := json.Unmarshal([]byte(text), &email); err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate email: parse response: %w; raw=%s", err, text)
	}
	return email, ai.GenerationMeta{Model: c.model, Tokens: tokens, Prompt: prompt, Raw: text}, nil
}

func (c *Client) GenerateFollowup(ctx context.Context, in ai.FollowupContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	idx := in.Sequence - 1
	if idx < 0 || idx > 2 {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate followup: invalid sequence %d (must be 1-3)", in.Sequence)
	}
	prompt, err := prompts.Render(c.prompts.Followups[idx], in)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate followup: %w", err)
	}
	text, tokens, err := c.generate(ctx, prompt, 0.9)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate followup: %w", err)
	}
	var email ai.GeneratedEmail
	if err := json.Unmarshal([]byte(text), &email); err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("gemini: generate followup: parse response: %w; raw=%s", err, text)
	}
	return email, ai.GenerationMeta{Model: c.model, Tokens: tokens, Prompt: prompt, Raw: text}, nil
}

type generateContentRequest struct {
	Contents         []content         `json:"contents"`
	GenerationConfig *generationConfig `json:"generationConfig,omitempty"`
}
type content struct {
	Parts []part `json:"parts"`
}
type part struct {
	Text string `json:"text"`
}
type generationConfig struct {
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
}

type generateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// generate calls Gemini's generateContent, requesting JSON output, retrying
// transient failures up to 3 times with exponential backoff.
func (c *Client) generate(ctx context.Context, prompt string, temperature float64) (text string, tokens int, err error) {
	reqBody := generateContentRequest{
		Contents:         []content{{Parts: []part{{Text: prompt}}}},
		GenerationConfig: &generationConfig{ResponseMimeType: "application/json", Temperature: temperature},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.BaseURL, c.model, c.apiKey)

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		text, tokens, lastErr = c.doGenerate(ctx, url, payload)
		if lastErr == nil {
			return text, tokens, nil
		}
		if attempt < maxAttempts {
			backoff := time.Duration(attempt*attempt) * 500 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", 0, ctx.Err()
			}
		}
	}
	return "", 0, fmt.Errorf("generateContent failed after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) doGenerate(ctx context.Context, url string, payload []byte) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var parsed generateContentResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", 0, fmt.Errorf("empty candidates in response")
	}
	return parsed.Candidates[0].Content.Parts[0].Text, parsed.UsageMetadata.TotalTokenCount, nil
}
