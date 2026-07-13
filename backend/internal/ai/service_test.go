package ai_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"watchup/automation/internal/ai"
	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/testutil"
)

type fakeProvider struct {
	analysis     ai.Analysis
	analysisMeta ai.GenerationMeta
	analysisErr  error

	email     ai.GeneratedEmail
	emailMeta ai.GenerationMeta
	emailErr  error

	followup     ai.GeneratedEmail
	followupMeta ai.GenerationMeta
	followupErr  error
}

func (f *fakeProvider) Analyze(_ context.Context, _ ai.CompanyContext) (ai.Analysis, ai.GenerationMeta, error) {
	return f.analysis, f.analysisMeta, f.analysisErr
}
func (f *fakeProvider) GenerateEmail(_ context.Context, _ ai.EmailContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	return f.email, f.emailMeta, f.emailErr
}
func (f *fakeProvider) GenerateFollowup(_ context.Context, _ ai.FollowupContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	return f.followup, f.followupMeta, f.followupErr
}

var _ ai.Provider = (*fakeProvider)(nil)

func TestService_AnalyzeCompany_UpdatesCompanyAndRecordsGeneration(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusCrawled}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	provider := &fakeProvider{
		analysis:     ai.Analysis{Summary: "Acme builds things.", Industry: "SaaS", ValueProposition: "fast", WatchUpAngle: "embed video"},
		analysisMeta: ai.GenerationMeta{Model: "gemini-test", Tokens: 42, Prompt: "p", Raw: "r"},
	}
	svc := ai.NewService(provider, repo, zap.NewNop())

	analysis, err := svc.AnalyzeCompany(ctx, company)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if analysis.Summary != "Acme builds things." {
		t.Errorf("unexpected analysis: %+v", analysis)
	}

	got, err := repo.Companies.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	if got.Status != models.CompanyStatusAnalyzed || got.Industry != "SaaS" || got.Description != "Acme builds things." {
		t.Errorf("unexpected company after analyze: %+v", got)
	}

	generations, err := repo.AIGenerations.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list generations: %v", err)
	}
	if len(generations) != 1 || generations[0].Kind != models.AIGenerationKindAnalysis || generations[0].Tokens != 42 {
		t.Fatalf("unexpected generations: %+v", generations)
	}
}

func TestService_AnalyzeCompany_ProviderError(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	provider := &fakeProvider{analysisErr: errors.New("boom")}
	svc := ai.NewService(provider, repo, zap.NewNop())

	if _, err := svc.AnalyzeCompany(ctx, company); err == nil {
		t.Fatal("expected error to propagate")
	}

	generations, err := repo.AIGenerations.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list generations: %v", err)
	}
	if len(generations) != 0 {
		t.Errorf("expected no generation recorded on provider failure, got %d", len(generations))
	}
}

func TestService_GenerateEmailForContact_CreatesDraftEmail(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com"}
	if err := repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	provider := &fakeProvider{
		email:     ai.GeneratedEmail{Subject: "Quick idea", Body: "Hi there.", CTA: "15 min this week?", PS: "Loved your launch."},
		emailMeta: ai.GenerationMeta{Model: "gemini-test", Tokens: 55},
	}
	svc := ai.NewService(provider, repo, zap.NewNop())

	email, err := svc.GenerateEmailForContact(ctx, company, contact, ai.Analysis{}, 1)
	if err != nil {
		t.Fatalf("generate email: %v", err)
	}
	if email.Status != models.EmailStatusDraft || email.Subject != "Quick idea" {
		t.Errorf("unexpected email: %+v", email)
	}
	if !containsAll(email.Body, "Hi there.", "15 min this week?", "P.S. Loved your launch.") {
		t.Errorf("expected composed body to include CTA and PS, got %q", email.Body)
	}

	stored, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if stored.CompanyID != company.ID || stored.ContactID != contact.ID {
		t.Errorf("unexpected stored email: %+v", stored)
	}

	generations, err := repo.AIGenerations.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list generations: %v", err)
	}
	if len(generations) != 1 || generations[0].Kind != models.AIGenerationKindEmail {
		t.Fatalf("unexpected generations: %+v", generations)
	}
}

func TestService_GenerateFollowup_RecordsGeneration(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	provider := &fakeProvider{
		followup:     ai.GeneratedEmail{Subject: "Following up", Body: "Just checking in."},
		followupMeta: ai.GenerationMeta{Model: "gemini-test", Tokens: 30},
	}
	svc := ai.NewService(provider, repo, zap.NewNop())

	generated, err := svc.GenerateFollowup(ctx, company, "Partnership idea", 1)
	if err != nil {
		t.Fatalf("generate followup: %v", err)
	}
	if generated.Subject != "Following up" {
		t.Errorf("unexpected followup: %+v", generated)
	}

	generations, err := repo.AIGenerations.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list generations: %v", err)
	}
	if len(generations) != 1 || generations[0].Kind != models.AIGenerationKindFollowup {
		t.Fatalf("unexpected generations: %+v", generations)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
