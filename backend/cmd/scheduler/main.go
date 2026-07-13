package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/config"
	"watchup/automation/internal/db"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/logging"
	"watchup/automation/internal/queue"
	"watchup/automation/internal/scheduler"
)

// The scheduler enqueues the hourly pipeline: discovery, a resilience sweep
// of companies stuck at any stage, due followups, and a reply/bounce scan.
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

	sched := scheduler.NewScheduler(repo, q, log)

	// Run one tick immediately at boot so a fresh deploy doesn't wait a full
	// hour before anything happens, then tick hourly thereafter.
	if err := sched.Tick(ctx); err != nil {
		log.Error("scheduler: initial tick failed", zap.Error(err))
	}

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			if err := sched.Tick(ctx); err != nil {
				log.Error("scheduler: tick failed", zap.Error(err))
			}
		case <-stop:
			log.Info("scheduler shutting down")
			return
		}
	}
}
