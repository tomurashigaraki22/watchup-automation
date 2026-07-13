package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/ai"
	"watchup/automation/internal/ai/groq"
	"watchup/automation/internal/config"
	"watchup/automation/internal/crawler"
	"watchup/automation/internal/db"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/discovery"
	emailimap "watchup/automation/internal/email/imap"
	emailsmtp "watchup/automation/internal/email/smtp"
	"watchup/automation/internal/logging"
	"watchup/automation/internal/queue"
	"watchup/automation/internal/sources"
	"watchup/automation/internal/validation"
	"watchup/automation/internal/workers"
)

// The worker consumes pipeline jobs from Redis and dispatches them through
// the full pipeline: discovery -> crawl -> validate -> analyze -> generate
// -> send -> followup, plus reply/bounce scanning.
func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log, err := logging.New(cfg.AppEnv)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	gdb, err := db.Connect(cfg)
	if err != nil {
		log.Fatal("database connect failed", zap.Error(err))
	}
	if err := db.Migrate(gdb); err != nil {
		log.Fatal("database migrate failed", zap.Error(err))
	}
	repo := repository.NewRepositories(gdb)

	ctx := context.Background()
	rdb, err := queue.NewRedis(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatal("redis connect failed", zap.Error(err))
	}
	defer func() { _ = rdb.Close() }()
	q := queue.NewQueue(rdb)

	// AI — Groq only (config.Load already enforces this at boot).
	groqClient, err := groq.New(cfg.GroqAPIKey, cfg.GroqModel, cfg.PromptsDir)
	if err != nil {
		log.Fatal("groq client init failed", zap.Error(err))
	}
	aiService := ai.NewService(groqClient, repo, log)

	// SMTP sending.
	sender := emailsmtp.NewSender(cfg)
	smtpService := emailsmtp.NewService(sender, repo, cfg.PublicBaseURL, log)

	// Crawling.
	crawlerService := crawler.NewService(repo, log)

	// Validation.
	validator := validation.NewValidator(cfg.ValidationSMTPProbe)
	validationService := validation.NewService(repo, validator, log)

	// Discovery.
	discoveryService := discovery.NewService(repo, log)
	registry := sources.BuildFromConfig(cfg)

	// Reply/bounce/unsubscribe scanning.
	fetcher := emailimap.NewClientFetcher(cfg.IMAPHost, cfg.IMAPPort, cfg.SMTPUsername, cfg.SMTPPassword)
	scanner := emailimap.NewScanner(fetcher, repo, log)

	handlers := &workers.Handlers{
		Repo:       repo,
		Discovery:  discoveryService,
		Registry:   registry,
		Crawler:    crawlerService,
		Validation: validationService,
		AI:         aiService,
		SMTP:       smtpService,
		Queue:      q,
		Replies:    scanner,
		Log:        log,
	}

	worker := workers.NewWorker(
		q, handlers,
		time.Duration(cfg.SendDelayMinSecs)*time.Second,
		time.Duration(cfg.SendDelayMaxSecs)*time.Second,
		log,
	)

	runCtx, cancel := context.WithCancel(ctx)
	go worker.Run(runCtx)
	log.Info("worker running", zap.String("send_mode_default", cfg.SendMode))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info("worker shutting down")
	cancel()
}
