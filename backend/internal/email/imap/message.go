// Package imap connects to the Hostinger inbox to detect replies (marking
// emails replied=true and canceling their pending followups), bounces
// (hard bounces suppress the contact permanently; soft bounces are marked
// but not retried automatically), and "unsubscribe" replies (suppress).
package imap

import (
	"context"
	"time"
)

// Message is a simplified view of one fetched inbox message — just the
// fields the reply/bounce scanner needs.
type Message struct {
	MessageID  string
	InReplyTo  string
	References []string
	From       string
	Subject    string
	BodyText   string
	Date       time.Time
}

// Fetcher retrieves recent inbox messages. The concrete IMAP implementation
// lives in client.go; tests use a fake Fetcher so scanning logic can be
// verified without a live IMAP connection.
type Fetcher interface {
	FetchRecent(ctx context.Context, since time.Time) ([]Message, error)
}
