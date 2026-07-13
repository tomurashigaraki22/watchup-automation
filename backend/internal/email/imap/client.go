package imap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

// clientFetcher is the concrete Fetcher, connecting to a real IMAP server
// (Hostinger) over implicit TLS.
type clientFetcher struct {
	addr     string
	username string
	password string
}

// NewClientFetcher builds a Fetcher against a live IMAP server.
func NewClientFetcher(host string, port int, username, password string) Fetcher {
	return &clientFetcher{addr: fmt.Sprintf("%s:%d", host, port), username: username, password: password}
}

func (f *clientFetcher) FetchRecent(ctx context.Context, since time.Time) ([]Message, error) {
	c, err := imapclient.DialTLS(f.addr, nil)
	if err != nil {
		return nil, fmt.Errorf("imap: dial %s: %w", f.addr, err)
	}
	defer c.Logout()

	if err := c.Login(f.username, f.password); err != nil {
		return nil, fmt.Errorf("imap: login: %w", err)
	}

	if _, err := c.Select("INBOX", false); err != nil {
		return nil, fmt.Errorf("imap: select inbox: %w", err)
	}

	criteria := goimap.NewSearchCriteria()
	criteria.Since = since

	ids, err := c.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("imap: search: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	seqset := new(goimap.SeqSet)
	seqset.AddNum(ids...)

	// Peek: true avoids marking messages as \Seen just by scanning them.
	section := &goimap.BodySectionName{Peek: true}
	items := []goimap.FetchItem{section.FetchItem()}

	messages := make(chan *goimap.Message, 32)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	var out []Message
	for msg := range messages {
		lit := msg.GetBody(section)
		if lit == nil {
			continue
		}
		raw, err := io.ReadAll(lit)
		if err != nil {
			continue
		}
		out = append(out, parseMessage(raw))
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap: fetch: %w", err)
	}
	return out, nil
}

// parseMessage extracts the fields the scanner needs from a raw RFC 5322
// message using the standard library's header parser.
func parseMessage(raw []byte) Message {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Message{}
	}

	body, _ := io.ReadAll(m.Body)

	var date time.Time
	if d, err := m.Header.Date(); err == nil {
		date = d
	}

	var references []string
	if refs := m.Header.Get("References"); refs != "" {
		references = strings.Fields(refs)
	}

	return Message{
		MessageID:  strings.TrimSpace(m.Header.Get("Message-Id")),
		InReplyTo:  strings.TrimSpace(m.Header.Get("In-Reply-To")),
		References: references,
		From:       m.Header.Get("From"),
		Subject:    m.Header.Get("Subject"),
		BodyText:   string(body),
		Date:       date,
	}
}
