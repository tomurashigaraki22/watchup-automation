package smtp

import (
	"context"
	"errors"
	"testing"

	"watchup/automation/internal/config"
)

type fakeTransport struct {
	calls   int
	failFor int // fail this many calls before succeeding; 0 = always succeed
	sent    []string
}

func (f *fakeTransport) Send(from, to string, raw []byte) error {
	f.calls++
	f.sent = append(f.sent, to)
	if f.calls <= f.failFor {
		return errors.New("simulated transient failure")
	}
	return nil
}

func testConfig() *config.Config {
	return &config.Config{SenderEmail: "partnership@watchup.space", SMTPHost: "smtp.hostinger.com", SMTPPort: 465}
}

func TestSender_Send_Success(t *testing.T) {
	ft := &fakeTransport{}
	s := NewSender(testConfig(), WithTransport(ft))

	result, err := s.Send(context.Background(), Message{To: "founder@acme.com", Subject: "Hi", BodyText: "Hello."})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.MessageID == "" {
		t.Error("expected a generated message id")
	}
	if ft.calls != 1 {
		t.Errorf("expected 1 transport call, got %d", ft.calls)
	}
}

func TestSender_Send_RetriesThenSucceeds(t *testing.T) {
	ft := &fakeTransport{failFor: 2}
	s := NewSender(testConfig(), WithTransport(ft))

	_, err := s.Send(context.Background(), Message{To: "founder@acme.com", Subject: "Hi", BodyText: "Hello."})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if ft.calls != 3 {
		t.Errorf("expected 3 attempts, got %d", ft.calls)
	}
}

func TestSender_Send_AllAttemptsFail(t *testing.T) {
	ft := &fakeTransport{failFor: 99}
	s := NewSender(testConfig(), WithTransport(ft))

	_, err := s.Send(context.Background(), Message{To: "founder@acme.com", Subject: "Hi", BodyText: "Hello."})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if ft.calls != 3 {
		t.Errorf("expected 3 attempts, got %d", ft.calls)
	}
}

func TestSender_Send_UsesConfiguredFromAddress(t *testing.T) {
	ft := &fakeTransport{}
	s := NewSender(testConfig(), WithTransport(ft))

	if s.From() != "partnership@watchup.space" {
		t.Errorf("unexpected From: %s", s.From())
	}
	if _, err := s.Send(context.Background(), Message{To: "founder@acme.com", Subject: "Hi", BodyText: "Hello."}); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestSender_Send_RespectsContextCancellation(t *testing.T) {
	ft := &fakeTransport{failFor: 99}
	s := NewSender(testConfig(), WithTransport(ft))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.Send(ctx, Message{To: "founder@acme.com", Subject: "Hi", BodyText: "Hello."}); err == nil {
		t.Fatal("expected error for canceled context")
	}
}
