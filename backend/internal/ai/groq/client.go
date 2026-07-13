// Package groq is the active concrete AI provider for this build, calling
// Groq's OpenAI-compatible chat completions endpoint. It implements ai.Provider.
package groq

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

const defaultBaseURL = "https://api.groq.com/openai/v1"

// Client is the Groq ai.Provider implementation.
type Client struct {
	apiKey     string
	model      string
	BaseURL    string // overridable in tests; defaults to the real Groq API
	httpClient *http.Client
	prompts    *prompts.Set
}

var _ ai.Provider = (*Client)(nil)

// New builds a Groq client. promptsDir must contain analysis.txt, email.txt,
// and followup_1/2/3.txt.
func New(apiKey, model, promptsDir string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("groq: GROQ_API_KEY is required")
	}
	if model == "" {
		return nil, fmt.Errorf("groq: model is required")
	}
	p, err := prompts.Load(promptsDir)
	if err != nil {
		return nil, fmt.Errorf("groq: load prompts: %w", err)
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
		return ai.Analysis{}, ai.GenerationMeta{}, fmt.Errorf("groq: analyze: %w", err)
	}
	text, tokens, err := c.generate(ctx, prompt, 0.4)
	if err != nil {
		return ai.Analysis{}, ai.GenerationMeta{}, fmt.Errorf("groq: analyze: %w", err)
	}
	var analysis ai.Analysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		return ai.Analysis{}, ai.GenerationMeta{}, fmt.Errorf("groq: analyze: parse response: %w; raw=%s", err, text)
	}
	return analysis, ai.GenerationMeta{Model: c.model, Tokens: tokens, Prompt: prompt, Raw: text}, nil
}

func (c *Client) GenerateEmail(ctx context.Context, in ai.EmailContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	prompt, err := prompts.Render(c.prompts.Email, in)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate email: %w", err)
	}
	// Higher temperature: every email must be unique, never templated-feeling.
	text, tokens, err := c.generate(ctx, prompt, 0.9)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate email: %w", err)
	}
	var email ai.GeneratedEmail
	if err := json.Unmarshal([]byte(text), &email); err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate email: parse response: %w; raw=%s", err, text)
	}
	return email, ai.GenerationMeta{Model: c.model, Tokens: tokens, Prompt: prompt, Raw: text}, nil
}

func (c *Client) GenerateFollowup(ctx context.Context, in ai.FollowupContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	idx := in.Sequence - 1
	if idx < 0 || idx > 2 {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate followup: invalid sequence %d (must be 1-3)", in.Sequence)
	}
	prompt, err := prompts.Render(c.prompts.Followups[idx], in)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate followup: %w", err)
	}
	text, tokens, err := c.generate(ctx, prompt, 0.9)
	if err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate followup: %w", err)
	}
	var email ai.GeneratedEmail
	if err := json.Unmarshal([]byte(text), &email); err != nil {
		return ai.GeneratedEmail{}, ai.GenerationMeta{}, fmt.Errorf("groq: generate followup: parse response: %w; raw=%s", err, text)
	}
	return email, ai.GenerationMeta{Model: c.model, Tokens: tokens, Prompt: prompt, Raw: text}, nil
}

type chatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// generate calls Groq's OpenAI-compatible chat completions endpoint,
// requesting JSON output, retrying transient failures up to 3 times with
// exponential backoff.
func (c *Client) generate(ctx context.Context, prompt string, temperature float64) (text string, tokens int, err error) {
	reqBody := chatCompletionRequest{
		Model:          c.model,
		Messages:       []chatMessage{{Role: "user", Content: prompt}},
		Temperature:    temperature,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"

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
	return "", 0, fmt.Errorf("chat completion failed after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) doGenerate(ctx context.Context, url string, payload []byte) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", 0, fmt.Errorf("empty choices in response")
	}
	return parsed.Choices[0].Message.Content, parsed.Usage.TotalTokens, nil
}
