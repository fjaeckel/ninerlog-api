package email

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
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
	return c.Host != "" && c.Username != ""
}

// Sender sends emails via SMTP
type Sender struct {
	config *SMTPConfig
}

// NewSender creates a new SMTP email sender
func NewSender(config *SMTPConfig) *Sender {
	return &Sender{config: config}
}

// Send sends an email
func (s *Sender) Send(to, subject, htmlBody string) error {
	if !s.config.IsConfigured() {
		log.Printf("📧 [DRY-RUN] Email to %s: %s (SMTP not configured)", to, subject)
		return nil
	}

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)

	headers := []string{
		fmt.Sprintf("From: NinerLog <%s>", s.config.From),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}

	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + htmlBody)

	if err := smtp.SendMail(addr, auth, s.config.From, []string{to}, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("📧 Email sent to %s: %s", to, subject)
	return nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
