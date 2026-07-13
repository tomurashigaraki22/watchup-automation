// Package ai defines the abstract AI provider interface used for company
// analysis and email generation. This build wires up exactly one concrete
// implementation — internal/ai/gemini — per explicit product decision;
// config.Load() rejects any other AI_PROVIDER value at boot.
package ai

import "context"

// CompanyContext is what we know about a company going into analysis or
// email generation.
type CompanyContext struct {
	Name        string
	Website     string
	Description string
	Industry    string
}

// Analysis is the AI's understanding of a company, per the PRD's required shape.
type Analysis struct {
	Summary          string `json:"summary"`
	Industry         string `json:"industry"`
	ValueProposition string `json:"value_proposition"`
	WatchUpAngle     string `json:"watchup_angle"`
}

// ContactContext is the recipient of a generated email.
type ContactContext struct {
	Name  string
	Email string
}

// EmailContext is the input for generating an initial partnership email.
type EmailContext struct {
	Company  CompanyContext
	Analysis Analysis
	Contact  ContactContext
}

// FollowupContext is the input for generating a follow-up in the sequence.
// Sequence is 1, 2, or 3, corresponding to the Day 5 / 12 / 20 emails.
type FollowupContext struct {
	Company         CompanyContext
	OriginalSubject string
	Sequence        int
}

// GeneratedEmail is a personalized outreach email.
type GeneratedEmail struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	CTA     string `json:"cta"`
	PS      string `json:"ps"`
}

// GenerationMeta captures what to persist in ai_generations for audit/cost tracking.
type GenerationMeta struct {
	Model  string
	Tokens int
	Prompt string
	Raw    string
}

// Provider is the abstract AI interface. Every method also returns
// GenerationMeta so callers can persist an ai_generations row.
type Provider interface {
	Analyze(ctx context.Context, in CompanyContext) (Analysis, GenerationMeta, error)
	GenerateEmail(ctx context.Context, in EmailContext) (GeneratedEmail, GenerationMeta, error)
	GenerateFollowup(ctx context.Context, in FollowupContext) (GeneratedEmail, GenerationMeta, error)
}
