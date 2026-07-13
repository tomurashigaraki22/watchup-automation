package main

import "testing"

func TestRandomSubject(t *testing.T) {
	subject := randomSubject()
	if subject == "" {
		t.Fatal("expected a non-empty subject")
	}
	if len(subject) > 120 {
		t.Fatalf("expected a reasonably short subject, got %q", subject)
	}
}
