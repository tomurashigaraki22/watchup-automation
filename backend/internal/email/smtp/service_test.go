package smtp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/config"
	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/email/smtp"
	"watchup/automation/internal/testutil"
)

type fakeTransport struct {
	calls int
	fail  bool
}

func (f *fakeTransport) Send(from, to string, raw []byte) error {
	f.calls++
	if f.fail {
		return errors.New("simulated failure")
	}
	return nil
}

func newTestService(t *testing.T, ft *fakeTransport) (*smtp.Service, *repository.Repositories) {
	t.Helper()
	repo := repository.NewRepositories(testutil.NewDB(t))
	cfg := &config.Config{SenderEmail: "partnership@watchup.space", SMTPHost: "smtp.hostinger.com", SMTPPort: 465}
	sender := smtp.NewSender(cfg, smtp.WithTransport(ft))
	svc := smtp.NewService(sender, repo, "https://watchup.space", zap.NewNop())
	return svc, repo
}

func seedCompanyContactCampaign(t *testing.T, repo *repository.Repositories, dailyLimit int) (*models.Company, *models.Contact, *models.OutreachCampaign) {
	t.Helper()
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "founder@acme.com"}
	if err := repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	campaign := &models.OutreachCampaign{Name: "Test", Status: models.CampaignStatusActive, DailyLimit: dailyLimit}
	if err := repo.Campaigns.Create(ctx, campaign); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return company, contact, campaign
}

func TestService_SendEmail_Success(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTransport{}
	svc, repo := newTestService(t, ft)
	company, contact, campaign := seedCompanyContactCampaign(t, repo, 25)

	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Body: "Hello there.", Status: models.EmailStatusQueued}
	if err := repo.Emails.Create(ctx, email); err != nil {
		t.Fatalf("seed email: %v", err)
	}

	if err := svc.SendEmail(ctx, email, contact, campaign, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if ft.calls != 1 {
		t.Errorf("expected 1 transport call, got %d", ft.calls)
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if got.Status != models.EmailStatusSent || got.MessageID == "" || got.SentAt == nil {
		t.Errorf("unexpected persisted email: %+v", got)
	}
}

func TestService_SendEmail_SkipsSuppressedRecipient(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTransport{}
	svc, repo := newTestService(t, ft)
	company, contact, campaign := seedCompanyContactCampaign(t, repo, 25)

	if err := repo.Suppressions.Create(ctx, &models.Suppression{Email: contact.Email, Reason: models.SuppressionReasonUnsubscribe}); err != nil {
		t.Fatalf("seed suppression: %v", err)
	}

	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Body: "Hello.", Status: models.EmailStatusQueued}
	if err := repo.Emails.Create(ctx, email); err != nil {
		t.Fatalf("seed email: %v", err)
	}

	err := svc.SendEmail(ctx, email, contact, campaign, nil)
	if !errors.Is(err, smtp.ErrSuppressed) {
		t.Fatalf("expected ErrSuppressed, got %v", err)
	}
	if ft.calls != 0 {
		t.Errorf("expected no transport call for suppressed recipient, got %d", ft.calls)
	}
}

func TestService_SendEmail_RespectsDailyLimit(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTransport{}
	svc, repo := newTestService(t, ft)
	company, contact, campaign := seedCompanyContactCampaign(t, repo, 1)

	now := time.Now().UTC()
	alreadySent := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Prior", Status: models.EmailStatusSent, SentAt: &now}
	if err := repo.Emails.Create(ctx, alreadySent); err != nil {
		t.Fatalf("seed sent email: %v", err)
	}

	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "New", Body: "Hello.", Status: models.EmailStatusQueued}
	if err := repo.Emails.Create(ctx, email); err != nil {
		t.Fatalf("seed email: %v", err)
	}

	err := svc.SendEmail(ctx, email, contact, campaign, nil)
	if !errors.Is(err, smtp.ErrDailyLimitReached) {
		t.Fatalf("expected ErrDailyLimitReached, got %v", err)
	}
	if ft.calls != 0 {
		t.Errorf("expected no transport call once limit reached, got %d", ft.calls)
	}
}

func TestService_SendEmail_TransportFailureMarksFailed(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTransport{fail: true}
	svc, repo := newTestService(t, ft)
	company, contact, campaign := seedCompanyContactCampaign(t, repo, 25)

	email := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Body: "Hello.", Status: models.EmailStatusQueued}
	if err := repo.Emails.Create(ctx, email); err != nil {
		t.Fatalf("seed email: %v", err)
	}

	if err := svc.SendEmail(ctx, email, contact, campaign, nil); err == nil {
		t.Fatal("expected send error")
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if got.Status != models.EmailStatusFailed || got.SMTPResp == "" {
		t.Errorf("unexpected persisted email after failure: %+v", got)
	}
}

func TestService_SendEmail_ThreadsOntoOriginal(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTransport{}
	svc, repo := newTestService(t, ft)
	company, contact, campaign := seedCompanyContactCampaign(t, repo, 25)

	original := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Original", Status: models.EmailStatusSent, MessageID: "<orig@watchup.space>"}
	if err := repo.Emails.Create(ctx, original); err != nil {
		t.Fatalf("seed original: %v", err)
	}

	followupEmail := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Re: Original", Body: "Following up.", Status: models.EmailStatusQueued}
	if err := repo.Emails.Create(ctx, followupEmail); err != nil {
		t.Fatalf("seed followup email: %v", err)
	}

	if err := svc.SendEmail(ctx, followupEmail, contact, campaign, original); err != nil {
		t.Fatalf("send: %v", err)
	}
	// Threading correctness is exercised in message_test.go; here we just
	// confirm the send path accepts and completes with a threadRoot.
	if ft.calls != 1 {
		t.Errorf("expected 1 transport call, got %d", ft.calls)
	}
}
