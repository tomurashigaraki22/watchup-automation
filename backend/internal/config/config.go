package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, loaded from environment / .env.
// Secrets only ever come from the environment — never hardcoded.
type Config struct {
	// Server
	AppEnv            string
	APIPort           string
	JWTSecret         string
	PublicBaseURL     string // used to build tracking-pixel / unsubscribe links embedded in outreach emails
	AdminUsername     string // dashboard login — this build has one admin user, no user table
	AdminPassword     string
	RateLimitPerMin   int
	CORSAllowedOrigin string // "*" in dev; the dashboard's real origin (e.g. https://app.watchup.space) once hosted

	// Postgres
	PostgresHost     string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDB       string

	// Redis
	RedisAddr string

	// AI — GROQ ONLY
	AIProvider string
	GroqAPIKey string
	GroqModel  string
	PromptsDir string

	// Retained but unused by the active provider wiring — the gemini
	// package still compiles/tests as an alternate ai.Provider implementation.
	GeminiAPIKey string
	GeminiModel  string

	// Email — Hostinger
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	IMAPHost     string
	IMAPPort     int

	// Sending policy
	SenderEmail      string
	DailyLimit       int
	SendDelayMinSecs int
	SendDelayMaxSecs int
	SendMode         string // manual | automatic

	// Discovery
	DiscoverySources []string // enabled scheduled sources: github, rss, product_hunt, yc_directory, ai_directory, saas_directory
	GitHubToken      string
	GitHubOrgsQuery  string
	RSSFeedURLs      []string

	// Validation
	ValidationSMTPProbe bool // live RCPT TO probe; off by default to protect sender reputation
}

// Load reads configuration from a .env file (if present) and the process
// environment, applies sane defaults, and validates required fields.
func Load() (*Config, error) {
	// .env is optional in containerized deploys where env is injected directly.
	_ = godotenv.Load()

	c := &Config{
		AppEnv:            getEnv("APP_ENV", "development"),
		APIPort:           getEnv("API_PORT", "8080"),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		PublicBaseURL:     getEnv("APP_BASE_URL", "http://localhost:8080"),
		AdminUsername:     getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:     getEnv("ADMIN_PASSWORD", ""),
		RateLimitPerMin:   getEnvInt("API_RATE_LIMIT_PER_MIN", 120),
		CORSAllowedOrigin: getEnv("CORS_ALLOWED_ORIGIN", "*"),

		PostgresHost:     getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresUser:     getEnv("POSTGRES_USER", "watchup"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", "watchup"),
		PostgresDB:       getEnv("POSTGRES_DB", "watchup"),

		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),

		AIProvider: strings.ToLower(getEnv("AI_PROVIDER", "groq")),
		GroqAPIKey: getEnv("GROQ_API_KEY", ""),
		GroqModel:  getEnv("GROQ_MODEL", "llama-3.3-70b-versatile"),
		PromptsDir: getEnv("PROMPTS_DIR", "../prompts"),

		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
		GeminiModel:  getEnv("GEMINI_MODEL", "gemini-2.0-flash"),

		SMTPHost:     getEnv("SMTP_HOST", "smtp.hostinger.com"),
		SMTPPort:     getEnvInt("SMTP_PORT", 465),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		IMAPHost:     getEnv("IMAP_HOST", "imap.hostinger.com"),
		IMAPPort:     getEnvInt("IMAP_PORT", 993),

		SenderEmail:      getEnv("SENDER_EMAIL", "partnership@watchup.space"),
		DailyLimit:       getEnvInt("DAILY_LIMIT", 25),
		SendDelayMinSecs: getEnvInt("SEND_DELAY_MIN_SECONDS", 45),
		SendDelayMaxSecs: getEnvInt("SEND_DELAY_MAX_SECONDS", 240),
		SendMode:         strings.ToLower(getEnv("SEND_MODE", "manual")),

		DiscoverySources: getEnvList("DISCOVERY_SOURCES", []string{"github"}),
		GitHubToken:      getEnv("GITHUB_TOKEN", ""),
		GitHubOrgsQuery:  getEnv("GITHUB_ORGS_QUERY", "video analytics"),
		RSSFeedURLs:      getEnvList("RSS_FEED_URLS", nil),

		ValidationSMTPProbe: getEnvBool("EMAIL_VALIDATION_SMTP_PROBE", false),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// validate enforces invariants that must hold for the app to run safely.
func (c *Config) validate() error {
	// Groq-only guarantee: any other provider is rejected at boot.
	if c.AIProvider != "groq" {
		return fmt.Errorf("config: AI_PROVIDER must be \"groq\" (this build is Groq-only), got %q", c.AIProvider)
	}
	if c.SendMode != "manual" && c.SendMode != "automatic" {
		return fmt.Errorf("config: SEND_MODE must be \"manual\" or \"automatic\", got %q", c.SendMode)
	}
	if c.DailyLimit <= 0 {
		return fmt.Errorf("config: DAILY_LIMIT must be > 0, got %d", c.DailyLimit)
	}
	if c.SendDelayMinSecs < 0 || c.SendDelayMaxSecs < c.SendDelayMinSecs {
		return fmt.Errorf("config: invalid send delay window [%d, %d]", c.SendDelayMinSecs, c.SendDelayMaxSecs)
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("config: JWT_SECRET is required")
	}
	return nil
}

// PostgresDSN builds the GORM Postgres connection string.
func (c *Config) PostgresDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDB,
	)
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// getEnvList reads a comma-separated env var into a trimmed, non-empty slice.
func getEnvList(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
