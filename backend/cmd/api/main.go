package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"watchup/automation/internal/api"
	"watchup/automation/internal/config"
	"watchup/automation/internal/db"
	"watchup/automation/internal/db/repository"
	"watchup/automation/internal/db/seed"
	"watchup/automation/internal/logging"
	"watchup/automation/internal/queue"
)

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
	log.Info("database ready, migrations applied")

	ctx := context.Background()
	if err := seed.Default(ctx, gdb); err != nil {
		log.Fatal("database seed failed", zap.Error(err))
	}

	repos := repository.NewRepositories(gdb)

	// Redis backs the send-trigger queue used by POST /emails/:id/send.
	// The API degrades gracefully (that one route returns 503) if unreachable.
	var q *queue.Queue
	if rdb, err := queue.NewRedis(ctx, cfg.RedisAddr); err != nil {
		log.Warn("redis not reachable at boot (send-trigger route degraded)", zap.Error(err))
	} else {
		log.Info("redis reachable")
		q = queue.NewQueue(rdb)
	}

	srv := api.New(cfg, gdb, repos, q, log)

	go func() {
		log.Info("api listening", zap.String("port", cfg.APIPort))
		if err := srv.App.Listen(":" + cfg.APIPort); err != nil {
			log.Fatal("api server stopped", zap.Error(err))
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info("shutting down api")
	_ = srv.App.Shutdown()
}
