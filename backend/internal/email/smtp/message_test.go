package smtp

import (
	"strings"
	"testing"
)

func TestBuildRawMessage_IncludesHeadersAndBothParts(t *testing.T) {
	msg := Message{
		From:      "partnership@watchup.space",
		To:        "founder@acme.com",
		Subject:   "Quick idea",
		BodyText:  "Hi there.\n\nWorth a chat?",
		MessageID: "<abc123@watchup.space>",
	}
	raw, err := buildRawMessage(msg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(raw)

	for _, want := range []string{
		"From: partnership@watchup.space",
		"To: founder@acme.com",
		"Message-ID: <abc123@watchup.space>",
		"MIME-Version: 1.0",
		"multipart/alternative",
		"text/plain; charset=UTF-8",
		"text/html; charset=UTF-8",
		"Hi there.",
		"<p>Hi there.</p>",
		"List-Unsubscribe",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected raw message to contain %q, got:\n%s", want, s)
		}
	}
}

func TestBuildRawMessage_ThreadingHeaders(t *testing.T) {
	msg := Message{
		From: "partnership@watchup.space", To: "founder@acme.com", Subject: "Following up",
		BodyText: "Just checking in.", MessageID: "<followup@watchup.space>",
		InReplyTo: "<original@watchup.space>", References: "<original@watchup.space>",
	}
	raw, err := buildRawMessage(msg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "In-Reply-To: <original@watchup.space>") {
		t.Error("expected In-Reply-To header")
	}
	if !strings.Contains(s, "References: <original@watchup.space>") {
		t.Error("expected References header")
	}
}

func TestBuildRawMessage_OpenTrackingPixel(t *testing.T) {
	msg := Message{
		From: "partnership@watchup.space", To: "founder@acme.com", Subject: "Hi",
		BodyText: "Hello.", MessageID: "<x@watchup.space>",
		OpenTrackingURL: "https://watchup.space/api/v1/t/o/42",
	}
	raw, err := buildRawMessage(msg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, `<img src="https://watchup.space/api/v1/t/o/42"`) {
		t.Errorf("expected tracking pixel in html part, got:\n%s", s)
	}
}

func TestGenerateMessageID_UniqueAndScopedToDomain(t *testing.T) {
	a := generateMessageID("watchup.space")
	b := generateMessageID("watchup.space")
	if a == b {
		t.Error("expected unique message IDs")
	}
	if !strings.HasSuffix(a, "@watchup.space>") {
		t.Errorf("expected message id scoped to domain, got %q", a)
	}
}

func TestTextToHTML_EscapesAndParagraphs(t *testing.T) {
	got := textToHTML("Hello <script>\n\nSecond paragraph.", "")
	if strings.Contains(got, "<script>") {
		t.Errorf("expected HTML escaping, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped script tag, got %q", got)
	}
	if strings.Count(got, "<p>") != 2 {
		t.Errorf("expected 2 paragraphs, got %q", got)
	}
}
