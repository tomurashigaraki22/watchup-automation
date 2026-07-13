package smtp

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
)

// transport delivers one raw RFC 5322 message. Abstracted so Sender's retry
// logic can be tested without a live SMTP connection.
type transport interface {
	Send(from, to string, raw []byte) error
}

// tlsTransport delivers via implicit-TLS SMTP (Hostinger uses port 465).
type tlsTransport struct {
	host     string
	addr     string
	username string
	password string
}

func newTLSTransport(host string, port int, username, password string) *tlsTransport {
	return &tlsTransport{host: host, addr: fmt.Sprintf("%s:%d", host, port), username: username, password: password}
}

func (t *tlsTransport) Send(from, to string, raw []byte) error {
	conn, err := tls.Dial("tcp", t.addr, &tls.Config{ServerName: t.host})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, t.host)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer client.Close()

	if err := client.Hello("watchup.space"); err != nil {
		return fmt.Errorf("ehlo: %w", err)
	}
	if err := client.Auth(smtp.PlainAuth("", t.username, t.password, t.host)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	return client.Quit()
}
