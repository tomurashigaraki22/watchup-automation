package imap_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/email/imap"
	"watchup/automation/internal/testutil"
)

type fakeFetcher struct {
	messages []imap.Message
	err      error
}

func (f *fakeFetcher) FetchRecent(_ context.Context, _ time.Time) ([]imap.Message, error) {
	return f.messages, f.err
}

func seedSentEmail(t *testing.T, repo *repository.Repositories, messageID string) (*models.Company, *models.Contact, *models.Email) {
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
	email := &models.Email{CompanyID: company.ID, ContactID: contact.ID, Subject: "Hi", Status: models.EmailStatusSent, MessageID: messageID}
	if err := repo.Emails.Create(ctx, email); err != nil {
		t.Fatalf("seed email: %v", err)
	}
	return company, contact, email
}

func TestScan_MatchesReplyByInReplyTo_MarksRepliedAndCancelsFollowups(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, _, email := seedSentEmail(t, repo, "<orig@watchup.space>")

	pending := &models.Followup{EmailID: email.ID, Sequence: 1, ScheduledAt: time.Now()}
	if err := repo.Followups.Create(ctx, pending); err != nil {
		t.Fatalf("seed followup: %v", err)
	}

	fetcher := &fakeFetcher{messages: []imap.Message{
		{MessageID: "<reply1@acme.com>", InReplyTo: "<orig@watchup.space>", From: "founder@acme.com", Subject: "Re: Hi", BodyText: "Sounds interesting!"},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if !got.Replied {
		t.Fatal("expected email to be marked replied")
	}

	gotFollowup, err := repo.Followups.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("get followup: %v", err)
	}
	if !gotFollowup.Canceled {
		t.Fatal("expected pending followup to be canceled")
	}
}

func TestScan_MatchesReplyByReferences(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, _, email := seedSentEmail(t, repo, "<orig2@watchup.space>")

	fetcher := &fakeFetcher{messages: []imap.Message{
		{MessageID: "<reply2@acme.com>", References: []string{"<something-else@x>", "<orig2@watchup.space>"}, From: "founder@acme.com", BodyText: "thanks"},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if !got.Replied {
		t.Fatal("expected email to be marked replied via References match")
	}
}

func TestScan_UnmatchedMessage_NoOp(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, _, email := seedSentEmail(t, repo, "<orig3@watchup.space>")

	fetcher := &fakeFetcher{messages: []imap.Message{
		{MessageID: "<unrelated@somewhere.com>", From: "nobody@somewhere.com", Subject: "Newsletter", BodyText: "buy now"},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if got.Replied {
		t.Fatal("expected unrelated message not to mark email replied")
	}
}

func TestScan_UnsubscribeReply_Suppresses(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, contact, _ := seedSentEmail(t, repo, "<orig4@watchup.space>")

	fetcher := &fakeFetcher{messages: []imap.Message{
		{MessageID: "<reply4@acme.com>", InReplyTo: "<orig4@watchup.space>", From: "founder@acme.com", BodyText: "Please unsubscribe me from this list."},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	_, suppressed, err := repo.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		t.Fatalf("check suppression: %v", err)
	}
	if !suppressed {
		t.Fatal("expected contact to be suppressed after unsubscribe reply")
	}
}

func TestScan_HardBounce_MarksBouncedAndSuppresses(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, contact, email := seedSentEmail(t, repo, "<orig5@watchup.space>")

	pending := &models.Followup{EmailID: email.ID, Sequence: 1, ScheduledAt: time.Now()}
	if err := repo.Followups.Create(ctx, pending); err != nil {
		t.Fatalf("seed followup: %v", err)
	}

	fetcher := &fakeFetcher{messages: []imap.Message{
		{
			MessageID: "<bounce1@mailer>", From: "Mail Delivery Subsystem <mailer-daemon@hostinger.com>",
			Subject:    "Undelivered Mail Returned to Sender",
			References: []string{"<orig5@watchup.space>"},
			BodyText:   "Final-Recipient: rfc822; founder@acme.com\nDiagnostic-Code: smtp; 550 5.1.1 user unknown\nAction: failed\nStatus: 5.1.1",
		},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if !got.Bounced {
		t.Fatal("expected email to be marked bounced")
	}

	_, suppressed, err := repo.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		t.Fatalf("check suppression: %v", err)
	}
	if !suppressed {
		t.Fatal("expected hard bounce to suppress contact")
	}

	gotFollowup, err := repo.Followups.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatalf("get followup: %v", err)
	}
	if !gotFollowup.Canceled {
		t.Fatal("expected pending followup to be canceled after hard bounce")
	}
}

func TestScan_SoftBounce_MarksBouncedButDoesNotSuppress(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, contact, email := seedSentEmail(t, repo, "<orig6@watchup.space>")

	fetcher := &fakeFetcher{messages: []imap.Message{
		{
			MessageID: "<bounce2@mailer>", From: "postmaster@hostinger.com",
			Subject:    "Delivery Status Notification (Failure)",
			References: []string{"<orig6@watchup.space>"},
			BodyText:   "Status: 4.2.2\nDiagnostic-Code: smtp; 452 mailbox full",
		},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("scan: %v", err)
	}

	got, err := repo.Emails.GetByID(ctx, email.ID)
	if err != nil {
		t.Fatalf("get email: %v", err)
	}
	if !got.Bounced {
		t.Fatal("expected email to be marked bounced")
	}

	_, suppressed, err := repo.Suppressions.First(ctx, "email = ?", contact.Email)
	if err != nil {
		t.Fatalf("check suppression: %v", err)
	}
	if suppressed {
		t.Fatal("expected soft bounce not to suppress contact")
	}
}

func TestScan_AlreadyRepliedEmail_IdempotentRescan(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))
	_, _, email := seedSentEmail(t, repo, "<orig7@watchup.space>")
	email.Replied = true
	if err := repo.Emails.Update(ctx, email); err != nil {
		t.Fatalf("mark replied: %v", err)
	}

	fetcher := &fakeFetcher{messages: []imap.Message{
		{MessageID: "<reply7@acme.com>", InReplyTo: "<orig7@watchup.space>", From: "founder@acme.com", BodyText: "thanks"},
	}}
	scanner := imap.NewScanner(fetcher, repo, zap.NewNop())

	if err := scanner.Scan(ctx); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	// No assertion needed beyond "doesn't error" — rescanning an already
	// replied thread must be a safe no-op.
}
