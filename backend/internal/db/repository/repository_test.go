package repository_test

import (
	"context"
	"fmt"
	"testing"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/testutil"
)

func TestMigrate_RoundTrip(t *testing.T) {
	db := testutil.NewDB(t)
	// AutoMigrate must be safe to run again against an already-migrated schema.
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestCompanyRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Company](testutil.NewDB(t))

	c := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusDiscovered}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("expected ID to be set after create")
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Acme" || got.Status != models.CompanyStatusDiscovered {
		t.Fatalf("unexpected company: %+v", got)
	}

	got.Status = models.CompanyStatusCrawled
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	list, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Status != models.CompanyStatusCrawled {
		t.Fatalf("expected updated status in list, got %+v", list)
	}
}

func TestContactRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Contact](testutil.NewDB(t))

	contact := &models.Contact{CompanyID: 1, Email: "partnership@acme.com", Priority: 1}
	if err := repo.Create(ctx, contact); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, contact.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Email != "partnership@acme.com" {
		t.Fatalf("unexpected contact: %+v", got)
	}

	got.Verified = true
	got.VerificationScore = 95
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	list, err := repo.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || !list[0].Verified || list[0].VerificationScore != 95 {
		t.Fatalf("expected verified contact in list, got %+v", list)
	}
}

func TestCampaignRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.OutreachCampaign](testutil.NewDB(t))

	campaign := &models.OutreachCampaign{Name: "Q3 Outreach", Status: models.CampaignStatusActive, DailyLimit: 50, SendMode: models.SendModeManual}
	if err := repo.Create(ctx, campaign); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DailyLimit != 50 {
		t.Fatalf("unexpected campaign: %+v", got)
	}

	got.Status = models.CampaignStatusPaused
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	got2, err := repo.GetByID(ctx, campaign.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got2.Status != models.CampaignStatusPaused {
		t.Fatalf("expected paused, got %+v", got2)
	}
}

func TestEmailRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Email](testutil.NewDB(t))

	email := &models.Email{CampaignID: 1, CompanyID: 1, ContactID: 1, Subject: "Partnership idea", Status: models.EmailStatusDraft}
	if err := repo.Create(ctx, email); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != models.EmailStatusDraft {
		t.Fatalf("unexpected email: %+v", got)
	}

	got.Status = models.EmailStatusSent
	got.MessageID = "<abc123@watchup.space>"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	list, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].MessageID != "<abc123@watchup.space>" {
		t.Fatalf("expected sent email with message id, got %+v", list)
	}
}

func TestFollowupRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Followup](testutil.NewDB(t))

	f := &models.Followup{EmailID: 1, Sequence: 1}
	if err := repo.Create(ctx, f); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	got.Sent = true
	got.Canceled = false
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	got2, err := repo.GetByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if !got2.Sent {
		t.Fatalf("expected sent=true, got %+v", got2)
	}
}

func TestAIGenerationRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.AIGeneration](testutil.NewDB(t))

	g := &models.AIGeneration{CompanyID: 1, Kind: models.AIGenerationKindAnalysis, Model: "gemini-2.5-flash", Tokens: 512}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Kind != models.AIGenerationKindAnalysis || got.Tokens != 512 {
		t.Fatalf("unexpected generation: %+v", got)
	}
}

func TestSuppressionRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Suppression](testutil.NewDB(t))

	s := &models.Suppression{Email: "bounced@example.com", Reason: models.SuppressionReasonHardBounce}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := repo.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Reason != models.SuppressionReasonHardBounce {
		t.Fatalf("unexpected suppression list: %+v", list)
	}
}

func TestAuditLogRepository_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.AuditLog](testutil.NewDB(t))

	a := &models.AuditLog{Actor: "system", Action: "company.discovered", Entity: "company", EntityID: 1}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Action != "company.discovered" {
		t.Fatalf("unexpected audit log: %+v", got)
	}
}

func TestRepository_Count(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Company](testutil.NewDB(t))

	statuses := []string{models.CompanyStatusDiscovered, models.CompanyStatusDiscovered, models.CompanyStatusCrawled}
	for i, status := range statuses {
		website := fmt.Sprintf("https://%s-%d.example", status, i)
		if err := repo.Create(ctx, &models.Company{Name: status, Website: website, Status: status}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	total, err := repo.Count(ctx, "")
	if err != nil {
		t.Fatalf("count all: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 total, got %d", total)
	}

	discovered, err := repo.Count(ctx, "status = ?", models.CompanyStatusDiscovered)
	if err != nil {
		t.Fatalf("count discovered: %v", err)
	}
	if discovered != 2 {
		t.Fatalf("expected 2 discovered, got %d", discovered)
	}
}

func TestRepository_ListWhere(t *testing.T) {
	ctx := context.Background()
	repo := repository.New[models.Company](testutil.NewDB(t))

	if err := repo.Create(ctx, &models.Company{Name: "A", Website: "https://a.example", Status: models.CompanyStatusDiscovered}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(ctx, &models.Company{Name: "B", Website: "https://b.example", Status: models.CompanyStatusCrawled}); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := repo.ListWhere(ctx, 0, 0, "status = ?", models.CompanyStatusCrawled)
	if err != nil {
		t.Fatalf("list where: %v", err)
	}
	if len(list) != 1 || list[0].Name != "B" {
		t.Fatalf("expected only B, got %+v", list)
	}
}

func TestNewRepositories_AllWired(t *testing.T) {
	repos := repository.NewRepositories(testutil.NewDB(t))
	if repos.Companies == nil || repos.Contacts == nil || repos.Campaigns == nil ||
		repos.Emails == nil || repos.Followups == nil || repos.AIGenerations == nil ||
		repos.Suppressions == nil || repos.AuditLogs == nil {
		t.Fatal("expected all repositories to be non-nil")
	}
}
