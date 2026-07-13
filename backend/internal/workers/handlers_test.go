package workers_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/ai"
	"watchup/automation/internal/config"
	"watchup/automation/internal/crawler"
	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/discovery"
	emailsmtp "watchup/automation/internal/email/smtp"
	"watchup/automation/internal/queue"
	"watchup/automation/internal/sources"
	"watchup/automation/internal/testutil"
	"watchup/automation/internal/validation"
	"watchup/automation/internal/workers"
)

// --- fakes ---

type fakeAIProvider struct {
	analysis ai.Analysis
	email    ai.GeneratedEmail
	followup ai.GeneratedEmail
}

func (f *fakeAIProvider) Analyze(_ context.Context, _ ai.CompanyContext) (ai.Analysis, ai.GenerationMeta, error) {
	return f.analysis, ai.GenerationMeta{Model: "fake", Tokens: 10}, nil
}
func (f *fakeAIProvider) GenerateEmail(_ context.Context, _ ai.EmailContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	return f.email, ai.GenerationMeta{Model: "fake", Tokens: 20}, nil
}
func (f *fakeAIProvider) GenerateFollowup(_ context.Context, _ ai.FollowupContext) (ai.GeneratedEmail, ai.GenerationMeta, error) {
	return f.followup, ai.GenerationMeta{Model: "fake", Tokens: 15}, nil
}

var _ ai.Provider = (*fakeAIProvider)(nil)

type fakeSMTPTransport struct {
	calls int
}

func (f *fakeSMTPTransport) Send(from, to string, raw []byte) error {
	f.calls++
	return nil
}

type fakeCompanySource struct {
	companies []sources.Company
}

func (f fakeCompanySource) Name() string { return "fake" }
func (f fakeCompanySource) Discover(_ context.Context) ([]sources.Company, error) {
	return f.companies, nil
}

type fakeReplyScanner struct {
	calls int
	err   error
}

func (f *fakeReplyScanner) Scan(_ context.Context) error {
	f.calls++
	return f.err
}

func fakeLookupMX(known map[string]bool) func(ctx context.Context, domain string) ([]*net.MX, error) {
	return func(_ context.Context, domain string) ([]*net.MX, error) {
		if known[domain] {
			return []*net.MX{{Host: "mail." + domain + ".", Pref: 10}}, nil
		}
		return nil, errors.New("no such host")
	}
}

// --- harness ---

type harness struct {
	repo     *repository.Repositories
	queue    *queue.Queue
	handlers *workers.Handlers
	smtpTr   *fakeSMTPTransport
	ai       *fakeAIProvider
	replies  *fakeReplyScanner
}

func newHarness(t *testing.T, verifiedDomain string, discoverable []sources.Company) *harness {
	t.Helper()
	repo := repository.NewRepositories(testutil.NewDB(t))
	q := queue.NewQueue(testutil.NewRedis(t))

	cfg := &config.Config{SenderEmail: "partnership@watchup.space", SMTPHost: "smtp.hostinger.com", SMTPPort: 465}
	smtpTr := &fakeSMTPTransport{}
	sender := emailsmtp.NewSender(cfg, emailsmtp.WithTransport(smtpTr))
	smtpSvc := emailsmtp.NewService(sender, repo, "https://watchup.space", zap.NewNop())

	provider := &fakeAIProvider{
		analysis: ai.Analysis{Summary: "They build things.", Industry: "SaaS", ValueProposition: "fast", WatchUpAngle: "embed"},
		email:    ai.GeneratedEmail{Subject: "Quick idea", Body: "Hi there.", CTA: "15 min?", PS: "Nice launch."},
		followup: ai.GeneratedEmail{Subject: "Following up", Body: "Checking in."},
	}
	aiSvc := ai.NewService(provider, repo, zap.NewNop())

	known := map[string]bool{}
	if verifiedDomain != "" {
		known[verifiedDomain] = true
	}
	validator := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(known)))
	validationSvc := validation.NewService(repo, validator, zap.NewNop())

	crawlerSvc := crawler.NewService(repo, zap.NewNop())
	discoverySvc := discovery.NewService(repo, zap.NewNop())
	registry := sources.NewRegistry(fakeCompanySource{companies: discoverable})
	replies := &fakeReplyScanner{}

	h := &workers.Handlers{
		Repo:       repo,
		Discovery:  discoverySvc,
		Registry:   registry,
		Crawler:    crawlerSvc,
		Validation: validationSvc,
		AI:         aiSvc,
		SMTP:       smtpSvc,
		Queue:      q,
		Replies:    replies,
		Log:        zap.NewNop(),
	}

	return &harness{repo: repo, queue: q, handlers: h, smtpTr: smtpTr, ai: provider, replies: replies}
}

func seedActiveCampaign(t *testing.T, repo *repository.Repositories, sendMode string, dailyLimit int) *models.OutreachCampaign {
	t.Helper()
	c := &models.OutreachCampaign{Name: "Test", Status: models.CampaignStatusActive, SendMode: sendMode, DailyLimit: dailyLimit}
	if err := repo.Campaigns.Create(context.Background(), c); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return c
}

// --- tests ---

func TestHandleDiscover_InsertsCompaniesAndDoesNotError(t *testing.T) {
	h := newHarness(t, "", []sources.Company{{Name: "Acme", Website: "https://acme.com"}})
	ctx := context.Background()

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobDiscover}); err != nil {
		t.Fatalf("process discover: %v", err)
	}

	companies, err := h.repo.Companies.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list companies: %v", err)
	}
	if len(companies) != 1 || companies[0].Status != models.CompanyStatusDiscovered {
		t.Fatalf("unexpected companies: %+v", companies)
	}
}

func TestHandleCrawl_ChainsToValidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><a href="mailto:partnership@acme.com">Email</a></body></html>`))
	}))
	defer server.Close()

	h := newHarness(t, "acme.com", nil)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: server.URL, Status: models.CompanyStatusDiscovered}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobCrawl, CompanyID: company.ID}); err != nil {
		t.Fatalf("process crawl: %v", err)
	}

	got, err := h.repo.Companies.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	if got.Status != models.CompanyStatusCrawled {
		t.Fatalf("expected crawled, got %q", got.Status)
	}

	job, ok, err := h.queue.Dequeue(ctx, time.Second)
	if err != nil || !ok {
		t.Fatalf("expected chained validate job: ok=%v err=%v", ok, err)
	}
	if job.Type != queue.JobValidate || job.CompanyID != company.ID {
		t.Fatalf("unexpected chained job: %+v", job)
	}
}

func TestHandleValidate_MarksValidatedAndChainsToAnalyze(t *testing.T) {
	h := newHarness(t, "acme.com", nil)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusCrawled}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com", Priority: 1}
	if err := h.repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobValidate, CompanyID: company.ID}); err != nil {
		t.Fatalf("process validate: %v", err)
	}

	gotCompany, err := h.repo.Companies.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	if gotCompany.Status != models.CompanyStatusValidated {
		t.Fatalf("expected validated, got %q", gotCompany.Status)
	}

	gotContact, err := h.repo.Contacts.GetByID(ctx, contact.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}
	if !gotContact.Verified {
		t.Fatalf("expected contact to be verified, got %+v", gotContact)
	}

	job, ok, err := h.queue.Dequeue(ctx, time.Second)
	if err != nil || !ok || job.Type != queue.JobAnalyze {
		t.Fatalf("expected chained analyze job, got job=%+v ok=%v err=%v", job, ok, err)
	}
}

func TestHandleAnalyze_NoVerifiedContact_DoesNotGenerate(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx := context.Background()

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusValidated}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobAnalyze, CompanyID: company.ID}); err != nil {
		t.Fatalf("process analyze: %v", err)
	}

	emails, err := h.repo.Emails.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list emails: %v", err)
	}
	if len(emails) != 0 {
		t.Fatalf("expected no email generated without a verified contact, got %+v", emails)
	}

	gotCompany, err := h.repo.Companies.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("get company: %v", err)
	}
	if gotCompany.Status != models.CompanyStatusAnalyzed {
		t.Fatalf("expected analyzed status regardless, got %q", gotCompany.Status)
	}
}

func TestHandleAnalyze_ManualMode_DraftsWithoutSending(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx := context.Background()
	seedActiveCampaign(t, h.repo, models.SendModeManual, 25)

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusValidated}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com", Priority: 1, Verified: true, VerificationScore: 90}
	if err := h.repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobAnalyze, CompanyID: company.ID}); err != nil {
		t.Fatalf("process analyze: %v", err)
	}

	emails, err := h.repo.Emails.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list emails: %v", err)
	}
	if len(emails) != 1 || emails[0].Status != models.EmailStatusDraft {
		t.Fatalf("expected 1 draft email, got %+v", emails)
	}

	if _, ok, err := h.queue.Dequeue(ctx, 200*time.Millisecond); err != nil || ok {
		t.Fatalf("expected no send job enqueued in manual mode: ok=%v err=%v", ok, err)
	}
}

func TestHandleAnalyze_AutomaticMode_EnqueuesSend(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx := context.Background()
	seedActiveCampaign(t, h.repo, models.SendModeAutomatic, 25)

	company := &models.Company{Name: "Acme", Website: "https://acme.com", Status: models.CompanyStatusValidated}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com", Priority: 1, Verified: true, VerificationScore: 90}
	if err := h.repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobAnalyze, CompanyID: company.ID}); err != nil {
		t.Fatalf("process analyze: %v", err)
	}

	job, ok, err := h.queue.Dequeue(ctx, time.Second)
	if err != nil || !ok || job.Type != queue.JobSend {
		t.Fatalf("expected chained send job, got job=%+v ok=%v err=%v", job, ok, err)
	}

	if err := h.handlers.Process(ctx, job); err != nil {
		t.Fatalf("process send: %v", err)
	}
	if h.smtpTr.calls != 1 {
		t.Fatalf("expected 1 smtp send, got %d", h.smtpTr.calls)
	}

	sentEmail, err := h.repo.Emails.GetByID(ctx, job.EmailID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if sentEmail.Status != models.EmailStatusSent {
		t.Fatalf("expected sent status, got %q", sentEmail.Status)
	}

	followups, err := h.repo.Followups.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list followups: %v", err)
	}
	if len(followups) != 3 {
		t.Fatalf("expected 3 scheduled followups, got %d", len(followups))
	}
}

func TestHandleFollowup_CanceledWhenOriginalReplied(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx := context.Background()
	campaign := seedActiveCampaign(t, h.repo, models.SendModeAutomatic, 25)

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com"}
	if err := h.repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	original := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Status: models.EmailStatusSent, Replied: true, MessageID: "<orig@watchup.space>"}
	if err := h.repo.Emails.Create(ctx, original); err != nil {
		t.Fatalf("seed original: %v", err)
	}
	followup := &models.Followup{EmailID: original.ID, Sequence: 1, ScheduledAt: time.Now().Add(-time.Hour)}
	if err := h.repo.Followups.Create(ctx, followup); err != nil {
		t.Fatalf("seed followup: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobFollowup, FollowupID: followup.ID}); err != nil {
		t.Fatalf("process followup: %v", err)
	}

	got, err := h.repo.Followups.GetByID(ctx, followup.ID)
	if err != nil {
		t.Fatalf("get followup: %v", err)
	}
	if !got.Canceled || got.Sent {
		t.Fatalf("expected canceled, got %+v", got)
	}
	if h.smtpTr.calls != 0 {
		t.Fatalf("expected no send for replied thread, got %d calls", h.smtpTr.calls)
	}
}

func TestHandleFollowup_SendsThreadedAndMarksSent(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx := context.Background()
	campaign := seedActiveCampaign(t, h.repo, models.SendModeAutomatic, 25)

	company := &models.Company{Name: "Acme", Website: "https://acme.com"}
	if err := h.repo.Companies.Create(ctx, company); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	contact := &models.Contact{CompanyID: company.ID, Email: "partnership@acme.com"}
	if err := h.repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	original := &models.Email{CampaignID: campaign.ID, CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Status: models.EmailStatusSent, MessageID: "<orig@watchup.space>"}
	if err := h.repo.Emails.Create(ctx, original); err != nil {
		t.Fatalf("seed original: %v", err)
	}
	followup := &models.Followup{EmailID: original.ID, Sequence: 1, ScheduledAt: time.Now().Add(-time.Hour)}
	if err := h.repo.Followups.Create(ctx, followup); err != nil {
		t.Fatalf("seed followup: %v", err)
	}

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobFollowup, FollowupID: followup.ID}); err != nil {
		t.Fatalf("process followup: %v", err)
	}

	if h.smtpTr.calls != 1 {
		t.Fatalf("expected 1 smtp send, got %d", h.smtpTr.calls)
	}

	got, err := h.repo.Followups.GetByID(ctx, followup.ID)
	if err != nil {
		t.Fatalf("get followup: %v", err)
	}
	if !got.Sent || got.SentAt == nil {
		t.Fatalf("expected sent, got %+v", got)
	}

	emails, err := h.repo.Emails.List(ctx, 0, 0)
	if err != nil {
		t.Fatalf("list emails: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("expected original + followup email rows, got %d", len(emails))
	}
}

func TestHandleReplyScan_CallsScanner(t *testing.T) {
	h := newHarness(t, "", nil)
	ctx := context.Background()

	if err := h.handlers.Process(ctx, queue.Job{Type: queue.JobReplyScan}); err != nil {
		t.Fatalf("process reply scan: %v", err)
	}
	if h.replies.calls != 1 {
		t.Fatalf("expected scanner to be called once, got %d", h.replies.calls)
	}
}

func TestProcess_UnknownJobType(t *testing.T) {
	h := newHarness(t, "", nil)
	if err := h.handlers.Process(context.Background(), queue.Job{Type: "bogus"}); err == nil {
		t.Fatal("expected error for unknown job type")
	}
}
