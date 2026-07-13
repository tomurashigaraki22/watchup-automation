package validation_test

import (
	"context"
	"net"
	"testing"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/testutil"
	"watchup/automation/internal/validation"
)

func TestService_ValidateContact_PersistsResult(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))

	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(map[string]bool{"acme.com": true})))
	svc := validation.NewService(repo, v, zap.NewNop())

	contact := &models.Contact{CompanyID: 1, Email: "partnership@acme.com"}
	if err := repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	result, err := svc.ValidateContact(ctx, contact)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got %+v", result)
	}

	got, err := repo.Contacts.GetByID(ctx, contact.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}
	if !got.Verified || got.VerificationScore != result.Score {
		t.Errorf("expected persisted verified=%v score=%d, got verified=%v score=%d",
			true, result.Score, got.Verified, got.VerificationScore)
	}
}

func TestService_ValidateContact_LowScoreForDisposable(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRepositories(testutil.NewDB(t))

	v := validation.NewValidator(false, validation.WithMXLookup(func(_ context.Context, _ string) ([]*net.MX, error) {
		t.Fatal("MX lookup should not be reached for a disposable domain")
		return nil, nil
	}))
	svc := validation.NewService(repo, v, zap.NewNop())

	contact := &models.Contact{CompanyID: 1, Email: "throwaway@mailinator.com"}
	if err := repo.Contacts.Create(ctx, contact); err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	if _, err := svc.ValidateContact(ctx, contact); err != nil {
		t.Fatalf("validate: %v", err)
	}

	got, err := repo.Contacts.GetByID(ctx, contact.ID)
	if err != nil {
		t.Fatalf("get contact: %v", err)
	}
	if got.Verified {
		t.Error("expected disposable contact to be unverified")
	}
	if got.VerificationScore >= 60 {
		t.Errorf("expected low score, got %d", got.VerificationScore)
	}
}
