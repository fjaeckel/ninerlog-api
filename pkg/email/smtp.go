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

// SetDeliveryRecorder attaches the delivery log. A sender without a recorder
// still sends and keeps no history.
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

// Send sends an email with a background context and no message type.
func (s *Sender) Send(to, subject, htmlBody string) error {
	return s.SendMessage(context.Background(), Message{To: to, Subject: subject, HTMLBody: htmlBody})
}

// SendMessage sends an email and records what the SMTP conversation said about
// it. The recipient is validated with net/mail.ParseAddress and conveyed only
// through the SMTP envelope; the subject is MIME Q-encoded. A returned error
// is always a *SendError.
func (s *Sender) SendMessage(ctx context.Context, msg Message) error {
	msgType := msg.Type
	if msgType == "" {
		msgType = "unspecified"
	}

	// Validate and canonicalize the recipient address.
	toAddr, err := mail.ParseAddress(msg.To)
	if err != nil {
		EmailSendTotal.WithLabelValues("invalid_address").Inc()
		return s.fail(ctx, msg.To, msgType, &SendError{
			Status: StatusInvalidAddress,
			Err:    fmt.Errorf("invalid recipient email address: %w", err),
		})
	}

	// A suppressed address is not dialled again.
	if s.recorder != nil && s.recorder.IsSuppressed(ctx, toAddr.Address) {
		return s.fail(ctx, toAddr.Address, msgType, &SendError{
			Status: StatusSuppressed,
			Err:    errors.New("recipient is suppressed after an earlier hard bounce"),
		})
	}

	fromAddr, err := mail.ParseAddress(s.config.From)
	if err != nil {
		// Fall back to the default sender when the configured From is empty
		// or invalid.
		fromAddr = &mail.Address{Address: "noreply@ninerlog.com"}
	}
	fromAddr.Name = "NinerLog"

	// Q-encode the subject.
	encodedSubject := mime.QEncoding.Encode("utf-8", msg.Subject)

	if !s.config.IsConfigured() {
		slog.Info("[DRY-RUN] Email not sent (SMTP not configured)", "to", toAddr.Address, "subject", msg.Subject)
		EmailSendTotal.WithLabelValues("dry_run").Inc()
		s.record(ctx, Attempt{Recipient: toAddr.Address, Type: msgType, Status: StatusDryRun, Detail: "SMTP not configured"})
		return nil
	}

	addr := net.JoinHostPort(s.config.Host, s.config.Port)

	// The recipient is omitted from the DATA headers; it travels only in the
	// SMTP envelope (RCPT TO below).
	headers := []string{
		"From: " + fromAddr.String(),
		"Subject: " + encodedSubject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}

	// Strip control characters from the body before composing the raw RFC822
	// message.
	sanitizedBody := sanitizeMessageBody(msg.HTMLBody)
	raw := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + sanitizedBody)

	sendStart := time.Now()
	sendErr := s.deliver(ctx, addr, fromAddr.Address, toAddr.Address, raw)
	// Observe SMTP call latency for every real attempt, success or failure.
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

// deliver runs the SMTP conversation step by step and classifies the failure
// by which command drew it.
func (s *Sender) deliver(ctx context.Context, addr, from, to string, raw []byte) *SendError {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("dial %s: %w", addr, err)}
	}
	// Bound the rest of the conversation by the caller's deadline.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		_ = conn.Close()
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("smtp greeting: %w", err)}
	}
	defer func() { _ = client.Close() }()

	if err := client.Hello("localhost"); err != nil {
		return &SendError{Status: StatusServerError, Err: fmt.Errorf("EHLO: %w", err)}
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return &SendError{Status: StatusServerError, Err: fmt.Errorf("STARTTLS: %w", err)}
		}
	}

	// PlainAuth only when a password is set.
	if s.config.Password != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
			if err := client.Auth(auth); err != nil {
				// Never a recipient failure, however permanent the reply code.
				return &SendError{Status: StatusServerError, Code: replyCode(err), Err: fmt.Errorf("SMTP auth: %w", err)}
			}
		}
	}

	if err := client.Mail(from); err != nil {
		return &SendError{Status: StatusServerError, Code: replyCode(err), Err: fmt.Errorf("MAIL FROM: %w", err)}
	}

	// RCPT TO replies are classified against the recipient.
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
	// A refusal of the terminating dot is classified as rejected (soft bounce
	// below 5xx).
	if err := w.Close(); err != nil {
		code := replyCode(err)
		status := StatusRejected
		if code > 0 && code < 500 {
			status = StatusSoftBounce
		}
		return &SendError{Status: status, Code: code, Err: fmt.Errorf("message rejected: %w", err)}
	}

	// QUIT failures after acceptance are ignored.
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
