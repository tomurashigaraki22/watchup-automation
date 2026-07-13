package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"watchup/automation/internal/config"
	emailsmtp "watchup/automation/internal/email/smtp"
	"watchup/automation/internal/logging"
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

	to := os.Getenv("SMOKE_TO")
	if to == "" {
		to = "esthereniayo@gmail.com"
	}

	sender := emailsmtp.NewSender(cfg)
	msg := emailsmtp.Message{
		From:      sender.From(),
		To:        to,
		Subject:   randomSubject(),
		BodyText:  fmt.Sprintf("This is a one-off SMTP smoke test from WatchUp automation at %s.\n\nIf you receive this, the SMTP relay is functioning.", time.Now().UTC().Format(time.RFC3339)),
		MessageID: fmt.Sprintf("<smoke-%d@%s>", time.Now().UnixNano(), strings.TrimPrefix(cfg.SenderEmail, "")),
	}
	if idx := strings.LastIndex(cfg.SenderEmail, "@"); idx >= 0 {
		msg.MessageID = fmt.Sprintf("<smoke-%d@%s>", time.Now().UnixNano(), cfg.SenderEmail[idx+1:])
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		cancel()
	}()

	result, err := sender.Send(ctx, msg)
	if err != nil {
		log.Fatal("smtp smoke send failed", zap.Error(err))
	}

	fmt.Printf("SMTP smoke test sent successfully to %s\n", to)
	fmt.Printf("Message-ID: %s\n", result.MessageID)
	fmt.Printf("SMTP response: %s\n", result.Response)
}

func randomSubject() string {
	adjectives := []string{"quick", "random", "warm", "friendly", "test", "hostinger"}
	objects := []string{"smtp", "relay", "probe", "delivery", "check", "message"}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return fmt.Sprintf("%s %s smoke test", adjectives[r.Intn(len(adjectives))], objects[r.Intn(len(objects))])
}
