package validation_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"watchup/automation/internal/validation"
)

func fakeLookupMX(known map[string]bool) func(ctx context.Context, domain string) ([]*net.MX, error) {
	return func(_ context.Context, domain string) ([]*net.MX, error) {
		if known[domain] {
			return []*net.MX{{Host: "mail." + domain + ".", Pref: 10}}, nil
		}
		return nil, errors.New("no such host")
	}
}

func TestValidate_InvalidSyntax(t *testing.T) {
	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(nil)))
	result, err := v.Validate(context.Background(), "not-an-email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid || result.Score != 0 {
		t.Errorf("expected invalid/score 0, got %+v", result)
	}
}

func TestValidate_DisposableDomain(t *testing.T) {
	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(map[string]bool{"mailinator.com": true})))
	result, err := v.Validate(context.Background(), "test@mailinator.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid || !result.Disposable {
		t.Errorf("expected disposable/invalid, got %+v", result)
	}
	if result.Score >= 60 {
		t.Errorf("expected low score for disposable domain, got %d", result.Score)
	}
}

func TestValidate_NoMXRecords(t *testing.T) {
	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(nil)))
	result, err := v.Validate(context.Background(), "person@no-mx-example.invalid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid || result.HasMX {
		t.Errorf("expected invalid/no MX, got %+v", result)
	}
}

func TestValidate_KnownGoodDomain_ScoresHigh(t *testing.T) {
	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(map[string]bool{"acme.com": true})))
	result, err := v.Validate(context.Background(), "partnership@acme.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected valid for known-good domain, got %+v", result)
	}
	if result.Score < 60 {
		t.Errorf("expected high score, got %d", result.Score)
	}
	if !result.HasMX || result.Disposable {
		t.Errorf("unexpected flags: %+v", result)
	}
}

func TestValidate_ScoreNeverExceeds100(t *testing.T) {
	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(map[string]bool{"acme.com": true})))
	result, err := v.Validate(context.Background(), "info@acme.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Score > 100 {
		t.Errorf("score should be capped at 100, got %d", result.Score)
	}
}

func TestValidate_EmptyDomain(t *testing.T) {
	v := validation.NewValidator(false, validation.WithMXLookup(fakeLookupMX(nil)))
	result, err := v.Validate(context.Background(), "nobody@")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Errorf("expected invalid for empty domain, got %+v", result)
	}
}
