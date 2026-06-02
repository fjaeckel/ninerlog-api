package email

import (
	"fmt"
	"log"
	"mime"
	"net/mail"
	"net/smtp"
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
		From:     getEnv("SMTP_FROM", "noreply@ninerlog.app"),
	}
}

// IsConfigured returns true if SMTP is properly configured
func (c *SMTPConfig) IsConfigured() bool {
	return c.Host != ""
}

// Sender sends emails via SMTP
type Sender struct {
	config *SMTPConfig
}

// NewSender creates a new SMTP email sender
func NewSender(config *SMTPConfig) *Sender {
	return &Sender{config: config}
}

// sanitizeHeader removes CR and LF characters to prevent email header injection.
// Deprecated: prefer net/mail.ParseAddress for addresses and mime.QEncoding.Encode
// for free-form header values; both are recognized by static analyzers as
// sanitizers for CWE-640 (email content injection).
func sanitizeHeader(value string) string {
	r := strings.NewReplacer("\r", "", "\n", "")
	return r.Replace(value)
}

// Send sends an email.
//
// Both `to` and the configured `From` address are parsed with net/mail.ParseAddress,
// which validates the address syntax and rejects CR/LF and other characters that
// could be used to inject additional headers (CWE-640). The subject is MIME
// Q-encoded so any non-ASCII or control bytes are escaped and cannot break out
// of the Subject header.
func (s *Sender) Send(to, subject, htmlBody string) error {
	// Validate and canonicalize the recipient address. ParseAddress refuses
	// CR/LF and other header-injection vectors; using its canonical String()
	// form is the sanitizer recognized by CodeQL's go/email-injection query.
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		EmailSendTotal.WithLabelValues("invalid_address").Inc()
		return fmt.Errorf("invalid recipient email address: %w", err)
	}

	fromAddr, err := mail.ParseAddress(s.config.From)
	if err != nil {
		// Fall back to the default sender if the configured From is empty
		// or invalid; this keeps the dry-run path usable when SMTP is not
		// configured at all (e.g. in tests).
		fromAddr = &mail.Address{Address: "noreply@ninerlog.app"}
	}
	fromAddr.Name = "NinerLog"

	// Q-encode the subject so any control characters or non-ASCII content
	// cannot inject additional headers.
	encodedSubject := mime.QEncoding.Encode("utf-8", subject)

	if !s.config.IsConfigured() {
		log.Printf("📧 [DRY-RUN] Email to %s: %s (SMTP not configured)", toAddr.Address, subject)
		EmailSendTotal.WithLabelValues("dry_run").Inc()
		return nil
	}

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	headers := []string{
		"From: " + fromAddr.String(),
		"To: " + toAddr.String(),
		"Subject: " + encodedSubject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}

	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + htmlBody)

	// Use PlainAuth when password is set, otherwise no auth
	// (supports test SMTP servers like MailPit that accept unauthenticated connections)
	var auth smtp.Auth
	if s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	sendStart := time.Now()
	if err := smtp.SendMail(addr, auth, fromAddr.Address, []string{toAddr.Address}, msg); err != nil {
		EmailSendTotal.WithLabelValues("failure").Inc()
		return fmt.Errorf("failed to send email: %w", err)
	}
	EmailSendDurationSeconds.Observe(time.Since(sendStart).Seconds())
	EmailSendTotal.WithLabelValues("success").Inc()

	log.Printf("📧 Email sent to %s: %s", toAddr.Address, subject)
	return nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
