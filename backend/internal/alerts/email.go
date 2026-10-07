package alerts

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/PrimitiveTechExperience/Steamscope/backend/internal/config"
)

// SMTPMailer sends email through an SMTP server: implicit TLS on port 465, or
// STARTTLS when the server offers it (port 587).
type SMTPMailer struct {
	host, from, username, password string
	port                           int
}

func NewSMTPMailer(c config.AlertsConfig) *SMTPMailer {
	return &SMTPMailer{host: c.SMTPHost, port: c.SMTPPort, from: c.SMTPFrom, username: c.SMTPUsername, password: c.SMTPPassword}
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	msg, sender, rcpt, err := buildMessage(m.from, to, subject, body, time.Now())
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(m.host, strconv.Itoa(m.port))
	dialer := &net.Dialer{Timeout: sendTimeout}
	var conn net.Conn
	if m.port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: m.host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("could not connect to the mail server: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(sendTimeout))

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("mail server greeting: %w", err)
	}
	defer c.Close()
	if m.port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
				return fmt.Errorf("starting TLS: %w", err)
			}
		}
	}
	if m.username != "" {
		// PlainAuth refuses to send the password over an unencrypted link (except to localhost).
		if err := c.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return fmt.Errorf("mail login: %w", err)
		}
	}
	if err := c.Mail(sender); err != nil {
		return fmt.Errorf("sender rejected: %w", err)
	}
	if err := c.Rcpt(rcpt); err != nil {
		return fmt.Errorf("recipient rejected: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// buildMessage makes the raw email. The addresses and subject come from users
// and from game names, so line breaks are never allowed to reach a header
// (that is how extra headers or recipients get injected). It returns the
// envelope sender and recipient as well.
func buildMessage(from, to, subject, body string, now time.Time) (msg []byte, sender, rcpt string, err error) {
	f, err := mail.ParseAddress(from)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid sender address: %w", err)
	}
	t, err := mail.ParseAddress(to)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid recipient address: %w", err)
	}
	subject = strings.Join(strings.Fields(subject), " ") // one line
	var b strings.Builder
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", f.String())
	header("To", t.String())
	header("Subject", mime.QEncoding.Encode("utf-8", subject))
	header("Date", now.Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "8bit")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String()), f.Address, t.Address, nil
}

func (n *Notifier) sendEmail(ctx context.Context, to string, m Message) error {
	subject := m.Subject
	if subject == "" {
		subject = "Steamscope: " + truncate(m.Body, 90)
	}
	body := m.Body + "\n\nSee it: " + m.URL +
		"\n\nYou are getting this because you asked for target-price alerts by email. " +
		"You can turn that off in your settings: " + strings.TrimRight(n.FrontendURL, "/") + "/account\n"
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return n.Mail.Send(ctx, to, subject, body)
}
