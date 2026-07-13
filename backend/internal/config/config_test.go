package config_test

import (
	"testing"

	"watchup/automation/internal/config"
)

// setEnv sets an env var for the duration of the test and restores the
// prior value (or unsets it) afterward.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func TestLoad_TrimsCRLFFromEnvValues(t *testing.T) {
	// A trailing \r is what Docker's env_file parser leaves behind when a
	// .env file crosses Windows->Linux with CRLF line endings — this exact
	// scenario caused a real "invalid credentials" bug (the loaded password
	// silently had a trailing \r the typed password didn't).
	setEnv(t, "ADMIN_PASSWORD", "secret123\r")
	setEnv(t, "JWT_SECRET", "test-secret\r")
	setEnv(t, "AI_PROVIDER", "groq")
	setEnv(t, "DAILY_LIMIT", "25\r")
	setEnv(t, "SEND_MODE", "manual")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminPassword != "secret123" {
		t.Errorf("expected trimmed password %q, got %q", "secret123", cfg.AdminPassword)
	}
	if cfg.JWTSecret != "test-secret" {
		t.Errorf("expected trimmed JWT secret %q, got %q", "test-secret", cfg.JWTSecret)
	}
	if cfg.DailyLimit != 25 {
		t.Errorf("expected trimmed+parsed DailyLimit 25, got %d", cfg.DailyLimit)
	}
}

func TestLoad_TrimsPlainWhitespaceToo(t *testing.T) {
	setEnv(t, "ADMIN_PASSWORD", "  secret123  ")
	setEnv(t, "AI_PROVIDER", "groq")
	setEnv(t, "JWT_SECRET", "test-secret")
	setEnv(t, "SEND_MODE", "manual")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AdminPassword != "secret123" {
		t.Errorf("expected trimmed password %q, got %q", "secret123", cfg.AdminPassword)
	}
}

func TestLoad_WhitespaceOnlyValueFallsBackToDefault(t *testing.T) {
	setEnv(t, "SEND_MODE", "   \r")
	setEnv(t, "AI_PROVIDER", "groq")
	setEnv(t, "JWT_SECRET", "test-secret")
	setEnv(t, "ADMIN_PASSWORD", "secret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SendMode != "manual" {
		t.Errorf("expected whitespace-only value to fall back to default %q, got %q", "manual", cfg.SendMode)
	}
}
