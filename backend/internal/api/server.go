package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"watchup/automation/internal/config"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/queue"
)

// Server wires the Fiber app together with its dependencies.
type Server struct {
	App   *fiber.App
	cfg   *config.Config
	db    *gorm.DB
	repos *repository.Repositories
	queue *queue.Queue // may be nil (e.g. in tests that don't exercise send-triggering routes)
	log   *zap.Logger
}

// New builds the Fiber app and registers routes. q may be nil if the caller
// doesn't need queue-backed routes (POST /emails/:id/send) to work.
func New(cfg *config.Config, gdb *gorm.DB, repos *repository.Repositories, q *queue.Queue, log *zap.Logger) *Server {
	app := fiber.New(fiber.Config{
		AppName:               "WatchUp Outreach API",
		DisableStartupMessage: true,
	})
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowedOrigin, // "*" in dev; set CORS_ALLOWED_ORIGIN to the real dashboard origin once hosted
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PATCH, DELETE, OPTIONS",
	}))
	app.Use(limiter.New(limiter.Config{
		Max:        maxInt(cfg.RateLimitPerMin, 1),
		Expiration: time.Minute,
	}))

	s := &Server{App: app, cfg: cfg, db: gdb, repos: repos, queue: q, log: log}
	app.Use(s.requestLogger)
	s.registerRoutes()
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// registerRoutes sets up public routes (health, login, tracking links — the
// latter are clicked by external recipients and can't require a bearer
// token) and the JWT-protected /api/v1 resource routes.
func (s *Server) registerRoutes() {
	s.App.Get("/health", s.health)

	public := s.App.Group("/api/v1")
	public.Post("/auth/login", s.login)
	public.Get("/t/o/:id", s.trackOpen)
	public.Get("/t/u/:id", s.trackUnsubscribe)

	v1 := s.App.Group("/api/v1", s.jwtAuth)

	v1.Get("/metrics", s.getMetrics)
	v1.Get("/search", s.search)

	v1.Get("/companies", s.listCompanies)
	v1.Get("/companies/:id", s.getCompany)
	v1.Post("/companies/import", s.importCompaniesCSV)

	v1.Get("/campaigns", s.listCampaigns)
	v1.Post("/campaigns", s.createCampaign)
	v1.Patch("/campaigns/:id", s.updateCampaign)
	v1.Delete("/campaigns/:id", s.deleteCampaign)
	v1.Post("/campaigns/:id/pause", s.pauseCampaign)
	v1.Post("/campaigns/:id/resume", s.resumeCampaign)
	v1.Post("/campaigns/:id/clone", s.cloneCampaign)

	v1.Get("/emails", s.listEmails)
	v1.Get("/emails/:id", s.getEmail)
	v1.Patch("/emails/:id", s.updateEmail)
	v1.Post("/emails/:id/send", s.sendEmail)
}

func (s *Server) health(c *fiber.Ctx) error {
	// Verify the DB is reachable so /health reflects real readiness.
	sqlDB, err := s.db.DB()
	if err == nil {
		err = sqlDB.Ping()
	}
	status := "ok"
	code := fiber.StatusOK
	if err != nil {
		status = "degraded"
		code = fiber.StatusServiceUnavailable
	}
	return c.Status(code).JSON(fiber.Map{
		"status":  status,
		"service": "watchup-outreach-api",
		"env":     s.cfg.AppEnv,
	})
}
