package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// SMTPConfig holds SMTP server configuration
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// LoadSMTPConfig loads SMTP configuration from environment variables
func LoadSMTPConfig() *SMTPConfig {
	return &SMTPConfig{
		Host:     getEnv("SMTP_HOST", ""),
		Port:     getEnv("SMTP_PORT", "587"),
		Username: getEnv("SMTP_USERNAME", ""),
		Password: getEnv("SMTP_PASSWORD", ""),
		From:     getEnv("SMTP_FROM", "noreply@ninerlog.com"),
	}
}

// IsConfigured returns true if SMTP is properly configured
func (c *SMTPConfig) IsConfigured() bool {
	return c.Host != ""
}

// Sender sends emails via SMTP
type Sender struct {
	config   *SMTPConfig
	recorder DeliveryRecorder
}

// NewSender creates a new SMTP email sender
func NewSender(config *SMTPConfig) *Sender {
	return &Sender{config: config}
}

// SetDeliveryRecorder attaches the delivery log. It is a setter rather than a
// constructor argument because the recorder is backed by a repository, and the
// sender is built before the database layer in cmd/api/main.go. A sender
// without a recorder still sends; it just keeps no history.
func (s *Sender) SetDeliveryRecorder(r DeliveryRecorder) {
	if s == nil {
		return
	}
	s.recorder = r
}

// IsConfigured returns true when the sender has a usable SMTP configuration.
func (s *Sender) IsConfigured() bool {
	if s == nil || s.config == nil {
		return false
	}
	return s.config.IsConfigured()
}

func sanitizeMessageBody(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

// Send sends an email without delivery classification context. It exists for
// callers that have no context.Context and no meaningful message type; new
// call sites should use SendMessage so the delivery log stays readable.
func (s *Sender) Send(to, subject, htmlBody string) error {
	return s.SendMessage(context.Background(), Message{To: to, Subject: subject, HTMLBody: htmlBody})
}

// SendMessage sends an email and records what the SMTP conversation said about
// it.
//
// The recipient is validated with net/mail.ParseAddress (which rejects CR/LF and
// other header-injection vectors) and is delivered only through the SMTP envelope
// (the RCPT TO argument). The user-controlled recipient is never concatenated
// into the message DATA headers, so it cannot be used to inject additional
// headers (CWE-640). The subject is MIME Q-encoded so any non-ASCII or control
// bytes are escaped and cannot break out of the Subject header.
//
// A returned error is always a *SendError, so callers can distinguish a dead
// address from a broken mail server with errors.As.
func (s *Sender) SendMessage(ctx context.Context, msg Message) error {
	msgType := msg.Type
	if msgType == "" {
		msgType = "unspecified"
	}

	// Validate and canonicalize the recipient address. ParseAddress refuses
	// CR/LF and other header-injection vectors.
	toAddr, err := mail.ParseAddress(msg.To)
	if err != nil {
		EmailSendTotal.WithLabelValues("invalid_address").Inc()
		return s.fail(ctx, msg.To, msgType, &SendError{
			Status: StatusInvalidAddress,
			Err:    fmt.Errorf("invalid recipient email address: %w", err),
		})
	}

	// An address that has already refused mail permanently is not dialled
	// again. This is checked before everything else so a bounced address costs
	// nothing but a lookup.
	if s.recorder != nil && s.recorder.IsSuppressed(ctx, toAddr.Address) {
		return s.fail(ctx, toAddr.Address, msgType, &SendError{
			Status: StatusSuppressed,
			Err:    errors.New("recipient is suppressed after an earlier hard bounce"),
		})
	}

	fromAddr, err := mail.ParseAddress(s.config.From)
	if err != nil {
		// Fall back to the default sender if the configured From is empty
		// or invalid; this keeps the dry-run path usable when SMTP is not
		// configured at all (e.g. in tests).
		fromAddr = &mail.Address{Address: "noreply@ninerlog.com"}
	}
	fromAddr.Name = "NinerLog"

	// Q-encode the subject so any control characters or non-ASCII content
	// cannot inject additional headers.
	encodedSubject := mime.QEncoding.Encode("utf-8", msg.Subject)

	if !s.config.IsConfigured() {
		slog.Info("[DRY-RUN] Email not sent (SMTP not configured)", "to", toAddr.Address, "subject", msg.Subject)
		EmailSendTotal.WithLabelValues("dry_run").Inc()
		s.record(ctx, Attempt{Recipient: toAddr.Address, Type: msgType, Status: StatusDryRun, Detail: "SMTP not configured"})
		return nil
	}

	addr := net.JoinHostPort(s.config.Host, s.config.Port)

	// The recipient is intentionally omitted from the DATA headers: it is
	// user-controlled and is conveyed authoritatively via the SMTP envelope
	// (the RCPT TO argument below). Keeping it out of the message bytes removes
	// any possibility of recipient-driven header injection.
	headers := []string{
		"From: " + fromAddr.String(),
		"Subject: " + encodedSubject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}

	// Sanitize body content before composing the raw RFC822 message to avoid
	// email content/header injection via control characters.
	sanitizedBody := sanitizeMessageBody(msg.HTMLBody)
	raw := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + sanitizedBody)

	sendStart := time.Now()
	sendErr := s.deliver(ctx, addr, fromAddr.Address, toAddr.Address, raw)
	// Observe the SMTP call latency for every real attempt (success or failure)
	// so timeouts and slow rejections are visible, not just happy-path latency.
	EmailSendDurationSeconds.Observe(time.Since(sendStart).Seconds())

	if sendErr != nil {
		EmailSendTotal.WithLabelValues("failure").Inc()
		return s.fail(ctx, toAddr.Address, msgType, sendErr)
	}

	EmailSendTotal.WithLabelValues("success").Inc()
	s.record(ctx, Attempt{Recipient: toAddr.Address, Type: msgType, Status: StatusDelivered, Code: smtpOKCode})
	slog.Info("Email sent", "to", toAddr.Address, "subject", msg.Subject)
	return nil
}

// smtpOKCode is the reply code a server gives when it accepts the message.
const smtpOKCode = 250

// deliver runs the SMTP conversation step by step instead of calling
// smtp.SendMail.
//
// SendMail collapses every step into a single opaque error, and which command
// failed is exactly the information that separates "this mailbox does not
// exist" from "our SMTP credentials are wrong". Both are 5xx replies. If they
// were treated alike, one expired password would hard-bounce — and permanently
// suppress — every address the system mails.
func (s *Sender) deliver(ctx context.Context, addr, from, to string, raw []byte) *SendError {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("dial %s: %w", addr, err)}
	}
	// Bound the rest of the conversation by the caller's deadline; without it a
	// server that accepts the connection and then stalls holds the goroutine
	// open indefinitely.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		_ = conn.Close()
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("smtp greeting: %w", err)}
	}
	defer func() { _ = client.Close() }()

	// "localhost" matches what net/smtp uses by default.
	if err := client.Hello("localhost"); err != nil {
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("EHLO: %w", err)}
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return &SendError{Status: StatusServerError, Err: fmt.Errorf("STARTTLS: %w", err)}
		}
	}

	// Use PlainAuth when a password is set, otherwise no auth (supports test
	// SMTP servers like MailPit that accept unauthenticated connections).
	if s.config.Password != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
			if err := client.Auth(auth); err != nil {
				// Never a recipient failure, however permanent the reply code.
				return &SendError{Status: StatusServerError, Code: replyCode(err), Err: fmt.Errorf("SMTP auth: %w", err)}
			}
		}
	}

	// The envelope sender is our own configured address, so a refusal here is
	// about our configuration or reputation, not about the recipient.
	if err := client.Mail(from); err != nil {
		return &SendError{Status: StatusServerError, Code: replyCode(err), Err: fmt.Errorf("MAIL FROM: %w", err)}
	}

	// This is the one command whose reply is genuinely about the recipient.
	if err := client.Rcpt(to); err != nil {
		code := replyCode(err)
		status := StatusSoftBounce
		if code >= 500 && code < 600 {
			status = StatusHardBounce
		}
		return &SendError{Status: status, Code: code, Err: fmt.Errorf("RCPT TO: %w", err)}
	}

	w, err := client.Data()
	if err != nil {
		return &SendError{Status: StatusServerError, Code: replyCode(err), Err: fmt.Errorf("DATA: %w", err)}
	}
	if _, err := w.Write(raw); err != nil {
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("writing message: %w", err)}
	}
	// The reply to the terminating dot judges the message, not the mailbox: a
	// size limit or a spam verdict lands here. Recording it as "rejected"
	// rather than a bounce keeps a content problem from condemning an address
	// that is perfectly real.
	if err := w.Close(); err != nil {
		code := replyCode(err)
		status := StatusRejected
		if code > 0 && code < 500 {
			status = StatusSoftBounce
		}
		return &SendError{Status: status, Code: code, Err: fmt.Errorf("message rejected: %w", err)}
	}

	// A failed QUIT after the message was accepted does not un-send it.
	_ = client.Quit()
	return nil
}

// replyCode extracts the SMTP status code from a net/textproto error, or 0 when
// the failure happened below the protocol (a dropped connection, a timeout).
func replyCode(err error) int {
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) {
		return protoErr.Code
	}
	return 0
}

// fail records a failed attempt and returns the error to the caller unchanged.
func (s *Sender) fail(ctx context.Context, recipient, msgType string, e *SendError) error {
	s.record(ctx, Attempt{
		Recipient: recipient,
		Type:      msgType,
		Status:    e.Status,
		Code:      e.Code,
		Detail:    e.Err.Error(),
	})
	slog.Warn("Email send failed", "to", recipient, "type", msgType, "status", string(e.Status), "smtpCode", e.Code, "error", e.Err)
	return e
}

func (s *Sender) record(ctx context.Context, a Attempt) {
	EmailDeliveryTotal.WithLabelValues(a.Type, string(a.Status)).Inc()
	if s.recorder == nil {
		return
	}
	s.recorder.RecordAttempt(ctx, a)
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
