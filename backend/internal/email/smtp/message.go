package smtp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"
)

// Message is a single email ready to send.
type Message struct {
	From            string
	To              string
	Subject         string
	BodyText        string
	MessageID       string // generated if empty
	InReplyTo       string // set for followups: the original email's Message-ID
	References      string // set for followups: usually the same as InReplyTo
	OpenTrackingURL string // embedded as a 1x1 pixel in the HTML part, if set
}

// generateMessageID builds an RFC 5322 Message-ID scoped to domain.
func generateMessageID(domain string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("<%s@%s>", hex.EncodeToString(b), domain)
}

// buildRawMessage renders msg as a multipart/alternative (text + HTML) RFC
// 5322 message with threading and List-Unsubscribe headers.
func buildRawMessage(msg Message) ([]byte, error) {
	var buf bytes.Buffer

	writeHeader := func(k, v string) {
		buf.WriteString(k)
		buf.WriteString(": ")
		buf.WriteString(v)
		buf.WriteString("\r\n")
	}

	writeHeader("From", msg.From)
	writeHeader("To", msg.To)
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	writeHeader("Date", time.Now().UTC().Format(time.RFC1123Z))
	writeHeader("Message-ID", msg.MessageID)
	if msg.InReplyTo != "" {
		writeHeader("In-Reply-To", msg.InReplyTo)
	}
	if msg.References != "" {
		writeHeader("References", msg.References)
	}
	writeHeader("List-Unsubscribe", fmt.Sprintf("<mailto:%s?subject=unsubscribe>", msg.From))
	writeHeader("MIME-Version", "1.0")

	mw := multipart.NewWriter(&buf)
	writeHeader("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, mw.Boundary()))
	buf.WriteString("\r\n")

	textPart, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}})
	if err != nil {
		return nil, fmt.Errorf("smtp: create text part: %w", err)
	}
	if _, err := textPart.Write([]byte(msg.BodyText)); err != nil {
		return nil, fmt.Errorf("smtp: write text part: %w", err)
	}

	htmlPart, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}})
	if err != nil {
		return nil, fmt.Errorf("smtp: create html part: %w", err)
	}
	if _, err := htmlPart.Write([]byte(textToHTML(msg.BodyText, msg.OpenTrackingURL))); err != nil {
		return nil, fmt.Errorf("smtp: write html part: %w", err)
	}

	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("smtp: close multipart writer: %w", err)
	}

	return buf.Bytes(), nil
}

// RenderPreviewHTML renders text as HTML for dashboard preview (no tracking
// pixel — previews aren't sent).
func RenderPreviewHTML(text string) string {
	return textToHTML(text, "")
}

// textToHTML renders plain text (paragraphs separated by blank lines) as
// simple escaped HTML paragraphs, with an optional invisible open-tracking pixel.
func textToHTML(text, pixelURL string) string {
	paragraphs := strings.Split(text, "\n\n")
	var b strings.Builder
	b.WriteString("<html><body>")
	for _, p := range paragraphs {
		escaped := strings.ReplaceAll(html.EscapeString(p), "\n", "<br>")
		b.WriteString("<p>" + escaped + "</p>")
	}
	if pixelURL != "" {
		b.WriteString(fmt.Sprintf(`<img src="%s" width="1" height="1" alt="" style="display:none">`, html.EscapeString(pixelURL)))
	}
	b.WriteString("</body></html>")
	return b.String()
}
