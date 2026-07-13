package ai

import (
	"context"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
)

// Service ties a Provider to persistence: every call is logged to
// ai_generations, and results are written back onto the relevant rows.
// It depends only on the Provider interface, not any concrete
// implementation, so it works unchanged if a provider is ever swapped.
type Service struct {
	provider Provider
	repo     *repository.Repositories
	log      *zap.Logger
}

// NewService builds an ai.Service.
func NewService(provider Provider, repo *repository.Repositories, log *zap.Logger) *Service {
	return &Service{provider: provider, repo: repo, log: log}
}

// AnalyzeCompany runs analysis on company, records an ai_generations row,
// folds the result into company.Industry/Description, and advances status
// to "analyzed".
func (s *Service) AnalyzeCompany(ctx context.Context, company *models.Company) (Analysis, error) {
	in := CompanyContext{
		Name:        company.Name,
		Website:     company.Website,
		Description: company.Description,
		Industry:    company.Industry,
	}

	analysis, meta, err := s.provider.Analyze(ctx, in)
	if err != nil {
		s.log.Error("ai: analyze failed", zap.Uint("company_id", company.ID), zap.Error(err))
		return Analysis{}, err
	}
	s.recordGeneration(ctx, company.ID, models.AIGenerationKindAnalysis, meta)

	if analysis.Industry != "" {
		company.Industry = analysis.Industry
	}
	if analysis.Summary != "" {
		company.Description = analysis.Summary
	}
	company.Status = models.CompanyStatusAnalyzed
	if err := s.repo.Companies.Update(ctx, company); err != nil {
		return analysis, err
	}

	s.log.Info("ai: company analyzed", zap.Uint("company_id", company.ID), zap.Int("tokens", meta.Tokens))
	return analysis, nil
}

// GenerateEmailForContact generates a personalized partnership email for
// contact, records an ai_generations row, and persists it as a draft Email.
func (s *Service) GenerateEmailForContact(ctx context.Context, company *models.Company, contact *models.Contact, analysis Analysis, campaignID uint) (*models.Email, error) {
	in := EmailContext{
		Company: CompanyContext{
			Name: company.Name, Website: company.Website,
			Description: company.Description, Industry: company.Industry,
		},
		Analysis: analysis,
		Contact:  ContactContext{Name: contact.Name, Email: contact.Email},
	}

	generated, meta, err := s.provider.GenerateEmail(ctx, in)
	if err != nil {
		s.log.Error("ai: generate email failed", zap.Uint("company_id", company.ID), zap.Error(err))
		return nil, err
	}
	s.recordGeneration(ctx, company.ID, models.AIGenerationKindEmail, meta)

	body := ComposeEmailBody(generated)
	email := &models.Email{
		CampaignID: campaignID,
		CompanyID:  company.ID,
		ContactID:  contact.ID,
		Subject:    generated.Subject,
		Body:       body,
		BodyText:   body,
		Status:     models.EmailStatusDraft,
	}
	if err := s.repo.Emails.Create(ctx, email); err != nil {
		return nil, err
	}

	s.log.Info("ai: email generated",
		zap.Uint("company_id", company.ID), zap.Uint("contact_id", contact.ID), zap.Int("tokens", meta.Tokens))
	return email, nil
}

// GenerateFollowup generates the sequence-numbered (1/2/3 -> Day 5/12/20)
// follow-up email content and records an ai_generations row. Persisting it
// into emails/followups is Phase 9's concern (it needs reply-state checks
// this service doesn't own).
func (s *Service) GenerateFollowup(ctx context.Context, company *models.Company, originalSubject string, sequence int) (GeneratedEmail, error) {
	in := FollowupContext{
		Company: CompanyContext{
			Name: company.Name, Website: company.Website,
			Description: company.Description, Industry: company.Industry,
		},
		OriginalSubject: originalSubject,
		Sequence:        sequence,
	}

	generated, meta, err := s.provider.GenerateFollowup(ctx, in)
	if err != nil {
		s.log.Error("ai: generate followup failed",
			zap.Uint("company_id", company.ID), zap.Int("sequence", sequence), zap.Error(err))
		return GeneratedEmail{}, err
	}
	s.recordGeneration(ctx, company.ID, models.AIGenerationKindFollowup, meta)

	return generated, nil
}

func (s *Service) recordGeneration(ctx context.Context, companyID uint, kind string, meta GenerationMeta) {
	record := &models.AIGeneration{
		CompanyID: companyID,
		Kind:      kind,
		Prompt:    meta.Prompt,
		Response:  meta.Raw,
		Model:     meta.Model,
		Tokens:    meta.Tokens,
	}
	if err := s.repo.AIGenerations.Create(ctx, record); err != nil {
		s.log.Error("ai: failed to record generation", zap.Uint("company_id", companyID), zap.String("kind", kind), zap.Error(err))
	}
}

// SenderSignOff is the fixed sign-off appended to every generated email.
// Deterministic in code rather than left to the AI: a signature is an
// identity fact, not generated content, and needs guaranteed placement
// (after the CTA, before the PS) regardless of what the model outputs.
const SenderSignOff = "Best,\nRaphael"

// ComposeEmailBody folds an AI-generated email's body/CTA/sign-off/PS into
// one plain text message. Exported so the workers package can reuse it for
// followups.
func ComposeEmailBody(g GeneratedEmail) string {
	body := g.Body
	if g.CTA != "" {
		body += "\n\n" + g.CTA
	}
	body += "\n\n" + SenderSignOff
	if g.PS != "" {
		body += "\n\nP.S. " + g.PS
	}
	return body
}
